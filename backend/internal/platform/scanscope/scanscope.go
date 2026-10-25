// Package scanscope decides which addresses a network scan may touch. The operator declares the ranges
// that may be scanned; every scan target — an address, a range, a CIDR block or a host name — is resolved
// in process and authorised against that list before any engine sees it. The package is pure policy: it
// opens no connection and starts no process.
package scanscope

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"

	"llm-proxy/internal/platform/network"
)

// DefaultMaxHosts caps how many addresses one scan request may name.
const DefaultMaxHosts = 1024

// Typed refusals; match them with errors.Is.
var (
	// ErrScanningOff means the operator has declared no range, so nothing may be scanned.
	ErrScanningOff = errors.New("scanning is off: no scan range has been declared")
	// ErrBadEntry means a target is not an address, range, CIDR block or resolvable name.
	ErrBadEntry = errors.New("not a valid scan target")
	// ErrNotScannable means an address is never a scan target: public, shared (CGNAT), IPv6 or always blocked.
	ErrNotScannable = errors.New("address cannot be scanned")
	// ErrOutOfScope means an address is scannable in principle but outside the declared ranges.
	ErrOutOfScope = errors.New("target is outside the declared scan ranges")
	// ErrTooManyHosts means a request names more addresses than the cap.
	ErrTooManyHosts = errors.New("target names too many addresses")
)

// ipv4Bits is the width of an IPv4 address, and the largest prefix length.
const ipv4Bits = 32

// smallestBlockWithEdges is the longest prefix whose first and last address are a network and a broadcast
// address; a /31 or /32 has neither.
const smallestBlockWithEdges = 30

// Resolver turns a host name into addresses. net.DefaultResolver satisfies it.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// Policy is the operator-declared set of scannable ranges and the per-request host cap.
type Policy struct {
	Allowed  []netip.Prefix
	MaxHosts int
}

// Parse turns one operator entry — an address, a CIDR block, a "first-last" range or a host name — into the
// prefixes it covers. Every resulting address must be scannable; a name must resolve only to scannable
// addresses.
func Parse(ctx context.Context, r Resolver, entry string) ([]netip.Prefix, error) {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return nil, fmt.Errorf("%w: empty", ErrBadEntry)
	}
	prefixes, err := parseLiteral(entry)
	if errors.Is(err, ErrBadEntry) {
		prefixes, err = parseName(ctx, r, entry)
	}
	if err != nil {
		return nil, err
	}
	for _, p := range prefixes {
		if err := checkScannable(p); err != nil {
			return nil, fmt.Errorf("%s: %w", entry, err)
		}
	}
	return prefixes, nil
}

// Authorize resolves the targets and returns every address to scan, deduplicated and in order. It refuses
// the whole request if scanning is off, if any target is not scannable or lies outside the declared ranges,
// or if the request names more than MaxHosts addresses.
func (p Policy) Authorize(ctx context.Context, r Resolver, targets []string) ([]netip.Addr, error) {
	if len(p.Allowed) == 0 {
		return nil, ErrScanningOff
	}
	maxHosts := p.MaxHosts
	if maxHosts <= 0 {
		maxHosts = DefaultMaxHosts
	}
	seen := map[netip.Addr]bool{}
	var out []netip.Addr
	for _, target := range targets {
		prefixes, err := Parse(ctx, r, target)
		if err != nil {
			return nil, err
		}
		for _, prefix := range prefixes {
			if !p.contains(prefix) {
				return nil, fmt.Errorf("%s: %w", prefix, ErrOutOfScope)
			}
			if HostCount(prefix) > uint64(maxHosts) {
				return nil, fmt.Errorf("%s: %w (limit %d)", prefix, ErrTooManyHosts, maxHosts)
			}
			for _, a := range hosts(prefix) {
				if seen[a] {
					continue
				}
				if len(out) == maxHosts {
					return nil, fmt.Errorf("%w (limit %d)", ErrTooManyHosts, maxHosts)
				}
				seen[a] = true
				out = append(out, a)
			}
		}
	}
	return out, nil
}

// contains reports whether prefix lies wholly inside one declared range.
func (p Policy) contains(prefix netip.Prefix) bool {
	for _, allowed := range p.Allowed {
		if allowed.Bits() <= prefix.Bits() && allowed.Contains(prefix.Addr()) {
			return true
		}
	}
	return false
}

