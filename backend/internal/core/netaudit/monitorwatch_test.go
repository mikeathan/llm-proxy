package netaudit

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"llm-proxy/models"
)

// monitorClock is a hand-moved clock for the watch.
type monitorClock struct {
	mu sync.Mutex
	at time.Time
}

func (c *monitorClock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.at }
func (c *monitorClock) add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.at = c.at.Add(d)
}

// monitorFake is one watched part whose reading the test sets.
type monitorFake struct {
	mu      sync.Mutex
	reading MonitorReading
}

func (f *monitorFake) set(r MonitorReading) { f.mu.Lock(); defer f.mu.Unlock(); f.reading = r }
func (f *monitorFake) read() MonitorReading { f.mu.Lock(); defer f.mu.Unlock(); return f.reading }

type monitorSent struct {
	mu   sync.Mutex
	msgs []string
	err  error
}

func (s *monitorSent) notify(_ context.Context, m string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	s.msgs = append(s.msgs, m)
	return nil
}

func (s *monitorSent) count() int { s.mu.Lock(); defer s.mu.Unlock(); return len(s.msgs) }

func newMonitorTest(t *testing.T) (*MonitorWatch, *monitorClock, *monitorFake, *monitorSent) {
	t.Helper()
	clock := &monitorClock{at: time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)}
	part := &monitorFake{}
	sent := &monitorSent{}
	w := NewMonitorWatch(MonitorWatchDeps{
		Parts:  []MonitorPart{{ID: MonitorPresence, Words: "the presence check", Read: part.read}},
		Notify: sent.notify,
		Now:    clock.now,
	})
	return w, clock, part, sent
}

func TestMonitorWatch_AStoppedPartRaisesOneAlertAndOneFinding(t *testing.T) {
	w, clock, part, sent := newMonitorTest(t)
	part.set(MonitorReading{On: true, LastGood: clock.now(), Limit: time.Hour})
	w.Tick(t.Context())
	clock.add(2 * time.Hour) // the part never succeeds again
	w.Tick(t.Context())
	w.Tick(t.Context())
	if sent.count() != 1 {
		t.Fatalf("alerts = %d, want 1", sent.count())
	}
	if !strings.Contains(sent.msgs[0], "the presence check") {
		t.Fatalf("message %q does not name the part", sent.msgs[0])
	}
	findings := MonitorFindings(w.Stalled())
	if len(findings) != 1 || findings[0].Check != CheckMonitorStalled || findings[0].Device != HostDevice {
		t.Fatalf("findings = %+v, want one monitor_stalled finding for this computer", findings)
	}
}

func TestMonitorWatch_ARecoveryClearsTheStallAndAllowsANewAlert(t *testing.T) {
	w, clock, part, sent := newMonitorTest(t)
	part.set(MonitorReading{On: true, LastGood: clock.now(), Limit: time.Hour})
	w.Tick(t.Context())
	clock.add(2 * time.Hour)
	w.Tick(t.Context())
	part.set(MonitorReading{On: true, LastGood: clock.now(), Limit: time.Hour})
	w.Tick(t.Context())
	if got := w.Stalled(); len(got) != 0 {
		t.Fatalf("stalled after recovery = %+v, want none", got)
	}
	clock.add(2 * time.Hour)
	w.Tick(t.Context())
	if sent.count() != 2 {
		t.Fatalf("alerts = %d, want 2 (one per stall)", sent.count())
	}
}

func TestMonitorWatch_APartThatIsOffNeverAlerts(t *testing.T) {
	w, clock, part, sent := newMonitorTest(t)
	part.set(MonitorReading{On: false})
	for range 5 {
		clock.add(10 * time.Hour)
		w.Tick(t.Context())
	}
	if sent.count() != 0 || len(w.Stalled()) != 0 {
		t.Fatalf("alerts = %d, stalled = %v; a part that is off must never count", sent.count(), w.Stalled())
	}
}

func TestMonitorWatch_GivesAPartItsLimitAfterItIsSwitchedOnOrTheAppStarts(t *testing.T) {
	w, clock, part, sent := newMonitorTest(t)
	part.set(MonitorReading{On: true, Limit: time.Hour}) // never succeeded yet: just started, or just switched on
	w.Tick(t.Context())
	clock.add(30 * time.Minute)
	w.Tick(t.Context())
	if sent.count() != 0 {
		t.Fatal("a part was called stalled before its limit had passed since it was first seen on")
	}
	clock.add(31 * time.Minute)
	w.Tick(t.Context())
	if sent.count() != 1 {
		t.Fatalf("alerts = %d, want 1 once the limit passed with no success", sent.count())
	}
}

func TestMonitorWatch_AFailedAlertIsSentAgainOnTheNextTick(t *testing.T) {
	w, clock, part, sent := newMonitorTest(t)
	part.set(MonitorReading{On: true, LastGood: clock.now(), Limit: time.Hour})
	w.Tick(t.Context())
	clock.add(2 * time.Hour)
	sent.err = errors.New("no connector")
	w.Tick(t.Context())
	sent.err = nil
	w.Tick(t.Context())
	w.Tick(t.Context())
	if sent.count() != 1 {
		t.Fatalf("alerts = %d, want 1 after the retry", sent.count())
	}
}

func TestMonitorWatch_RunStopsWithItsContext(t *testing.T) {
	w, _, part, _ := newMonitorTest(t)
	part.set(MonitorReading{On: false})
	w.deps.Every = time.Millisecond
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()
	time.Sleep(5 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not stop with its context")
	}
}

func TestMonitorFindings_NoStallNoFinding(t *testing.T) {
	if got := MonitorFindings(nil); got != nil {
		t.Fatalf("findings = %v, want none", got)
	}
}

type fakeStalls []MonitorStall

func (f fakeStalls) Stalled() []MonitorStall { return f }

func TestRunReporter_AStalledMonitorIsAFindingAboutThisComputer(t *testing.T) {
	store := &memBaselines{data: map[string]models.ScanBaseline{}}
	stall := MonitorStall{ID: MonitorDecoys, Words: "the decoy services", Since: reportTime.Add(-time.Hour)}
	r := reporter(store, reportTime).WithHost(fakeHostLooker{}).WithMonitors(fakeStalls{stall})
	got := r.Augment(scanned(finding("10.0.0.1", 22, "")), "ws", "nightly", "")
	if !strings.Contains(got, "the decoy services") {
		t.Errorf("the stalled monitor must be in the report:\n%s", got)
	}
	if !slices.ContainsFunc(store.data["ws/nightly"].Findings, func(f Finding) bool { return f.Check == CheckMonitorStalled }) {
		t.Fatalf("baseline lacks the monitor finding: %+v", store.data["ws/nightly"].Findings)
	}
}
