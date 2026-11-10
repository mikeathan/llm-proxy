package netaudit

import (
	"context"
	"errors"
	"testing"
	"time"

	"llm-proxy/models"
)

var recheckNow = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

func newRecheck(base models.ScanBaseline, scan func(context.Context, Request) (Result, error)) (*Recheck, *memLife) {
	life := &memLife{}
	r := NewRecheck(RecheckDeps{Scan: scan, Baselines: baselineReader{base}, Life: life, Now: func() time.Time { return recheckNow }})
	return r, life
}

func smbBase() models.ScanBaseline {
	return models.ScanBaseline{ScannedAt: recheckNow.Add(-8 * time.Hour), Findings: []models.ScanFinding{smbFinding("10.0.0.1")}}
}

func scanOf(hosts ...Host) func(context.Context, Request) (Result, error) {
	return func(context.Context, Request) (Result, error) { return Result{Engine: "nmap", Hosts: hosts}, nil }
}

func TestRecheck_AskedOneDeviceOnePortWithTheChecks(t *testing.T) {
	var got Request
	r, _ := newRecheck(smbBase(), func(_ context.Context, req Request) (Result, error) {
		got = req
		return Result{Engine: "nmap", Hosts: []Host{smbHost("10.0.0.1", true)}}, nil
	})
	if _, err := r.Check(context.Background(), "ws", "nightly", smbKey("10.0.0.1")); err != nil {
		t.Fatal(err)
	}
	if len(got.Targets) != 1 || got.Targets[0] != "10.0.0.1" || got.Ports != "445" || got.Mode != ModeChecks || !got.Unattended {
		t.Errorf("request %+v: one address, one port, the checks, never the limited engine", got)
	}
}

func TestRecheck_FixedWhenTheDeviceAnswersAndTheFindingIsGone(t *testing.T) {
	r, life := newRecheck(smbBase(), scanOf(smbHost("10.0.0.1", true)))
	res, err := r.Check(context.Background(), "ws", "nightly", smbKey("10.0.0.1"))
	if err != nil || res.Outcome != RecheckFixed {
		t.Fatalf("got %+v, %v", res, err)
	}
	l := lifeOf(t, life.records, smbFinding("10.0.0.1"))
	if l.Open() || l.ResolvedBy != models.LifeByRecheck || !l.ResolvedAt.Equal(recheckNow) {
		t.Errorf("history not closed by the recheck: %+v", l)
	}
}

func TestRecheck_StillThere(t *testing.T) {
	r, life := newRecheck(smbBase(), scanOf(smbHost("10.0.0.1", false)))
	res, _ := r.Check(context.Background(), "ws", "nightly", smbKey("10.0.0.1"))
	if res.Outcome != RecheckStillThere {
		t.Errorf("got %+v", res)
	}
	if l := lifeOf(t, life.records, smbFinding("10.0.0.1")); !l.Open() || !l.LastSeen.Equal(recheckNow) {
		t.Errorf("history: %+v", l)
	}
}

// A check that could not be made is never "fixed": a device that did not answer, one the engine ran out of time on and a failed
// scan all say they could not check.
func TestRecheck_ACheckThatCouldNotBeMadeIsNeverFixed(t *testing.T) {
	down := Host{Addr: smbHost("10.0.0.1", true).Addr, Up: false}
	unscanned := smbHost("10.0.0.1", true)
	unscanned.Unscanned = true
	cases := map[string]func(context.Context, Request) (Result, error){
		"down":      scanOf(down),
		"missing":   scanOf(),
		"unscanned": scanOf(unscanned),
		"error":     func(context.Context, Request) (Result, error) { return Result{}, errors.New("boom") },
		"no nmap":   func(context.Context, Request) (Result, error) { return Result{}, ErrNmapRequired },
	}
	for name, scan := range cases {
		r, life := newRecheck(smbBase(), scan)
		res, err := r.Check(context.Background(), "ws", "nightly", smbKey("10.0.0.1"))
		if err != nil || res.Outcome != RecheckCouldNot || res.Reason == "" {
			t.Errorf("%s: %+v, %v", name, res, err)
		}
		if len(life.records) != 0 {
			t.Errorf("%s: a check that was not made must change no history: %+v", name, life.records)
		}
	}
}

func TestRecheck_ABusyScannerIsReportedAsBusy(t *testing.T) {
	r, _ := newRecheck(smbBase(), func(context.Context, Request) (Result, error) { return Result{}, ErrScanBusy })
	if _, err := r.Check(context.Background(), "ws", "nightly", smbKey("10.0.0.1")); !errors.Is(err, ErrScanBusy) {
		t.Errorf("want ErrScanBusy, got %v", err)
	}
}

func TestRecheck_RefusesWhatAScanOfOneDeviceCannotJudge(t *testing.T) {
	deep := Finding{Device: "10.0.0.1", Port: 22, Proto: "tcp", Check: CheckSSHPasswordLogin, Severity: "medium"}
	other := Finding{Device: "10.0.0.1", Check: "dns_router_differs", Severity: "high"}
	udp := Finding{Device: "10.0.0.1", Port: 161, Proto: "udp", Check: CheckSMBv1, Severity: "high"}
	base := models.ScanBaseline{ScannedAt: recheckNow, Findings: []models.ScanFinding{deep, other, udp}}
	r, _ := newRecheck(base, func(context.Context, Request) (Result, error) { t.Error("must not scan"); return Result{}, nil })
	if _, err := r.Check(context.Background(), "ws", "a", deep.Key()); !errors.Is(err, ErrUseDeepCheck) {
		t.Errorf("a deep finding is rechecked with the deep check: %v", err)
	}
	for _, f := range []Finding{other, udp} {
		if _, err := r.Check(context.Background(), "ws", "a", f.Key()); !errors.Is(err, ErrNotRecheckable) {
			t.Errorf("%s: %v", f.Check, err)
		}
	}
	if _, err := r.Check(context.Background(), "ws", "a", "nope"); !errors.Is(err, ErrNoSuchFinding) {
		t.Errorf("unknown key: %v", err)
	}
}

// Every check a scan of one port produces is rechecked; a check the table says nothing about is not guessed at.
func TestRecheck_EveryPortRuleCheckIsRecheckable(t *testing.T) {
	for _, rule := range portRules {
		if !scanChecks[rule.check] {
			t.Errorf("%s is produced by the port rules but is not in scanChecks", rule.check)
		}
	}
}
