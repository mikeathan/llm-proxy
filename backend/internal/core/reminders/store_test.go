package reminders

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"llm-proxy/internal/platform/db"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	p, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = p.DB().Close() })
	s, err := NewStore(p)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return s
}

var base = time.Date(2026, 10, 12, 8, 0, 0, 0, time.UTC)

func TestStore_AddListOrderedByFireAt(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	later, _ := s.Add(ctx, Reminder{Message: "later", FireAt: base.Add(2 * time.Hour)})
	sooner, _ := s.Add(ctx, Reminder{Message: "sooner", FireAt: base.Add(time.Hour)})

	got, err := s.ListOpen(ctx)
	if err != nil {
		t.Fatalf("ListOpen: %v", err)
	}
	if len(got) != 2 || got[0].ID != sooner || got[1].ID != later {
		t.Fatalf("want [sooner later], got %+v", got)
	}
	if got[0].Status != StatusScheduled {
		t.Errorf("new reminder status = %q, want scheduled", got[0].Status)
	}
}

func TestStore_DueReturnsOnlyScheduledAtOrBeforeNow(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	due, _ := s.Add(ctx, Reminder{Message: "due", FireAt: base})
	_, _ = s.Add(ctx, Reminder{Message: "future", FireAt: base.Add(time.Minute)})
	done, _ := s.Add(ctx, Reminder{Message: "done", FireAt: base.Add(-time.Hour)})
	r, _ := s.Get(ctx, done)
	r.Status = StatusDone
	if err := s.Update(ctx, r); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := s.Due(ctx, base)
	if err != nil {
		t.Fatalf("Due: %v", err)
	}
	if len(got) != 1 || got[0].ID != due {
		t.Fatalf("want only the due reminder, got %+v", got)
	}
}

func TestStore_CancelExcludesFromOpen(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	id, _ := s.Add(ctx, Reminder{Message: "x", FireAt: base})
	if err := s.Cancel(ctx, id); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	open, _ := s.ListOpen(ctx)
	if len(open) != 0 {
		t.Fatalf("cancelled reminder still open: %+v", open)
	}
	if n, _ := s.CountOpen(ctx); n != 0 {
		t.Fatalf("CountOpen = %d, want 0", n)
	}
	if err := s.Cancel(ctx, id); err != ErrNotFound {
		t.Fatalf("second Cancel = %v, want ErrNotFound", err)
	}
}

func TestStore_MessageStoredVerbatim(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	msg := "<b>x</b> '; DROP TABLE reminders;--"
	id, _ := s.Add(ctx, Reminder{Message: msg, FireAt: base})
	got, err := s.Get(ctx, id)
	if err != nil || got.Message != msg {
		t.Fatalf("Get = %+v, %v; want message %q", got, err, msg)
	}
}

func TestStore_RoundTripsFields(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	until := base.Add(24 * time.Hour)
	id, _ := s.Add(ctx, Reminder{Message: "m", FireAt: base, Cron: "0 9 * * 1", Until: &until,
		Timezone: "Europe/London", Connector: "telegram", CreatedBy: "chat"})
	got, _ := s.Get(ctx, id)
	if got.Cron != "0 9 * * 1" || got.Timezone != "Europe/London" || got.Connector != "telegram" ||
		got.Until == nil || !got.Until.Equal(until) || !got.FireAt.Equal(base) {
		t.Fatalf("round trip lost fields: %+v", got)
	}
}

func TestStore_UpdateNeverRevivesACancelledReminder(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	id, _ := s.Add(ctx, Reminder{Message: "x", FireAt: base})
	stale, _ := s.Get(ctx, id)
	if err := s.Cancel(ctx, id); err != nil {
		t.Fatal(err)
	}
	stale.Status = StatusDone
	if err := s.Update(ctx, stale); err != ErrNotFound {
		t.Fatalf("Update of a cancelled reminder = %v, want ErrNotFound", err)
	}
	if got, _ := s.Get(ctx, id); got.Status != StatusCancelled {
		t.Fatalf("status = %q, a late Update overwrote the cancel", got.Status)
	}
}

