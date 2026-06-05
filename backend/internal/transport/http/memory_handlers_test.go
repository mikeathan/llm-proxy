package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"llm-proxy/internal/platform/db"
	"llm-proxy/internal/platform/memory"
)

func newTestMemoryStore(t *testing.T) *memory.Store {
	t.Helper()
	f, err := os.CreateTemp("", "memory-handler-test-*.db")
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	path := f.Name()
	f.Close()
	t.Cleanup(func() { os.Remove(path) })

	p, err := db.Open(path)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { p.DB().Close() })

	store, err := memory.New(p)
	if err != nil {
		t.Fatalf("memory.New: %v", err)
	}
	return store
}

func seedMemories(t *testing.T, store *memory.Store) {
	t.Helper()
	ctx := context.Background()
	store.Insert(ctx, "ws-1", memory.LongTerm, "topic-a", "content a", "agent")
	store.Insert(ctx, "ws-1", memory.Daily, "topic-b", "content b", "agent")
	store.Insert(ctx, "ws-2", memory.LongTerm, "topic-c", "content c", "agent")
}

func TestClearWorkspace_All(t *testing.T) {
	store := newTestMemoryStore(t)
	seedMemories(t, store)
	handlers := NewMemoryHandlers(store)

	req := httptest.NewRequest("DELETE", "/admin/api/memory/ws-1", nil)
	req.SetPathValue("workspace", "ws-1")
	rr := httptest.NewRecorder()
	handlers.ClearWorkspace(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	entries, _ := store.List(context.Background(), "ws-1", "", 10, 0)
	if len(entries) != 0 {
		t.Errorf("expected 0 entries in ws-1 after clear, got %d", len(entries))
	}

	ws2Entries, _ := store.List(context.Background(), "ws-2", "", 10, 0)
	if len(ws2Entries) != 1 {
		t.Errorf("expected ws-2 untouched (1 entry), got %d", len(ws2Entries))
	}
}

func TestClearWorkspace_ByType(t *testing.T) {
	store := newTestMemoryStore(t)
	seedMemories(t, store)
	handlers := NewMemoryHandlers(store)

	req := httptest.NewRequest("DELETE", "/admin/api/memory/ws-1?type=daily", nil)
	req.SetPathValue("workspace", "ws-1")
	rr := httptest.NewRecorder()
	handlers.ClearWorkspace(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	entries, _ := store.List(context.Background(), "ws-1", "", 10, 0)
	if len(entries) != 1 {
		t.Fatalf("expected 1 remaining entry in ws-1, got %d", len(entries))
	}
	if entries[0].MemoryType != memory.LongTerm {
		t.Errorf("expected remaining entry to be long_term, got %s", entries[0].MemoryType)
	}
}

func TestClearWorkspace_MissingWorkspace(t *testing.T) {
	store := newTestMemoryStore(t)
	handlers := NewMemoryHandlers(store)

	req := httptest.NewRequest("DELETE", "/admin/api/memory/", nil)
	req.SetPathValue("workspace", "")
	rr := httptest.NewRecorder()
	handlers.ClearWorkspace(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing workspace, got %d", rr.Code)
	}
}
