package memory

import (
	"context"
	"strconv"
	"testing"
	"time"
)

// age backdates every entry of a type so the reaper sees it as old.
func age(t *testing.T, s *Store, memType MemoryType, days int) {
	t.Helper()
	_, err := s.db.ExecContext(context.Background(),
		`UPDATE memories SET created_at = datetime('now', ?) WHERE memory_type = ?`,
		"-"+strconv.Itoa(days)+" days", string(memType))
	if err != nil {
		t.Fatalf("age: %v", err)
	}
}

func count(t *testing.T, s *Store, memType MemoryType) int {
	t.Helper()
	entries, err := s.List(context.Background(), "ws-1", memType, 100, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	return len(entries)
}

// Only session entries are ever reaped: long-term facts and the user profile
// are the operator's data, however old.
func TestSessionReaper_ReapsOldSessionEntriesOnly(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	store.Insert(ctx, "ws-1", Session, "old-session", "stale run fact", nil, "run:r1")
	store.Insert(ctx, "ws-1", LongTerm, "old-long", "permanent fact", nil, "agent")
	store.Insert(ctx, "ws-1", UserProfile, "old-profile", "likes tea", nil, "agent")
	age(t, store, Session, 120)
	age(t, store, LongTerm, 120)
	age(t, store, UserProfile, 120)
	store.Insert(ctx, "ws-1", Session, "fresh-session", "recent run fact", nil, "run:r2")

	r := NewSessionReaper(func() *Store { return store }, func() time.Duration { return 90 * 24 * time.Hour }, time.Hour)
	if n := r.reap(ctx); n != 1 {
		t.Errorf("reaped %d entries, want 1", n)
	}
	if got := count(t, store, Session); got != 1 {
		t.Errorf("session entries left = %d, want 1 (the fresh one)", got)
	}
	if count(t, store, LongTerm) != 1 || count(t, store, UserProfile) != 1 {
		t.Error("long_term and user_profile entries must never be reaped")
	}
}

func TestSessionReaper_NonPositiveRetentionReapsNothing(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	store.Insert(ctx, "ws-1", Session, "old", "stale", nil, "run:r1")
	age(t, store, Session, 400)

	r := NewSessionReaper(func() *Store { return store }, func() time.Duration { return 0 }, time.Hour)
	if n := r.reap(ctx); n != 0 || count(t, store, Session) != 1 {
		t.Errorf("a non-positive retention must leave entries alone (reaped %d)", n)
	}
}

// The store is replaced on factory reset and absent when the DB failed to
// open: the reaper must re-resolve it each tick and tolerate nil.
func TestSessionReaper_ToleratesMissingStore(t *testing.T) {
	r := NewSessionReaper(func() *Store { return nil }, func() time.Duration { return time.Hour }, time.Hour)
	if n := r.reap(context.Background()); n != 0 {
		t.Errorf("reaped %d from a nil store", n)
	}
}

func TestSessionReaper_StopsWhenContextCancelled(t *testing.T) {
	store := newTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	r := NewSessionReaper(func() *Store { return store }, func() time.Duration { return time.Hour }, 10*time.Millisecond)

	done := make(chan struct{})
	go func() {
		r.Start(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after the context was cancelled (leaked goroutine)")
	}
}
