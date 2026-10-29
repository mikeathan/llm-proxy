package netaudit

import (
	"context"
	"errors"
	"net/netip"
	"testing"
	"time"

	"llm-proxy/models"
)

func deepHost(addr string) []Host {
	return []Host{{Addr: netip.MustParseAddr(addr), Up: true, Ports: []Port{
		{Number: 22, Proto: "tcp", State: "open", Service: "ssh", Scripts: []Script{{ID: "ssh-auth-methods", Output: sshPassword}}},
	}}}
}

func deepService(engine, fallback *fakeEngine, auth *fixedAuth) *Service {
	if fallback == nil {
		return NewService(auth, engine, nil)
	}
	return NewService(auth, engine, fallback)
}

func TestDeepScan_RunsOneDeviceOnTheGivenPortsWithNmapOnly(t *testing.T) {
	dev := netip.MustParseAddr("10.0.0.5")
	primary := &fakeEngine{name: "nmap", modes: []Mode{ModeDeep}, result: Result{Hosts: deepHost("10.0.0.5")}}
	fallback := &fakeEngine{name: "limited", modes: []Mode{ModeDeep, ModeDiscover, ModePorts}}
	auth := &fixedAuth{addrs: []netip.Addr{dev}}
	res, err := deepService(primary, fallback, auth).DeepScan(context.Background(), "10.0.0.5", []int{22, 443})
	if err != nil {
		t.Fatal(err)
	}
	if primary.calls != 1 || fallback.calls != 0 {
		t.Errorf("only nmap does a deep check, and there is no sweep: nmap %d, limited %d", primary.calls, fallback.calls)
	}
	got := primary.gotReq
	if got.Mode != ModeDeep || got.Ports != "22,443" || len(got.Targets) != 1 || got.Targets[0] != dev || got.Prefiltered || got.Thorough {
		t.Errorf("request = %+v", got)
	}
	if len(auth.got) != 1 || auth.got[0] != "10.0.0.5" {
		t.Errorf("the device is authorised against the scan scope right now: %v", auth.got)
	}
	if res.Engine != "nmap" || len(res.Findings) != 1 || res.Findings[0].Check != CheckSSHPasswordLogin {
		t.Errorf("the findings are the deep checks and nothing else: %+v", res)
	}
}

func TestDeepScan_RefusesWhatIsNotAnOperatorRequestForOneDevice(t *testing.T) {
	dev := netip.MustParseAddr("10.0.0.5")
	primary := &fakeEngine{name: "nmap", modes: []Mode{ModeDeep}}
	svc := deepService(primary, nil, &fixedAuth{addrs: []netip.Addr{dev}})

	if _, err := svc.DeepScan(models.WithUnattendedRun(context.Background()), "10.0.0.5", []int{22}); !errors.Is(err, ErrIntrusiveRefused) {
		t.Errorf("an unattended run must never start one: %v", err)
	}
	for name, device := range map[string]string{"a name": "printer.local", "a range": "10.0.0.0/24", "two": "10.0.0.5,10.0.0.6", "empty": "", "IPv6": "::1"} {
		if _, err := svc.DeepScan(context.Background(), device, []int{22}); !errors.Is(err, ErrBadDevice) {
			t.Errorf("%s: err = %v, want ErrBadDevice", name, err)
		}
	}
	if _, err := svc.DeepScan(context.Background(), "10.0.0.5", nil); !errors.Is(err, ErrNoPorts) {
		t.Errorf("no ports: %v", err)
	}
	if _, err := svc.DeepScan(context.Background(), "10.0.0.5", []int{0}); err == nil {
		t.Error("an invalid port must be refused")
	}
	if primary.calls != 0 {
		t.Errorf("nothing refused may reach the engine (%d calls)", primary.calls)
	}
}

func TestDeepScan_OutOfScopeAndNoNmapAreRefusedBeforeAnyProbe(t *testing.T) {
	scopeErr := errors.New("out of scope")
	primary := &fakeEngine{name: "nmap", modes: []Mode{ModeDeep}}
	if _, err := deepService(primary, nil, &fixedAuth{err: scopeErr}).DeepScan(context.Background(), "10.0.0.5", []int{22}); !errors.Is(err, scopeErr) {
		t.Errorf("a device outside the scan scope: %v", err)
	}
	missing := &fakeEngine{name: "nmap", modes: []Mode{ModeDeep}, unavail: errors.New("nmap not installed")}
	limited := &fakeEngine{name: "limited", modes: []Mode{ModeDeep}}
	_, err := deepService(missing, limited, &fixedAuth{addrs: []netip.Addr{netip.MustParseAddr("10.0.0.5")}}).DeepScan(context.Background(), "10.0.0.5", []int{22})
	if !errors.Is(err, ErrNmapRequired) || limited.calls != 0 || missing.calls != 0 {
		t.Errorf("a host without nmap cannot do a deep check, and the limited engine never stands in: %v", err)
	}
}

// A deep check shares the one-scan-at-a-time lock with every other scan, so it never overlaps a nightly audit.
func TestDeepScan_NeverRunsAlongsideAnotherScan(t *testing.T) {
	dev := netip.MustParseAddr("10.0.0.5")
	hold := make(chan struct{})
	primary := &fakeEngine{name: "nmap", modes: []Mode{ModeServices, ModeDeep}, hold: hold, started: make(chan struct{}), result: Result{Hosts: deepHost("10.0.0.5")}}
	svc := deepService(primary, nil, &fixedAuth{addrs: []netip.Addr{dev}})
	done := make(chan error, 1)
	go func() {
		_, err := svc.Scan(context.Background(), Request{Targets: []string{"10.0.0.5"}, Mode: ModeServices})
		done <- err
	}()
	<-primary.started
	if _, err := svc.DeepScan(context.Background(), "10.0.0.5", []int{22}); !errors.Is(err, ErrScanBusy) {
		t.Errorf("while a scan runs: %v", err)
	}
	close(hold)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestDeepScan_HasAHardTimeLimit(t *testing.T) {
	dev := netip.MustParseAddr("10.0.0.5")
	primary := &fakeEngine{name: "nmap", modes: []Mode{ModeDeep}, result: Result{Hosts: deepHost("10.0.0.5")}}
	ctx, cancel := context.WithCancel(context.Background())
	deadlineSeen := time.Duration(0)
	primary.onScan = func(c context.Context) {
		if d, ok := c.Deadline(); ok {
			deadlineSeen = time.Until(d)
		}
	}
	_, _ = deepService(primary, nil, &fixedAuth{addrs: []netip.Addr{dev}}).DeepScan(ctx, "10.0.0.5", []int{22})
	cancel()
	if deadlineSeen <= 0 || deadlineSeen > DeepBudget {
		t.Errorf("the engine must run under a deadline of at most %v, saw %v", DeepBudget, deadlineSeen)
	}
}