func TestStore_ReplaceChangesEditableFieldsOnly(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	id, _ := s.Add(ctx, Reminder{Message: "old", FireAt: base, Timezone: "Europe/London", CreatedBy: "assistant", Connector: "telegram"})
	r, _ := s.Get(ctx, id)
	r.Attempts, r.LastError = 2, "boom"
	_ = s.Update(ctx, r)

	until := base.Add(48 * time.Hour)
	edited := Reminder{ID: id, Message: "new", FireAt: base.Add(time.Hour), Cron: "0 9 * * 1", Until: &until, Connector: "in-app"}
	if err := s.Replace(ctx, edited); err != nil {
		t.Fatalf("Replace: %v", err)
	}
	got, _ := s.Get(ctx, id)
	if got.Message != "new" || !got.FireAt.Equal(base.Add(time.Hour)) || got.Cron != "0 9 * * 1" || got.Until == nil || got.Connector != "in-app" {
		t.Fatalf("not replaced: %+v", got)
	}
	if got.Timezone != "Europe/London" || got.CreatedBy != "assistant" || got.Status != StatusScheduled {
		t.Errorf("identity fields must survive an edit: %+v", got)
	}
	if got.Attempts != 0 || got.LastError != "" {
		t.Errorf("an edit starts delivery afresh, got attempts=%d err=%q", got.Attempts, got.LastError)
	}
}

func TestStore_ReplaceRefusesAClosedReminder(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	id, _ := s.Add(ctx, Reminder{Message: "x", FireAt: base})
	_ = s.Cancel(ctx, id)
	if err := s.Replace(ctx, Reminder{ID: id, Message: "y", FireAt: base}); err != ErrNotFound {
		t.Fatalf("Replace of a cancelled reminder = %v, want ErrNotFound", err)
	}
}

func TestStore_DeleteRemovesAnyStatus(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	open, _ := s.Add(ctx, Reminder{Message: "open", FireAt: base})
	done, _ := s.Add(ctx, Reminder{Message: "done", FireAt: base})
	r, _ := s.Get(ctx, done)
	r.Status = StatusDone
	_ = s.Update(ctx, r)
	for _, id := range []int64{open, done} {
		if err := s.Delete(ctx, id); err != nil {
			t.Fatalf("Delete(%d): %v", id, err)
		}
		if _, err := s.Get(ctx, id); err != ErrNotFound {
			t.Fatalf("reminder %d still there: %v", id, err)
		}
	}
	if err := s.Delete(ctx, 999); err != ErrNotFound {
		t.Fatalf("Delete of unknown = %v, want ErrNotFound", err)
	}
}

