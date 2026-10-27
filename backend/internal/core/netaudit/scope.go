// Package netaudit is the network-audit domain: the operator-declared scan scope today, and the scan engines,
// findings and reports as they are built.
package netaudit

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"strings"
	"time"

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
	// DeviceTimeoutMinutes is the per-device time limit chosen (0 = standard).
	DeviceTimeoutMinutes int `json:"device_timeout_minutes"`
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
	view := ScopeView{Ranges: []Range{}, AllowUnconfined: cfg.AllowUnconfined, DeviceTimeoutMinutes: cfg.DeviceTimeoutMinutes}
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

// AllowedDeviceTimeouts are the per-device time limits (minutes) an operator may choose; 0 is the standard. The longest
// stays well inside the scan budget (ScanBudget), so a slow device can use it and the scan still finishes.
var AllowedDeviceTimeouts = []int{0, 5, 10}

// ErrBadDeviceTimeout is returned for a time limit that is not one of AllowedDeviceTimeouts.
var ErrBadDeviceTimeout = errors.New("that time limit is not one of the choices")

// SetDeviceTimeout records how many minutes a scan may spend on one device (0 = standard).
func (s *Scope) SetDeviceTimeout(minutes int) error {
	if !slices.Contains(AllowedDeviceTimeouts, minutes) {
		return fmt.Errorf("%w: %d minutes", ErrBadDeviceTimeout, minutes)
	}
	if err := s.store.Update(func(c *models.NetauditConfig) error {
		c.DeviceTimeoutMinutes = minutes
		return nil
	}); err != nil {
		return fmt.Errorf("failed to save the time limit: %w", err)
	}
	return nil
}

// DeviceTimeout is the per-device time limit scans use now; zero means each depth's standard.
func (s *Scope) DeviceTimeout() time.Duration {
	return time.Duration(s.store.Get().DeviceTimeoutMinutes) * time.Minute
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

// fallbackInterface names the one network found without listing interfaces.
const (
	fallbackInterface = "default route"
	// fallbackProbeTarget is only used to ask the kernel which local address it would send from; an UDP "connection"
	// sends nothing.
	fallbackProbeTarget = "8.8.8.8:80"
	fallbackMaskBits    = 24
)

// systemInterfaces reads the machine's interfaces. On a host where the service may not open a netlink socket (a systemd
// unit with RestrictAddressFamilies) the list fails; it then falls back to the one address the default route sends from,
// assumed to sit on a /24, so the operator still gets a network to pick.
func systemInterfaces() (map[string][]net.Addr, error) {
	return interfacesWithFallback(listInterfaces, outboundIP)
}

func interfacesWithFallback(list func() (map[string][]net.Addr, error), outbound func() (net.IP, error)) (map[string][]net.Addr, error) {
	out, err := list()
	if err == nil {
		return out, nil
	}
	ip, outErr := outbound()
	if outErr != nil {
		return nil, err
	}
	return map[string][]net.Addr{fallbackInterface: {&net.IPNet{IP: ip, Mask: net.CIDRMask(fallbackMaskBits, 8*net.IPv4len)}}}, nil
}

func listInterfaces() (map[string][]net.Addr, error) {
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

// outboundIP is the local IPv4 address the default route would send from.
func outboundIP() (net.IP, error) {
	conn, err := net.Dial("udp4", fallbackProbeTarget)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || addr.IP == nil {
		return nil, fmt.Errorf("no local address")
	}
	return addr.IP, nil
}

// TargetProblem is one target an audit could not scan, in words for the operator. Entry is empty when the problem is
// with the whole list.
type TargetProblem struct {
	Entry  string `json:"entry,omitempty"`
	Reason string `json:"reason"`
}

// Words for the reasons a target cannot be scanned.
const (
	reasonScanningOff = "No network has been allowed for scanning yet."
	reasonOutside     = "It is outside the networks allowed for scanning."
	reasonTooMany     = "It covers more devices than one scan may."
	reasonUnusable    = "It is not an address, range or name that can be scanned (a name must resolve to a local address)."
)

// CheckTargets says, before a run, which of an audit's targets the run would refuse. The same policy a scan is held to
// decides, so the answer cannot drift from what happens. An empty list means every allowed network and is fine unless
// none is allowed.
func (s *Scope) CheckTargets(ctx context.Context, targets []string) []TargetProblem {
	policy := s.Policy()
	if len(policy.Allowed) == 0 {
		return []TargetProblem{{Reason: reasonScanningOff}}
	}
	var out []TargetProblem
	for _, entry := range targets {
		_, err := policy.Authorize(ctx, s.resolver, []string{entry})
		switch {
		case err == nil:
		case errors.Is(err, scanscope.ErrOutOfScope):
			out = append(out, TargetProblem{Entry: entry, Reason: reasonOutside})
		case errors.Is(err, scanscope.ErrTooManyHosts):
			out = append(out, TargetProblem{Entry: entry, Reason: reasonTooMany})
		default:
			out = append(out, TargetProblem{Entry: entry, Reason: reasonUnusable})
		}
	}
	return out
}
