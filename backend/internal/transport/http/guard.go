package api

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// GuardConfig names the hosts the browser surface answers to.
type GuardConfig struct {
	// Hostname is this machine's name (os.Hostname); it and "<name>.local" are accepted.
	Hostname string
	// AllowedHosts are further names the operator serves the UI under (a reverse proxy, a custom DNS name). Read on every
	// request so a settings change applies at once.
	AllowedHosts func() []string
}

const (
	adminPrefix    = "/admin/"
	adminAPIPrefix = "/admin/api/"

	headerFetchSite = "Sec-Fetch-Site"
)

// Fetch-Metadata values that are not a cross-site request: none is the address bar or a bookmark.
var sameOriginSites = map[string]bool{"": true, "same-origin": true, "none": true}

// Guard protects the browser surface (/admin/ and /admin/api/) from pages that are not the UI. It does not touch /v1 or
// the webhooks: those are called by other programs, under names this server cannot know.
//
//   - Host: only an IP literal, a loopback or machine name, or a configured name is served; any other name gets 421. A
//     DNS-rebinding page must use a name, so this stops it. It does not stop a cross-site request to an IP address.
//   - Fetch-Metadata: on /admin/api a request the browser says is cross-site or same-site is refused whatever its method,
//     because some GETs have side effects. A request with no such header is a script and passes (sign-in is what stops it).
//   - Writes also pass net/http's CrossOriginProtection, which compares Origin with Host for browsers that send no
//     Fetch-Metadata.
//   - The shell and the API refuse to be framed and to be sniffed; API answers are not cached.
func Guard(next http.Handler, cfg GuardConfig) http.Handler {
	cross := http.NewCrossOriginProtection()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, adminPrefix) {
			next.ServeHTTP(w, r)
			return
		}
		if !cfg.hostAllowed(r.Host) {
			http.Error(w, "this name is not allowed to reach the admin interface", http.StatusMisdirectedRequest)
			return
		}
		setBrowserHeaders(w.Header(), strings.HasPrefix(r.URL.Path, adminAPIPrefix))
		if strings.HasPrefix(r.URL.Path, adminAPIPrefix) && !sameOrigin(r, cross) {
			WriteJSONError(w, http.StatusForbidden, "cross-site request refused")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func sameOrigin(r *http.Request, cross *http.CrossOriginProtection) bool {
	if !sameOriginSites[r.Header.Get(headerFetchSite)] {
		return false
	}
	return cross.Check(r) == nil
}

func setBrowserHeaders(h http.Header, api bool) {
	h.Set("X-Frame-Options", "DENY")
	h.Set("Content-Security-Policy", "frame-ancestors 'none'")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	if api {
		h.Set("Cache-Control", "no-store")
	}
}

func (c GuardConfig) hostAllowed(hostport string) bool {
	host := strings.ToLower(hostOf(hostport))
	if _, err := netip.ParseAddr(host); err == nil {
		return true
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	if name := strings.ToLower(c.Hostname); name != "" && (host == name || host == name+".local") {
		return true
	}
	if c.AllowedHosts == nil {
		return false
	}
	for _, allowed := range c.AllowedHosts() {
		if strings.EqualFold(strings.TrimSpace(allowed), host) {
			return true
		}
	}
	return false
}

// hostOf removes the port, and the brackets of an IPv6 literal, from a Host header value.
func hostOf(hostport string) string {
	if host, _, err := net.SplitHostPort(hostport); err == nil {
		return host
	}
	return strings.Trim(hostport, "[]")
}
