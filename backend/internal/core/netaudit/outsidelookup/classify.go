package outsidelookup

import "net/netip"

// Kind says what sort of address the router reports as its internet address.
type Kind int

const (
	// Unusable is an address that is not a single unicast IPv4 address (none, loopback, multicast, unspecified, IPv6).
	Unusable Kind = iota
	// Public is an address the internet can reach: the only kind that is ever sent to an outside service.
	Public
	// Private is an address of a private network (RFC 1918): another router sits in front of this one.
	Private
	// Shared is carrier-grade NAT (100.64.0.0/10): the provider shares one public address between many homes.
	Shared
	// LinkLocal is 169.254.0.0/16: the router has no real address yet.
	LinkLocal
)

var shared = netip.MustParsePrefix("100.64.0.0/10")

// Classify sorts an address into a Kind.
func Classify(a netip.Addr) Kind {
	a = a.Unmap()
	switch {
	case !a.IsValid() || !a.Is4() || a.IsUnspecified() || a.IsLoopback() || a.IsMulticast() || a == netip.AddrFrom4([4]byte{255, 255, 255, 255}):
		return Unusable
	case a.IsLinkLocalUnicast():
		return LinkLocal
	case a.IsPrivate():
		return Private
	case shared.Contains(a):
		return Shared
	}
	return Public
}
