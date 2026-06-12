package tools

import (
	"context"
	"os"
	"strings"
	"testing"

	"llm-proxy/internal/platform/db"
	"llm-proxy/internal/platform/memory"
	"llm-proxy/models"
)

// mockStore wraps memory.Store, using real backing for Insert so we can verify writes.
type mockMemoryStore struct {
	*memory.Store
	searchFn func(ctx context.Context, workspaceID, query string, limit int, opts ...memory.SearchOption) ([]memory.MemoryEntry, error)
}

func (m *mockMemoryStore) Search(ctx context.Context, workspaceID, query string, limit int, opts ...memory.SearchOption) ([]memory.MemoryEntry, error) {
	if m.searchFn != nil {
		return m.searchFn(ctx, workspaceID, query, limit, opts...)
	}
	return m.Store.Search(ctx, workspaceID, query, limit, opts...)
}

func TestResolveParams_AllCombos(t *testing.T) {
	cases := []struct {
		scope        memory.Scope
		mode         memory.Mode
		keep         memory.Keep
		wantWS       string
		wantMemType  string
		wantTags     []string
	}{
		{memory.ScopeUser, memory.ModeAlways, memory.KeepPermanent, "global", "user_profile", []string{"hot"}},
		{memory.ScopeUser, memory.ModeOnDemand, memory.KeepPermanent, "global", "user_profile", nil},
		{memory.ScopeUser, memory.ModeOnDemand, memory.KeepSession, "global", "user_profile", nil},
		{memory.ScopeWorkspace, memory.ModeAlways, memory.KeepPermanent, "my-ws", "long_term", []string{"hot"}},
		{memory.ScopeWorkspace, memory.ModeOnDemand, memory.KeepPermanent, "my-ws", "long_term", nil},
		{memory.ScopeWorkspace, memory.ModeOnDemand, memory.KeepSession, "my-ws", "session", nil},
	}
	for _, c := range cases {
		route, err := resolveParams(c.scope, c.mode, c.keep, "my-ws")
		if err != nil {
			t.Errorf("resolveParams(%s, %s, %s) unexpected error: %v", c.scope, c.mode, c.keep, err)
			continue
		}
		if route.WorkspaceID != c.wantWS {
			t.Errorf("resolveParams(%s, %s, %s) WorkspaceID = %q, want %q", c.scope, c.mode, c.keep, route.WorkspaceID, c.wantWS)
		}
		if route.MemoryType != c.wantMemType {
			t.Errorf("resolveParams(%s, %s, %s) MemoryType = %q, want %q", c.scope, c.mode, c.keep, route.MemoryType, c.wantMemType)
		}
		if len(route.Tags) != len(c.wantTags) {
			t.Errorf("resolveParams(%s, %s, %s) tags = %v, want %v", c.scope, c.mode, c.keep, route.Tags, c.wantTags)
		} else {
			for i := range route.Tags {
				if route.Tags[i] != c.wantTags[i] {
					t.Errorf("resolveParams(%s, %s, %s) tags[%d] = %q, want %q", c.scope, c.mode, c.keep, i, route.Tags[i], c.wantTags[i])
				}
			}
		}
	}
}

func TestResolveParams_InvalidCombination(t *testing.T) {
	_, err := resolveParams("garbage", "garbage", "garbage", "ws-1")
	if err == nil {
		t.Fatal("expected error for invalid combination")
	}
}

func TestMemorySearchTool(t *testing.T) {
	store := &mockMemoryStore{
		searchFn: func(ctx context.Context, workspaceID, query string, limit int, opts ...memory.SearchOption) ([]memory.MemoryEntry, error) {
			return []memory.MemoryEntry{
				{Title: "port", Content: "test DB port is 5433", MemoryType: memory.LongTerm},
			}, nil
		},
	}
	provider := NewMemoryToolProvider(store.Store)
	args := struct {
		Query interface{}  `json:"query"`
		Limit int          `json:"limit"`
		Scope memory.Scope `json:"scope"`
		Tags  []string     `json:"tags"`
	}{
		Query: "port",
		Limit: 5,
	}
	result, err := provider.Search(context.Background(), args)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	resultStr, ok := result.(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result)
	}
	if resultStr == "" {
		t.Error("expected non-empty result string")
	}
}

