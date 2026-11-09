package app

import (
	"net/netip"
	"slices"
	"sync"
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
	vendors   netaudit.VendorLookup
}

func (l *nearbyLooks) Request() error { return l.collector.Request() }

func (l *nearbyLooks) View() (netaudit.NearbyView, error) {
	state, err := l.collector.State()
	if err != nil {
		return netaudit.NearbyView{}, err
	}
	var devices []netaudit.DeviceView
	if inv, err := l.inventory.View(); err == nil {
		devices = netaudit.ViewOf(inv, time.Now(), l.vendors).Devices
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
	own := cachedOwnAddress(scope, ownAddressTTL, time.Now)
	collector := nearby.New(nearby.Deps{
		Enabled:    config.Enabled,
		LANAllowed: func() bool { return guard().AllowLanAccess },
		Interfaces: func() ([]mcast.Interface, error) { return nearbyInterfaces(scope, inScope) },
		Exchange:   mcast.Exchange,
		Accept: func(a netip.Addr) bool {
			return inScope(a) && !slices.Contains(guard().BlockedIPs, a.String()) && !own(a)
		},
		Identify: upnpClient(netTools).Identify,
		Store:    state,
		OnError:  func(err error) { logging.Warn("network nearby", "error", err.Error()) },
	})
	return nearbyServices{
		Config:    config,
		Collector: collector,
		Looks:     &nearbyLooks{config: config, collector: collector, inventory: inventory, gateway: gateway, vendors: cardMakerOf},
		Findings: netaudit.NearbyFindings{State: collector.State, Gateway: gateway, Refresh: func() {
			_ = collector.Request() // off, busy and too soon are all fine here: the audit reads what is kept
		}},
		Services: netaudit.NewServiceChecks(netaudit.ServiceDeps{Dial: netTools.DialContext(), HTTP: netTools.HTTPClient()}),
	}
}

// nearbyInterfaces lists the interfaces to ask on. Where the service may not open a netlink socket (a systemd unit with
// RestrictAddressFamilies; the deployment on vertex is one) the system list fails, so the addresses the scan scope reads, with its
// default-route fallback, stand in: only the address is needed to send from it.
func nearbyInterfaces(scope *netaudit.Scope, inScope func(netip.Addr) bool) ([]mcast.Interface, error) {
	if ifs, err := mcast.Interfaces(inScope); err == nil {
		return ifs, nil
	}
	addrs, err := scope.LocalAddrs()
	if err != nil {
		return nil, err
	}
	var out []mcast.Interface
	for _, a := range addrs {
		if inScope(a.Addr) && len(out) < mcast.MaxInterfaces {
			out = append(out, mcast.Interface{Name: a.Name, Addr: a.Addr})
		}
	}
	return out, nil
}

// ownAddressTTL is how long this computer's addresses are remembered: one look asks about hundreds of datagrams.
const ownAddressTTL = 10 * time.Second

// cachedOwnAddress says whether an address is this computer's own, reading the addresses at most once per ttl. When they cannot be
// read it says no: the collector itself drops anything that comes from the address it sends from, which is the case that matters
// (the local responder answering).
func cachedOwnAddress(scope *netaudit.Scope, ttl time.Duration, now func() time.Time) func(netip.Addr) bool {
	var (
		mu   sync.Mutex
		at   time.Time
		held []netaudit.LocalAddr
	)
	return func(a netip.Addr) bool {
		mu.Lock()
		defer mu.Unlock()
		if at.IsZero() || now().Sub(at) > ttl {
			if addrs, err := scope.LocalAddrs(); err == nil {
				held = addrs
			}
			at = now()
		}
		return slices.ContainsFunc(held, func(l netaudit.LocalAddr) bool { return l.Addr == a })
	}
}
