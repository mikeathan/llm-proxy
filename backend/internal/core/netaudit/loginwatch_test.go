package netaudit

import (
	"context"
	"errors"
	"io/fs"
	"slices"
	"strings"
	"testing"
	"time"
)

var loginNow = time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)

// loginRig is a watch over a list of failure times it is given, with a clock the test moves.
type loginRig struct {
	w     *LoginWatch
	times []time.Time
	err   error
	reads int
	on    bool
	sent  []string
	fail  bool
	now   time.Time
	errs  []error
}

func newLoginRig(t *testing.T) *loginRig {
	t.Helper()
	r := &loginRig{on: true, now: loginNow}
	r.w = NewLoginWatch(LoginWatchDeps{
		Read: func(after time.Time) ([]time.Time, error) {
			r.reads++
			var out []time.Time
			for _, at := range r.times {
				if at.After(after) {
					out = append(out, at)
				}
			}
			return out, r.err
		},
		Enabled: func() bool { return r.on },
		Notify: func(_ context.Context, m string) error {
			if r.fail {
				return errors.New("no connector")
			}
			r.sent = append(r.sent, m)
			return nil
		},
		OnError: func(err error) { r.errs = append(r.errs, err) },
		Now:     func() time.Time { return r.now },
	})
	return r
}

func (r *loginRig) failures(n int, within time.Duration) {
	for i := 0; i < n; i++ {
		r.times = append(r.times, r.now.Add(-within*time.Duration(n-i)/time.Duration(n+1)))
	}
}

func TestLoginWatch_TellsOfABurstOfFailedLoginsOnceAndNotOfAFew(t *testing.T) {
	r := newLoginRig(t)
	r.failures(loginBurstFailures-1, loginBurstWindow)
	r.w.Tick(context.Background())
	if len(r.sent) != 0 {
		t.Fatalf("sent = %q for %d failures, want none", r.sent, loginBurstFailures-1)
	}
	r.failures(1, time.Minute)
	r.w.Tick(context.Background())
	r.w.Tick(context.Background())
	if len(r.sent) != 1 || !strings.Contains(r.sent[0], "failed") {
		t.Fatalf("sent = %q, want one message", r.sent)
	}
}

func TestLoginWatch_AMessageNamesNeitherAUserNorAnAddress(t *testing.T) {
	msg := loginBurstMessage()
	for _, bad := range []string{"@", "root", "from "} {
		if strings.Contains(msg, bad) {
			t.Errorf("message %q holds %q", msg, bad)
		}
	}
}

func TestLoginWatch_TellsAgainOnlyAfterTheGapAndWithANewBurst(t *testing.T) {
	r := newLoginRig(t)
	r.failures(loginBurstFailures, loginBurstWindow)
	r.w.Tick(context.Background())
	r.now = r.now.Add(loginAlertGap - time.Minute)
	r.failures(loginBurstFailures, loginBurstWindow)
	r.w.Tick(context.Background())
	if len(r.sent) != 1 {
		t.Fatalf("sent = %d inside the gap, want 1", len(r.sent))
	}
	r.now = r.now.Add(2 * time.Minute)
	r.failures(loginBurstFailures, loginBurstWindow)
	r.w.Tick(context.Background())
	if len(r.sent) != 2 {
		t.Fatalf("sent = %d after the gap, want 2", len(r.sent))
	}
}

func TestLoginWatch_ADeliveryThatFailedIsTriedAgainAtTheNextLook(t *testing.T) {
	r := newLoginRig(t)
	r.failures(loginBurstFailures, loginBurstWindow)
	r.fail = true
	r.w.Tick(context.Background())
	r.fail = false
	r.w.Tick(context.Background())
	if len(r.sent) != 1 {
		t.Fatalf("sent = %d, want the message tried again once it can be delivered", len(r.sent))
	}
}

func TestLoginWatch_ReadsNothingWhileSwitchedOff(t *testing.T) {
	r := newLoginRig(t)
	r.on = false
	r.failures(loginBurstFailures, loginBurstWindow)
	r.w.Tick(context.Background())
	if r.reads != 0 || len(r.sent) != 0 {
		t.Fatalf("reads = %d sent = %d while off", r.reads, len(r.sent))
	}
	if s := r.w.Snapshot(r.now); s.Failures != 0 || s.Bursts != 0 {
		t.Fatalf("snapshot = %+v while off", s)
	}
}

func TestLoginWatch_AFileItCannotReadIsAFailedPartOnceSaidAndNeverABurst(t *testing.T) {
	r := newLoginRig(t)
	r.err = fs.ErrPermission
	r.w.Tick(context.Background())
	r.w.Tick(context.Background())
	if len(r.errs) != 1 {
		t.Fatalf("errors told = %d, want the same one only once", len(r.errs))
	}
	if reading := r.w.Reading(); !reading.On || !reading.Failed {
		t.Fatalf("reading = %+v, want a failed part", reading)
	}
	r.err = nil
	r.w.Tick(context.Background())
	if reading := r.w.Reading(); reading.Failed || reading.LastGood.IsZero() {
		t.Fatalf("reading = %+v, want a working part once the file can be read", reading)
	}
}

