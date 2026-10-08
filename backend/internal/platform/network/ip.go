package network

import (
	"errors"
	"fmt"
	"llm-proxy/models"
	"net"
)

// ErrAlwaysBlockedAddress marks a destination class no agent traffic may reach
// under any policy; match it with errors.Is.
var ErrAlwaysBlockedAddress = errors.New("destination address is always blocked")

// cgnatRange is RFC 6598 shared address space (carrier-grade NAT, and the
// range Tailscale tailnets use). net.IP.IsPrivate does not cover it.
var cgnatRange = &net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}

// CheckAlwaysBlocked is the single classifier for destinations that agent
// egress must never reach, independent of LAN/internet policy: unspecified
// (0.0.0.0, :: — the OS routes these to localhost), loopback, link-local
// (incl. cloud metadata at 169.254.169.254) and multicast. Both the in-process
// network tools and the egress proxy enforce it on the *resolved* IP.
func CheckAlwaysBlocked(ip net.IP) error {
	if ip.IsUnspecified() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsMulticast() {
		return fmt.Errorf("%w: '%s' (unspecified/loopback/link-local/multicast)", ErrAlwaysBlockedAddress, ip)
	}
	return nil
}

// IsLAN reports whether ip is on a local network: RFC 1918 / ULA private space
// or CGNAT shared space (so a tailnet counts as LAN, not as internet).
func IsLAN(ip net.IP) bool {
	return ip.IsPrivate() || cgnatRange.Contains(ip)
}

// ResolveOrigin constructs the MCP client origin URL.
// It takes an optional override (from env var) and a bind address.
// If the bind address is generic (0.0.0.0) or empty, it resolves the outbound IP.
func ResolveOrigin(bindAddr string) string {

	if bindAddr == "" {
		bindAddr = ":" + models.DefaultAppPort
	}

	host, port, err := net.SplitHostPort(bindAddr)
	if err != nil {
		// Fallback for malformed bind address
		return FormatURL(models.AddrLocalhost, models.DefaultAppPort)
	}

	// If host is specific (e.g. 192.168.1.50), use it
	if host != "" && host != models.AddrAllInterfaces {
		return FormatURL(host, port)
	}

	// If generic, find outbound IP
	ip := getOutboundIP()
	if ip == "" {
		return FormatURL(models.AddrLocalhost, port)
	}

	return FormatURL(ip, port)
}

// getOutboundIP gets the preferred outbound ip of this machine
func getOutboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return ""
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}
