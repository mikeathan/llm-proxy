package traceroute

import (
	"context"
	"errors"
	"net/netip"
	"sync"
	"testing"
	"time"

	"llm-proxy/internal/platform/routeprobe"
	"llm-proxy/models"
)

var target = netip.MustParseAddr("1.1.1.1")

// route answers each distance from a fixed list; past its end the address itself answers, or nothing does when silent is set.
type route struct {
	mu     sync.Mutex
	hops   []string
	silent bool
	sent   []routeprobe.Request
	err    error
}

func (r *route) probe(_ context.Context, req routeprobe.Request) (routeprobe.Reply, error) {
	r.mu.Lock()
	r.sent = append(r.sent, req)
	r.mu.Unlock()
	if r.err != nil {
		return routeprobe.Reply{}, r.err
	}
	if req.TTL <= len(r.hops) {
		if r.hops[req.TTL-1] == "" {
			return routeprobe.Reply{Kind: routeprobe.Silent}, nil
		}
		return routeprobe.Reply{Kind: routeprobe.Hop, From: netip.MustParseAddr(r.hops[req.TTL-1]), RTT: time.Duration(req.TTL) * time.Millisecond}, nil
	}
	if r.silent {
		return routeprobe.Reply{Kind: routeprobe.Silent}, nil
	}
	return routeprobe.Reply{Kind: routeprobe.Reached, From: req.Dst, RTT: 20 * time.Millisecond}, nil
}

func owners() Owners {
	return func(a netip.Addr) (models.TrafficOwner, bool) {
		if a.String() == "62.252.1.1" {
			return models.TrafficOwner{ASN: 5089, Name: "Virgin Media", City: "London"}, true
		}
		return models.TrafficOwner{}, false
	}
}

// running builds a manager and starts its Run, as the service does.
func running(t *testing.T, life context.Context, deps Deps) *Manager {
	t.Helper()
	ctx, cancel := context.WithCancel(life)
	t.Cleanup(cancel)
	m := New(deps)
	go m.Run(ctx)
	for {
		m.mu.Lock()
		ready := m.life != nil
		m.mu.Unlock()
		if ready {
			return m
		}
		time.Sleep(time.Millisecond)
	}
}

func TestNoTraceStartsBeforeTheServiceRunsOrAfterItStops(t *testing.T) {
	m := New(Deps{Probe: (&route{}).probe})
	if err := m.Start(target); !errors.Is(err, ErrNotRunning) {
		t.Fatal(err)
	}
	life, stop := context.WithCancel(context.Background())
	m = running(t, life, Deps{Probe: (&route{}).probe})
	stop()
	if err := m.Start(target); !errors.Is(err, ErrNotRunning) {
		t.Fatal(err)
	}
}

