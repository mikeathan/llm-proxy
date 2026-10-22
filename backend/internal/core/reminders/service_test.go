package reminders

import (
	"context"
	"errors"
	"slices"
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
	onDeliver  func() // runs inside Deliver, before it returns (for races)
	mu         sync.Mutex
	sends      []sent
	err        error
	configured map[string]bool
	failFor    map[string]error // per connector; err fails them all
}

func (d *fakeDeliverer) Deliver(_ context.Context, connector, message string) error {
	if d.onDeliver != nil {
		d.onDeliver()
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.err != nil {
		return d.err
	}
	if err := d.failFor[connector]; err != nil {
		return err
	}
	d.sends = append(d.sends, sent{connector, message})
	return nil
}

func (d *fakeDeliverer) Connectors() []string {
	var types []string
	for c, on := range d.configured {
		if on {
			types = append(types, c)
		}
	}
	slices.Sort(types)
	return types
}

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

func newRig(t *testing.T, skipped ...string) *rig {
	t.Helper()
	r := &rig{
		store: newTestStore(t),
		clock: &fakeClock{now: time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)},
		del:   &fakeDeliverer{configured: map[string]bool{"telegram": true}},
		notes: &fakeNotice{},
	}
	r.svc = NewService(r.store, r.del, r.notes, r.clock, Config{
		Skipped: func() []string { return skipped },
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

func (r *rig) statusOf(t *testing.T, id int64) Status {
	t.Helper()
	got, err := r.store.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return got.Status
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
	r := newRig(t)
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
	r := newRig(t)
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
	r := newRig(t)
	r.create(t, "a", time.Hour, "")
	r.create(t, "b", time.Hour, "")
	r.start(t)
	eventually(t, "timer armed", r.clock.armed)
	r.clock.Advance(time.Hour)
	eventually(t, "both delivered", func() bool { return len(r.del.all()) == 2 })
}

func TestRun_CancelBeforeFireNeverDelivers(t *testing.T) {
	r := newRig(t)
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
	r := newRig(t)
	r.create(t, "water plants", time.Hour, "")
	r.clock.Advance(4 * time.Hour) // service was "down" past the fire time
	r.start(t)
	eventually(t, "late delivery", func() bool { return len(r.del.all()) == 1 })
	if m := r.del.all()[0].message; !strings.HasPrefix(m, "(late) ") || !strings.HasSuffix(m, "water plants") {
		t.Fatalf("message = %q, want (late) prefix and original text", m)
	}
}

func TestRun_RestartMarksMissedOver12h(t *testing.T) {
	r := newRig(t)
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
	r := newRig(t)
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
	r := newRig(t)
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
	r := newRig(t, "telegram") // telegram ticked off
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
	r := newRig(t)
	r.create(t, "x", time.Hour, "")
	cancel := r.start(t)
	eventually(t, "timer armed", r.clock.armed)
	cancel() // the Cleanup in start asserts Run returns
}

func TestCreate_UnconfiguredNamedConnectorErrors(t *testing.T) {
	r := newRig(t)
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
	r := newRig(t)
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
	r := newRig(t)
	if got := r.svc.Channel(""); got != "telegram" {
		t.Errorf("default channel = %q", got)
	}
	if got := r.svc.Channel("slack"); got != "slack" {
		t.Errorf("named channel = %q", got)
	}
	none := newRig(t, "telegram")
	if got := none.svc.Channel(""); got != "in-app" {
		t.Errorf("no default = %q, want in-app", got)
	}
}

func TestRun_CancelDuringSendIsNotUndone(t *testing.T) {
	r := newRig(t)
	rem, err := r.svc.Create(context.Background(), Reminder{Message: "standup", Cron: "0 13 * * *", Timezone: "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	r.del.onDeliver = func() { _ = r.svc.Cancel(context.Background(), rem.ID) }
	r.start(t)
	eventually(t, "timer armed", r.clock.armed)
	r.clock.Advance(time.Hour)
	eventually(t, "delivered", func() bool { return len(r.del.all()) == 1 })
	time.Sleep(30 * time.Millisecond)
	got, _ := r.store.Get(context.Background(), rem.ID)
	if got.Status != StatusCancelled {
		t.Fatalf("status = %q: a cancel made while the send was in flight was reversed", got.Status)
	}
}

// A store that cannot be written must not turn the loop into a spin that re-sends
// the same message as fast as the connector answers.
func TestRun_FailedSaveDoesNotSpin(t *testing.T) {
	r := newRig(t)
	r.create(t, "once", time.Hour, "")
	// Reads keep working, every write fails (a busy or read-only database).
	if _, err := r.store.db.Exec(`CREATE TRIGGER no_writes BEFORE UPDATE ON reminders BEGIN SELECT RAISE(ABORT, 'database is read-only'); END`); err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(2 * time.Hour)
	r.start(t)
	time.Sleep(150 * time.Millisecond)
	if n := len(r.del.all()); n > 1 {
		t.Fatalf("delivered %d times with the store down, want at most 1", n)
	}
}

func TestRun_RecurringSurvivesFailedDelivery(t *testing.T) {
	r := newRig(t)
	r.del.err = errors.New("connector down")
	rem, _ := r.svc.Create(context.Background(), Reminder{Message: "standup", Cron: "0 13 * * *", Timezone: "UTC"})
	r.start(t)
	for i := 0; i < 3; i++ {
		eventually(t, "timer armed", r.clock.armed)
		r.clock.Advance(time.Hour)
		time.Sleep(30 * time.Millisecond)
	}
	eventually(t, "failure noticed", func() bool { return len(r.notes.all()) > 0 })
	// The announcement comes before the new occurrence is saved: wait for the save, not for the notice alone.
	eventually(t, "moved on", func() bool {
		saved, _ := r.store.Get(context.Background(), rem.ID)
		return saved.FireAt.After(r.clock.Now())
	})
	got, _ := r.store.Get(context.Background(), rem.ID)
	if got.Status != StatusScheduled || !got.FireAt.After(r.clock.Now()) {
		t.Fatalf("status=%q fireAt=%v: a recurring reminder must move on to its next occurrence after a failed one", got.Status, got.FireAt)
	}
	if ev := r.notes.all(); ev[len(ev)-1] != "failed:standup" {
		t.Fatalf("notice events = %v", ev)
	}
}

func TestEdit_ChangesTheReminderAndRearmsTheLoop(t *testing.T) {
	r := newRig(t)
	rem := r.create(t, "old", 5*time.Hour, "")
	r.start(t)
	eventually(t, "timer armed", r.clock.armed)

	got, err := r.svc.Edit(context.Background(), rem.ID, Reminder{Message: "new text", FireAt: r.clock.Now().Add(time.Hour), Connector: ""})
	if err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if got.Message != "new text" || got.Timezone != "UTC" {
		t.Fatalf("edited = %+v", got)
	}
	time.Sleep(20 * time.Millisecond) // let the loop re-arm for the earlier time
	r.clock.Advance(time.Hour)
	eventually(t, "delivered at the new time", func() bool { return len(r.del.all()) == 1 })
	if r.del.all()[0].message != "new text" {
		t.Fatalf("sent %+v, want the edited text", r.del.all())
	}
}

func TestEdit_Validates(t *testing.T) {
	r := newRig(t)
	rem := r.create(t, "x", time.Hour, "")
	soon := r.clock.Now().Add(time.Hour)
	cases := map[string]Reminder{
		"blank message":        {Message: " ", FireAt: soon},
		"past time":            {Message: "x", FireAt: r.clock.Now().Add(-time.Minute)},
		"unconfigured channel": {Message: "x", FireAt: soon, Connector: "slack"},
		"every minute":         {Message: "x", Cron: "* * * * *"},
	}
	for name, in := range cases {
		_, err := r.svc.Edit(context.Background(), rem.ID, in)
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
	if got, _ := r.store.Get(context.Background(), rem.ID); got.Message != "x" {
		t.Errorf("a rejected edit must change nothing, got %+v", got)
	}
	if _, err := r.svc.Edit(context.Background(), 9999, Reminder{Message: "x", FireAt: soon}); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown id err = %v, want ErrNotFound", err)
	}
	if _, err := r.svc.Edit(context.Background(), rem.ID, Reminder{Message: "x", FireAt: soon, Connector: InAppChannel}); err != nil {
		t.Errorf("in-app is a valid explicit channel: %v", err)
	}
}

func TestEdit_OneOffCanBecomeRecurring(t *testing.T) {
	r := newRig(t)
	rem := r.create(t, "x", time.Hour, "")
	got, err := r.svc.Edit(context.Background(), rem.ID, Reminder{Message: "x", Cron: "0 13 * * *"})
	if err != nil || got.Cron == "" || !got.FireAt.Equal(time.Date(2026, 10, 11, 13, 0, 0, 0, time.UTC)) {
		t.Fatalf("got=%+v err=%v, want a daily reminder starting 13:00 today", got, err)
	}
}

func TestRemove_DeletesAndNeverDelivers(t *testing.T) {
	r := newRig(t)
	rem := r.create(t, "bye", time.Hour, "")
	r.start(t)
	eventually(t, "timer armed", r.clock.armed)
	if err := r.svc.Remove(context.Background(), rem.ID); err != nil {
		t.Fatal(err)
	}
	r.clock.Advance(2 * time.Hour)
	time.Sleep(30 * time.Millisecond)
	if len(r.del.all()) != 0 {
		t.Fatal("a removed reminder was delivered")
	}
	if err := r.svc.Remove(context.Background(), rem.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second Remove = %v, want ErrNotFound", err)
	}
}

func TestCreate_RepliesWhereAsked(t *testing.T) {
	r := newRig(t)
	r.del.configured["slack"] = true
	got, err := r.svc.Create(context.Background(), Reminder{Message: "a", FireAt: r.clock.Now().Add(time.Hour), Timezone: "UTC", Origin: "slack"})
	if err != nil || got.Connector != "slack" {
		t.Fatalf("asked in slack: connector=%q err=%v", got.Connector, err)
	}
	got, _ = r.svc.Create(context.Background(), Reminder{Message: "b", FireAt: r.clock.Now().Add(time.Hour), Timezone: "UTC"})
	if got.Connector != "" || r.svc.Channel(got.Connector) != "slack, telegram" {
		t.Errorf("web chat follows the default (every enabled connector): connector=%q channel=%q", got.Connector, r.svc.Channel(got.Connector))
	}
	got, _ = r.svc.Create(context.Background(), Reminder{Message: "c", FireAt: r.clock.Now().Add(time.Hour), Timezone: "UTC", Origin: "discord"})
	if got.Connector != "" {
		t.Errorf("an origin that is not configured must not be stored, got %q", got.Connector)
	}
}

func TestEdit_ReschedulesAFinishedReminder(t *testing.T) {
	r := newRig(t)
	for _, finished := range []Status{StatusDone, StatusMissed, StatusFailed} {
		rem := r.create(t, "once "+string(finished), time.Hour, "")
		cur, _ := r.store.Get(context.Background(), rem.ID)
		cur.Status, cur.Attempts, cur.LastError = finished, 3, "boom"
		if err := r.store.Update(context.Background(), cur); err != nil {
			t.Fatal(err)
		}
		got, err := r.svc.Edit(context.Background(), rem.ID, Reminder{Message: rem.Message, FireAt: r.clock.Now().Add(2 * time.Hour)})
		if err != nil || got.Status != StatusScheduled || got.Attempts != 0 || got.LastError != "" {
			t.Fatalf("%s: got %+v err=%v, want a fresh scheduled reminder", finished, got, err)
		}
	}
}

func TestEdit_ACancelledReminderCannotBeEdited(t *testing.T) {
	r := newRig(t)
	rem := r.create(t, "x", time.Hour, "")
	_ = r.svc.Cancel(context.Background(), rem.ID)
	if _, err := r.svc.Edit(context.Background(), rem.ID, Reminder{Message: "y", FireAt: r.clock.Now().Add(time.Hour)}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// The result of a send must not overwrite what the user changed while it was in flight.
func TestRun_EditDuringSendIsNotClobbered(t *testing.T) {
	r := newRig(t)
	rem := r.create(t, "old text", time.Hour, "")
	newTime := r.clock.Now().Add(48 * time.Hour)
	r.del.onDeliver = func() {
		if _, err := r.svc.Edit(context.Background(), rem.ID, Reminder{Message: "edited", FireAt: newTime}); err != nil {
			t.Errorf("Edit: %v", err)
		}
	}
	r.start(t)
	eventually(t, "timer armed", r.clock.armed)
	r.clock.Advance(time.Hour)
	eventually(t, "delivered", func() bool { return len(r.del.all()) == 1 })
	time.Sleep(30 * time.Millisecond)
	got, _ := r.store.Get(context.Background(), rem.ID)
	if got.Message != "edited" || got.Status != StatusScheduled || !got.FireAt.Equal(newTime) {
		t.Fatalf("the edit was overwritten by the send's result: %+v", got)
	}
}

func repeating(t *testing.T, r *rig) Reminder {
	t.Helper()
	rem, err := r.svc.Create(context.Background(), Reminder{Message: "standup", Cron: "0 13 * * *", Timezone: "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	return rem
}

func TestService_PauseResume(t *testing.T) {
	r := newRig(t)
	rem := repeating(t, r) // first at 13:00 today
	r.start(t)
	eventually(t, "timer armed", r.clock.armed)

	got, err := r.svc.Pause(context.Background(), rem.ID)
	if err != nil || got.Status != StatusPaused {
		t.Fatalf("Pause: %+v err=%v", got, err)
	}
	r.clock.Advance(26 * time.Hour) // past two occurrences
	time.Sleep(30 * time.Millisecond)
	if len(r.del.all()) != 0 {
		t.Fatalf("a paused reminder was delivered: %+v", r.del.all())
	}
	if open, _ := r.svc.List(context.Background()); len(open) != 1 || open[0].Status != StatusPaused {
		t.Fatalf("a paused reminder stays in the list: %+v", open)
	}

	got, err = r.svc.Resume(context.Background(), rem.ID)
	if err != nil || got.Status != StatusScheduled || !got.FireAt.After(r.clock.Now()) {
		t.Fatalf("Resume: %+v err=%v", got, err)
	}
	if len(r.del.all()) != 0 {
		t.Fatal("resuming must not replay what was silenced")
	}
	time.Sleep(20 * time.Millisecond)
	r.clock.Advance(24 * time.Hour)
	eventually(t, "fires again after resume", func() bool { return len(r.del.all()) == 1 })
}

func TestService_Skip(t *testing.T) {
	r := newRig(t)
	rem := repeating(t, r) // next 13:00 today
	r.start(t)
	eventually(t, "timer armed", r.clock.armed)
	got, err := r.svc.Skip(context.Background(), rem.ID)
	if err != nil || !got.FireAt.Equal(time.Date(2026, 10, 12, 13, 0, 0, 0, time.UTC)) {
		t.Fatalf("Skip: %+v err=%v, want tomorrow 13:00", got, err)
	}
	time.Sleep(20 * time.Millisecond)
	r.clock.Advance(2 * time.Hour) // today's 13:00 passes
	time.Sleep(30 * time.Millisecond)
	if len(r.del.all()) != 0 {
		t.Fatal("the skipped occurrence was delivered")
	}
}

func TestPauseResumeSkip_Errors(t *testing.T) {
	r := newRig(t)
	oneoff := r.create(t, "x", time.Hour, "")
	rep := repeating(t, r)
	ctx := context.Background()
	if _, err := r.svc.Pause(ctx, oneoff.ID); !errors.Is(err, ErrInvalid) {
		t.Errorf("pausing a one-off: %v", err)
	}
	if _, err := r.svc.Resume(ctx, rep.ID); !errors.Is(err, ErrInvalid) {
		t.Errorf("resuming one that is not paused: %v", err)
	}
	if _, err := r.svc.Skip(ctx, oneoff.ID); !errors.Is(err, ErrInvalid) {
		t.Errorf("skipping a one-off: %v", err)
	}
	for name, fn := range map[string]func(context.Context, int64) (Reminder, error){"Pause": r.svc.Pause, "Resume": r.svc.Resume, "Skip": r.svc.Skip} {
		if _, err := fn(ctx, 9999); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s of an unknown id: %v", name, err)
		}
	}
}

// A reminder that follows the default goes to every enabled connector the owner has not ticked off.
func TestRun_DefaultGoesToEveryEnabledConnector(t *testing.T) {
	r := newRig(t)
	r.del.configured["slack"] = true
	r.create(t, "call the bank", time.Hour, "")
	r.start(t)
	eventually(t, "timer armed", r.clock.armed)
	r.clock.Advance(time.Hour)
	eventually(t, "both sends", func() bool { return len(r.del.all()) == 2 })
	got := r.del.all()
	if got[0] != (sent{"slack", "call the bank"}) || got[1] != (sent{"telegram", "call the bank"}) {
		t.Errorf("sends = %+v, want slack and telegram with the same text", got)
	}
	eventually(t, "one-off closed", func() bool { return r.statusOf(t, 1) == StatusDone })
}

func TestRun_TickedOffConnectorIsNotUsed(t *testing.T) {
	r := newRig(t, "slack")
	r.del.configured["slack"] = true
	r.create(t, "call the bank", time.Hour, "")
	r.start(t)
	eventually(t, "timer armed", r.clock.armed)
	r.clock.Advance(time.Hour)
	eventually(t, "send", func() bool { return len(r.del.all()) == 1 })
	if got := r.del.all()[0]; got.connector != "telegram" {
		t.Errorf("sent to %q, want telegram only", got.connector)
	}
}

// One connector failing while another took it is reported but not retried: a retry would send the text twice.
func TestRun_PartialDeliveryIsReportedNotRetried(t *testing.T) {
	r := newRig(t)
	r.del.configured["slack"] = true
	r.del.failFor = map[string]error{"slack": errors.New("channel not found")}
	r.create(t, "call the bank", time.Hour, "")
	r.start(t)
	eventually(t, "timer armed", r.clock.armed)
	r.clock.Advance(time.Hour)
	eventually(t, "partial event", func() bool { return slices.Contains(r.notes.all(), "partial:call the bank") })
	// The service announces the event before it saves the new status, so the announcement can be heard while the store
	// still says "scheduled": wait for the saved status instead of reading it at once.
	eventually(t, "status saved", func() bool { return r.statusOf(t, 1) == StatusDone })
	got, err := r.store.Get(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusDone || !strings.Contains(got.LastError, "slack: channel not found") {
		t.Errorf("status=%s last_error=%q, want done with the slack failure kept", got.Status, got.LastError)
	}
	r.clock.Advance(10 * time.Minute)
	time.Sleep(30 * time.Millisecond)
	if n := len(r.del.all()); n != 1 {
		t.Errorf("%d sends, want 1: the connector that failed is not retried and telegram is not repeated", n)
	}
}

func TestRun_FailsOnlyWhenEveryConnectorFails(t *testing.T) {
	r := newRig(t)
	r.del.configured["slack"] = true
	r.del.err = errors.New("down")
	r.create(t, "call the bank", time.Hour, "")
	r.start(t)
	eventually(t, "timer armed", r.clock.armed)
	r.clock.Advance(time.Hour)
	eventually(t, "retry scheduled", func() bool {
		got, err := r.store.Get(context.Background(), 1)
		return err == nil && got.Attempts == 1 && got.Status == StatusScheduled
	})
}

// Each final outcome is saved on the reminder with the time it happened; a retry that is still pending saves none,
// so one reminder never reports three failures.
func TestRun_SavesTheOutcomeOnTheReminder(t *testing.T) {
	eventOf := func(r *rig, id int64) Reminder {
		got, err := r.store.Get(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	t.Run("delivered on time", func(t *testing.T) {
		r := newRig(t)
		rem := r.create(t, "call bank", time.Hour, "")
		r.start(t)
		eventually(t, "timer armed", r.clock.armed)
		r.clock.Advance(time.Hour)
		eventually(t, "event saved", func() bool { return eventOf(r, rem.ID).Event == EventDue })
		if at := eventOf(r, rem.ID).EventAt; !at.Equal(r.clock.Now()) {
			t.Errorf("event time = %v, want the delivery time %v", at, r.clock.Now())
		}
	})
	t.Run("delivered late", func(t *testing.T) {
		r := newRig(t)
		rem := r.create(t, "water plants", time.Hour, "")
		r.clock.Advance(4 * time.Hour)
		r.start(t)
		eventually(t, "event saved", func() bool { return eventOf(r, rem.ID).Event == EventLate })
	})
	t.Run("missed", func(t *testing.T) {
		r := newRig(t)
		rem := r.create(t, "old", time.Hour, "")
		r.clock.Advance(14 * time.Hour)
		r.start(t)
		eventually(t, "event saved", func() bool { return eventOf(r, rem.ID).Event == EventMissed })
	})
	t.Run("failed only when out of attempts", func(t *testing.T) {
		r := newRig(t)
		r.del.err = errors.New("connector down")
		rem := r.create(t, "flaky", time.Hour, "")
		r.start(t)
		for i := range 3 {
			eventually(t, "timer armed", r.clock.armed)
			r.clock.Advance(time.Hour)
			time.Sleep(30 * time.Millisecond)
			if i < 2 && eventOf(r, rem.ID).Event != "" {
				t.Fatalf("attempt %d saved an event while a retry was pending", i+1)
			}
		}
		eventually(t, "event saved", func() bool { return eventOf(r, rem.ID).Event == EventFailed })
	})
}

// A client asks "what happened after the newest event I have seen", so event times must only ever go up: two outcomes
// in the same millisecond, or a clock that stands still, must not be given the same time, or the later one is skipped.
func TestRun_EventTimesOnlyGoUp(t *testing.T) {
	r := newRig(t)
	a := r.create(t, "a", time.Hour, "")
	b := r.create(t, "b", time.Hour, "")
	r.start(t)
	eventually(t, "timer armed", r.clock.armed)
	r.clock.Advance(time.Hour) // the clock does not move between the two deliveries
	eventually(t, "both delivered", func() bool {
		ga, _ := r.store.Get(context.Background(), a.ID)
		gb, _ := r.store.Get(context.Background(), b.ID)
		return ga.Event != "" && gb.Event != ""
	})
	ga, _ := r.store.Get(context.Background(), a.ID)
	gb, _ := r.store.Get(context.Background(), b.ID)
	first, second := ga.EventAt, gb.EventAt
	if second.Before(first) {
		first, second = second, first
	}
	if !second.After(first) {
		t.Errorf("both events are at %v: the later one could be skipped by a cursor", first)
	}
}
