package memory

import (
	"context"
	"errors"
	"os"
	"slices"
	"testing"

	"llm-proxy/internal/platform/db"
)

func hotTitles(t *testing.T, s *Store) []string {
	t.Helper()
	entries, err := s.SearchHot(context.Background(), "ws-1")
	if err != nil {
		t.Fatal(err)
	}
	titles := make([]string, len(entries))
	for i, e := range entries {
		titles[i] = e.Title
	}
	return titles
}

func TestPriority_NewEntriesAreNormalEverywhereTheyAreRead(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	id, _ := s.Insert(ctx, "ws-1", LongTerm, "build", "run go build", []string{HotTag}, "agent")

	got, _ := s.Get(ctx, "ws-1", id)
	if got.Priority != PriorityNormal {
		t.Errorf("Get: priority = %d, want %d", got.Priority, PriorityNormal)
	}
	listed, _ := s.List(ctx, "ws-1", "", 10, 0)
	hot, _ := s.SearchHot(ctx, "ws-1")
	found, _ := s.Search(ctx, "ws-1", "build", 5)
	tagged, _ := s.Search(ctx, "ws-1", "", 5, SearchOption{Tags: []string{HotTag}})
	for name, entries := range map[string][]MemoryEntry{"List": listed, "SearchHot": hot, "Search": found, "Search by tags": tagged} {
		if len(entries) != 1 || entries[0].Priority != PriorityNormal {
			t.Errorf("%s: %+v, want one entry with normal priority", name, entries)
		}
	}
}

func TestSetPriority_RoundTripsAndValidates(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	id, _ := s.Insert(ctx, "ws-1", LongTerm, "a", "x", nil, "agent")

	if err := s.SetPriority(ctx, "ws-1", id, PriorityHigh); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Get(ctx, "ws-1", id); got.Priority != PriorityHigh {
		t.Errorf("priority = %d, want %d", got.Priority, PriorityHigh)
	}
	for _, bad := range []int{-1, 3, 99} {
		if err := s.SetPriority(ctx, "ws-1", id, bad); !errors.Is(err, ErrInvalidPriority) {
			t.Errorf("SetPriority(%d) = %v, want ErrInvalidPriority", bad, err)
		}
	}
	if err := s.SetPriority(ctx, "ws-1", 9999, PriorityLow); err == nil {
		t.Error("a missing entry must be an error")
	}
	if err := s.SetPriority(ctx, "ws-other", id, PriorityLow); err == nil {
		t.Error("priority is workspace-scoped like every other write")
	}
}

// Priority is what protects a fact when the budget cuts the tail: it must beat
// recency in the order the prompt is built from.
func TestSearchHot_OrdersByPriorityThenRecency(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	hot := []string{HotTag}
	oldHigh, _ := s.Insert(ctx, "ws-1", LongTerm, "old-high", "x", hot, "agent")
	s.Insert(ctx, "ws-1", LongTerm, "mid-normal", "x", hot, "agent")
	newLow, _ := s.Insert(ctx, "ws-1", LongTerm, "new-low", "x", hot, "agent")
	s.Insert(ctx, "ws-1", LongTerm, "newest-normal", "x", hot, "agent")
	s.SetPriority(ctx, "ws-1", oldHigh, PriorityHigh)
	s.SetPriority(ctx, "ws-1", newLow, PriorityLow)
	// Same-second inserts tie on updated_at; make recency explicit.
	for id, ts := range map[string]string{"old-high": "2026-01-01 00:00:00", "mid-normal": "2026-02-01 00:00:00", "new-low": "2026-03-01 00:00:00", "newest-normal": "2026-04-01 00:00:00"} {
		s.db.ExecContext(ctx, `UPDATE memories SET updated_at = ? WHERE title = ?`, ts, id)
	}

	want := []string{"old-high", "newest-normal", "mid-normal", "new-low"}
	if got := hotTitles(t, s); !slices.Equal(got, want) {
		t.Errorf("hot order = %v, want %v", got, want)
	}
}

func TestUpdate_EditingTextKeepsPriority(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	id, _ := s.Insert(ctx, "ws-1", LongTerm, "a", "x", []string{HotTag}, "agent")
	s.SetPriority(ctx, "ws-1", id, PriorityHigh)
	if err := s.Update(ctx, "ws-1", id, "a", "reworded", []string{HotTag}, ReplaceTags); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Get(ctx, "ws-1", id); got.Priority != PriorityHigh {
		t.Errorf("an edit must not reset priority, got %d", got.Priority)
	}
}

// Databases created before this column exist: opening them must add it, keep
// every row, and treat old rows as normal priority. Opening twice is harmless.
func TestNew_AddsPriorityColumnToAnExistingDatabase(t *testing.T) {
	f, err := os.CreateTemp("", "memory-migrate-*.db")
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

	// The schema as it was before priority existed, with one saved fact.
	if _, err := p.DB().Exec(`CREATE TABLE memories (
		id INTEGER PRIMARY KEY AUTOINCREMENT, workspace_id TEXT NOT NULL, memory_type TEXT NOT NULL DEFAULT 'long_term',
		title TEXT NOT NULL DEFAULT '', content TEXT NOT NULL, tags TEXT NOT NULL DEFAULT '[]', source TEXT NOT NULL DEFAULT 'agent',
		created_at DATETIME NOT NULL DEFAULT (datetime('now')), updated_at DATETIME NOT NULL DEFAULT (datetime('now')))`); err != nil {
		t.Fatal(err)
	}
	if _, err := p.DB().Exec(`INSERT INTO memories (workspace_id, title, content, tags) VALUES ('ws-1', 'old', 'kept', '["hot"]')`); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		s, err := New(p)
		if err != nil {
			t.Fatalf("New on an old database: %v", err)
		}
		hot, err := s.SearchHot(context.Background(), "ws-1")
		if err != nil || len(hot) != 1 || hot[0].Content != "kept" || hot[0].Priority != PriorityNormal {
			t.Fatalf("after migration: %+v, %v", hot, err)
		}
	}
}
