package netaudit

import (
	"context"
	"errors"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"testing"
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
func (f *fakeEngine) Scan(_ context.Context, req ScanRequest) (Result, error) {
	f.calls++
	f.gotReq = req
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
