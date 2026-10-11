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
