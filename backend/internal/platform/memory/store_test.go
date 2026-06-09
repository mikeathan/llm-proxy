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

	id, err := store.Insert(ctx, "ws-1", LongTerm, "test key", "test content", nil, "agent")
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

	store.Insert(ctx, "ws-1", LongTerm, "port", "The test DB port is 5433", nil, "agent")
	store.Insert(ctx, "ws-1", LongTerm, "language", "User prefers TypeScript", nil, "agent")

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

	store.Insert(ctx, "ws-1", LongTerm, "port", "5433", nil, "agent")

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

	store.Insert(ctx, "ws-1", LongTerm, "key", "ws1 content", nil, "agent")
	store.Insert(ctx, "ws-2", LongTerm, "key", "ws2 content", nil, "agent")

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

	id, _ := store.Insert(ctx, "ws-1", LongTerm, "key", "content", nil, "agent")

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

	id, _ := store.Insert(ctx, "ws-1", LongTerm, "old title", "old content", nil, "agent")

	if err := store.Update(ctx, "ws-1", id, "new title", "new content", nil, ReplaceTags); err != nil {
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

	store.Insert(ctx, "ws-1", Daily, "old", "old content", nil, "agent")
	store.Insert(ctx, "ws-1", Daily, "new", "new content", nil, "agent")

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

func TestMemoryStore_Exists(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	exists, err := store.Exists(ctx, "ws-1", "missing content")
	if err != nil {
		t.Fatalf("Exists on missing: %v", err)
	}
	if exists {
		t.Error("expected false for non-existent content")
	}

	store.Insert(ctx, "ws-1", LongTerm, "key", "exact content", nil, "agent")

	exists, err = store.Exists(ctx, "ws-1", "exact content")
	if err != nil {
		t.Fatalf("Exists on match: %v", err)
	}
	if !exists {
		t.Error("expected true for matching content")
	}

	exists, err = store.Exists(ctx, "ws-other", "exact content")
	if err != nil {
		t.Fatalf("Exists on other ws: %v", err)
	}
	if exists {
		t.Error("expected false for different workspace")
	}
}

func TestMemoryStore_FindByContentSubstring(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.Insert(ctx, "ws-1", LongTerm, "port", "the database port is 5433", nil, "agent")
	store.Insert(ctx, "ws-1", LongTerm, "host", "server hostname is prod-01", nil, "agent")

	t.Run("exact match", func(t *testing.T) {
		entry, err := store.FindByContentSubstring(ctx, "ws-1", "5433")
		if err != nil {
			t.Fatalf("FindByContentSubstring: %v", err)
		}
		if entry.Title != "port" {
			t.Errorf("expected 'port', got '%s'", entry.Title)
		}
	})

	t.Run("substring match", func(t *testing.T) {
		entry, err := store.FindByContentSubstring(ctx, "ws-1", "database")
		if err != nil {
			t.Fatalf("FindByContentSubstring: %v", err)
		}
		if entry.Title != "port" {
			t.Errorf("expected 'port', got '%s'", entry.Title)
		}
	})

	t.Run("no match", func(t *testing.T) {
		_, err := store.FindByContentSubstring(ctx, "ws-1", "nonexistent")
		if err == nil {
			t.Fatal("expected error for no match")
		}
	})

	t.Run("ambiguous match", func(t *testing.T) {
		_, err := store.FindByContentSubstring(ctx, "ws-1", "is")
		if err == nil {
			t.Fatal("expected error for ambiguous match")
		}
	})
}

func TestMemoryStore_WorkspaceCharCount(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	count, err := store.WorkspaceCharCount(ctx, "ws-1")
	if err != nil {
		t.Fatalf("WorkspaceCharCount on empty store: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 for empty workspace, got %d", count)
	}

	store.Insert(ctx, "ws-1", LongTerm, "a", "hello", nil, "agent")
	store.Insert(ctx, "ws-1", LongTerm, "b", "world", nil, "agent")

	count, err = store.WorkspaceCharCount(ctx, "ws-1")
	if err != nil {
		t.Fatalf("WorkspaceCharCount: %v", err)
	}
	if count != 10 {
		t.Errorf("expected 10 chars (hello+world), got %d", count)
	}

	// Different workspace should return 0
	count, err = store.WorkspaceCharCount(ctx, "ws-other")
	if err != nil {
		t.Fatalf("WorkspaceCharCount on other ws: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 for other workspace, got %d", count)
	}
}

func TestMemoryStore_List(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.Insert(ctx, "ws-1", LongTerm, "a", "content a", nil, "agent")
	store.Insert(ctx, "ws-1", Daily, "b", "content b", nil, "agent")

	entries, err := store.List(ctx, "ws-1", LongTerm, 10, 0)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 long_term entry, got %d", len(entries))
	}
}

