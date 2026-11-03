package app

import (
	"net"
	"net/netip"
	"slices"
	"time"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/core/netaudit/nearby"
	"llm-proxy/internal/core/tools"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/mcast"
	"llm-proxy/internal/platform/neighbors"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/models"
)

// nearbyServices is Nearby discovery (Constitution I.9): its switch, the collector that makes the looks, what the card reads,
// what the nightly audit reads, and the service checks that probe open ports.
type nearbyServices struct {
	Config    *netaudit.NearbyConfig
	Collector *nearby.Collector
	Looks     *nearbyLooks
	Findings  netaudit.NearbyFindings
	Services  *netaudit.ServiceChecks
}

// nearbyLooks joins the collector with the device list for the card.
type nearbyLooks struct {
	config    *netaudit.NearbyConfig
	collector *nearby.Collector
	inventory *netaudit.Inventory
	gateway   func() netip.Addr
}

func (l *nearbyLooks) Request() error { return l.collector.Request() }

func (l *nearbyLooks) View() (netaudit.NearbyView, error) {
	state, err := l.collector.State()
	if err != nil {
		return netaudit.NearbyView{}, err
	}
	var devices []netaudit.DeviceView
	if inv, err := l.inventory.View(); err == nil {
		devices = netaudit.ViewOf(inv, time.Now()).Devices
	}
	return netaudit.BuildNearbyView(l.config.Enabled(), l.collector.Running(), state, devices, l.gateway()), nil
}

// newNearby builds the collector and the service checks. Every guard the network tools would have applied to a connection is
// applied here by hand to the multicast socket: the Guardrails local-network switch, the blocked list, the declared scan scope
// and this computer's own addresses (the questions loop back to the local mDNS responder, which must not count as a device).
func (s *AppServices) newNearby(store netaudit.PolicyStore, scope *netaudit.Scope, netTools *tools.NetworkTools, guard func() models.NetworkGuardrailsConfig,
	inventory *netaudit.Inventory) nearbyServices {
	config := netaudit.NewNearbyConfig(store)
	state := withReset(s.AppCtx, "What devices said about themselves (Nearby)", persistence.NewNearbyStore(s.AppCtx.RootDir()))
	gateway := cachedGateway(neighborSource{reader: neighbors.NewReader()}, gatewayTTL, time.Now)
	inScope := func(a netip.Addr) bool { return scope.Policy().Check(a) == nil }
	collector := nearby.New(nearby.Deps{
		Enabled:    config.Enabled,
		LANAllowed: func() bool { return guard().AllowLanAccess },
		Interfaces: func() ([]mcast.Interface, error) { return mcast.Interfaces(inScope) },
		Exchange:   mcast.Exchange,
		Accept: func(a netip.Addr) bool {
			return inScope(a) && !slices.Contains(guard().BlockedIPs, a.String()) && !isOwnAddress(a)
		},
		Identify: upnpClient(netTools).Identify,
		Store:    state,
		OnError:  func(err error) { logging.Warn("network nearby", "error", err.Error()) },
	})
	return nearbyServices{
		Config:    config,
		Collector: collector,
		Looks:     &nearbyLooks{config: config, collector: collector, inventory: inventory, gateway: gateway},
		Findings: netaudit.NearbyFindings{State: collector.State, Gateway: gateway, Refresh: func() {
			_ = collector.Request() // off, busy and too soon are all fine here: the audit reads what is kept
		}},
		Services: netaudit.NewServiceChecks(netaudit.ServiceDeps{Dial: netTools.DialContext(), HTTP: netTools.HTTPClient()}),
	}
}

// isOwnAddress is true for an address of this computer.
func isOwnAddress(a netip.Addr) bool {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return true // cannot tell, so do not count it as a device
	}
	for _, x := range addrs {
		if p, ok := x.(*net.IPNet); ok {
			if own, good := netip.AddrFromSlice(p.IP); good && own.Unmap() == a {
				return true
			}
		}
	}
	return false
}