func TestStore_UpdateIsRefusedWhenTheReminderWasEditedMeanwhile(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	id, _ := s.Add(ctx, Reminder{Message: "old", FireAt: base})
	loopsCopy, _ := s.Get(ctx, id) // what the delivery loop read before it started sending

	if err := s.Replace(ctx, Reminder{ID: id, Message: "edited", FireAt: base.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	loopsCopy.Status = StatusDone
	if err := s.Update(ctx, loopsCopy); err != ErrNotFound {
		t.Fatalf("a stale write after an edit = %v, want ErrNotFound", err)
	}
	got, _ := s.Get(ctx, id)
	if got.Message != "edited" || got.Status != StatusScheduled || !got.FireAt.Equal(base.Add(time.Hour)) {
		t.Fatalf("the edit was overwritten: %+v", got)
	}

	// A write made from a fresh read goes through.
	got.Attempts = 1
	if err := s.Update(ctx, got); err != nil {
		t.Fatalf("Update from a fresh copy: %v", err)
	}
}

func TestStore_ReplaceReArmsAFinishedReminder(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	for _, finished := range []Status{StatusDone, StatusMissed, StatusFailed} {
		id, _ := s.Add(ctx, Reminder{Message: "x", FireAt: base})
		r, _ := s.Get(ctx, id)
		r.Status, r.Attempts, r.LastError = finished, 3, "boom"
		_ = s.Update(ctx, r)

		if err := s.Replace(ctx, Reminder{ID: id, Message: "again", FireAt: base.Add(24 * time.Hour)}); err != nil {
			t.Fatalf("Replace of a %s reminder: %v", finished, err)
		}
		got, _ := s.Get(ctx, id)
		if got.Status != StatusScheduled || got.Attempts != 0 || got.LastError != "" {
			t.Errorf("%s: want a fresh scheduled reminder, got %+v", finished, got)
		}
	}
}

func TestStore_ReplaceKeepsAPausedReminderPaused(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	id, _ := s.Add(ctx, Reminder{Message: "x", FireAt: base, Cron: "0 9 * * *", Timezone: "UTC"})
	r, _ := s.Get(ctx, id)
	r.Status = StatusPaused
	if err := s.Transition(ctx, r, StatusScheduled); err != nil {
		t.Fatal(err)
	}
	_ = s.Replace(ctx, Reminder{ID: id, Message: "edited", FireAt: base, Cron: "0 9 * * *"})
	if got, _ := s.Get(ctx, id); got.Status != StatusPaused || got.Message != "edited" {
		t.Fatalf("editing a paused reminder must not un-pause it: %+v", got)
	}
}

func TestStore_PausedIsOpenButNeverDue(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	live, _ := s.Add(ctx, Reminder{Message: "live", FireAt: base})
	paused, _ := s.Add(ctx, Reminder{Message: "paused", FireAt: base.Add(-time.Hour)})
	r, _ := s.Get(ctx, paused)
	r.Status = StatusPaused
	_ = s.Transition(ctx, r, StatusScheduled)

	open, _ := s.ListOpen(ctx)
	if len(open) != 2 {
		t.Fatalf("ListOpen = %+v, want both (the UI shows a paused reminder)", open)
	}
	scheduled, _ := s.ListScheduled(ctx)
	if len(scheduled) != 1 || scheduled[0].ID != live {
		t.Fatalf("ListScheduled = %+v, want only the live one (the timer must ignore a paused one)", scheduled)
	}
	due, _ := s.Due(ctx, base)
	if len(due) != 1 || due[0].ID != live {
		t.Fatalf("Due = %+v, a paused reminder must never be due", due)
	}
	if n, _ := s.CountOpen(ctx); n != 2 {
		t.Errorf("CountOpen = %d, a paused reminder still counts against the cap", n)
	}
	if err := s.Cancel(ctx, paused); err != nil {
		t.Errorf("a paused reminder can be cancelled: %v", err)
	}
}

func TestStore_TransitionOnlyFromTheExpectedStates(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	id, _ := s.Add(ctx, Reminder{Message: "x", FireAt: base})
	r, _ := s.Get(ctx, id)
	r.Status = StatusPaused
	if err := s.Transition(ctx, r, StatusPaused); err != ErrNotFound {
		t.Fatalf("pausing from a state not in the allowed list = %v, want ErrNotFound", err)
	}
	if err := s.Transition(ctx, r, StatusScheduled); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Get(ctx, id); got.Status != StatusPaused {
		t.Fatalf("status = %q", got.Status)
	}
}

// What a delivery did to a reminder (due, late, partial, failed, missed) is saved with the outcome, in the same write,
// so the web UI can be told about it after the fact: by a poll, after a reload, or when it was closed at the time.
func TestStore_EventIsSavedWithTheOutcomeAndQueriedByCursor(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	at := base.Add(time.Hour)
	fire := func(msg, event string, offset time.Duration) int64 {
		id, _ := s.Add(ctx, Reminder{Message: msg, FireAt: base})
		r, _ := s.Get(ctx, id)
		r.Status, r.Event, r.EventAt = StatusDone, event, at.Add(offset)
		if err := s.Update(ctx, r); err != nil {
			t.Fatalf("Update: %v", err)
		}
		return id
	}
	first := fire("first", EventDue, 0)
	second := fire("second", EventFailed, time.Minute)
	third := fire("third", EventLate, 2*time.Minute)
	_, _ = s.Add(ctx, Reminder{Message: "never fired", FireAt: base})

	got, err := s.Get(ctx, first)
	if err != nil || got.Event != EventDue || !got.EventAt.Equal(at) {
		t.Fatalf("round trip: event %q at %v, err %v", got.Event, got.EventAt, err)
	}
	if got.EventAt.Sub(at) != 0 || got.EventAt.UnixMilli() != at.UnixMilli() {
		t.Errorf("the event time keeps millisecond precision")
	}

	after, err := s.EventsAfter(ctx, at, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 2 || after[0].ID != third || after[1].ID != second {
		t.Fatalf("want the two later events newest first, got %+v", after)
	}
	if latest, _ := s.EventsAfter(ctx, at.Add(2*time.Minute), 10); len(latest) != 0 {
		t.Errorf("nothing is after the newest event, got %d", len(latest))
	}
	if capped, _ := s.EventsAfter(ctx, time.Time{}, 1); len(capped) != 1 || capped[0].ID != third {
		t.Errorf("the page is capped and newest first, got %+v", capped)
	}
	newest, err := s.LatestEventAt(ctx)
	if err != nil || !newest.Equal(at.Add(2*time.Minute)) {
		t.Errorf("LatestEventAt = %v, %v", newest, err)
	}
}

func TestStore_LatestEventAtIsZeroWithoutEvents(t *testing.T) {
	s := newTestStore(t)
	if got, err := s.LatestEventAt(context.Background()); err != nil || !got.IsZero() {
		t.Errorf("got %v, %v", got, err)
	}
}