func TestMemorySearchTool_EmptyQuery(t *testing.T) {
	store := &mockMemoryStore{}
	provider := NewMemoryToolProvider(store.Store)
	args := struct {
		Query interface{}  `json:"query"`
		Limit int          `json:"limit"`
		Scope memory.Scope `json:"scope"`
		Tags  []string     `json:"tags"`
	}{
		Query: "",
		Limit: 5,
	}
	result, err := provider.Search(context.Background(), args)
	if err != nil {
		t.Fatalf("Search should not error on empty query, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestMemorySearchTool_EmptyQueryReturnsEntries(t *testing.T) {
	store := newRealTestStore(t)
	provider := NewMemoryToolProvider(store)
	ctx := models.WithWorkspaceID(context.Background(), "ws-1")

	// Save a few entries.
	for _, content := range []string{"first entry", "second entry", "third entry"} {
		_, err := provider.Update(ctx, struct {
			Content string        `json:"content"`
			Scope   memory.Scope  `json:"scope"`
			Mode    memory.Mode   `json:"mode"`
			Keep    memory.Keep   `json:"keep"`
			OldText string        `json:"old_text"`
		}{Content: content, Scope: "workspace", Mode: "on_demand", Keep: "permanent"})
		if err != nil {
			t.Fatalf("save %q failed: %v", content, err)
		}
	}

	// Empty query with workspace scope should return the 3 saved entries.
	args := struct {
		Query interface{}  `json:"query"`
		Limit int          `json:"limit"`
		Scope memory.Scope `json:"scope"`
		Tags  []string     `json:"tags"`
	}{Query: "", Limit: 10, Scope: "workspace"}

	result, err := provider.Search(ctx, args)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	resultStr, ok := result.(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result)
	}
	if !strings.Contains(resultStr, "Entry 1/3") {
		t.Errorf("expected 'Entry 1/3' in result, got: %s", resultStr)
	}
	if strings.Contains(resultStr, "no memories found matching") {
		t.Errorf("empty query returned no results when entries exist: %s", resultStr)
	}
}

func TestMemorySearchTool_ListAllMemories(t *testing.T) {
	store := newRealTestStore(t)
	provider := NewMemoryToolProvider(store)
	ctx := models.WithWorkspaceID(context.Background(), "ws-1")

	// Save workspace and user entries.
	provider.Update(ctx, struct {
		Content string        `json:"content"`
		Scope   memory.Scope  `json:"scope"`
		Mode    memory.Mode   `json:"mode"`
		Keep    memory.Keep   `json:"keep"`
		OldText string        `json:"old_text"`
	}{Content: "workspace fact", Scope: "workspace", Mode: "on_demand", Keep: "permanent"})
	provider.Update(ctx, struct {
		Content string        `json:"content"`
		Scope   memory.Scope  `json:"scope"`
		Mode    memory.Mode   `json:"mode"`
		Keep    memory.Keep   `json:"keep"`
		OldText string        `json:"old_text"`
	}{Content: "user fact", Scope: "user", Mode: "on_demand", Keep: "permanent"})

	t.Run("workspace scope returns workspace entry", func(t *testing.T) {
		entries, err := provider.listAllMemories(ctx, "ws-1", "workspace", 10)
		if err != nil {
			t.Fatalf("listAllMemories failed: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("expected 1 workspace entry, got %d", len(entries))
		}
		if !strings.Contains(entries[0].Content, "workspace fact") {
			t.Errorf("expected workspace fact, got: %s", entries[0].Content)
		}
	})

	t.Run("user scope returns user entry", func(t *testing.T) {
		entries, err := provider.listAllMemories(ctx, "ws-1", "user", 10)
		if err != nil {
			t.Fatalf("listAllMemories failed: %v", err)
		}
		if len(entries) != 1 {
			t.Fatalf("expected 1 user entry, got %d", len(entries))
		}
		if !strings.Contains(entries[0].Content, "user fact") {
			t.Errorf("expected user fact, got: %s", entries[0].Content)
		}
	})

	t.Run("no scope returns both entries merged", func(t *testing.T) {
		entries, err := provider.listAllMemories(ctx, "ws-1", "", 10)
		if err != nil {
			t.Fatalf("listAllMemories failed: %v", err)
		}
		if len(entries) != 2 {
			t.Fatalf("expected 2 entries (workspace + user), got %d", len(entries))
		}
	})
}

func TestMergeAndCap(t *testing.T) {
	a := []memory.MemoryEntry{{ID: 1, Content: "a1"}, {ID: 2, Content: "a2"}}
	b := []memory.MemoryEntry{{ID: 2, Content: "b2 (dup)"}, {ID: 3, Content: "b3"}}

	result := mergeAndCap(a, b, 10)
	if len(result) != 3 {
		t.Fatalf("expected 3 merged entries, got %d", len(result))
	}
	// ID 2 from b should be skipped (duplicate).
	for _, e := range result {
		if e.Content == "b2 (dup)" {
			t.Error("duplicate entry with ID 2 should not be in result")
		}
	}

	resultCapped := mergeAndCap(a, b, 2)
	if len(resultCapped) != 2 {
		t.Fatalf("expected 2 capped entries, got %d", len(resultCapped))
	}
}

func TestMemoryUpdateTool(t *testing.T) {
	store := &mockMemoryStore{}
	provider := NewMemoryToolProvider(store.Store)
	args := struct {
		Content string        `json:"content"`
		Scope   memory.Scope  `json:"scope"`
		Mode    memory.Mode   `json:"mode"`
		Keep    memory.Keep   `json:"keep"`
		OldText string        `json:"old_text"`
	}{
		Content: "test content",
		Scope:   "workspace",
		Mode:    "on_demand",
		Keep:    "permanent",
	}

	ctx := models.WithWorkspaceID(context.Background(), "ws-1")
	result, err := provider.Update(ctx, args)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestMemoryUpdateTool_MissingRequired(t *testing.T) {
	store := &mockMemoryStore{}
	provider := NewMemoryToolProvider(store.Store)
	args := struct {
		Content string        `json:"content"`
		Scope   memory.Scope  `json:"scope"`
		Mode    memory.Mode   `json:"mode"`
		Keep    memory.Keep   `json:"keep"`
		OldText string        `json:"old_text"`
	}{
		Content: "",
	}
	ctx := models.WithWorkspaceID(context.Background(), "ws-1")
	result, err := provider.Update(ctx, args)
	if err != nil {
		t.Fatalf("Update should not error on empty args, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	resultStr, ok := result.(string)
	if !ok {
		t.Fatalf("expected string, got %T", result)
	}
	if resultStr == "" {
		t.Error("expected non-empty result message")
	}
}

func newRealTestStore(t *testing.T) *memory.Store {
	t.Helper()
	f, err := os.CreateTemp("", "memory-tools-test-*.db")
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

func TestMemoryUpdateTool_Duplicate(t *testing.T) {
	store := newRealTestStore(t)
	provider := NewMemoryToolProvider(store)
	ctx := models.WithWorkspaceID(context.Background(), "ws-1")

	args := struct {
		Content string        `json:"content"`
		Scope   memory.Scope  `json:"scope"`
		Mode    memory.Mode   `json:"mode"`
		Keep    memory.Keep   `json:"keep"`
		OldText string        `json:"old_text"`
	}{
		Content: "exact duplicate content",
		Scope: memory.ScopeWorkspace, Mode: memory.ModeOnDemand, Keep: memory.KeepPermanent,
	}

	result, err := provider.Update(ctx, args)
	if err != nil {
		t.Fatalf("first Update failed: %v", err)
	}
	resultStr, ok := result.(string)
	if !ok {
		t.Fatalf("expected string, got %T", result)
	}
	if !strings.Contains(resultStr, "saved to memory") {
		t.Errorf("expected 'saved to memory', got: %s", resultStr)
	}

	result, err = provider.Update(ctx, args)
	if err != nil {
		t.Fatalf("second Update should not error: %v", err)
	}
	resultStr, ok = result.(string)
	if !ok {
		t.Fatalf("expected string, got %T", result)
	}
	// Jaccard dedup + exact content match catches duplicates.
	if !strings.Contains(resultStr, "already saved") {
		t.Errorf("expected 'already saved', got: %s", resultStr)
	}

	entries, err := store.List(ctx, "ws-1", "", 10, 0)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 entry after duplicate insert, got %d", len(entries))
	}
}

func TestMemoryUpdateTool_UpdateByOldText(t *testing.T) {
	store := newRealTestStore(t)
	provider := NewMemoryToolProvider(store)
	ctx := models.WithWorkspaceID(context.Background(), "ws-1")

	createArgs := struct {
		Content string        `json:"content"`
		Scope   memory.Scope  `json:"scope"`
		Mode    memory.Mode   `json:"mode"`
		Keep    memory.Keep   `json:"keep"`
		OldText string        `json:"old_text"`
	}{
		Content: "database port is 5433",
		Scope: memory.ScopeWorkspace, Mode: memory.ModeOnDemand, Keep: memory.KeepPermanent,
	}

	result, err := provider.Update(ctx, createArgs)
	if err != nil {
		t.Fatalf("create Update failed: %v", err)
	}
	if !strings.Contains(result.(string), "saved to memory") {
		t.Errorf("expected 'saved to memory', got: %s", result)
	}

	updateArgs := struct {
		Content string        `json:"content"`
		Scope   memory.Scope  `json:"scope"`
		Mode    memory.Mode   `json:"mode"`
		Keep    memory.Keep   `json:"keep"`
		OldText string        `json:"old_text"`
	}{
		Content: "database port is 5433 (updated)",
		Scope: memory.ScopeWorkspace, Mode: memory.ModeOnDemand, Keep: memory.KeepPermanent,
		OldText: "5433",
	}

	result, err = provider.Update(ctx, updateArgs)
	if err != nil {
		t.Fatalf("update by old_text failed: %v", err)
	}
	if !strings.Contains(result.(string), "updated memory entry") {
		t.Errorf("expected 'updated memory entry', got: %s", result)
	}

	entries, err := store.List(ctx, "ws-1", "", 10, 0)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 entry after update, got %d", len(entries))
	}
	if !strings.Contains(entries[0].Content, "updated") {
		t.Errorf("expected updated content, got: %s", entries[0].Content)
	}
}

func TestMemoryUpdateTool_UpdateByOldText_NotFound(t *testing.T) {
	store := newRealTestStore(t)
	provider := NewMemoryToolProvider(store)

	createArgs := struct {
		Content string        `json:"content"`
		Scope   memory.Scope  `json:"scope"`
		Mode    memory.Mode   `json:"mode"`
		Keep    memory.Keep   `json:"keep"`
		OldText string        `json:"old_text"`
	}{
		Content: "database port is 5433",
		Scope: memory.ScopeWorkspace, Mode: memory.ModeOnDemand, Keep: memory.KeepPermanent,
		OldText: "nonexistent text",
	}

	ctx := models.WithWorkspaceID(context.Background(), "ws-1")
	result, err := provider.Update(ctx, createArgs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.(string), "saved to memory") {
		t.Fatalf("expected new entry to be created, got: %s", result)
	}
}

func TestMemoryUpdateTool_SeparateSaves(t *testing.T) {
	store := newRealTestStore(t)
	provider := NewMemoryToolProvider(store)
	ctx := models.WithWorkspaceID(context.Background(), "ws-1")

	args := struct {
		Content string        `json:"content"`
		Scope   memory.Scope  `json:"scope"`
		Mode    memory.Mode   `json:"mode"`
		Keep    memory.Keep   `json:"keep"`
		OldText string        `json:"old_text"`
	}{
		Content: "Step 1 done",
		Scope: memory.ScopeWorkspace, Mode: memory.ModeOnDemand, Keep: memory.KeepPermanent,
	}

	result, err := provider.Update(ctx, args)
	if err != nil {
		t.Fatalf("first Update failed: %v", err)
	}
	if !strings.Contains(result.(string), "saved to memory") {
		t.Errorf("expected 'saved to memory', got: %s", result)
	}

	args.Content = "Steps 1-3 done"
	result, err = provider.Update(ctx, args)
	if err != nil {
		t.Fatalf("second Update (different content) failed: %v", err)
	}
	if !strings.Contains(result.(string), "saved to memory") {
		t.Errorf("expected 'saved to memory' (separate entry), got: %s", result)
	}

	entries, err := store.List(ctx, "ws-1", "", 10, 0)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 separate entries, got %d", len(entries))
	}
}

func TestMemoryUpdateTool_OldTextBackwardCompat(t *testing.T) {
	store := newRealTestStore(t)
	provider := NewMemoryToolProvider(store)
	ctx := models.WithWorkspaceID(context.Background(), "ws-1")

	createArgs := struct {
		Content string        `json:"content"`
		Scope   memory.Scope  `json:"scope"`
		Mode    memory.Mode   `json:"mode"`
		Keep    memory.Keep   `json:"keep"`
		OldText string        `json:"old_text"`
	}{
		Content: "this is entry one",
		Scope: memory.ScopeWorkspace, Mode: memory.ModeOnDemand, Keep: memory.KeepPermanent,
	}

	result, err := provider.Update(ctx, createArgs)
	if err != nil {
		t.Fatalf("first Update failed: %v", err)
	}
	if !strings.Contains(result.(string), "saved to memory") {
		t.Errorf("expected 'saved to memory', got: %s", result)
	}

	createArgs.Content = "this is entry two"

	result, err = provider.Update(ctx, createArgs)
	if err != nil {
		t.Fatalf("second Update failed: %v", err)
	}
	if !strings.Contains(result.(string), "saved to memory") {
		t.Errorf("expected 'saved to memory', got: %s", result)
	}

	entries, err := store.List(ctx, "ws-1", "", 10, 0)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries (separate content = separate entries), got %d", len(entries))
	}
}

func TestMemoryUpdateTool_SemanticDedup(t *testing.T) {
	store := newRealTestStore(t)
	provider := NewMemoryToolProvider(store)
	ctx := models.WithWorkspaceID(context.Background(), "ws-1")

	args := struct {
		Content string        `json:"content"`
		Scope   memory.Scope  `json:"scope"`
		Mode    memory.Mode   `json:"mode"`
		Keep    memory.Keep   `json:"keep"`
		OldText string        `json:"old_text"`
	}{
		Content: "Smoke test executed successfully",
		Scope: memory.ScopeWorkspace, Mode: memory.ModeOnDemand, Keep: memory.KeepPermanent,
	}

	result, err := provider.Update(ctx, args)
	if err != nil {
		t.Fatalf("first Update failed: %v", err)
	}
	if !strings.Contains(result.(string), "saved to memory") {
		t.Errorf("expected 'saved to memory', got: %s", result)
	}

	args.Content = "LLM smoke test completed — all outputs captured and verified"

	result, err = provider.Update(ctx, args)
	if err != nil {
		t.Fatalf("second Update failed: %v", err)
	}
	// Different content with only 2 shared words ("smoke", "test") — below 0.90 Jaccard threshold → separate entries.
	if !strings.Contains(result.(string), "saved to memory") {
		t.Errorf("expected 'saved to memory' (separate entry), got: %s", result)
	}

	entries, err := store.List(ctx, "ws-1", "", 10, 0)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (distinct content, low Jaccard overlap), got %d", len(entries))
	}
}

func TestMemoryUpdateTool_SemanticDedup_NoFalsePositive(t *testing.T) {
	store := newRealTestStore(t)
	provider := NewMemoryToolProvider(store)
	ctx := models.WithWorkspaceID(context.Background(), "ws-1")

	args := struct {
		Content string        `json:"content"`
		Scope   memory.Scope  `json:"scope"`
		Mode    memory.Mode   `json:"mode"`
		Keep    memory.Keep   `json:"keep"`
		OldText string        `json:"old_text"`
	}{
		Content: "this is entry one",
		Scope: memory.ScopeWorkspace, Mode: memory.ModeOnDemand, Keep: memory.KeepPermanent,
	}

	result, err := provider.Update(ctx, args)
	if err != nil {
		t.Fatalf("first Update failed: %v", err)
	}
	if !strings.Contains(result.(string), "saved to memory") {
		t.Errorf("expected 'saved to memory', got: %s", result)
	}

	args.Content = "this is entry two"

	result, err = provider.Update(ctx, args)
	if err != nil {
		t.Fatalf("second Update failed: %v", err)
	}
	// Content "this is entry one" and "this is entry two" share "this", "is", "entry"
	// but the Jaccard threshold is 0.90 for content.  With 3 shared / 4 unique ≈ 0.75, no match → separate entries.
	if !strings.Contains(result.(string), "saved to memory") {
		t.Errorf("expected 'saved to memory' (separate entry), got: %s", result)
	}

	entries, err := store.List(ctx, "ws-1", "", 10, 0)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries (no semantic overlap), got %d", len(entries))
	}
}