func wait(t *testing.T, m *Manager) models.RouteTrace {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if ts := m.Traces(); len(ts) > 0 && ts[0].State != models.RouteRunning {
			return ts[0]
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("the trace did not end")
	return models.RouteTrace{}
}

func TestATraceNamesEachRouterAndEndsWhenTheAddressAnswers(t *testing.T) {
	r := &route{hops: []string{"192.168.50.1", "100.64.0.1", "", "62.252.1.1"}}
	m := running(t, context.Background(), Deps{Probe: r.probe, Owners: owners})
	if err := m.Start(target); err != nil {
		t.Fatal(err)
	}
	tr := wait(t, m)
	if tr.State != models.RouteDone || !tr.Reached || len(tr.Hops) != 5 || tr.EndedAt == nil {
		t.Fatalf("%+v", tr)
	}
	want := []struct{ addr, where string }{
		{"192.168.50.1", models.HopThisNetwork}, {"100.64.0.1", models.HopProviderShared}, {"", ""}, {"62.252.1.1", models.HopInternet}, {"1.1.1.1", models.HopInternet},
	}
	for i, w := range want {
		if h := tr.Hops[i]; h.TTL != i+1 || h.Address != w.addr || h.Where != w.where || h.Sent != ProbesPerHop {
			t.Errorf("hop %d = %+v", i+1, h)
		}
	}
	if o := tr.Hops[3].Owner; o == nil || o.Name != "Virgin Media" || tr.Hops[3].BestMs != 4 {
		t.Errorf("the internet router was not named: %+v", tr.Hops[3])
	}
	if tr.Hops[0].Owner != nil || tr.Hops[2].Answered != 0 || !tr.Hops[4].Reached {
		t.Errorf("%+v", tr.Hops)
	}
}

func TestEachProbeUsesItsOwnPortAndNoMoreThanThreeAreSentPerDistance(t *testing.T) {
	r := &route{hops: []string{"192.168.50.1"}}
	m := running(t, context.Background(), Deps{Probe: r.probe})
	_ = m.Start(target)
	wait(t, m)
	seen := map[uint16]bool{}
	for _, req := range r.sent {
		if seen[req.Port] || req.Wait != ProbeWait || req.Dst != target {
			t.Fatalf("%+v", req)
		}
		seen[req.Port] = true
	}
	if len(r.sent) != 2*ProbesPerHop {
		t.Fatalf("sent %d probes", len(r.sent))
	}
}

func TestATraceStopsAfterFiveSilentDistancesInARow(t *testing.T) {
	r := &route{hops: []string{"192.168.50.1"}, silent: true}
	m := running(t, context.Background(), Deps{Probe: r.probe})
	_ = m.Start(target)
	tr := wait(t, m)
	if tr.Reached || len(tr.Hops) != 1+silentStop || tr.State != models.RouteDone {
		t.Fatalf("%+v", tr)
	}
}

func TestOnlyASingleInternetAddressIsTracedAndThePermitIsAsked(t *testing.T) {
	m := running(t, context.Background(), Deps{Probe: (&route{}).probe})
	for _, a := range []string{"192.168.50.1", "10.0.0.1", "100.64.1.1", "224.0.0.251", "255.255.255.255", "127.0.0.1", "fe80::1", "fd00::1", "ff02::fb"} {
		if err := m.Start(netip.MustParseAddr(a)); !errors.Is(err, ErrNotInternet) {
			t.Errorf("%s: %v", a, err)
		}
	}
	refused := errors.New("internet access is off")
	m = running(t, context.Background(), Deps{Probe: (&route{}).probe, Permit: func(netip.Addr) error { return refused }})
	if err := m.Start(target); !errors.Is(err, refused) || len(m.Traces()) != 0 {
		t.Fatalf("%v", err)
	}
}

func TestOneTraceAtATimeAndAShortGapAfterIt(t *testing.T) {
	block := make(chan struct{})
	probe := func(ctx context.Context, r routeprobe.Request) (routeprobe.Reply, error) {
		<-block
		return routeprobe.Reply{Kind: routeprobe.Reached, From: r.Dst, RTT: time.Millisecond}, nil
	}
	now := time.Unix(1000, 0)
	var mu sync.Mutex
	clock := func() time.Time { mu.Lock(); defer mu.Unlock(); return now }
	m := running(t, context.Background(), Deps{Probe: probe, Now: clock})
	if err := m.Start(target); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(netip.MustParseAddr("8.8.8.8")); !errors.Is(err, ErrBusy) {
		t.Fatalf("a second trace started while one ran: %v", err)
	}
	close(block)
	wait(t, m)
	if err := m.Start(target); !errors.Is(err, ErrBusy) {
		t.Fatalf("a trace started right after one ended: %v", err)
	}
	mu.Lock()
	now = now.Add(gap)
	mu.Unlock()
	if err := m.Start(target); err != nil {
		t.Fatal(err)
	}
	wait(t, m)
	if len(m.Traces()) != 1 {
		t.Fatalf("a second trace of one address was kept beside the first: %d", len(m.Traces()))
	}
}

func TestOnlyTheLastFewTracesAreKeptNewestFirst(t *testing.T) {
	now := time.Unix(1000, 0)
	m := running(t, context.Background(), Deps{Probe: (&route{}).probe, Now: func() time.Time { return now }})
	for i := range kept + 2 {
		now = now.Add(gap)
		a := netip.AddrFrom4([4]byte{1, 1, 1, byte(i + 1)})
		if err := m.Start(a); err != nil {
			t.Fatal(err)
		}
		wait(t, m)
	}
	ts := m.Traces()
	if len(ts) != kept || ts[0].Address != "1.1.1.10" {
		t.Fatalf("%d %s", len(ts), ts[0].Address)
	}
}

func TestATraceWhereProbesCannotBeSentFailsInFixedWords(t *testing.T) {
	m := running(t, context.Background(), Deps{Probe: (&route{err: routeprobe.ErrUnsupported}).probe})
	_ = m.Start(target)
	if tr := wait(t, m); tr.State != models.RouteFailed || tr.Error != "unsupported" {
		t.Fatalf("%+v", tr)
	}
}

func TestATraceEndsWhenTheApplicationStops(t *testing.T) {
	life, stop := context.WithCancel(context.Background())
	probe := func(ctx context.Context, r routeprobe.Request) (routeprobe.Reply, error) {
		<-ctx.Done()
		return routeprobe.Reply{}, ctx.Err()
	}
	m := running(t, life, Deps{Probe: probe})
	_ = m.Start(target)
	stop()
	if tr := wait(t, m); tr.State != models.RouteFailed || tr.Error != "stopped" {
		t.Fatalf("%+v", tr)
	}
}
