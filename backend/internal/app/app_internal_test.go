package app

import (
	"context"
	"os"
	"testing"
	"time"

	"llm-proxy/internal/platform/db"
	"llm-proxy/internal/platform/memory"
)

// The memory jobs are tethered to the app context: cancelling it (shutdown)
// must flush the usage counts still pending in memory.
func TestStartMemoryJobs_FlushesPendingUsageOnShutdown(t *testing.T) {
	f, err := os.CreateTemp("", "app-memory-jobs-*.db")
	if err != nil {
		t.Fatal(err)
	}
	path := f.Name()
	f.Close()
	t.Cleanup(func() { os.Remove(path) })
	p, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.DB().Close() })
	store, err := memory.New(p)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := store.Insert(context.Background(), "ws-1", memory.LongTerm, "build", "x", []string{memory.HotTag}, "agent")
	entry, _ := store.Get(context.Background(), "ws-1", id)

	ctx, cancel := context.WithCancel(context.Background())
	startMemoryJobs(ctx, func() *memory.Store { return store }, func() time.Duration { return time.Hour })
	store.RecordInjected([]memory.MemoryEntry{*entry})
	cancel()

	deadline := time.Now().Add(3 * time.Second)
	for {
		got, _ := store.Get(context.Background(), "ws-1", id)
		if got != nil && got.InjectedCount == 1 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("pending usage was not flushed when the app context was cancelled")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Shutdown must not rely on a goroutine finishing before the process exits: it
// flushes the pending usage counts itself.
func TestApp_FlushMemoryUsageWritesPendingCountsSynchronously(t *testing.T) {
	f, err := os.CreateTemp("", "app-memory-flush-*.db")
	if err != nil {
		t.Fatal(err)
	}
	path := f.Name()
	f.Close()
	t.Cleanup(func() { os.Remove(path) })
	p, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.DB().Close() })
	store, err := memory.New(p)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := store.Insert(context.Background(), "ws-1", memory.LongTerm, "build", "x", nil, "agent")
	entry, _ := store.Get(context.Background(), "ws-1", id)
	store.RecordSearched([]memory.MemoryEntry{*entry})

	a := &App{services: &AppServices{AppCtx: &AppContext{memoryStore: store}}}
	a.flushMemoryUsage(context.Background())

	if got, _ := store.Get(context.Background(), "ws-1", id); got.SearchedCount != 1 {
		t.Errorf("searched = %d, want 1 immediately after flushMemoryUsage", got.SearchedCount)
	}
}

func TestApp_FlushMemoryUsageToleratesNoServicesOrStore(t *testing.T) {
	(&App{}).flushMemoryUsage(context.Background())
	(&App{services: &AppServices{AppCtx: &AppContext{}}}).flushMemoryUsage(context.Background())
}
