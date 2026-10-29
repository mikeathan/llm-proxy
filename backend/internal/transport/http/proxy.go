package api

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// IsHTTPS reports whether the browser's connection is encrypted. A request that arrived over TLS is. Behind a TLS front
// end (Caddy, nginx, Tailscale serve) the server sees plain HTTP, so the proxy's X-Forwarded-Proto is believed, but only
// when the connection comes from an address in trusted (server.trusted_proxies): anyone can send that header, and
// believing a stranger would mark a plain connection encrypted. The value that counts is the last one in the header, the
// one the nearest proxy added after whatever the client sent. trusted is read on every call so a change applies without a
// restart; nil trusts no proxy.
func IsHTTPS(r *http.Request, trusted func() []string) bool {
	if r.TLS != nil {
		return true
	}
	if trusted == nil || !fromTrustedProxy(r, trusted()) {
		return false
	}
	values := r.Header.Values("X-Forwarded-Proto")
	if len(values) == 0 {
		return false
	}
	parts := strings.Split(values[len(values)-1], ",")
	return strings.EqualFold(strings.TrimSpace(parts[len(parts)-1]), "https")
}

// fromTrustedProxy reports whether the connection's peer address is in one of the listed addresses or ranges. An entry
// that is not an address or a range trusts nothing.
func fromTrustedProxy(r *http.Request, trusted []string) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	peer, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	peer = peer.Unmap()
	for _, entry := range trusted {
		if prefix, ok := parseProxyEntry(entry); ok && prefix.Contains(peer) {
			return true
		}
	}
	return false
}

// parseProxyEntry reads "10.0.0.5", "::1" or "10.0.0.0/8" as a range.
func parseProxyEntry(entry string) (netip.Prefix, bool) {
	entry = strings.TrimSpace(entry)
	if prefix, err := netip.ParsePrefix(entry); err == nil {
		return netip.PrefixFrom(prefix.Addr().Unmap(), unmappedBits(prefix)), true
	}
	if addr, err := netip.ParseAddr(entry); err == nil {
		addr = addr.Unmap()
		return netip.PrefixFrom(addr, addr.BitLen()), true
	}
	return netip.Prefix{}, false
}

// unmappedBits is the prefix length of p once an IPv4-mapped IPv6 address is written as plain IPv4.
func unmappedBits(p netip.Prefix) int {
	if p.Addr().Is4In6() {
		return p.Bits() - 96
	}
	return p.Bits()
}

// InvalidProxyEntries lists the entries that are neither an address nor a range, so the server can name them at start
// instead of silently trusting less than the operator wrote.
func InvalidProxyEntries(entries []string) []string {
	var bad []string
	for _, e := range entries {
		if _, ok := parseProxyEntry(e); !ok {
			bad = append(bad, e)
		}
	}
	return bad
}
