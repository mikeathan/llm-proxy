package netaudit

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/core/netaudit/routerhistory"
)

type fakeRouterConn struct {
	on       bool
	err      error
	refreshs int
	signals  []routerhistory.Signal
}

func (f *fakeRouterConn) Enabled() bool { return f.on }
func (f *fakeRouterConn) Refresh(context.Context) error {
	f.refreshs++
	return f.err
}
func (f *fakeRouterConn) Signals(time.Time) []routerhistory.Signal { return f.signals }

func routerConnReporter(src RouterConnSource, health *RunHealth) *RunReporter {
	r := NewRunReporter(nil).WithRouterConn(src)
	r.health = health
	return r
}

func resultOf(ips ...string) Result {
	var res Result
	for _, ip := range ips {
		res.Hosts = append(res.Hosts, Host{Addr: netip.MustParseAddr(ip)})
	}
	return res
}

func TestRouterConnFindingsAreKeyedByTheDeviceAndSayOnlyFixedWordsAndTheHoldersNames(t *testing.T) {
	got := RouterConnFindings([]routerhistory.Signal{
		{Kind: routerhistory.NewCompany, IP: "192.168.50.9", Names: []string{"Zeta", "Alibaba (US)"}},
		{Kind: routerhistory.NewCountry, IP: "192.168.50.9", Names: []string{"DE"}},
	})
	if len(got) != 2 || got[0].Check != CheckRouterNewCompany || got[1].Check != CheckRouterNewCountry {
		t.Fatalf("findings = %+v", got)
	}
	if got[0].Device != "192.168.50.9" || got[0].Severity != SeverityLow || !strings.Contains(got[0].Evidence, "Alibaba (US), Zeta") {
		t.Fatalf("company finding = %+v", got[0])
	}
	if got[0].Version == "" || got[0].Version == got[1].Version {
		t.Fatalf("versions must be a digest of the set: %q %q", got[0].Version, got[1].Version)
	}
	if CheckTitle(CheckRouterNewCompany) == "" || CheckTitle(CheckRouterNewCountry) == "" {
		t.Fatal("every check has a title")
	}
}

func TestTheNightlyReadDoesNothingWhileTheWatchIsOff(t *testing.T) {
	src := &fakeRouterConn{on: false, signals: []routerhistory.Signal{{Kind: routerhistory.NewCompany, IP: "192.168.50.9", Names: []string{"X"}}}}
	res := resultOf("192.168.50.9")
	note := routerConnReporter(src, NewRunHealth()).addRouterConn(context.Background(), &res)
	if src.refreshs != 0 || len(res.Findings) != 0 || note != "" {
		t.Fatalf("a watch that is off must not read the router or add findings (refreshs=%d findings=%d)", src.refreshs, len(res.Findings))
	}
}

func TestTheNightlyReadReadsOnceAndAddsFindingsOnlyForScannedDevices(t *testing.T) {
	src := &fakeRouterConn{on: true, signals: []routerhistory.Signal{
		{Kind: routerhistory.NewCompany, IP: "192.168.50.9", Names: []string{"X"}},
		{Kind: routerhistory.NewCompany, IP: "192.168.50.77", Names: []string{"Y"}},
	}}
	health := NewRunHealth()
	res := resultOf("192.168.50.9")
	note := routerConnReporter(src, health).addRouterConn(context.Background(), &res)
	if src.refreshs != 1 || note != "" || len(res.Findings) != 1 || res.Findings[0].Device != "192.168.50.9" {
		t.Fatalf("refreshs=%d note=%q findings=%+v", src.refreshs, note, res.Findings)
	}
	if r := health.Reading(MonitorRouterConn, true, time.Hour); r.Failed || r.LastGood.IsZero() {
		t.Fatalf("a read that worked is recorded as good: %+v", r)
	}
}

func TestAFailedNightlyReadIsSaidAndRecordedNeverReadAsNothingNew(t *testing.T) {
	src := &fakeRouterConn{on: true, err: errors.New("the router refused the key")}
	health := NewRunHealth()
	res := resultOf("192.168.50.9")
	note := routerConnReporter(src, health).addRouterConn(context.Background(), &res)
	if !strings.Contains(note, "could not be read") || !strings.Contains(note, "refused the key") {
		t.Fatalf("note = %q", note)
	}
	if r := health.Reading(MonitorRouterConn, true, time.Hour); !r.Failed {
		t.Fatalf("a failed read must show as failed: %+v", r)
	}
}

func TestANightWithNothingToReadIsNeitherGoodNorFailed(t *testing.T) {
	src := &fakeRouterConn{on: true, err: ErrRouterConnSkipped}
	health := NewRunHealth()
	res := resultOf("192.168.50.9")
	if note := routerConnReporter(src, health).addRouterConn(context.Background(), &res); note != "" {
		t.Fatalf("note = %q", note)
	}
	if r := health.Reading(MonitorRouterConn, true, time.Hour); r.Failed || !r.LastGood.IsZero() {
		t.Fatalf("a skipped night records nothing: %+v", r)
	}
}