func TestMemoryUpdateTool_JaccardDedup_ContentIdentical(t *testing.T) {
	store := newRealTestStore(t)
	provider := NewMemoryToolProvider(store)
	ctx := models.WithWorkspaceID(context.Background(), "ws-1")

	args := struct {
		Content string        `json:"content"`
		Scope   memory.Scope  `json:"scope"`
		Mode    memory.Mode   `json:"mode"`
		Keep    memory.Keep   `json:"keep"`
		OldText string        `json:"old_text"`
	}{
		Content: "TypeScript 6.0.3 is installed",
		Scope: memory.ScopeWorkspace, Mode: memory.ModeOnDemand, Keep: memory.KeepPermanent,
	}

	result, err := provider.Update(ctx, args)
	if err != nil {
		t.Fatalf("first Update failed: %v", err)
	}
	if !strings.Contains(result.(string), "saved to memory") {
		t.Errorf("expected 'saved to memory', got: %s", result)
	}

	// Same content — exact content match via Exists should catch it.
	result, err = provider.Update(ctx, args)
	if err != nil {
		t.Fatalf("second Update failed: %v", err)
	}
	if !strings.Contains(result.(string), "already saved") {
		t.Errorf("expected 'already saved' (duplicate content), got: %s", result)
	}

	entries, err := store.List(ctx, "ws-1", "", 10, 0)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry (duplicate content deduped), got %d", len(entries))
	}
}
