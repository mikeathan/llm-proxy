package netaudit

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"
)

// CheckUPnPEnabled is the router offering UPnP, which lets any device on the network open a port to the internet by itself.
const CheckUPnPEnabled = "upnp_enabled"

// CheckUPnPForward is a port the router forwards from the internet to a device on the network because that device (or a
// program on it) asked for it through UPnP. Anyone on the internet can try to connect to it.
const CheckUPnPForward = "upnp_forward"

// sensitiveForwards are internal ports that should never face the internet: remote logins and file sharing, remote desktop and
// screen sharing, and databases and control ports that often have no password.
var sensitiveForwards = []int{21, 22, 23, 139, 445, 2375, 3306, 3389, 5432, 5900, 6379, 11211, 27017}

// ErrNoRouter means the router's address is not known yet (the device inventory has not seen a default route), so it cannot
// be asked. That is said, not read as "UPnP is off".
var ErrNoRouter = errors.New("the router's address is not known yet")

// RouterMapping is one port the router forwards from its internet side to a device on the network, as the Network stack's router sensor read it.
type RouterMapping struct {
	ExternalPort int
	InternalPort int
	// Protocol is "tcp" or "udp".
	Protocol string
	// Client is the IPv4 address of the device the traffic is forwarded to.
	Client  netip.Addr
	Enabled bool
}

// MaxRouterMappings is the most forwarded ports one router report keeps.
const MaxRouterMappings = 64

// Valid says whether a mapping is one the core will trust: both ports in range, a protocol of tcp or udp and an IPv4 client. The sensor drops
// what it cannot read, and the core checks again what crosses the port (ADR-0038 rule 3).
func (m RouterMapping) Valid() bool {
	return m.ExternalPort >= 1 && m.ExternalPort <= 65535 && m.InternalPort >= 1 && m.InternalPort <= 65535 &&
		(m.Protocol == "tcp" || m.Protocol == "udp") && m.Client.Is4()
}

// RouterReport is what the router said, as fixed facts.
type RouterReport struct {
	// Answered is true when the router replied to the discovery request. A router with UPnP off says nothing, so false does not prove UPnP is off.
	Answered bool
	// Gateway is true when the router's description page names an Internet Gateway Device.
	Gateway bool
	// HasService is true when the page names an internet connection service, which holds the port mappings.
	HasService bool
	// Mappings are the ports the router forwards, enabled or not. MappingsErr is why they could not be listed, in words; the rest still stands.
	Mappings    []RouterMapping
	MappingsErr string
	// ExternalAddr is the address of the router's internet side, set only when it was asked for and the router stated one.
	ExternalAddr netip.Addr
}

// routerProblems are the words for each reason the router's sensor gives for not reading a router. The ids are the stack's (a test holds the two
// equal); the words are the core's, and nothing the router said is in them.
var routerProblems = map[string]string{
	"unreachable":     "could not reach the router for UPnP",
	"not_router":      "the UPnP answer did not come from the router's own address",
	"page_failed":     "the router's UPnP page could not be read",
	"list_failed":     "could not list the router's forwarded ports",
	"list_refused":    "the router refused to list its forwarded ports",
	"list_unreadable": "the router's list of forwarded ports could not be read",
	"list_unusable":   "the router named a connection service this program does not recognise",
}

// RouterProblemIDs lists every problem id the core has words for, in no order.
func RouterProblemIDs() []string {
	ids := make([]string, 0, len(routerProblems))
	for id := range routerProblems {
		ids = append(ids, id)
	}
	return ids
}

// RouterProblemWords says why a router could not be read. An id the core does not know is said plainly, never passed on.
func RouterProblemWords(id string) string {
	if words, ok := routerProblems[id]; ok {
		return words
	}
	return "the router could not be read"
}

// RouterProber asks one router about UPnP: the Network stack's router sensor. wantAddress asks it also for the address of the router's internet
// side, which the core sets only when the outside lookup is on.
type RouterProber interface {
	Probe(ctx context.Context, gateway netip.Addr, wantAddress bool) (RouterReport, error)
}

// RouterCheck looks at the router the device inventory found. It runs in the nightly audit only, never in the presence check.
type RouterCheck struct {
	probe   RouterProber
	gateway func() (netip.Addr, error)
	allowed func(netip.Addr) error
	// exposure is the optional look at what the internet can reach, made from what the router says about its own address.
	exposure *Exposure
}

