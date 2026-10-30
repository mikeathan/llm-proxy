package netaudit

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"slices"

	"llm-proxy/internal/core/netaudit/upnp"
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

// RouterProber asks one router about UPnP: *upnp.Client.
type RouterProber interface {
	Probe(ctx context.Context, gateway netip.Addr) (upnp.Report, error)
}

// RouterCheck looks at the router the device inventory found. It runs in the nightly audit only, never in the presence check.
type RouterCheck struct {
	probe   RouterProber
	gateway func() (netip.Addr, error)
	allowed func(netip.Addr) error
}

// NewRouterCheck builds the check. gateway reads the router's address (the zero Addr when unknown); allowed is the scan
// scope's test, so a router outside the declared ranges is never contacted.
func NewRouterCheck(probe RouterProber, gateway func() (netip.Addr, error), allowed func(netip.Addr) error) *RouterCheck {
	return &RouterCheck{probe: probe, gateway: gateway, allowed: allowed}
}

// Findings probes the router and returns its findings: UPnP being on, and each port it forwards. A router that does not
// answer gives none: it may have UPnP off, or the datagram may have been lost, so nothing is claimed either way. Trouble is
// an error. If only the list of forwarded ports could not be read, the findings that were read are returned with the error,
// so a partly readable router is neither hidden nor reported as clean.
func (c *RouterCheck) Findings(ctx context.Context) ([]Finding, error) {
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
	report, err := c.probe.Probe(ctx, gw)
	if err != nil {
		return nil, err
	}
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
	if report.MappingsErr != "" {
		return out, fmt.Errorf("the router's forwarded ports could not be listed: %s", report.MappingsErr)
	}
	return out, nil
}

// forwardFinding turns one enabled mapping into a finding on the device it forwards to. The wording is fixed: only validated
// numbers are in it, never the description the router stores for the mapping.
func forwardFinding(m upnp.Mapping) (Finding, bool) {
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
