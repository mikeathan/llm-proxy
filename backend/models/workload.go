// workload.go — WorkloadClass value object and the single workload
// classifier.  This is the unified answer to "is this workload local?" that
// replaces the three disagreeing predicates (budget_squeezer.go isLocalWorkload,
// proxy/client.go IsLocalModelURL, and the Provider == "local" string gate).
// Budget, ICU, and the reasoning wire all key on WorkloadClass.  The
// classifier is pure: it takes cached inputs (modelHost, localInterfaceIPs)
// and performs no DNS or network calls at classify time.
package models

import (
	"net"
	"net/url"
	"strings"
)

// WorkloadClass is a typed enum — never a boolean flag — describing whether a
// model's effective serving endpoint is local or cloud.
type WorkloadClass string

const (
	WorkloadLocal WorkloadClass = "local"
	WorkloadCloud WorkloadClass = "cloud"
)

// WorkloadClassifier classifies model configs by effective endpoint.  It is
// constructed once at the runtime boundary with cached inputs and reused for
// budget/ICU selection, the reasoning wire, and the proxy client factory.
type WorkloadClassifier struct {
	modelHost         string
	localInterfaceIPs []net.IP
}

// NewWorkloadClassifier builds a classifier with the configured inference host
// and the set of local interface IPs (enumerated once at bootstrap).
func NewWorkloadClassifier(modelHost string, localInterfaceIPs []net.IP) WorkloadClassifier {
	return WorkloadClassifier{modelHost: modelHost, localInterfaceIPs: localInterfaceIPs}
}

// Classify returns the workload class for cfg.
//
// Local ⟺ provider == "local"
//
//	∨ .gguf artifact (Filename | Path | Name)
//	∨ host(ProviderConfig.BaseURL) ∈ {localhost, 127.0.0.1, ::1, 0.0.0.0}
//	∨ host(ProviderConfig.BaseURL) matches modelHost
//	∨ host(ProviderConfig.BaseURL) ∈ localInterfaceIPs
//
// The effective endpoint is expected to be the hydrated base URL used for
// inference (per-credential overrides applied) — see
// providers.ProviderRegistrar.EffectiveEndpoint.
func (c WorkloadClassifier) Classify(cfg ModelConfig) WorkloadClass {
	if strings.EqualFold(cfg.Provider, "local") {
		return WorkloadLocal
	}
	if HasGGUFArtifact(cfg.Filename) || HasGGUFArtifact(cfg.Path) || HasGGUFArtifact(cfg.Name) {
		return WorkloadLocal
	}
	if cfg.ProviderConfig != nil && c.ClassifyEndpoint(cfg.ProviderConfig.BaseURL) {
		return WorkloadLocal
	}
	return WorkloadCloud
}

// ClassifyEndpoint reports whether rawURL targets a local serving host:
// loopback/unspecified, the configured model host, or a cached local-interface
// IP.  No DNS, no network — pure host comparison.
func (c WorkloadClassifier) ClassifyEndpoint(rawURL string) bool {
	host := endpointHost(rawURL)
	if host == "" {
		return false
	}
	host = strings.ToLower(host)
	if isLoopbackHost(host) {
		return true
	}
	if c.modelHost != "" && hostMatches(rawURL, c.modelHost) {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, li := range c.localInterfaceIPs {
		if li.Equal(ip) {
			return true
		}
	}
	return false
}

// HasGGUFArtifact reports whether a model identifier string ends in .gguf.
func HasGGUFArtifact(s string) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(s)), ".gguf")
}

// endpointHost extracts the hostname (no port) from a URL.  A bare host
// without a scheme is treated as an http URL so host-only inputs classify.
func endpointHost(rawURL string) string {
	if strings.TrimSpace(rawURL) == "" {
		return ""
	}
	if !strings.Contains(rawURL, "://") {
		rawURL = "http://" + rawURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// isLoopbackHost reports whether host is a loopback or unspecified address
// (localhost, 127.0.0.1, ::1, 0.0.0.0, ::) — a local serving endpoint by
// definition regardless of the configured model host.
func isLoopbackHost(host string) bool {
	switch host {
	case "localhost", "127.0.0.1", "::1", "0.0.0.0", "::":
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsUnspecified())
}

// hostMatches implements the exact IsLocalModelURL matching semantics: strip
// scheme and trailing slash, then compare bare host or host:port/host/ prefix.
func hostMatches(rawURL, modelHost string) bool {
	if modelHost == "" {
		return false
	}
	norm := func(s string) string {
		s = strings.TrimPrefix(s, "http://")
		s = strings.TrimPrefix(s, "https://")
		return strings.TrimSuffix(s, "/")
	}
	target := norm(rawURL)
	host := norm(modelHost)
	if target == host {
		return true
	}
	// rawURL includes the /v1/chat/completions path or a port; compare host:port.
	if strings.HasPrefix(target, host+":") || strings.HasPrefix(target, host+"/") {
		return true
	}
	return false
}

// HasContext reports whether the classifier carries a model host or local
// interface IPs (i.e. it was constructed with real inputs rather than left
// zero-valued).
func (c WorkloadClassifier) HasContext() bool {
	return c.modelHost != "" || len(c.localInterfaceIPs) > 0
}

// LocalInterfaceIPs enumerates the addresses bound to local network
// interfaces.  Called once at bootstrap and cached — never per-request.
func LocalInterfaceIPs() []net.IP {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var ips []net.IP
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip != nil {
				ips = append(ips, ip)
			}
		}
	}
	return ips
}
