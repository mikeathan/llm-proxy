package netaudit

import (
	"context"
	"errors"
	"net/netip"
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
	limited := func() *fakeEngine { return &fakeEngine{name: "portprobe", modes: []Mode{ModeDiscover, ModePorts}} }
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
