package memory

import (
	"context"
	"testing"
	"time"
)

// Only expired facts are reaped, whatever their scope or type: a user-scope session fact (stored as a user profile
// entry, which the old session-type reaper never reached) goes too, while facts with no expiry or a future one stay.
func TestExpiryReaper_ReapsExpiredFactsOfAnyScopeOnly(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	past, future := time.Now().Add(-time.Hour), time.Now().Add(48*time.Hour)
	for _, e := range []NewEntry{
		{WorkspaceID: "ws-1", MemoryType: Session, Title: "expired-session", Content: "stale run fact", ExpiresAt: past},
		{WorkspaceID: "global", MemoryType: UserProfile, Title: "expired-user", Content: "stale personal note", ExpiresAt: past},
		{WorkspaceID: "ws-1", MemoryType: LongTerm, Title: "expired-long", Content: "dated project fact", ExpiresAt: past},
		{WorkspaceID: "ws-1", MemoryType: Session, Title: "fresh-session", Content: "recent run fact", ExpiresAt: future},
		{WorkspaceID: "ws-1", MemoryType: LongTerm, Title: "forever", Content: "permanent fact"},
		{WorkspaceID: "global", MemoryType: UserProfile, Title: "profile", Content: "likes tea"},
	} {
		e.Source = "test"
		if _, err := store.Insert(ctx, e); err != nil {
			t.Fatal(err)
		}
	}

	r := NewExpiryReaper(func() *Store { return store }, time.Hour)
	if n := r.reap(ctx); n != 3 {
		t.Errorf("reaped %d facts, want the 3 expired ones", n)
	}
	left := map[string]bool{}
	for _, ws := range []string{"ws-1", "global"} {
		entries, _ := store.List(ctx, ws, "", 20, 0)
		for _, e := range entries {
			left[e.Title] = true
		}
	}
	for _, want := range []string{"fresh-session", "forever", "profile"} {
		if !left[want] {
			t.Errorf("%q must survive the sweep", want)
		}
	}
	if len(left) != 3 {
		t.Errorf("only the 3 facts without a passed expiry should remain, got %v", left)
	}
}

// The store is replaced on factory reset and absent when the DB failed to
// open: the reaper must re-resolve it each tick and tolerate nil.
func TestExpiryReaper_ToleratesMissingStore(t *testing.T) {
	r := NewExpiryReaper(func() *Store { return nil }, time.Hour)
	if n := r.reap(context.Background()); n != 0 {
		t.Errorf("reaped %d from a nil store", n)
	}
}

func TestExpiryReaper_StopsWhenContextCancelled(t *testing.T) {
	store := newTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	r := NewExpiryReaper(func() *Store { return store }, 10*time.Millisecond)

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
