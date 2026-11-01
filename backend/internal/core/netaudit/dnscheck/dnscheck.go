// Package dnscheck asks the same few questions of the router's DNS and of an outside resolver, so the two answers can be
// compared. It reads only address records, and only as addresses: every name, text and code in an answer is dropped.
package dnscheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"slices"
	"strings"
)

// DefaultResolverURL is the outside resolver asked when none is set: Cloudflare's DNS-over-HTTPS JSON endpoint.
const DefaultResolverURL = "https://cloudflare-dns.com/dns-query"

// DefaultNames are the names compared until the operator chooses others: the public DNS services' own names, whose
// addresses are fixed by design, so an honest router and an outside resolver should always agree on them.
var DefaultNames = []string{"dns.google", "one.one.one.one", "dns.quad9.net"}

// MaxNames bounds how many names one check asks about.
const MaxNames = 5

// MaxAddresses bounds how many addresses are read from one answer.
const MaxAddresses = 16

var (
	// ErrNoRouter means the router's address is not known, so its DNS cannot be asked.
	ErrNoRouter = errors.New("the router's address is not known")
	// ErrBadAnswer means an answer could not be read as addresses.
	ErrBadAnswer = errors.New("the answer did not hold addresses")
)

// Dial is the guarded dial function (NetworkTools.DialContext).
type Dial func(ctx context.Context, network, address string) (net.Conn, error)

// Router asks the router's own DNS, on port 53 of the address Server gives, and nobody else.
type Router struct {
	Dial   Dial
	Server func() netip.Addr
}

// LookupA returns the IPv4 addresses the router gives for name.
func (r Router) LookupA(ctx context.Context, name string) ([]netip.Addr, error) {
	server := r.Server()
	if !server.IsValid() {
		return nil, ErrNoRouter
	}
	target := net.JoinHostPort(server.String(), "53")
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return r.Dial(ctx, network, target)
		},
	}
	// The trailing dot keeps the system's search domains out of the question.
	found, err := resolver.LookupNetIP(ctx, "ip4", name+".")
	if err != nil {
		return nil, fmt.Errorf("the router's DNS did not answer: %w", err)
	}
	return tidy(found), nil
}

// tidy drops anything that is not an IPv4 address, repeats and the surplus beyond MaxAddresses.
func tidy(in []netip.Addr) []netip.Addr {
	var out []netip.Addr
	for _, a := range in {
		a = a.Unmap()
		if a.Is4() && !slices.Contains(out, a) && len(out) < MaxAddresses {
			out = append(out, a)
		}
	}
	return out
}

// Verdict is how two address sets compare.
type Verdict struct {
	// Same is true when both sets hold the same addresses, in any order.
	Same bool
	// Overlap is true when the sets share at least one address.
	Overlap bool
}

// Compare says whether two answers agree. Order and repeats do not matter.
func Compare(router, outside []netip.Addr) Verdict {
	r, o := set(router), set(outside)
	v := Verdict{Same: len(r) == len(o)}
	for a := range r {
		if _, ok := o[a]; ok {
			v.Overlap = true
		} else {
			v.Same = false
		}
	}
	return v
}

func set(in []netip.Addr) map[netip.Addr]struct{} {
	out := make(map[netip.Addr]struct{}, len(in))
	for _, a := range in {
		out[a] = struct{}{}
	}
	return out
}

// ParseDoH reads the addresses from a DNS-over-HTTPS JSON answer.
func ParseDoH(raw []byte) ([]netip.Addr, error) {
	var body struct {
		Status *int `json:"Status"`
		Answer []struct {
			Type int    `json:"type"`
			Data string `json:"data"`
		} `json:"Answer"`
	}
	if err := json.Unmarshal(raw, &body); err != nil || body.Status == nil || *body.Status != 0 {
		return nil, ErrBadAnswer
	}
	var found []netip.Addr
	for _, rec := range body.Answer {
		if a, err := netip.ParseAddr(rec.Data); rec.Type == typeA && err == nil {
			found = append(found, a)
		}
	}
	if found = tidy(found); len(found) == 0 {
		return nil, ErrBadAnswer
	}
	return found, nil
}

// typeA is the DNS record type of an IPv4 address.
const typeA = 1

// ValidateNames checks the names an operator typed and returns them tidy.
func ValidateNames(in []string) ([]string, error) {
	var out []string
	for _, raw := range in {
		name := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(raw)), ".")
		if name == "" {
			continue
		}
		if !hostName.MatchString(name) || len(name) > maxNameLength {
			return nil, errors.New("a name must be a host name such as dns.google: letters, digits and hyphens, in labels up to 63 characters")
		}
		if _, err := netip.ParseAddr(name); err == nil {
			return nil, errors.New("a name must be a host name, not an address")
		}
		if !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	if len(out) == 0 || len(out) > MaxNames {
		return nil, fmt.Errorf("give between 1 and %d names", MaxNames)
	}
	return out, nil
}

const maxNameLength = 253

// hostName is dot-separated labels of letters, digits and hyphens, each 1 to 63 characters, not starting or ending with a hyphen.
var hostName = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)*$`)
