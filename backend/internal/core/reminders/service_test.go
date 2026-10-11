package reminders

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// ── fakes ───────────────────────────────────────────────────

type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fakeTimer
}

type fakeTimer struct {
	clock   *fakeClock
	c       chan time.Time
	at      time.Time
	stopped bool
	fired   bool
}

func (t *fakeTimer) C() <-chan time.Time { return t.c }
func (t *fakeTimer) Stop() bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	t.stopped = true
	return true
}

func (c *fakeClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.now }

func (c *fakeClock) NewTimer(d time.Duration) Timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &fakeTimer{clock: c, c: make(chan time.Time, 1), at: c.now.Add(d)}
	c.timers = append(c.timers, t)
	return t
}

// Advance moves time forward and fires every armed timer that is now due.
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	for _, t := range c.timers {
		if !t.stopped && !t.fired && !t.at.After(c.now) {
			t.fired = true
			t.c <- c.now
		}
	}
}

func (c *fakeClock) armed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, t := range c.timers {
		if !t.stopped && !t.fired {
			return true
		}
	}
	return false
}

type sent struct{ connector, message string }

type fakeDeliverer struct {
	mu         sync.Mutex
	sends      []sent
	err        error
	configured map[string]bool
}

func (d *fakeDeliverer) Deliver(_ context.Context, connector, message string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.err != nil {
		return d.err
	}
	d.sends = append(d.sends, sent{connector, message})
	return nil
}

func (d *fakeDeliverer) HasConnector(connector string) bool { return d.configured[connector] }

func (d *fakeDeliverer) all() []sent {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]sent(nil), d.sends...)
}

type fakeNotice struct {
	mu     sync.Mutex
	events []string
}

func (n *fakeNotice) Notify(_ context.Context, r Reminder, event string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.events = append(n.events, event+":"+r.Message)
}

func (n *fakeNotice) all() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]string(nil), n.events...)
}

type rig struct {
	svc   *Service
	store *Store
	clock *fakeClock
	del   *fakeDeliverer
	notes *fakeNotice
}

func newRig(t *testing.T, defaultConnector string) *rig {
	t.Helper()
	r := &rig{
		store: newTestStore(t),
		clock: &fakeClock{now: time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)},
		del:   &fakeDeliverer{configured: map[string]bool{"telegram": true}},
		notes: &fakeNotice{},
	}
	r.svc = NewService(r.store, r.del, r.notes, r.clock, Config{
		DefaultConnector: func() string { return defaultConnector },
		RetryDelays:      []time.Duration{time.Minute, 5 * time.Minute},
	})
	return r
}

// start runs the loop and waits until it has armed its timer (or is idle).
func (r *rig) start(t *testing.T) context.CancelFunc {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.svc.Run(ctx); close(done) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("Run did not exit after cancel")
		}
	})
	return cancel
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func (r *rig) create(t *testing.T, msg string, in time.Duration, connector string) Reminder {
	t.Helper()
	got, err := r.svc.Create(context.Background(), Reminder{
		Message: msg, FireAt: r.clock.Now().Add(in), Connector: connector, Timezone: "UTC"})
	if err != nil {
		t.Fatalf("Create(%q): %v", msg, err)
	}
	return got
}

// ── tests ───────────────────────────────────────────────────

func TestRun_FiresAtDueTime(t *testing.T) {
	r := newRig(t, "telegram")
	r.create(t, "call bank", time.Hour, "")
	r.start(t)
	eventually(t, "timer armed", r.clock.armed)

	r.clock.Advance(59 * time.Minute)
	time.Sleep(20 * time.Millisecond)
	if len(r.del.all()) != 0 {
		t.Fatal("delivered before due time")
	}
	r.clock.Advance(time.Minute)
	eventually(t, "delivery", func() bool { return len(r.del.all()) == 1 })
	if got := r.del.all()[0]; got != (sent{"telegram", "call bank"}) {
		t.Fatalf("sent %+v, want telegram/call bank verbatim", got)
	}
	eventually(t, "done", func() bool { n, _ := r.store.CountOpen(context.Background()); return n == 0 })
	if ev := r.notes.all(); len(ev) != 1 || ev[0] != "due:call bank" {
		t.Fatalf("notice events = %v", ev)
	}
}

func TestRun_EarlierInsertWakesLoop(t *testing.T) {
	r := newRig(t, "telegram")
	r.create(t, "later", 3*time.Hour, "")
	r.start(t)
	eventually(t, "timer armed", r.clock.armed)

	r.create(t, "sooner", time.Hour, "")
	time.Sleep(20 * time.Millisecond) // let the loop re-arm for the earlier item
	r.clock.Advance(time.Hour)
	eventually(t, "sooner delivered", func() bool { return len(r.del.all()) == 1 })
	if r.del.all()[0].message != "sooner" {
		t.Fatalf("delivered %+v, want sooner first", r.del.all())
	}
}

func TestRun_TwoDueSameInstantBothFire(t *testing.T) {
	r := newRig(t, "telegram")
	r.create(t, "a", time.Hour, "")
	r.create(t, "b", time.Hour, "")
	r.start(t)
	eventually(t, "timer armed", r.clock.armed)
	r.clock.Advance(time.Hour)
	eventually(t, "both delivered", func() bool { return len(r.del.all()) == 2 })
}

func TestRun_CancelBeforeFireNeverDelivers(t *testing.T) {
	r := newRig(t, "telegram")
	rem := r.create(t, "nope", time.Hour, "")
	r.start(t)
	eventually(t, "timer armed", r.clock.armed)
	if err := r.svc.Cancel(context.Background(), rem.ID); err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(2 * time.Hour)
	time.Sleep(30 * time.Millisecond)
	if len(r.del.all()) != 0 {
		t.Fatalf("cancelled reminder delivered: %+v", r.del.all())
	}
}

