// Package traceroute traces the route from this computer to one internet address the traffic look saw, when the operator presses for it
// (Constitution I.14). It sends at most three small probes per distance through routeprobe, one trace at a time, and names each router that
// answers from the tables kept on this computer: nothing is sent anywhere to look a router up. A trace is kept in memory only, and only the last
// few.
package traceroute

import (
	"context"
	"errors"
	"net/netip"
	"slices"
	"sync"
	"time"

	"llm-proxy/internal/platform/routeprobe"
	"llm-proxy/models"
)

const (
	// MaxHops is the farthest distance probed; ProbesPerHop the probes sent at each; ProbeWait how long each waits for its answer.
	MaxHops      = 30
	ProbesPerHop = 3
	ProbeWait    = time.Second
	// silentStop ends a trace after this many distances in a row where no router answered: the rest would only wait.
	silentStop = 5
	// traceLimit bounds one trace, whatever the routers do.
	traceLimit = 90 * time.Second
	// gap is the least time between the end of one trace and the start of the next.
	gap = 3 * time.Second
	// kept is how many traces are remembered, newest first.
	kept = 8
	// basePort is the first destination port of the probes; each probe uses the next, the convention traceroute follows.
	basePort = 33434
)

// Prober sends one probe: routeprobe.Probe satisfies it.
type Prober func(ctx context.Context, r routeprobe.Request) (routeprobe.Reply, error)

// Owners says who holds an internet address and where it is used, from the tables kept on this computer.
type Owners func(netip.Addr) (models.TrafficOwner, bool)

// Deps are what a Manager needs. Permit decides whether this address may be traced now (the network switches, the blocked list, and whether the
// last traffic look saw it); Owners may be nil or return nil when no table is held.
type Deps struct {
	Probe  Prober
	Permit func(netip.Addr) error
	Owners func() Owners
	Now    func() time.Time
}

var (
	// ErrBusy is returned while a trace runs or just ended.
	ErrBusy = errors.New("a route trace is already running or has just ended; wait a few seconds")
	// ErrNotInternet is returned for an address that is not a single internet address: only those are traced.
	ErrNotInternet = errors.New("only an internet address is traced")
	// ErrNotRunning is returned before Run has started or after the application has stopped.
	ErrNotRunning = errors.New("route traces are not available until the service has started")
)

// Manager runs one trace at a time on the application's lifecycle context (given to Run) and keeps the last few.
type Manager struct {
	deps Deps

	mu      sync.Mutex
	life    context.Context
	running bool
	ended   time.Time
	traces  []*models.RouteTrace
}

// New builds a manager; traces start only once Run is running.
func New(deps Deps) *Manager {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &Manager{deps: deps}
}

// Run holds the application's lifecycle context until it ends: every trace runs under it and ends with it. It is started from the one place that
// starts the Network module's background work.
func (m *Manager) Run(ctx context.Context) {
	m.mu.Lock()
	m.life = ctx
	m.mu.Unlock()
	<-ctx.Done()
}

var shared = netip.MustParsePrefix("100.64.0.0/10")

// Where says where an address sits: on this network (private, link-local, loopback), in the provider's shared range (carrier-grade NAT), or on
// the internet.
func Where(a netip.Addr) string {
	a = a.Unmap()
	switch {
	case a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() || a.IsUnspecified():
		return models.HopThisNetwork
	case shared.Contains(a):
		return models.HopProviderShared
	}
	return models.HopInternet
}

// isTraceable is a single internet address: not multicast, not broadcast, not on this network.
func isTraceable(a netip.Addr) bool {
	a = a.Unmap()
	return a.IsValid() && !a.IsMulticast() && a != netip.AddrFrom4([4]byte{255, 255, 255, 255}) && Where(a) == models.HopInternet
}