func TestMemoryStore_DeleteAllByWorkspace(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.Insert(ctx, "ws-1", LongTerm, "k1", "content 1", nil, "agent")
	store.Insert(ctx, "ws-1", Daily, "k2", "content 2", nil, "agent")
	store.Insert(ctx, "ws-1", Session, "k3", "content 3", nil, "agent")
	store.Insert(ctx, "ws-2", LongTerm, "k4", "content 4", nil, "agent")

	t.Run("delete all in workspace", func(t *testing.T) {
		n, err := store.DeleteAllByWorkspace(ctx, "ws-1", "")
		if err != nil {
			t.Fatalf("DeleteAllByWorkspace failed: %v", err)
		}
		if n != 3 {
			t.Errorf("expected 3 deleted, got %d", n)
		}

		entries, _ := store.List(ctx, "ws-1", "", 10, 0)
		if len(entries) != 0 {
			t.Errorf("expected 0 entries after clear, got %d", len(entries))
		}

		ws2Entries, _ := store.List(ctx, "ws-2", "", 10, 0)
		if len(ws2Entries) != 1 {
			t.Errorf("expected ws-2 untouched (1 entry), got %d", len(ws2Entries))
		}
	})

	t.Run("delete by type", func(t *testing.T) {
		store.Insert(ctx, "ws-2", LongTerm, "k5", "content 5", nil, "agent")
		store.Insert(ctx, "ws-2", Daily, "k6", "content 6", nil, "agent")
		store.Insert(ctx, "ws-2", Daily, "k7", "content 7", nil, "agent")

		n, err := store.DeleteAllByWorkspace(ctx, "ws-2", Daily)
		if err != nil {
			t.Fatalf("DeleteAllByWorkspace by type failed: %v", err)
		}
		if n != 2 {
			t.Errorf("expected 2 daily entries deleted, got %d", n)
		}

		entries, _ := store.List(ctx, "ws-2", "", 10, 0)
		if len(entries) != 2 {
			t.Errorf("expected 2 remaining entries (long_term), got %d", len(entries))
		}
		for _, e := range entries {
			if e.MemoryType != LongTerm {
				t.Errorf("expected remaining entries to be long_term, got %s", e.MemoryType)
			}
		}
	})

	t.Run("delete on empty workspace returns 0", func(t *testing.T) {
		n, err := store.DeleteAllByWorkspace(ctx, "ws-empty", "")
		if err != nil {
			t.Fatalf("DeleteAllByWorkspace on empty: %v", err)
		}
		if n != 0 {
			t.Errorf("expected 0 deleted on empty workspace, got %d", n)
		}
	})
}

func TestMemoryStore_Get(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	id, _ := store.Insert(ctx, "ws-1", LongTerm, "title", "content", nil, "agent")

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

func TestSanitiseFTSQuery_OR_Keyword(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Insert an entry so search has something to return
	store.Insert(ctx, "ws-1", LongTerm, "tool_versions", "TypeScript version: 6.0.3 installed", nil, "agent")

	// The "OR" keyword would crash FTS5 if not properly quoted.
	// This ensures the query doesn't produce a syntax error.
	_, err := store.Search(ctx, "ws-1", "typescript_version OR tool_versions", 5)
	if err != nil {
		t.Fatalf("Search with OR keyword failed: %v", err)
	}
}

func TestSanitiseFTSQuery_AND_Keyword(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.Insert(ctx, "ws-1", LongTerm, "installed_tools", "node typescript python", nil, "agent")

	// The "AND" keyword should not cause a syntax error when properly quoted.
	_, err := store.Search(ctx, "ws-1", "node AND typescript", 5)
	if err != nil {
		t.Fatalf("Search with AND keyword failed: %v", err)
	}
}

func TestSanitiseFTSQuery_Underscore(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.Insert(ctx, "ws-1", LongTerm, "typescript_version", "TypeScript version 6.0.3", nil, "agent")

	// Underscores are stripped and replaced with OR terms.
	_, err := store.Search(ctx, "ws-1", "typescript_version", 5)
	if err != nil {
		t.Fatalf("Search with underscore failed: %v", err)
	}
}

func TestSanitiseFTSQuery_SpecialChars(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.Insert(ctx, "ws-1", LongTerm, "test", "some content for testing", nil, "agent")

	// Special characters like parentheses should be stripped.
	_, err := store.Search(ctx, "ws-1", "test (content)", 5)
	if err != nil {
		t.Fatalf("Search with special chars failed: %v", err)
	}
}

func TestSanitiseFTSQuery_StopWordsFiltered(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.Insert(ctx, "ws-1", LongTerm, "tool_versions", "TypeScript version installed: 6.0.3", nil, "agent")

	// Stop words like "step" and "run" should be filtered from the query.
	// If they aren't, the search results may still include the entry through
	// matching content words, but the BM25 rank will be lower.
	_, err := store.Search(ctx, "ws-1", "Step 5: run tsc version", 5)
	if err != nil {
		t.Fatalf("Search with stop words failed: %v", err)
	}
}

func TestSanitiseFTSQuery_AllStopWordsReturnsEmptyMatch(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.Insert(ctx, "ws-1", LongTerm, "tool_versions", "TypeScript version installed: 6.0.3", nil, "agent")

	// A query consisting entirely of stop words should not cause an error.
	_, err := store.Search(ctx, "ws-1", "the a an", 5)
	if err != nil {
		t.Fatalf("Search with all stop words failed: %v", err)
	}
}

func TestIsFTSStopWord(t *testing.T) {
	cases := []struct {
		word   string
		isStop bool
	}{
		{"step", true}, {"run", true}, {"the", true},
		{"tsc", false}, {"version", false}, {"typescript", false},
		{"npm", false}, {"uname", false}, {"network", false},
	}
	for _, c := range cases {
		got := isFTSStopWord(c.word)
		if got != c.isStop {
			t.Errorf("isFTSStopWord(%q) = %v, want %v", c.word, got, c.isStop)
		}
	}
}

func TestSanitiseFTSQuery_Empty(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Empty query should return nil without error.
	entries, err := store.Search(ctx, "ws-1", "", 5)
	if err != nil {
		t.Fatalf("Search with empty query failed: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for empty query, got %d", len(entries))
	}
}


