package memory

import (
	"context"
	"os"
	"strings"
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

	id, err := store.Insert(ctx, NewEntry{WorkspaceID: "ws-1", MemoryType: LongTerm, Title: "test key", Content: "test content", Tags: nil, Source: "agent"})
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

	store.Insert(ctx, NewEntry{WorkspaceID: "ws-1", MemoryType: LongTerm, Title: "port", Content: "The test DB port is 5433", Tags: nil, Source: "agent"})
	store.Insert(ctx, NewEntry{WorkspaceID: "ws-1", MemoryType: LongTerm, Title: "language", Content: "User prefers TypeScript", Tags: nil, Source: "agent"})

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

	store.Insert(ctx, NewEntry{WorkspaceID: "ws-1", MemoryType: LongTerm, Title: "port", Content: "5433", Tags: nil, Source: "agent"})

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

	store.Insert(ctx, NewEntry{WorkspaceID: "ws-1", MemoryType: LongTerm, Title: "key", Content: "ws1 content", Tags: nil, Source: "agent"})
	store.Insert(ctx, NewEntry{WorkspaceID: "ws-2", MemoryType: LongTerm, Title: "key", Content: "ws2 content", Tags: nil, Source: "agent"})

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

	id, _ := store.Insert(ctx, NewEntry{WorkspaceID: "ws-1", MemoryType: LongTerm, Title: "key", Content: "content", Tags: nil, Source: "agent"})

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

	id, _ := store.Insert(ctx, NewEntry{WorkspaceID: "ws-1", MemoryType: LongTerm, Title: "old title", Content: "old content", Tags: nil, Source: "agent"})

	if err := store.Update(ctx, "ws-1", id, Edit{Title: "new title", Content: "new content", Tags: nil, TagMode: ReplaceTags}); err != nil {
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

	store.Insert(ctx, NewEntry{WorkspaceID: "ws-1", MemoryType: LongTerm, Title: "key", Content: "exact content", Tags: nil, Source: "agent"})

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

	store.Insert(ctx, NewEntry{WorkspaceID: "ws-1", MemoryType: LongTerm, Title: "port", Content: "the database port is 5433", Tags: nil, Source: "agent"})
	store.Insert(ctx, NewEntry{WorkspaceID: "ws-1", MemoryType: LongTerm, Title: "host", Content: "server hostname is prod-01", Tags: nil, Source: "agent"})

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

func TestMemoryStore_List(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.Insert(ctx, NewEntry{WorkspaceID: "ws-1", MemoryType: LongTerm, Title: "a", Content: "content a", Tags: nil, Source: "agent"})
	store.Insert(ctx, NewEntry{WorkspaceID: "ws-1", MemoryType: Daily, Title: "b", Content: "content b", Tags: nil, Source: "agent"})

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

	store.Insert(ctx, NewEntry{WorkspaceID: "ws-1", MemoryType: LongTerm, Title: "k1", Content: "content 1", Tags: nil, Source: "agent"})
	store.Insert(ctx, NewEntry{WorkspaceID: "ws-1", MemoryType: Daily, Title: "k2", Content: "content 2", Tags: nil, Source: "agent"})
	store.Insert(ctx, NewEntry{WorkspaceID: "ws-1", MemoryType: Session, Title: "k3", Content: "content 3", Tags: nil, Source: "agent"})
	store.Insert(ctx, NewEntry{WorkspaceID: "ws-2", MemoryType: LongTerm, Title: "k4", Content: "content 4", Tags: nil, Source: "agent"})

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
		store.Insert(ctx, NewEntry{WorkspaceID: "ws-2", MemoryType: LongTerm, Title: "k5", Content: "content 5", Tags: nil, Source: "agent"})
		store.Insert(ctx, NewEntry{WorkspaceID: "ws-2", MemoryType: Daily, Title: "k6", Content: "content 6", Tags: nil, Source: "agent"})
		store.Insert(ctx, NewEntry{WorkspaceID: "ws-2", MemoryType: Daily, Title: "k7", Content: "content 7", Tags: nil, Source: "agent"})

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

	id, _ := store.Insert(ctx, NewEntry{WorkspaceID: "ws-1", MemoryType: LongTerm, Title: "title", Content: "content", Tags: nil, Source: "agent"})

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
	store.Insert(ctx, NewEntry{WorkspaceID: "ws-1", MemoryType: LongTerm, Title: "tool_versions", Content: "TypeScript version: 6.0.3 installed", Tags: nil, Source: "agent"})

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

	store.Insert(ctx, NewEntry{WorkspaceID: "ws-1", MemoryType: LongTerm, Title: "installed_tools", Content: "node typescript python", Tags: nil, Source: "agent"})

	// The "AND" keyword should not cause a syntax error when properly quoted.
	_, err := store.Search(ctx, "ws-1", "node AND typescript", 5)
	if err != nil {
		t.Fatalf("Search with AND keyword failed: %v", err)
	}
}

func TestSanitiseFTSQuery_Underscore(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.Insert(ctx, NewEntry{WorkspaceID: "ws-1", MemoryType: LongTerm, Title: "typescript_version", Content: "TypeScript version 6.0.3", Tags: nil, Source: "agent"})

	// Underscores are stripped and replaced with OR terms.
	_, err := store.Search(ctx, "ws-1", "typescript_version", 5)
	if err != nil {
		t.Fatalf("Search with underscore failed: %v", err)
	}
}

func TestSanitiseFTSQuery_SpecialChars(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.Insert(ctx, NewEntry{WorkspaceID: "ws-1", MemoryType: LongTerm, Title: "test", Content: "some content for testing", Tags: nil, Source: "agent"})

	// Special characters like parentheses should be stripped.
	_, err := store.Search(ctx, "ws-1", "test (content)", 5)
	if err != nil {
		t.Fatalf("Search with special chars failed: %v", err)
	}
}

func TestSanitiseFTSQuery_StopWordsFiltered(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.Insert(ctx, NewEntry{WorkspaceID: "ws-1", MemoryType: LongTerm, Title: "tool_versions", Content: "TypeScript version installed: 6.0.3", Tags: nil, Source: "agent"})

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

	store.Insert(ctx, NewEntry{WorkspaceID: "ws-1", MemoryType: LongTerm, Title: "tool_versions", Content: "TypeScript version installed: 6.0.3", Tags: nil, Source: "agent"})

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

func TestSearchHot(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.Insert(ctx, NewEntry{WorkspaceID: "ws-1", MemoryType: LongTerm, Title: "build", Content: "use go build ./...", Tags: []string{"hot"}, Source: "agent"})
	store.Insert(ctx, NewEntry{WorkspaceID: "ws-1", MemoryType: LongTerm, Title: "version", Content: "TypeScript 6.0.3", Tags: []string{"hot"}, Source: "agent"})
	store.Insert(ctx, NewEntry{WorkspaceID: "ws-1", MemoryType: LongTerm, Title: "port", Content: "database port is 5433", Tags: nil, Source: "agent"})
	store.Insert(ctx, NewEntry{WorkspaceID: "global", MemoryType: UserProfile, Title: "name", Content: "Alice", Tags: []string{"hot"}, Source: "agent"})

	entries, err := store.SearchHot(ctx, "ws-1")
	if err != nil {
		t.Fatalf("SearchHot failed: %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("expected 3 hot entries (2 ws-1 + 1 global), got %d", len(entries))
	}

	titles := make(map[string]bool)
	for _, e := range entries {
		titles[e.Title] = true
	}
	if !titles["build"] {
		t.Error("expected 'build' in hot results")
	}
	if !titles["version"] {
		t.Error("expected 'version' in hot results")
	}
	if !titles["name"] {
		t.Error("expected global 'name' in hot results")
	}
	if titles["port"] {
		t.Error("'port' has no hot tag and should NOT appear")
	}
}

func TestSearchHot_ScopeExclusion(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.Insert(ctx, NewEntry{WorkspaceID: "ws-1", MemoryType: LongTerm, Title: "local", Content: "local hot entry", Tags: []string{"hot"}, Source: "agent"})
	store.Insert(ctx, NewEntry{WorkspaceID: "ws-2", MemoryType: LongTerm, Title: "other", Content: "other workspace hot entry", Tags: []string{"hot"}, Source: "agent"})

	entries, err := store.SearchHot(ctx, "ws-1")
	if err != nil {
		t.Fatalf("SearchHot failed: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry for ws-1, got %d", len(entries))
	}
	if entries[0].Title != "local" {
		t.Errorf("expected 'local', got '%s'", entries[0].Title)
	}
}

func TestScopeValidate(t *testing.T) {
	if err := Scope("user").Validate(); err != nil {
		t.Errorf("expected nil for scope 'user', got: %v", err)
	}
	if err := Scope("workspace").Validate(); err != nil {
		t.Errorf("expected nil for scope 'workspace', got: %v", err)
	}
	if err := Scope("garbage").Validate(); err == nil {
		t.Error("expected error for invalid scope 'garbage'")
	}
}

func TestModeValidate(t *testing.T) {
	if err := Mode("always").Validate(); err != nil {
		t.Errorf("expected nil for mode 'always', got: %v", err)
	}
	if err := Mode("on_demand").Validate(); err != nil {
		t.Errorf("expected nil for mode 'on_demand', got: %v", err)
	}
	if err := Mode("garbage").Validate(); err == nil {
		t.Error("expected error for invalid mode 'garbage'")
	}
}

func TestKeepValidate(t *testing.T) {
	if err := Keep("permanent").Validate(); err != nil {
		t.Errorf("expected nil for keep 'permanent', got: %v", err)
	}
	if err := Keep("session").Validate(); err != nil {
		t.Errorf("expected nil for keep 'session', got: %v", err)
	}
	if err := Keep("garbage").Validate(); err == nil {
		t.Error("expected error for invalid keep 'garbage'")
	}
}

func TestSearchHot_EmptyResult(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	store.Insert(ctx, NewEntry{WorkspaceID: "ws-1", MemoryType: LongTerm, Title: "not-hot", Content: "no hot tag here", Tags: nil, Source: "agent"})

	entries, err := store.SearchHot(ctx, "ws-1")
	if err != nil {
		t.Fatalf("SearchHot failed: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries (no hot tags), got %d", len(entries))
	}

	entries, err = store.SearchHot(ctx, "ws-empty")
	if err != nil {
		t.Fatalf("SearchHot on empty workspace failed: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries for empty workspace, got %d", len(entries))
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

// Search matches the forms of a word, not only its exact spelling: a question rarely repeats a stored fact's
// inflections ("meeting" vs "meetings", "prefers" vs "preferred"). The index stems words (FTS5 porter); no word list
// or synonym table is involved, and words that merely look alike elsewhere must not match.
func TestSearch_MatchesWordFormsNotUnrelatedWords(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	facts := map[string]string{
		"meetings": "Meetings are scheduled on Tuesdays.",
		"walking":  "She prefers walking to work.",
		"reports":  "Reports are generated nightly.",
		"garden":   "The garden needs new tools.",
	}
	ids := map[string]int64{}
	for k, content := range facts {
		id, err := s.Insert(ctx, NewEntry{WorkspaceID: "ws", MemoryType: LongTerm, Title: k, Content: content, Tags: nil, Source: "test"})
		if err != nil {
			t.Fatal(err)
		}
		ids[k] = id
	}
	for _, tc := range []struct {
		name, query string
		want        string // key of the fact that must be found; "" means nothing may match
	}{
		{"plural and participle", "meeting scheduled", "meetings"},
		{"third person and past", "preferred walk", "walking"},
		{"plural and verb form", "report generates", "reports"},
		{"an unrelated word matches nothing", "database", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := s.Search(ctx, "ws", tc.query, 5, SearchOption{SearchAllWorkspaces: true})
			if err != nil {
				t.Fatal(err)
			}
			if tc.want == "" {
				if len(got) != 0 {
					t.Fatalf("query %q matched %d facts, want none", tc.query, len(got))
				}
				return
			}
			if len(got) == 0 || got[0].ID != ids[tc.want] {
				t.Errorf("query %q: got %+v, want the %q fact first", tc.query, got, tc.want)
			}
		})
	}
}

// A fact carries a kind, a one-line reason and an optional expiry, and every read path returns them.
func TestStore_InsertStoresKindWhyAndExpiry(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	exp := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	id, err := store.Insert(ctx, NewEntry{WorkspaceID: "ws", MemoryType: LongTerm, Title: "t", Content: "Review requests are answered within a day.",
		Source: "agent", Kind: KindFeedback, Why: "reviewers asked for faster turnaround", ExpiresAt: exp})
	if err != nil {
		t.Fatal(err)
	}
	plainID, _ := store.Insert(ctx, NewEntry{WorkspaceID: "ws", MemoryType: LongTerm, Title: "p", Content: "A fact with no typing.", Source: "agent"})

	got, _ := store.Get(ctx, "ws", id)
	if got.Kind != KindFeedback || got.Why != "reviewers asked for faster turnaround" || got.ExpiresAt != "2030-01-02T03:04:05Z" {
		t.Errorf("Get lost the typing: %+v", got)
	}
	plain, _ := store.Get(ctx, "ws", plainID)
	if plain.Kind != "" || plain.Why != "" || plain.ExpiresAt != "" {
		t.Errorf("an untyped fact must read back untyped: %+v", plain)
	}
	listed, _ := store.List(ctx, "ws", "", 10, 0)
	found, _ := store.Search(ctx, "ws", "reviewers requests", 5)
	for name, entries := range map[string][]MemoryEntry{"List": listed, "Search": found} {
		ok := false
		for _, e := range entries {
			ok = ok || (e.ID == id && e.Kind == KindFeedback && e.Why != "" && e.ExpiresAt != "")
		}
		if !ok {
			t.Errorf("%s did not return the typing: %+v", name, entries)
		}
	}
}

// A bad kind or reason is refused at the store, so nothing malformed is ever persisted.
func TestStore_InsertRefusesBadTyping(t *testing.T) {
	store := newTestStore(t)
	for name, e := range map[string]NewEntry{
		"unknown kind":   {Kind: "opinion"},
		"multi-line why": {Why: "line one\nline two"},
		"too long a why": {Why: strings.Repeat("w", MaxWhyChars+1)},
	} {
		t.Run(name, func(t *testing.T) {
			e.WorkspaceID, e.MemoryType, e.Content, e.Source = "ws", LongTerm, "content", "agent"
			if _, err := store.Insert(context.Background(), e); err == nil {
				t.Error("Insert accepted a malformed typing")
			}
		})
	}
}

// Update replaces the typing; a zero expiry clears it.
func TestStore_UpdateReplacesTypingAndClearsExpiry(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	id, _ := store.Insert(ctx, NewEntry{WorkspaceID: "ws", MemoryType: LongTerm, Title: "t", Content: "old", Source: "agent",
		Kind: KindProject, Why: "old reason", ExpiresAt: time.Now().Add(time.Hour)})

	if err := store.Update(ctx, "ws", id, Edit{Title: "t", Content: "new", Kind: KindReference, Why: "where it lives"}); err != nil {
		t.Fatal(err)
	}
	got, _ := store.Get(ctx, "ws", id)
	if got.Content != "new" || got.Kind != KindReference || got.Why != "where it lives" || got.ExpiresAt != "" {
		t.Errorf("Update must replace kind and why and clear the expiry: %+v", got)
	}
	if err := store.Update(ctx, "ws", id, Edit{Content: "x", Kind: "bogus"}); err == nil {
		t.Error("Update accepted a bad kind")
	}
}

func TestStore_DeleteExpiredCountsAndRemovesOnlyThePassed(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	now := time.Now()
	store.Insert(ctx, NewEntry{WorkspaceID: "ws", MemoryType: LongTerm, Content: "gone", Source: "a", ExpiresAt: now.Add(-time.Minute)})
	store.Insert(ctx, NewEntry{WorkspaceID: "ws", MemoryType: LongTerm, Content: "later", Source: "a", ExpiresAt: now.Add(time.Hour)})
	store.Insert(ctx, NewEntry{WorkspaceID: "ws", MemoryType: LongTerm, Content: "never", Source: "a"})
	n, err := store.DeleteExpired(ctx, now)
	if err != nil || n != 1 {
		t.Fatalf("DeleteExpired = %d, %v; want 1", n, err)
	}
	if left, _ := store.List(ctx, "ws", "", 10, 0); len(left) != 2 {
		t.Errorf("two facts should remain, got %d", len(left))
	}
}

// Sessions facts written before expiry existed have no expiry; the schema migration gives them one so the reaper
// does not leave them forever.
func TestNew_GivesLegacySessionFactsAnExpiry(t *testing.T) {
	f, _ := os.CreateTemp("", "memory-legacy-*.db")
	path := f.Name()
	f.Close()
	t.Cleanup(func() { os.Remove(path) })
	p, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.DB().Close() })
	store, err := New(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.DB().Exec(`INSERT INTO memories (workspace_id, memory_type, title, content, source, created_at, expires_at)
		VALUES ('ws', 'session', 't', 'legacy session fact', 'run:1', '2026-01-01 00:00:00', NULL),
		       ('ws', 'long_term', 't2', 'legacy permanent fact', 'agent', '2026-01-01 00:00:00', NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := New(p); err != nil { // the next start runs the migration again
		t.Fatal(err)
	}
	all, _ := store.List(context.Background(), "ws", "", 10, 0)
	for _, e := range all {
		switch e.Content {
		case "legacy session fact":
			if e.ExpiresAt != "2026-04-01T00:00:00Z" { // created + the 90-day default
				t.Errorf("the legacy session fact should expire 90 days after it was created, got %q", e.ExpiresAt)
			}
		case "legacy permanent fact":
			if e.ExpiresAt != "" {
				t.Errorf("a permanent fact must stay without an expiry, got %q", e.ExpiresAt)
			}
		}
	}
}