func TestLoginWatch_ARecordFromTheFutureCountsForNothing(t *testing.T) {
	r := newLoginRig(t)
	for i := 0; i < 3*loginBurstFailures; i++ {
		r.times = append(r.times, r.now.Add(48*time.Hour))
	}
	r.w.Tick(context.Background())
	if len(r.sent) != 0 || r.w.Snapshot(r.now).Failures != 0 {
		t.Fatal("records dated in the future were counted")
	}
}

func TestLoginWatch_SnapshotCountsTheLastDaysFailuresAndBursts(t *testing.T) {
	r := newLoginRig(t)
	// Two bursts a few hours apart and a few single failures, plus one older than a day.
	r.times = append(r.times, r.now.Add(-30*time.Hour))
	for i := 0; i < loginBurstFailures; i++ {
		r.times = append(r.times, r.now.Add(-5*time.Hour+time.Duration(i)*time.Second))
		r.times = append(r.times, r.now.Add(-2*time.Hour+time.Duration(i)*time.Second))
	}
	r.times = append(r.times, r.now.Add(-time.Hour), r.now.Add(-30*time.Minute))
	slicesSortTimes(r.times)
	got := r.w.Snapshot(r.now)
	if got.Failures != 2*loginBurstFailures+2 || got.Bursts != 2 {
		t.Fatalf("snapshot = %+v, want %d failures and 2 bursts", got, 2*loginBurstFailures+2)
	}
}

func TestLoginWatch_WhatItHoldsDoesNotGrowWithTheNumberOfFailures(t *testing.T) {
	r := newLoginRig(t)
	for i := 0; i < 50000; i++ {
		r.times = append(r.times, r.now.Add(-time.Duration(i)*time.Millisecond))
	}
	slicesSortTimes(r.times)
	r.w.Tick(context.Background())
	r.w.Tick(context.Background())
	if len(r.sent) != 1 {
		t.Fatalf("sent = %d for a flood, want one message", len(r.sent))
	}
}

func slicesSortTimes(ts []time.Time) { slices.SortFunc(ts, time.Time.Compare) }

func TestLoginFindings_OnlyABurstIsAFindingAndItIsKeyedByThisComputer(t *testing.T) {
	if got := LoginFindings(LoginSnapshot{Failures: 6}); len(got) != 0 {
		t.Fatalf("findings = %+v for single failures, want none", got)
	}
	got := LoginFindings(LoginSnapshot{Failures: 40, Bursts: 2})
	if len(got) != 1 || got[0].Device != HostDevice || got[0].Check != CheckHostLoginFailures || got[0].Severity != SeverityMedium {
		t.Fatalf("findings = %+v", got)
	}
	if !strings.Contains(got[0].Evidence, "2 burst") || CheckTitle(CheckHostLoginFailures) == "" || RemediationFor(CheckHostLoginFailures) == "" {
		t.Fatalf("evidence = %q", got[0].Evidence)
	}
	if _, ok := guides[CheckHostLoginFailures]; !ok {
		t.Fatal("the check needs a fix guide")
	}
}

func TestLoginWatchConfig_IsOffUntilSwitchedOn(t *testing.T) {
	store := &memConfig{}
	c := NewLoginWatchConfig(store)
	if c.Enabled() {
		t.Fatal("reading the system's record of failed logins is off until the operator turns it on")
	}
	if err := c.Set(true); err != nil || !c.Enabled() || !store.Get().LoginWatchEnabled {
		t.Errorf("on: err=%v enabled=%v", err, c.Enabled())
	}
}

type fakeLogins struct{ snap LoginSnapshot }

func (f fakeLogins) Snapshot(time.Time) LoginSnapshot { return f.snap }

func TestRunReporter_AddsTheLoginFindingOfThisComputer(t *testing.T) {
	r := NewRunReporter(nil).WithLoginWatch(fakeLogins{LoginSnapshot{Failures: 30, Bursts: 1}})
	result := Result{}
	r.addLogins(&result)
	if len(result.Findings) != 1 || result.Findings[0].Check != CheckHostLoginFailures {
		t.Fatalf("findings = %+v", result.Findings)
	}
	NewRunReporter(nil).addLogins(&result) // a reporter without the watch adds nothing and does not fail
}

func TestLoginFindings_ADayThatHitTheReadBoundSaysAtLeast(t *testing.T) {
	got := LoginFindings(LoginSnapshot{Failures: 4096, Bursts: 400, Capped: true})
	if len(got) != 1 || !strings.Contains(got[0].Evidence, "at least 4096 failures") {
		t.Fatalf("findings = %+v", got)
	}
	r := newLoginRig(t)
	r.w.deps.Cap = 5
	for i := 0; i < 5; i++ {
		r.times = append(r.times, r.now.Add(-time.Duration(i)*time.Minute))
	}
	slicesSortTimes(r.times)
	if s := r.w.Snapshot(r.now); !s.Capped || s.Failures != 5 {
		t.Fatalf("snapshot = %+v, want it capped at the bound", s)
	}
}
