package netaudit

import (
	"context"
	"errors"
	"fmt"
	"net/netip"

	"llm-proxy/internal/core/netaudit/upnp"
)

// CheckUPnPEnabled is the router offering UPnP, which lets any device on the network open a port to the internet by itself.
const CheckUPnPEnabled = "upnp_enabled"

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

// Findings probes the router and returns its finding, if any. A router that does not answer gives none: it may have UPnP
// off, or the datagram may have been lost, so nothing is claimed either way. Trouble is an error.
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
	return []Finding{{
		Device: gw.String(), Check: CheckUPnPEnabled, Severity: SeverityLow,
		Evidence: "The router offers UPnP, which lets any device on the network open a port to the internet by itself, without asking you. Malware and some apps use this to make a device reachable from outside",
	}}, nil
}
