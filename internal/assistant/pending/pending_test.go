package pending_test

import (
	"testing"
	"time"

	"llm-proxy/internal/assistant/devices"
	"llm-proxy/internal/assistant/pending"
)

type fakeClock struct {
	now time.Time
}

func (f *fakeClock) NowUtc() time.Time {
	return f.now.UTC()
}

func TestPendingToolCallStore_SetGetClear(t *testing.T) {
	clock := &fakeClock{now: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}
	store := pending.NewInMemoryPendingToolCallStoreWithOptions(10*time.Second, clock, time.Hour)

	state := pending.PendingToolCallState{
		Target: "attic",
		Expose: "temperature",
	}
	store.Set("conv-1", state)

	got, ok := store.Get("conv-1")
	if !ok || got.Target != "attic" {
		t.Fatalf("expected pending state to be stored")
	}

	store.Clear("conv-1")
	if _, ok := store.Get("conv-1"); ok {
		t.Fatalf("expected pending state to be cleared")
	}
}

func TestPendingToolCallStore_Isolation(t *testing.T) {
	clock := &fakeClock{now: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}
	store := pending.NewInMemoryPendingToolCallStoreWithOptions(10*time.Second, clock, time.Hour)

	store.Set("conv-1", pending.PendingToolCallState{Target: "attic"})
	store.Set("conv-2", pending.PendingToolCallState{Target: "kitchen"})

	got, ok := store.Get("conv-2")
	if !ok || got.Target != "kitchen" {
		t.Fatalf("expected isolated state for conv-2")
	}
}

func TestPendingToolCallStore_TTL(t *testing.T) {
	clock := &fakeClock{now: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}
	store := pending.NewInMemoryPendingToolCallStoreWithOptions(2*time.Second, clock, time.Hour)

	store.Set("conv-1", pending.PendingToolCallState{Target: "attic"})
	clock.now = clock.now.Add(3 * time.Second)

	if _, ok := store.Get("conv-1"); ok {
		t.Fatalf("expected pending state to expire")
	}
}

func TestResolvePendingToolCall(t *testing.T) {
	candidates := []devices.Candidate{
		{ID: "dev1", Name: "Attic Air Sensor"},
		{ID: "dev2", Name: "Attic Room Sensor"},
	}

	candidate, ok := pending.ResolvePendingToolCall("2", candidates)
	if !ok || candidate.ID != "dev2" {
		t.Fatalf("expected candidate dev2, got %+v", candidate)
	}
}
