package memory

import (
	"context"
	"os"
	"testing"
	"time"

	"llm-proxy/internal/platform/db"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	f, err := os.CreateTemp("", "memory-test-*.db")
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

	memStore, err := New(p)
	if err != nil {
		t.Fatalf("memory.New: %v", err)
	}
	return memStore
}

func TestMemoryStore_Insert(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	id, err := store.Insert(ctx, "ws-1", LongTerm, "test key", "test content", "agent")
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}
	if id == 0 {
		t.Fatal("expected non-zero id")
	}
}

func TestMemoryStore_Search(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.Insert(ctx, "ws-1", LongTerm, "port", "The test DB port is 5433", "agent")
	store.Insert(ctx, "ws-1", LongTerm, "language", "User prefers TypeScript", "agent")

	entries, err := store.Search(ctx, "ws-1", "port", 5)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least 1 result")
	}
	foundPort := false
	for _, e := range entries {
		if e.Title == "port" {
			foundPort = true
			break
		}
	}
	if !foundPort {
		t.Error("expected to find 'port' memory via search")
	}
}

func TestMemoryStore_SearchNoMatch(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.Insert(ctx, "ws-1", LongTerm, "port", "5433", "agent")

	entries, err := store.Search(ctx, "ws-1", "nonexistent", 5)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 results for nonexistent query, got %d", len(entries))
	}
}

func TestMemoryStore_WorkspaceIsolation(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.Insert(ctx, "ws-1", LongTerm, "key", "ws1 content", "agent")
	store.Insert(ctx, "ws-2", LongTerm, "key", "ws2 content", "agent")

	entries, err := store.Search(ctx, "ws-1", "content", 5)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	for _, e := range entries {
		if e.WorkspaceID != "ws-1" {
			t.Errorf("expected ws-1 only, got %s", e.WorkspaceID)
		}
	}
}

func TestMemoryStore_Delete(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	id, _ := store.Insert(ctx, "ws-1", LongTerm, "key", "content", "agent")

	if err := store.Delete(ctx, "ws-1", id); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	entry, _ := store.Get(ctx, "ws-1", id)
	if entry != nil {
		t.Error("expected nil after delete")
	}
}

func TestMemoryStore_Update(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	id, _ := store.Insert(ctx, "ws-1", LongTerm, "old title", "old content", "agent")

	if err := store.Update(ctx, "ws-1", id, "new title", "new content"); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	entry, _ := store.Get(ctx, "ws-1", id)
	if entry == nil {
		t.Fatal("expected entry after update")
	}
	if entry.Title != "new title" {
		t.Errorf("expected 'new title', got '%s'", entry.Title)
	}
	if entry.Content != "new content" {
		t.Errorf("expected 'new content', got '%s'", entry.Content)
	}
}

func TestMemoryStore_DeleteOlderThan(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.Insert(ctx, "ws-1", Daily, "old", "old content", "agent")
	store.Insert(ctx, "ws-1", Daily, "new", "new content", "agent")

	n, err := store.DeleteOlderThan(ctx, Daily, time.Now().Add(-1*time.Hour))
	if err != nil {
		t.Fatalf("DeleteOlderThan failed: %v", err)
	}
	if n != 0 {
		t.Logf("deleted %d old memories (expected 0 since all are recent)", n)
	}

	entries, _ := store.List(ctx, "ws-1", Daily, 10, 0)
	if len(entries) != 2 {
		t.Errorf("expected 2 entries after deleting none, got %d", len(entries))
	}
}

func TestMemoryStore_List(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.Insert(ctx, "ws-1", LongTerm, "a", "content a", "agent")
	store.Insert(ctx, "ws-1", Daily, "b", "content b", "agent")

	entries, err := store.List(ctx, "ws-1", LongTerm, 10, 0)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 long_term entry, got %d", len(entries))
	}
}

func TestMemoryStore_Get(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	id, _ := store.Insert(ctx, "ws-1", LongTerm, "title", "content", "agent")

	entry, err := store.Get(ctx, "ws-1", id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if entry == nil {
		t.Fatal("expected non-nil entry")
	}
	if entry.Title != "title" {
		t.Errorf("expected 'title', got '%s'", entry.Title)
	}
	if entry.Content != "content" {
		t.Errorf("expected 'content', got '%s'", entry.Content)
	}
	if entry.Source != "agent" {
		t.Errorf("expected source 'agent', got '%s'", entry.Source)
	}
}
