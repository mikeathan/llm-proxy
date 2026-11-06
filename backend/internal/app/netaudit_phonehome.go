package app

import (
	"context"
	"net/netip"
	"sync"
	"time"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/core/netaudit/phonehome"
	"llm-proxy/internal/core/netaudit/routerlog"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/neighbors"
	"llm-proxy/internal/platform/persistence"
)

// The phone-home watch (Constitution I.8, DNS query lines): the router log's listener hands each DNS query line to the intake, which
// turns the sender's address into a hardware address with this computer's neighbour table and remembers the name for that device.

// phoneHome is the watch and its switch.
type phoneHome struct {
	Config *netaudit.PhoneHomeConfig
	Watch  *phonehome.Watch
	Intake phoneHomeIntake
}

// macTTL is how long the neighbour table is remembered between lookups of a sender's hardware address.
const macTTL = 10 * time.Second

// newPhoneHome builds the watch over its file and the intake the router log listener calls. A file that cannot be read has been set
// aside by the store, so the watch starts empty and the problem is logged.
func (s *AppServices) newPhoneHome(store netaudit.PolicyStore, scope *netaudit.Scope) phoneHome {
	config := netaudit.NewPhoneHomeConfig(store)
	file := withReset(s.AppCtx, "The names devices asked the router for", persistence.NewPhoneHomeStore(s.AppCtx.RootDir()))
	watch, err := phonehome.New(file)
	if err != nil {
		logging.Warn("network phone-home", "error", err.Error())
		watch = phonehome.NewEmpty(file)
	}
	return phoneHome{
		Config: config, Watch: watch,
		Intake: phoneHomeIntake{
			enabled: config.Enabled, watch: watch,
			resolve: cachedMACs(neighborSource{reader: neighbors.NewReader()}, macTTL, time.Now),
			own:     cachedOwnAddress(scope, ownAddressTTL, time.Now),
		},
	}
}

// phoneHomeObserver is what the intake hands a lookup to: *phonehome.Watch.
type phoneHomeObserver interface {
	Observe(mac string, q routerlog.Query, at time.Time)
}

// phoneHomeIntake is the listener's callback. It must not block: it reads two small caches and adds to memory.
type phoneHomeIntake struct {
	enabled func() bool
	resolve func(netip.Addr) (string, bool)
	own     func(netip.Addr) bool
	watch   phoneHomeObserver
}

// Observe keeps a lookup while the watch is on, unless this computer asked, or the sender has no hardware address in the neighbour
// table (the device is keyed by hardware address, never by IP address).
func (p phoneHomeIntake) Observe(q routerlog.Query, at time.Time) {
	if !p.enabled() || p.own(q.From) {
		return
	}
	if mac, ok := p.resolve(q.From); ok {
		p.watch.Observe(mac, q, at)
	}
}

// cachedMACs maps an address to the hardware address the neighbour table holds for it, reading the table at most once per ttl.
func cachedMACs(src netaudit.NeighborSource, ttl time.Duration, now func() time.Time) func(netip.Addr) (string, bool) {
	var (
		mu   sync.Mutex
		read time.Time
		list map[netip.Addr]string
	)
	return func(a netip.Addr) (string, bool) {
		mu.Lock()
		defer mu.Unlock()
		if list == nil || now().Sub(read) >= ttl {
			table, err := src.Neighbors(context.Background())
			read = now()
			list = map[netip.Addr]string{}
			if err == nil {
				for _, n := range append(table.Entries, table.Entries6...) {
					list[n.Addr] = n.MAC
				}
			}
		}
		mac, ok := list[a]
		return mac, ok
	}
}

// phoneHomeSignals is what the nightly report reads: the watch's signals while the watch is on, nothing while it is off.
type phoneHomeSignals struct {
	enabled func() bool
	watch   *phonehome.Watch
	// bad compares a name with the malware list; nil means no list.
	bad func(string) (string, bool)
}

func (p phoneHomeSignals) Signals(devices []phonehome.InventoryDevice, now time.Time) []phonehome.Signal {
	if !p.enabled() {
		return nil
	}
	state := p.watch.Snapshot()
	return phonehome.Judge(state, devices, p.bad, now)
}
