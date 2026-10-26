package netaudit

import (
	"net/netip"

	"llm-proxy/models"
)

// Port is one TCP port seen on a host. Service, Product and Version come from the network and are untrusted text.
type Port struct {
	Number  int    `json:"number"`
	Proto   string `json:"proto"`
	State   string `json:"state"`
	Service string `json:"service,omitempty"`
	Product string `json:"product,omitempty"`
	Version string `json:"version,omitempty"`
	// Guessed means Service is only the usual name for that port number (nmap's port table), not something the scan
	// confirmed by talking to the service.
	Guessed bool `json:"guessed,omitempty"`
	// Scripts are what nmap's scripts printed for this port (a checks scan only). They are read by the checks and
	// never serialised: the text is whatever the device sent.
	Scripts []Script `json:"-"`
}

// Script is the output of one nmap script on one port, cleaned of control characters and secret-shaped text.
type Script struct {
	ID     string
	Output string
}

// Host is one address a scan looked at.
type Host struct {
	Addr  netip.Addr `json:"addr"`
	Up    bool       `json:"up"`
	Ports []Port     `json:"ports,omitempty"`
	// Unscanned marks a live address the engine did not finish looking at (it ran out of time on it), so its ports are
	// unknown: not "none open".
	Unscanned bool `json:"unscanned,omitempty"`
}

// Finding is one fact worth reporting about a host (see models.ScanFinding, which is what gets persisted).
type Finding = models.ScanFinding

// Check names and severities of the findings a scan alone can produce.
const (
	CheckOpenPort = "open_port"

	SeverityInfo = "info"
)

// FindingsOf turns scan hosts into findings: one per open port, then whatever the checks found on that port.
func FindingsOf(hosts []Host) []Finding {
	var out []Finding
	for _, h := range hosts {
		for _, p := range h.Ports {
			if p.State != "open" {
				continue
			}
			out = append(out, Finding{
				Device: h.Addr.String(), Port: p.Number, Proto: p.Proto, Service: serviceLabel(p), Version: joinVersion(p),
				Check: CheckOpenPort, Severity: SeverityInfo,
			})
			out = append(out, checkFindings(h.Addr.String(), p)...)
		}
	}
	return out
}

// guessedSuffix marks a service name that is only the usual one for the port.
const guessedSuffix = " (guess)"

func serviceLabel(p Port) string {
	if p.Guessed && p.Service != "" {
		return p.Service + guessedSuffix
	}
	return p.Service
}

func joinVersion(p Port) string {
	switch {
	case p.Product != "" && p.Version != "":
		return p.Product + " " + p.Version
	case p.Product != "":
		return p.Product
	}
	return p.Version
}