func TestRun_RestartDeliversLateUnder12h(t *testing.T) {
	r := newRig(t, "telegram")
	r.create(t, "water plants", time.Hour, "")
	r.clock.Advance(4 * time.Hour) // service was "down" past the fire time
	r.start(t)
	eventually(t, "late delivery", func() bool { return len(r.del.all()) == 1 })
	if m := r.del.all()[0].message; !strings.HasPrefix(m, "(late) ") || !strings.HasSuffix(m, "water plants") {
		t.Fatalf("message = %q, want (late) prefix and original text", m)
	}
}

func TestRun_RestartMarksMissedOver12h(t *testing.T) {
	r := newRig(t, "telegram")
	rem := r.create(t, "old", time.Hour, "")
	r.clock.Advance(14 * time.Hour)
	r.start(t)
	eventually(t, "missed", func() bool {
		got, _ := r.store.Get(context.Background(), rem.ID)
		return got.Status == StatusMissed
	})
	if len(r.del.all()) != 0 {
		t.Fatal("a missed reminder must not be sent to the connector")
	}
	if ev := r.notes.all(); len(ev) != 1 || ev[0] != "missed:old" {
		t.Fatalf("notice events = %v", ev)
	}
}

func TestRun_RecurringAdvances(t *testing.T) {
	r := newRig(t, "telegram")
	rem, err := r.svc.Create(context.Background(), Reminder{
		Message: "standup", Cron: "0 13 * * *", Timezone: "UTC"}) // next 13:00 today
	if err != nil {
		t.Fatal(err)
	}
	r.start(t)
	eventually(t, "timer armed", r.clock.armed)
	r.clock.Advance(time.Hour)
	eventually(t, "delivered", func() bool { return len(r.del.all()) == 1 })
	eventually(t, "advanced to tomorrow", func() bool {
		got, _ := r.store.Get(context.Background(), rem.ID)
		return got.Status == StatusScheduled && got.FireAt.Equal(time.Date(2026, 10, 12, 13, 0, 0, 0, time.UTC))
	})
}

func TestRun_RetryThenFailed(t *testing.T) {
	r := newRig(t, "telegram")
	r.del.err = errors.New("connector down")
	rem := r.create(t, "flaky", time.Hour, "")
	r.start(t)
	for i := 0; i < 3; i++ {
		eventually(t, "timer armed", r.clock.armed)
		r.clock.Advance(time.Hour)
		time.Sleep(30 * time.Millisecond)
	}
	eventually(t, "failed", func() bool {
		got, _ := r.store.Get(context.Background(), rem.ID)
		return got.Status == StatusFailed
	})
	got, _ := r.store.Get(context.Background(), rem.ID)
	if got.Attempts != 3 || !strings.Contains(got.LastError, "connector down") {
		t.Fatalf("attempts=%d lastError=%q", got.Attempts, got.LastError)
	}
	if ev := r.notes.all(); ev[len(ev)-1] != "failed:flaky" {
		t.Fatalf("notice events = %v", ev)
	}
}

func TestRun_NoConnectorMeansInAppOnly(t *testing.T) {
	r := newRig(t, "") // no default channel configured
	r.create(t, "in-app", time.Hour, "")
	r.start(t)
	eventually(t, "timer armed", r.clock.armed)
	r.clock.Advance(time.Hour)
	eventually(t, "notice", func() bool { return len(r.notes.all()) == 1 })
	if len(r.del.all()) != 0 {
		t.Fatal("nothing should be sent to a connector")
	}
}

func TestRun_ExitsOnContextCancel(t *testing.T) {
	r := newRig(t, "telegram")
	r.create(t, "x", time.Hour, "")
	cancel := r.start(t)
	eventually(t, "timer armed", r.clock.armed)
	cancel() // the Cleanup in start asserts Run returns
}

func TestCreate_UnconfiguredNamedConnectorErrors(t *testing.T) {
	r := newRig(t, "telegram")
	_, err := r.svc.Create(context.Background(), Reminder{
		Message: "x", FireAt: r.clock.Now().Add(time.Hour), Connector: "slack", Timezone: "UTC"})
	if err == nil || !strings.Contains(err.Error(), "slack") {
		t.Fatalf("want an error naming slack, got %v", err)
	}
	if n, _ := r.store.CountOpen(context.Background()); n != 0 {
		t.Fatal("nothing must be stored when the connector is unavailable")
	}
}

func TestCreate_LimitOf50(t *testing.T) {
	r := newRig(t, "telegram")
	for i := 0; i < MaxOpen; i++ {
		r.create(t, "n", time.Hour, "")
	}
	_, err := r.svc.Create(context.Background(), Reminder{
		Message: "one too many", FireAt: r.clock.Now().Add(time.Hour), Timezone: "UTC"})
	if err == nil {
		t.Fatal("expected the open-reminder limit to apply")
	}
}

func TestChannel_Resolution(t *testing.T) {
	r := newRig(t, "telegram")
	if got := r.svc.Channel(""); got != "telegram" {
		t.Errorf("default channel = %q", got)
	}
	if got := r.svc.Channel("slack"); got != "slack" {
		t.Errorf("named channel = %q", got)
	}
	none := newRig(t, "")
	if got := none.svc.Channel(""); got != "in-app" {
		t.Errorf("no default = %q, want in-app", got)
	}
}
