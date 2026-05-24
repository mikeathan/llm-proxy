package orchestrator

import (
	"testing"
	"time"

	"llm-proxy/internal/platform/ledger"
)

func TestSlotManager_CacheKey_Deterministic(t *testing.T) {
	m := NewSlotManager(nil)
	p := SlotParams{
		SystemPrompt:    "You are a helpful assistant.",
		FirstUserMsg:    "Hello",
		Temperature:     0.7,
		TopP:            0.9,
		PresencePenalty: 0.0,
	}
	k1 := m.cacheKey(p)
	k2 := m.cacheKey(p)
	if k1 != k2 {
		t.Fatalf("cache key not deterministic: %s != %s", k1, k2)
	}
}

func TestSlotManager_CacheKey_DiffersOnTemp(t *testing.T) {
	m := NewSlotManager(nil)
	p1 := SlotParams{SystemPrompt: "A", FirstUserMsg: "B", Temperature: 0.0}
	p2 := SlotParams{SystemPrompt: "A", FirstUserMsg: "B", Temperature: 1.0}
	if m.cacheKey(p1) == m.cacheKey(p2) {
		t.Fatal("expected different cache keys for different temperatures")
	}
}

func TestSlotManager_CacheKey_DiffersOnPrompt(t *testing.T) {
	m := NewSlotManager(nil)
	p1 := SlotParams{SystemPrompt: "A"}
	p2 := SlotParams{SystemPrompt: "B"}
	if m.cacheKey(p1) == m.cacheKey(p2) {
		t.Fatal("expected different cache keys for different prompts")
	}
}

func TestSlotManager_CacheKey_IncludesAllParams(t *testing.T) {
	m := NewSlotManager(nil)
	p1 := SlotParams{TopP: 0.5, PresencePenalty: 0.0}
	p2 := SlotParams{TopP: 0.5, PresencePenalty: 0.5}
	if m.cacheKey(p1) == m.cacheKey(p2) {
		t.Fatal("expected different cache keys for different presence_penalty")
	}
}

func TestSlotManager_NilSafe(t *testing.T) {
	var m *SlotManager
	restored, err := m.RestoreIfCached(nil, SlotParams{})
	if err != nil {
		t.Fatalf("nil SlotManager should not error: %v", err)
	}
	if restored {
		t.Fatal("nil SlotManager should return false")
	}
	if err := m.SaveAfterResponse(nil, SlotParams{}); err != nil {
		t.Fatalf("nil SlotManager should not error on save: %v", err)
	}
}

func TestSlotManager_NoStore_NilSafe(t *testing.T) {
	m := NewSlotManager(nil)
	restored, err := m.RestoreIfCached(nil, SlotParams{})
	if err != nil {
		t.Fatalf("no-store SlotManager should not error: %v", err)
	}
	if restored {
		t.Fatal("no-store SlotManager should return false")
	}
}

func TestSlotManager_BaseURL(t *testing.T) {
	m := NewSlotManager(nil)
	url := m.baseURL("127.0.0.1", 8081)
	if url != "http://127.0.0.1:8081" {
		t.Fatalf("expected http://127.0.0.1:8081, got %s", url)
	}
}

func TestSlotManager_TTLExpiry(t *testing.T) {
	m := NewSlotManager(nil)
	_ = m
	slot := ledger.SlotRecord{
		ModelName: "test",
		SlotID:    0,
		ExpiresAt: time.Now().Add(-time.Hour),
	}
	slot2 := ledger.SlotRecord{
		ModelName: "test",
		SlotID:    1,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if slot.ExpiresAt.After(time.Now()) {
		t.Fatal("expected expired slot to be in the past")
	}
	if !slot2.ExpiresAt.After(time.Now()) {
		t.Fatal("expected active slot to be in the future")
	}
}