// NewRouterCheck builds the check. gateway reads the router's address (the zero Addr when unknown); allowed is the scan
// scope's test, so a router outside the declared ranges is never contacted.
func NewRouterCheck(probe RouterProber, gateway func() (netip.Addr, error), allowed func(netip.Addr) error) *RouterCheck {
	return &RouterCheck{probe: probe, gateway: gateway, allowed: allowed}
}

// WithExposure adds the outside lookup to what the check reports. Without it nothing is asked of an outside service.
func (c *RouterCheck) WithExposure(e *Exposure) *RouterCheck {
	c.exposure = e
	return c
}

// wantsAddress says whether the router is to be asked for its internet address: only while the outside lookup is configured and on.
func (c *RouterCheck) wantsAddress() bool { return c.exposure != nil && c.exposure.Wants() }

// Carry returns the earlier outside-lookup findings that a look which could not be made must not drop (see Exposure.Carry).
func (c *RouterCheck) Carry(earlier []Finding) []Finding {
	if c.exposure == nil {
		return nil
	}
	return c.exposure.Carry(earlier)
}

// Findings probes the router and returns its findings: UPnP being on, and each port it forwards. A router that does not
// answer gives none: it may have UPnP off, or the datagram may have been lost, so nothing is claimed either way. Trouble is
// an error. If only the list of forwarded ports could not be read, the findings that were read are returned with the error,
// so a partly readable router is neither hidden nor reported as clean.
func (c *RouterCheck) Findings(ctx context.Context) ([]Finding, error) {
	if c.exposure != nil {
		c.exposure.Begin()
	}
	gw, err := c.gateway()
	if err != nil {
		return nil, err
	}
	if !gw.IsValid() {
		return nil, ErrNoRouter
	}
	if err := c.allowed(gw); err != nil {
		return nil, fmt.Errorf("the router is outside the networks you allowed: %w", err)
	}
	report, err := c.probe.Probe(ctx, gw, c.wantsAddress())
	if err != nil {
		return nil, err
	}
	exposed := c.exposedPorts(ctx, gw, report)
	if !report.Answered || !report.Gateway {
		return nil, nil
	}
	out := []Finding{{
		Device: gw.String(), Check: CheckUPnPEnabled, Severity: SeverityLow,
		Evidence: "The router offers UPnP, which lets any device on the network open a port to the internet by itself, without asking you. Malware and some apps use this to make a device reachable from outside",
	}}
	seen := map[string]bool{}
	for _, m := range report.Mappings {
		f, ok := forwardFinding(m)
		if ok && !seen[f.Key()] {
			seen[f.Key()] = true
			out = append(out, f)
		}
	}
	out = append(out, exposed...)
	if report.MappingsErr != "" {
		return out, fmt.Errorf("the router's forwarded ports could not be listed: %s", report.MappingsErr)
	}
	return out, nil
}

// forwardFinding turns one enabled mapping into a finding on the device it forwards to. The wording is fixed: only validated
// numbers are in it, never the description the router stores for the mapping.
func forwardFinding(m RouterMapping) (Finding, bool) {
	if !m.Enabled {
		return Finding{}, false
	}
	f := Finding{Device: m.Client.String(), Port: m.InternalPort, Proto: m.Protocol, Check: CheckUPnPForward, Severity: SeverityMedium,
		Evidence: fmt.Sprintf("The router forwards port %d from the internet to this device's port %d (%s) because something asked it to through UPnP. Anyone on the internet can try to connect to it", m.ExternalPort, m.InternalPort, m.Protocol)}
	if slices.Contains(sensitiveForwards, m.InternalPort) {
		f.Severity = SeverityHigh
	}
	if !m.Client.IsPrivate() {
		f.Severity = SeverityHigh
		f.Evidence = fmt.Sprintf("The router forwards port %d from the internet to an address outside your network (port %d, %s). That is how a router is made to relay traffic for someone else", m.ExternalPort, m.InternalPort, m.Protocol)
	}
	return f, true
}

// exposedPorts is the outside lookup's findings, none when it is not set up or not switched on.
func (c *RouterCheck) exposedPorts(ctx context.Context, gw netip.Addr, report RouterReport) []Finding {
	if c.exposure == nil {
		return nil
	}
	return c.exposure.Findings(ctx, gw, report)
}