// checkScannable refuses what is never a scan target: IPv6, public addresses, CGNAT shared space and the
// always-blocked classes (loopback, link-local, multicast, unspecified). A block is checked at both ends,
// which is enough because every refused class is itself a contiguous range that cannot sit inside a
// private block.
func checkScannable(prefix netip.Prefix) error {
	if !prefix.Addr().Is4() {
		return fmt.Errorf("%w: %s is not IPv4", ErrNotScannable, prefix.Addr())
	}
	for _, a := range []netip.Addr{prefix.Masked().Addr(), lastAddr(prefix)} {
		ip := net.IP(a.AsSlice())
		if err := network.CheckAlwaysBlocked(ip); err != nil {
			return fmt.Errorf("%w: %v", ErrNotScannable, err)
		}
		if !ip.IsPrivate() {
			return fmt.Errorf("%w: %s is not in private address space", ErrNotScannable, a)
		}
	}
	return nil
}

// parseLiteral reads an address, CIDR block or "first-last" range. It returns ErrBadEntry for anything else
// so the caller can try a host name.
func parseLiteral(entry string) ([]netip.Prefix, error) {
	if prefix, err := netip.ParsePrefix(entry); err == nil {
		return []netip.Prefix{prefix.Masked()}, nil
	}
	if a, err := netip.ParseAddr(entry); err == nil {
		a = a.Unmap()
		return []netip.Prefix{netip.PrefixFrom(a, a.BitLen())}, nil
	}
	first, last, ok := strings.Cut(entry, "-")
	if !ok {
		return nil, ErrBadEntry
	}
	from, errFrom := netip.ParseAddr(strings.TrimSpace(first))
	to, errTo := netip.ParseAddr(strings.TrimSpace(last))
	if errFrom != nil || errTo != nil || !from.Is4() || !to.Is4() || to.Less(from) {
		return nil, ErrBadEntry
	}
	return coverRange(from, to), nil
}

// parseName resolves a host name; every address it resolves to becomes a single-address prefix.
func parseName(ctx context.Context, r Resolver, name string) ([]netip.Prefix, error) {
	if r == nil || strings.ContainsAny(name, " /\\:") {
		return nil, fmt.Errorf("%w: %q", ErrBadEntry, name)
	}
	addrs, err := r.LookupNetIP(ctx, "ip", name)
	if err != nil || len(addrs) == 0 {
		return nil, fmt.Errorf("%w: %q does not resolve", ErrBadEntry, name)
	}
	out := make([]netip.Prefix, 0, len(addrs))
	for _, a := range addrs {
		a = a.Unmap()
		out = append(out, netip.PrefixFrom(a, a.BitLen()))
	}
	return out, nil
}

// coverRange returns the fewest aligned blocks that exactly cover first..last.
func coverRange(first, last netip.Addr) []netip.Prefix {
	var out []netip.Prefix
	start, end := addrToUint(first), addrToUint(last)
	for start <= end {
		bits := ipv4Bits
		for bits > 0 {
			size := uint64(1) << (ipv4Bits - (bits - 1))
			if start%size != 0 || start+size-1 > end {
				break
			}
			bits--
		}
		out = append(out, netip.PrefixFrom(uintToAddr(start), bits))
		start += uint64(1) << (ipv4Bits - bits)
	}
	return out
}

// PolicyFromCIDRs builds a Policy from stored range strings. An entry that is not a valid CIDR block is left out,
// which only narrows the policy.
func PolicyFromCIDRs(cidrs []string) Policy {
	var p Policy
	for _, c := range cidrs {
		if prefix, err := netip.ParsePrefix(c); err == nil {
			p.Allowed = append(p.Allowed, prefix.Masked())
		}
	}
	return p
}

// HostCount is how many addresses a block holds for scanning (its network and broadcast addresses excluded where it
// has them).
func HostCount(prefix netip.Prefix) uint64 {
	total := uint64(1) << (ipv4Bits - prefix.Bits())
	if prefix.Bits() <= smallestBlockWithEdges {
		return total - 2
	}
	return total
}

// hosts lists the scannable addresses of a block, leaving out the network and broadcast addresses of a block
// that has them.
func hosts(prefix netip.Prefix) []netip.Addr {
	first, last := addrToUint(prefix.Masked().Addr()), addrToUint(lastAddr(prefix))
	if prefix.Bits() <= smallestBlockWithEdges {
		first++
		last--
	}
	out := make([]netip.Addr, 0, last-first+1)
	for v := first; v <= last; v++ {
		out = append(out, uintToAddr(v))
	}
	return out
}

func lastAddr(prefix netip.Prefix) netip.Addr {
	return uintToAddr(addrToUint(prefix.Masked().Addr()) + uint64(1)<<(ipv4Bits-prefix.Bits()) - 1)
}

func addrToUint(a netip.Addr) uint64 {
	b := a.As4()
	return uint64(b[0])<<24 | uint64(b[1])<<16 | uint64(b[2])<<8 | uint64(b[3])
}

func uintToAddr(v uint64) netip.Addr {
	return netip.AddrFrom4([4]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)})
}
