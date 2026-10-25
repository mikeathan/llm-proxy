// Package netaudit is the network-audit domain: the operator-declared scan scope today, and the scan engines,
// findings and reports as they are built.
package netaudit

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"strings"

	"llm-proxy/internal/platform/scanscope"
	"llm-proxy/models"
)

// suggestionMinBits is the shortest prefix suggested: an interface on a larger network (a /8) is offered as the /24
// around its address, which is the size an operator usually means.
const suggestionMinBits = 24

// PolicyStore is where the scan policy is kept; the data manager's netaudit view satisfies it.
type PolicyStore interface {
	Get() models.NetauditConfig
	Update(func(*models.NetauditConfig) error) error
}

// InterfaceAddrs lists the machine's interface addresses by interface name.
type InterfaceAddrs func() (map[string][]net.Addr, error)

// Range is one declared scan range with the number of devices it covers.
type Range struct {
	CIDR    string `json:"cidr"`
	Devices uint64 `json:"devices"`
}

// ScopeView is the scan scope as the operator sees it.
type ScopeView struct {
	Ranges          []Range `json:"ranges"`
	AllowUnconfined bool    `json:"allow_unconfined"`
}

// Suggestion is a network the machine is attached to, offered as a one-click range.
type Suggestion struct {
	Interface string `json:"interface"`
	CIDR      string `json:"cidr"`
	Devices   uint64 `json:"devices"`
}

// Scope keeps the operator-declared scan scope. It turns what the operator types into CIDR blocks before saving,
// so what is stored is exactly what a scan will be checked against.
type Scope struct {
	store      PolicyStore
	resolver   scanscope.Resolver
	interfaces InterfaceAddrs
}

// NewScope builds a Scope over a store. A nil interfaces lists the machine's real interfaces.
func NewScope(store PolicyStore, resolver scanscope.Resolver, interfaces InterfaceAddrs) *Scope {
	if interfaces == nil {
		interfaces = systemInterfaces
	}
	return &Scope{store: store, resolver: resolver, interfaces: interfaces}
}

// Policy is the scan policy the stored ranges describe. No ranges means scanning is off.
func (s *Scope) Policy() scanscope.Policy {
	return scanscope.PolicyFromCIDRs(s.store.Get().ScanRanges)
}

// View returns the stored scope.
func (s *Scope) View() ScopeView {
	cfg := s.store.Get()
	view := ScopeView{Ranges: []Range{}, AllowUnconfined: cfg.AllowUnconfined}
	for _, c := range cfg.ScanRanges {
		if prefix, err := netip.ParsePrefix(c); err == nil {
			view.Ranges = append(view.Ranges, Range{CIDR: c, Devices: scanscope.HostCount(prefix)})
		}
	}
	return view
}

// SetRanges replaces the declared ranges with what the entries stand for. One bad entry refuses the whole save and
// the stored scope is unchanged. An empty list turns scanning off.
func (s *Scope) SetRanges(ctx context.Context, entries []string) (ScopeView, error) {
	var cidrs []string
	for _, entry := range entries {
		prefixes, err := scanscope.Parse(ctx, s.resolver, entry)
		if err != nil {
			return ScopeView{}, err
		}
		for _, p := range prefixes {
			cidrs = append(cidrs, p.String())
		}
	}
	slices.Sort(cidrs)
	cidrs = slices.Compact(cidrs)
	if err := s.store.Update(func(c *models.NetauditConfig) error {
		c.ScanRanges = cidrs
		return nil
	}); err != nil {
		return ScopeView{}, fmt.Errorf("failed to save the scan scope: %w", err)
	}
	return s.View(), nil
}

// SetAllowUnconfined records whether scans may run on a host with no OS file confinement.
func (s *Scope) SetAllowUnconfined(allow bool) error {
	if err := s.store.Update(func(c *models.NetauditConfig) error {
		c.AllowUnconfined = allow
		return nil
	}); err != nil {
		return fmt.Errorf("failed to save the unconfined setting: %w", err)
	}
	return nil
}

// Suggestions lists every private IPv4 network the machine is attached to. The operator picks; the first
// non-loopback interface is never assumed to be the right one.
func (s *Scope) Suggestions() ([]Suggestion, error) {
	byName, err := s.interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to list network interfaces: %w", err)
	}
	var out []Suggestion
	for name, addrs := range byName {
		for _, a := range addrs {
			if sug, ok := suggestionFor(name, a); ok {
				out = append(out, sug)
			}
		}
	}
	slices.SortFunc(out, func(a, b Suggestion) int {
		if a.Interface != b.Interface {
			return strings.Compare(a.Interface, b.Interface)
		}
		return strings.Compare(a.CIDR, b.CIDR)
	})
	return out, nil
}

// suggestionFor turns an interface address into a scannable suggestion, or reports that it is not one.
func suggestionFor(name string, a net.Addr) (Suggestion, bool) {
	ipnet, ok := a.(*net.IPNet)
	if !ok {
		return Suggestion{}, false
	}
	addr, ok := netip.AddrFromSlice(ipnet.IP)
	if !ok {
		return Suggestion{}, false
	}
	ones, _ := ipnet.Mask.Size()
	prefix := netip.PrefixFrom(addr.Unmap(), min(max(ones, suggestionMinBits), addr.Unmap().BitLen())).Masked()
	if _, err := scanscope.Parse(context.Background(), nil, prefix.String()); err != nil {
		return Suggestion{}, false
	}
	return Suggestion{Interface: name, CIDR: prefix.String(), Devices: scanscope.HostCount(prefix)}, true
}

// systemInterfaces reads the machine's interfaces.
func systemInterfaces() (map[string][]net.Addr, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	out := make(map[string][]net.Addr, len(ifaces))
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		out[iface.Name] = addrs
	}
	return out, nil
}