// Start begins a trace to a and returns at once; Traces shows it filling in.
func (m *Manager) Start(a netip.Addr) error {
	a = a.Unmap()
	if !isTraceable(a) {
		return ErrNotInternet
	}
	if m.deps.Permit != nil {
		if err := m.deps.Permit(a); err != nil {
			return err
		}
	}
	m.mu.Lock()
	if m.life == nil || m.life.Err() != nil {
		m.mu.Unlock()
		return ErrNotRunning
	}
	if m.running || (!m.ended.IsZero() && m.deps.Now().Sub(m.ended) < gap) {
		m.mu.Unlock()
		return ErrBusy
	}
	life := m.life
	m.running = true
	t := &models.RouteTrace{Address: a.String(), State: models.RouteRunning, StartedAt: m.deps.Now(), Hops: []models.RouteHop{}}
	m.traces = slices.DeleteFunc(m.traces, func(old *models.RouteTrace) bool { return old.Address == t.Address })
	m.traces = append([]*models.RouteTrace{t}, m.traces...)[:min(len(m.traces)+1, kept)]
	m.mu.Unlock()
	go m.run(life, a, t)
	return nil
}

// Traces is a copy of the kept traces, newest first.
func (m *Manager) Traces() []models.RouteTrace {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]models.RouteTrace, 0, len(m.traces))
	for _, t := range m.traces {
		c := *t
		c.Hops = slices.Clone(t.Hops)
		out = append(out, c)
	}
	return out
}

// run probes each distance in turn until the address answers, a router says it cannot be reached, too many distances are silent, or the limit
// or the application's end stops it.
func (m *Manager) run(life context.Context, a netip.Addr, t *models.RouteTrace) {
	ctx, cancel := context.WithTimeout(life, traceLimit)
	defer cancel()
	var owners Owners
	if m.deps.Owners != nil {
		owners = m.deps.Owners()
	}
	silent, failure := 0, ""
	for ttl := 1; ttl <= MaxHops && silent < silentStop; ttl++ {
		hop, stop, err := m.hop(ctx, a, ttl)
		if err != nil {
			failure = failureWords(err)
			break
		}
		describe(&hop, owners)
		m.mu.Lock()
		t.Hops = append(t.Hops, hop)
		t.Reached = t.Reached || hop.Reached
		m.mu.Unlock()
		if hop.Address == "" {
			silent++
		} else {
			silent = 0
		}
		if stop {
			break
		}
	}
	m.finish(t, failure)
}

// hop sends the probes of one distance. stop says the trace is over: the address answered, or a router said it cannot be reached.
func (m *Manager) hop(ctx context.Context, a netip.Addr, ttl int) (models.RouteHop, bool, error) {
	hop := models.RouteHop{TTL: ttl}
	var best time.Duration
	var from netip.Addr
	stop := false
	for i := range ProbesPerHop {
		r, err := m.deps.Probe(ctx, routeprobe.Request{Dst: a, TTL: ttl, Port: uint16(basePort + (ttl-1)*ProbesPerHop + i), Wait: ProbeWait})
		if err != nil {
			return hop, true, err
		}
		hop.Sent++
		if r.Kind == routeprobe.Silent || !r.From.IsValid() {
			continue
		}
		hop.Answered++
		if !from.IsValid() {
			from = r.From
		}
		if best == 0 || r.RTT < best {
			best = r.RTT
		}
		switch r.Kind {
		case routeprobe.Reached:
			hop.Reached, stop = true, true
		case routeprobe.Unreachable:
			stop = true
		}
	}
	if from.IsValid() {
		hop.Address, hop.BestMs = from.String(), float64(best.Microseconds())/float64(time.Millisecond/time.Microsecond)
	}
	return hop, stop, nil
}

// describe adds where a hop sits and, for an internet router, who holds it and where it is used.
func describe(hop *models.RouteHop, owners Owners) {
	if hop.Address == "" {
		return
	}
	a := netip.MustParseAddr(hop.Address)
	hop.Where = Where(a)
	if hop.Where != models.HopInternet || owners == nil {
		return
	}
	if o, ok := owners(a); ok {
		hop.Owner = &o
	}
}

// failureWords are the fixed words a failed trace says.
func failureWords(err error) string {
	switch {
	case errors.Is(err, routeprobe.ErrUnsupported):
		return "unsupported"
	case errors.Is(err, context.DeadlineExceeded):
		return "time_limit"
	case errors.Is(err, context.Canceled):
		return "stopped"
	}
	return "probe_failed"
}

func (m *Manager) finish(t *models.RouteTrace, failure string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	end := m.deps.Now()
	t.EndedAt = &end
	t.State = models.RouteDone
	if failure != "" {
		t.State, t.Error = models.RouteFailed, failure
	}
	m.running, m.ended = false, end
}
