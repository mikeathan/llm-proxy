package netaudit

import (
	"context"
	"errors"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeEngine struct {
	name    string
	unavail error
	modes   []Mode
	calls   int
	gotReq  ScanRequest
	hold    chan struct{} // when set, Scan blocks until closed
	started chan struct{}
	once    sync.Once
	result  Result
	onScan  func(context.Context) // when set, called with the engine's context
}

func (f *fakeEngine) Name() string                    { return f.name }
func (f *fakeEngine) Available(context.Context) error { return f.unavail }
func (f *fakeEngine) Supports(m Mode) bool {
	for _, x := range f.modes {
		if x == m {
			return true
		}
	}
	return false
}
func (f *fakeEngine) Scan(ctx context.Context, req ScanRequest) (Result, error) {
	f.calls++
	f.gotReq = req
	if f.onScan != nil {
		f.onScan(ctx)
	}
	if f.started != nil {
		f.once.Do(func() { close(f.started) })
	}
	if f.hold != nil {
		<-f.hold
	}
	return f.result, nil
}

type fixedAuth struct {
	addrs []netip.Addr
	err   error
	got   []string
}

func (f *fixedAuth) Authorize(_ context.Context, targets []string) ([]netip.Addr, error) {
	f.got = targets
	return f.addrs, f.err
}

var allModes = []Mode{ModeDiscover, ModePorts, ModeServices}

func newAuth() *fixedAuth { return &fixedAuth{addrs: []netip.Addr{netip.MustParseAddr("192.168.1.5")}} }

func TestService_PrefersNmapAndPassesOnlyAuthorisedAddresses(t *testing.T) {
	nmap := &fakeEngine{name: "nmap", modes: allModes}
	auth := newAuth()
	res, err := NewService(auth, nmap, nil).Scan(context.Background(), Request{Targets: []string{"nas"}, Mode: ModePorts, Ports: "22, 80"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Engine != "nmap" || len(res.Warnings) != 0 {
		t.Errorf("result = %+v", res)
	}
	if len(nmap.gotReq.Targets) != 1 || nmap.gotReq.Targets[0] != auth.addrs[0] || nmap.gotReq.Ports != "22,80" {
		t.Errorf("the engine must get the authorised addresses and normalised ports, got %+v", nmap.gotReq)
	}
}

func TestService_RefusesBeforeAnyEngineRuns(t *testing.T) {
	nmap := &fakeEngine{name: "nmap", modes: allModes}
	denied := errors.New("out of scope")
	tests := []struct {
		name string
		auth *fixedAuth
		req  Request
		want error
	}{
		{"unknown mode", newAuth(), Request{Mode: "vuln"}, ErrBadMode},
		{"bad ports", newAuth(), Request{Mode: ModePorts, Ports: "22;rm"}, ErrBadPorts},
		{"port zero", newAuth(), Request{Mode: ModePorts, Ports: "0"}, ErrBadPorts},
		{"port too high", newAuth(), Request{Mode: ModePorts, Ports: "70000"}, ErrBadPorts},
		{"reversed range", newAuth(), Request{Mode: ModePorts, Ports: "90-80"}, ErrBadPorts},
		{"out of scope", &fixedAuth{err: denied}, Request{Mode: ModePorts}, denied},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewService(tt.auth, nmap, nil).Scan(context.Background(), tt.req); !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}
	if nmap.calls != 0 {
		t.Error("no engine may run for a refused request")
	}
}

func TestService_FallbackRules(t *testing.T) {
	missing := errors.New("nmap is not installed")
	limited := func() *sweepEngine {
		return &sweepEngine{fakeEngine: fakeEngine{name: "portprobe", modes: []Mode{ModeDiscover, ModePorts}}, live: []netip.Addr{netip.MustParseAddr("192.168.1.5")}}
	}
	nmap := func() *fakeEngine { return &fakeEngine{name: "nmap", unavail: missing, modes: allModes} }

	t.Run("an attended scan falls back, labelled", func(t *testing.T) {
		res, err := NewService(newAuth(), nmap(), limited()).Scan(context.Background(), Request{Mode: ModePorts})
		if err != nil || res.Engine != "portprobe" || len(res.Warnings) != 1 {
			t.Fatalf("res = %+v, err = %v", res, err)
		}
	})
	t.Run("an unattended scan never falls back", func(t *testing.T) {
		if _, err := NewService(newAuth(), nmap(), limited()).Scan(context.Background(), Request{Mode: ModePorts, Unattended: true}); !errors.Is(err, ErrNmapRequired) {
			t.Errorf("err = %v, want ErrNmapRequired", err)
		}
	})
	t.Run("a mode that needs nmap never falls back", func(t *testing.T) {
		if _, err := NewService(newAuth(), nmap(), limited()).Scan(context.Background(), Request{Mode: ModeServices}); !errors.Is(err, ErrNmapRequired) {
			t.Errorf("err = %v, want ErrNmapRequired", err)
		}
	})
	t.Run("no fallback engine means no scan", func(t *testing.T) {
		if _, err := NewService(newAuth(), nmap(), nil).Scan(context.Background(), Request{Mode: ModePorts}); !errors.Is(err, ErrNmapRequired) {
			t.Errorf("err = %v, want ErrNmapRequired", err)
		}
	})
}

func TestService_RunsOneScanAtATime(t *testing.T) {
	nmap := &fakeEngine{name: "nmap", modes: allModes, hold: make(chan struct{}), started: make(chan struct{})}
	svc := NewService(newAuth(), nmap, nil)

	done := make(chan error, 1)
	go func() {
		_, err := svc.Scan(context.Background(), Request{Mode: ModePorts})
		done <- err
	}()
	<-nmap.started

	if _, err := svc.Scan(context.Background(), Request{Mode: ModePorts}); !errors.Is(err, ErrScanBusy) {
		t.Errorf("a second scan must be refused while one runs, got %v", err)
	}
	close(nmap.hold)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Scan(context.Background(), Request{Mode: ModePorts}); err != nil {
		t.Errorf("a scan must be allowed once the last one ended: %v", err)
	}
}

// sweepEngine answers a discover scan from a fixed list of live addresses, as the limited prober does.
type sweepEngine struct {
	fakeEngine
	live []netip.Addr
}

func (s *sweepEngine) Scan(ctx context.Context, req ScanRequest) (Result, error) {
	s.calls++
	s.gotReq = req
	var hosts []Host
	for _, a := range req.Targets {
		hosts = append(hosts, Host{Addr: a, Up: slices.Contains(s.live, a)})
	}
	return Result{Hosts: hosts}, nil
}

func fiveAddrs() []netip.Addr {
	return []netip.Addr{
		netip.MustParseAddr("10.0.0.1"), netip.MustParseAddr("10.0.0.2"), netip.MustParseAddr("10.0.0.3"),
	}
}

func TestService_QuickSweepsFirstThenOnlyLooksAtLiveHosts(t *testing.T) {
	addrs := fiveAddrs()
	sweep := &sweepEngine{fakeEngine: fakeEngine{name: "portprobe", modes: []Mode{ModeDiscover, ModePorts}}, live: addrs[1:2]}
	nmap := &fakeEngine{name: "nmap", modes: allModes, result: Result{Hosts: []Host{{Addr: addrs[1], Up: true, Ports: []Port{{Number: 80, Proto: "tcp", State: "open"}}}}}}
	res, err := NewService(&fixedAuth{addrs: addrs}, nmap, sweep).Scan(context.Background(), Request{Mode: ModePorts})
	if err != nil {
		t.Fatal(err)
	}
	if sweep.gotReq.Mode != ModeDiscover || len(sweep.gotReq.Targets) != 3 {
		t.Errorf("the sweep must cover every address in discover mode: %+v", sweep.gotReq)
	}
	if len(nmap.gotReq.Targets) != 1 || nmap.gotReq.Targets[0] != addrs[1] || !nmap.gotReq.Prefiltered || nmap.gotReq.Thorough {
		t.Errorf("the engine must get only the live host, marked prefiltered: %+v", nmap.gotReq)
	}
	if res.Engine != "nmap" || len(res.Hosts) != 3 {
		t.Fatalf("result = %+v", res)
	}
	if res.Hosts[0].Up || !res.Hosts[1].Up || len(res.Hosts[1].Ports) != 1 || res.Hosts[2].Up {
		t.Errorf("dead hosts are reported as down, the live one with its ports: %+v", res.Hosts)
	}
	if len(res.Findings) != 1 {
		t.Errorf("findings = %+v", res.Findings)
	}
}

func TestService_QuickDiscoverIsTheSweepAlone(t *testing.T) {
	addrs := fiveAddrs()
	sweep := &sweepEngine{fakeEngine: fakeEngine{name: "portprobe", modes: allModes}, live: addrs[:2]}
	nmap := &fakeEngine{name: "nmap", modes: allModes}
	res, err := NewService(&fixedAuth{addrs: addrs}, nmap, sweep).Scan(context.Background(), Request{Mode: ModeDiscover})
	if err != nil || res.Engine != "portprobe" || nmap.calls != 0 {
		t.Fatalf("res = %+v, err = %v, nmap calls = %d", res, err, nmap.calls)
	}
	if up := liveAddrs(res.Hosts); len(up) != 2 {
		t.Errorf("live = %v", up)
	}
}

func TestService_NothingAliveMeansNoDeepScan(t *testing.T) {
	sweep := &sweepEngine{fakeEngine: fakeEngine{name: "portprobe", modes: allModes}}
	nmap := &fakeEngine{name: "nmap", modes: allModes}
	res, err := NewService(&fixedAuth{addrs: fiveAddrs()}, nmap, sweep).Scan(context.Background(), Request{Mode: ModeServices})
	if err != nil || nmap.calls != 0 || len(res.Hosts) != 3 {
		t.Errorf("res = %+v, err = %v, nmap calls = %d (nmap must not start when nothing answered)", res, err, nmap.calls)
	}
}

// A thorough scan also sweeps first, on the wider liveness ports, and then hands the live hosts to the engine with the
// thorough profile. Probing every dead address instead took ten minutes for a /24.
func TestService_ThoroughSweepsWiderThenScansTheLiveHostsDeeper(t *testing.T) {
	addrs := fiveAddrs()
	sweep := &sweepEngine{fakeEngine: fakeEngine{name: "portprobe", modes: allModes}, live: addrs[:1]}
	nmap := &fakeEngine{name: "nmap", modes: allModes}
	if _, err := NewService(&fixedAuth{addrs: addrs}, nmap, sweep).Scan(context.Background(), Request{Mode: ModePorts, Thorough: true}); err != nil {
		t.Fatal(err)
	}
	if sweep.calls != 1 || !sweep.gotReq.Thorough || sweep.gotReq.Mode != ModeDiscover {
		t.Errorf("a thorough scan sweeps on the wider ports: %+v", sweep.gotReq)
	}
	if len(nmap.gotReq.Targets) != 1 || !nmap.gotReq.Thorough || !nmap.gotReq.Prefiltered {
		t.Errorf("nmap must get only the live host, with the thorough profile: %+v", nmap.gotReq)
	}
}

func TestService_WithoutASweeperEveryScanGoesStraightToTheEngine(t *testing.T) {
	nmap := &fakeEngine{name: "nmap", modes: allModes}
	res, err := NewService(newAuth(), nmap, nil).Scan(context.Background(), Request{Mode: ModeDiscover})
	if err != nil || res.Engine != "nmap" || nmap.calls != 1 || nmap.gotReq.Prefiltered {
		t.Errorf("res = %+v, err = %v, calls = %d, req = %+v", res, err, nmap.calls, nmap.gotReq)
	}
}

// A live address the engine never reported on (it ran out of time on it) is "not scanned", never "no open ports".
func TestService_ALiveHostTheEngineGaveUpOnIsReportedAsUnscanned(t *testing.T) {
	addrs := fiveAddrs()
	sweep := &sweepEngine{fakeEngine: fakeEngine{name: "portprobe", modes: allModes}, live: addrs[:2]}
	// The engine only reports on the first live host.
	nmap := &fakeEngine{name: "nmap", modes: allModes, result: Result{Hosts: []Host{{Addr: addrs[0], Up: true, Ports: []Port{{Number: 22, Proto: "tcp", State: "open"}}}}}}
	res, err := NewService(&fixedAuth{addrs: addrs}, nmap, sweep).Scan(context.Background(), Request{Mode: ModePorts})
	if err != nil {
		t.Fatal(err)
	}
	if res.Hosts[0].Unscanned || !res.Hosts[1].Up || !res.Hosts[1].Unscanned || res.Hosts[2].Unscanned {
		t.Errorf("hosts = %+v", res.Hosts)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "1 live address(es) were not scanned to completion") {
		t.Errorf("warnings = %v", res.Warnings)
	}
}

// scriptedEngine answers each Scan call with the next result and records every request.
type scriptedEngine struct {
	fakeEngine
	results []Result
	reqs    []ScanRequest
}

func (s *scriptedEngine) Scan(_ context.Context, req ScanRequest) (Result, error) {
	s.reqs = append(s.reqs, req)
	r := s.results[len(s.reqs)-1]
	return r, nil
}

func slowHostScenario() (addrs []netip.Addr, sweep *sweepEngine, open Host) {
	addrs = fiveAddrs()
	sweep = &sweepEngine{fakeEngine: fakeEngine{name: "portprobe", modes: allModes}, live: addrs[:2]}
	open = Host{Addr: addrs[1], Up: true, Ports: []Port{{Number: 22, Proto: "tcp", State: "open"}}}
	return addrs, sweep, open
}

func TestService_ADeviceThatRanOutOfTimeGetsOneLongerTry(t *testing.T) {
	addrs, sweep, open := slowHostScenario()
	first := Result{Hosts: []Host{{Addr: addrs[0], Up: true, Ports: []Port{{Number: 80, Proto: "tcp", State: "open"}}}}} // addrs[1] timed out
	nmap := &scriptedEngine{fakeEngine: fakeEngine{name: "nmap", modes: allModes}, results: []Result{first, {Hosts: []Host{open}}}}
	res, err := NewService(&fixedAuth{addrs: addrs}, nmap, sweep).Scan(context.Background(), Request{Mode: ModeServices, Thorough: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(nmap.reqs) != 2 {
		t.Fatalf("the engine must run twice, got %d", len(nmap.reqs))
	}
	retry := nmap.reqs[1]
	if len(retry.Targets) != 1 || retry.Targets[0] != addrs[1] || retry.HostTimeoutFactor != 2 || retry.Timeout <= 0 {
		t.Errorf("the retry must cover only the slow device, with twice the time: %+v", retry)
	}
	if countUnscanned(res.Hosts) != 0 || len(res.Hosts[1].Ports) != 1 {
		t.Errorf("a device the retry finished is no longer unscanned: %+v", res.Hosts)
	}
	for _, w := range res.Warnings {
		if strings.Contains(w, "not scanned to completion") {
			t.Errorf("nothing is left to warn about: %v", res.Warnings)
		}
	}
}

func TestService_ADeviceThatStillTimesOutStaysFlagged(t *testing.T) {
	addrs, sweep, _ := slowHostScenario()
	nmap := &scriptedEngine{fakeEngine: fakeEngine{name: "nmap", modes: allModes}, results: []Result{{}, {}}}
	res, err := NewService(&fixedAuth{addrs: addrs}, nmap, sweep).Scan(context.Background(), Request{Mode: ModeServices})
	if err != nil {
		t.Fatal(err)
	}
	if len(nmap.reqs) != 2 || countUnscanned(res.Hosts) != 2 {
		t.Errorf("tried %d times, unscanned %d: %+v", len(nmap.reqs), countUnscanned(res.Hosts), res.Hosts)
	}
	if len(res.Warnings) == 0 || !strings.Contains(res.Warnings[len(res.Warnings)-1], "not scanned to completion") {
		t.Errorf("the warning must remain: %v", res.Warnings)
	}
}

func TestService_NoRetryWhenTheBudgetIsAlmostSpent(t *testing.T) {
	addrs, sweep, _ := slowHostScenario()
	nmap := &scriptedEngine{fakeEngine: fakeEngine{name: "nmap", modes: allModes}, results: []Result{{}}}
	svc := NewService(&fixedAuth{addrs: addrs}, nmap, sweep)
	clock := time.Unix(0, 0)
	svc.now = func() time.Time { c := clock; clock = clock.Add(ScanBudget - time.Minute); return c } // first read starts, later reads are late
	if _, err := svc.Scan(context.Background(), Request{Mode: ModeServices}); err != nil {
		t.Fatal(err)
	}
	if len(nmap.reqs) != 1 {
		t.Errorf("with under %s left the device must not be retried, engine ran %d times", retryMinRemaining, len(nmap.reqs))
	}
}

func TestService_UsesTheOperatorsPerDeviceTimeWhenSet(t *testing.T) {
	nmap := &fakeEngine{name: "nmap", modes: allModes}
	svc := NewService(newAuth(), nmap, nil).WithDeviceTimeout(func() time.Duration { return 5 * time.Minute })
	if _, err := svc.Scan(context.Background(), Request{Mode: ModePorts}); err != nil {
		t.Fatal(err)
	}
	if nmap.gotReq.Timeout != ScanBudget {
		t.Errorf("the first pass is bounded by the scan budget, got %v", nmap.gotReq.Timeout)
	}
	if nmap.gotReq.HostTimeout != 5*time.Minute {
		t.Errorf("HostTimeout = %v, want 5m", nmap.gotReq.HostTimeout)
	}
}

func TestService_ADeviceNmapGaveUpOnIsRetriedAndOneWithNothingOpenIsNotFlagged(t *testing.T) {
	addrs := fiveAddrs()
	sweep := &sweepEngine{fakeEngine: fakeEngine{name: "portprobe", modes: allModes}, live: addrs[:2]}
	// First pass: addrs[0] hit the per-device limit (nmap prints it as a host with no ports, flagged), addrs[1] answered
	// but has nothing open (printed as an up host with no open ports).
	first := Result{Hosts: []Host{{Addr: addrs[0], Up: true, Unscanned: true}, {Addr: addrs[1], Up: true}}}
	retry := Result{Hosts: []Host{{Addr: addrs[0], Up: true, Ports: []Port{{Number: 8080, Proto: "tcp", State: "open"}}}}}
	nmap := &scriptedEngine{fakeEngine: fakeEngine{name: "nmap", modes: allModes}, results: []Result{first, retry}}
	res, err := NewService(&fixedAuth{addrs: addrs}, nmap, sweep).Scan(context.Background(), Request{Mode: ModeServices, Thorough: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(nmap.reqs) != 2 || len(nmap.reqs[1].Targets) != 1 || nmap.reqs[1].Targets[0] != addrs[0] {
		t.Fatalf("only the device nmap gave up on is retried: %+v", nmap.reqs)
	}
	if countUnscanned(res.Hosts) != 0 {
		t.Errorf("the retried device finished and the quiet one was never in doubt: %+v", res.Hosts)
	}
	for _, w := range res.Warnings {
		if strings.Contains(w, "not scanned to completion") {
			t.Errorf("nothing is left to warn about: %v", res.Warnings)
		}
	}
}

// An intrusive mode is refused by the ordinary scan path, whoever asks (an agent's tool call and a nightly audit both come
// through it), before the network or the authorizer is touched.
func TestScan_RefusesAnIntrusiveMode(t *testing.T) {
	primary := &fakeEngine{name: "nmap", modes: []Mode{ModeDeep}}
	auth := &fixedAuth{addrs: []netip.Addr{netip.MustParseAddr("10.0.0.5")}}
	svc := NewService(auth, primary, nil)
	for _, unattended := range []bool{false, true} {
		_, err := svc.Scan(context.Background(), Request{Targets: []string{"10.0.0.5"}, Mode: ModeDeep, Ports: "22", Unattended: unattended})
		if !errors.Is(err, ErrIntrusiveRefused) {
			t.Errorf("unattended=%v: err = %v, want ErrIntrusiveRefused", unattended, err)
		}
	}
	if primary.calls != 0 || auth.got != nil {
		t.Errorf("a refused mode must not reach the engine (%d calls) or the authorizer (%v)", primary.calls, auth.got)
	}
}
