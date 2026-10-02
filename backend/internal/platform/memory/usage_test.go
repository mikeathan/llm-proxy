package memory

import (
	"context"
	"sync"
	"testing"
	"time"
)

func insertHot(t *testing.T, s *Store, ws, title string) MemoryEntry {
	t.Helper()
	id, err := s.Insert(context.Background(), ws, LongTerm, title, "content of "+title, []string{HotTag}, "agent")
	if err != nil {
		t.Fatal(err)
	}
	e, _ := s.Get(context.Background(), ws, id)
	return *e
}

func reload(t *testing.T, s *Store, e MemoryEntry) MemoryEntry {
	t.Helper()
	got, err := s.Get(context.Background(), e.WorkspaceID, e.ID)
	if err != nil || got == nil {
		t.Fatalf("reload %d: %v", e.ID, err)
	}
	return *got
}

// The run path must never wait on the database: recording is an in-memory
// increment, and only the background flush writes.
func TestUsage_RecordingDoesNotWriteUntilFlushed(t *testing.T) {
	s := newTestStore(t)
	e := insertHot(t, s, "ws-1", "build")

	s.RecordInjected([]MemoryEntry{e})
	if got := reload(t, s, e); got.InjectedCount != 0 || got.LastUsedAt != "" {
		t.Fatalf("a recorded use must not reach the database before FlushUsage, got %+v", got)
	}
	if err := s.FlushUsage(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := reload(t, s, e)
	if got.InjectedCount != 1 || got.SearchedCount != 0 || got.LastUsedAt == "" {
		t.Errorf("after flush: injected=%d searched=%d last_used=%q", got.InjectedCount, got.SearchedCount, got.LastUsedAt)
	}
}

func TestUsage_CountsAccumulateIntoOneFlushPerEntryAndKind(t *testing.T) {
	s := newTestStore(t)
	ws := insertHot(t, s, "ws-1", "ws fact")
	user := insertHot(t, s, "global", "user fact")

	for range 3 {
		s.RecordInjected([]MemoryEntry{ws, user})
	}
	s.RecordSearched([]MemoryEntry{ws})
	s.RecordSearched([]MemoryEntry{ws})
	if err := s.FlushUsage(context.Background()); err != nil {
		t.Fatal(err)
	}

	if got := reload(t, s, ws); got.InjectedCount != 3 || got.SearchedCount != 2 {
		t.Errorf("workspace fact: injected=%d searched=%d, want 3 and 2", got.InjectedCount, got.SearchedCount)
	}
	if got := reload(t, s, user); got.InjectedCount != 3 || got.SearchedCount != 0 {
		t.Errorf("user-wide fact (stored under global): injected=%d searched=%d, want 3 and 0", got.InjectedCount, got.SearchedCount)
	}
	// A second flush with nothing new must not double count.
	if err := s.FlushUsage(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := reload(t, s, ws); got.InjectedCount != 3 {
		t.Errorf("flushing twice changed the count to %d", got.InjectedCount)
	}
}

// A flush that fails (database busy, shutdown racing) must not lose the counts.
func TestUsage_FailedFlushKeepsTheCountsForNextTime(t *testing.T) {
	s := newTestStore(t)
	e := insertHot(t, s, "ws-1", "build")
	s.RecordInjected([]MemoryEntry{e})

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.FlushUsage(cancelled); err == nil {
		t.Fatal("a flush with a cancelled context should report the failure")
	}
	if err := s.FlushUsage(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := reload(t, s, e); got.InjectedCount != 1 {
		t.Errorf("counts lost after a failed flush: injected=%d, want 1", got.InjectedCount)
	}
}

func TestUsage_DeletedEntryIsHarmless(t *testing.T) {
	s := newTestStore(t)
	e := insertHot(t, s, "ws-1", "gone")
	s.RecordInjected([]MemoryEntry{e})
	if err := s.Delete(context.Background(), "ws-1", e.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.FlushUsage(context.Background()); err != nil {
		t.Errorf("recording a use of an entry deleted since must not fail the flush: %v", err)
	}
}

// Editing a fact is not using it, and neither is changing its priority.
func TestUsage_EditsAndPriorityDoNotCountAsUse(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	e := insertHot(t, s, "ws-1", "build")
	s.Update(ctx, "ws-1", e.ID, "build", "reworded", []string{HotTag}, ReplaceTags)
	s.SetPriority(ctx, "ws-1", e.ID, PriorityHigh)
	if got := reload(t, s, e); got.InjectedCount != 0 || got.SearchedCount != 0 || got.LastUsedAt != "" {
		t.Errorf("an edit must not look like a use: %+v", got)
	}
}

func TestListUnused_ReturnsOnlyFactsNeverSentOrFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	sent := insertHot(t, s, "ws-1", "sent")
	found := insertHot(t, s, "ws-1", "found")
	insertHot(t, s, "ws-1", "never")
	s.RecordInjected([]MemoryEntry{sent})
	s.RecordSearched([]MemoryEntry{found})
	s.FlushUsage(ctx)

	unused, err := s.ListUnused(ctx, "ws-1", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(unused) != 1 || unused[0].Title != "never" {
		t.Errorf("unused = %+v, want only 'never'", unused)
	}
}

func TestUsage_SafeUnderConcurrentRecordAndFlush(t *testing.T) {
	s := newTestStore(t)
	e := insertHot(t, s, "ws-1", "busy")
	const writers, perWriter = 8, 50

	var wg sync.WaitGroup
	for range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range perWriter {
				s.RecordInjected([]MemoryEntry{e})
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 5 {
			s.FlushUsage(context.Background())
		}
	}()
	wg.Wait()
	if err := s.FlushUsage(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := reload(t, s, e); got.InjectedCount != writers*perWriter {
		t.Errorf("injected = %d, want %d (no use may be lost or doubled)", got.InjectedCount, writers*perWriter)
	}
}

// The flusher writes on an interval and once more on shutdown, so a stop never
// drops the last window of counts.
func TestUsageFlusher_FlushesPeriodicallyAndOnShutdown(t *testing.T) {
	s := newTestStore(t)
	e := insertHot(t, s, "ws-1", "build")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		NewUsageFlusher(func() *Store { return s }, 10*time.Millisecond).Start(ctx)
		close(done)
	}()

	s.RecordInjected([]MemoryEntry{e})
	deadline := time.Now().Add(2 * time.Second)
	for reload(t, s, e).InjectedCount != 1 {
		if time.Now().After(deadline) {
			t.Fatal("the periodic flush never wrote the count")
		}
		time.Sleep(5 * time.Millisecond)
	}

	s.RecordInjected([]MemoryEntry{e}) // recorded just before shutdown
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after cancel (leaked goroutine)")
	}
	if got := reload(t, s, e); got.InjectedCount != 2 {
		t.Errorf("the final flush on shutdown was lost: injected=%d, want 2", got.InjectedCount)
	}
}

func TestUsageFlusher_ToleratesMissingStore(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		NewUsageFlusher(func() *Store { return nil }, 5*time.Millisecond).Start(ctx)
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("flusher with no store did not stop")
	}
}
