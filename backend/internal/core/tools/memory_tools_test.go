package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

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
		scope       memory.Scope
		mode        memory.Mode
		keep        memory.Keep
		wantWS      string
		wantMemType string
		wantTags    []string
	}{
		{memory.ScopeUser, memory.ModeAlways, memory.KeepPermanent, "global", "user_profile", []string{"hot"}},
		{memory.ScopeUser, memory.ModeOnDemand, memory.KeepPermanent, "global", "user_profile", nil},
		{memory.ScopeUser, memory.ModeOnDemand, memory.KeepSession, "global", "user_profile", nil},
		{memory.ScopeWorkspace, memory.ModeAlways, memory.KeepPermanent, "my-ws", "long_term", []string{"hot"}},
		{memory.ScopeWorkspace, memory.ModeOnDemand, memory.KeepPermanent, "my-ws", "long_term", nil},
		{memory.ScopeWorkspace, memory.ModeOnDemand, memory.KeepSession, "my-ws", "session", nil},
	}
	for _, c := range cases {
		route, err := ResolveMemoryRoute(c.scope, c.mode, c.keep, "my-ws")
		if err != nil {
			t.Errorf("ResolveMemoryRoute(%s, %s, %s) unexpected error: %v", c.scope, c.mode, c.keep, err)
			continue
		}
		if route.WorkspaceID != c.wantWS {
			t.Errorf("ResolveMemoryRoute(%s, %s, %s) WorkspaceID = %q, want %q", c.scope, c.mode, c.keep, route.WorkspaceID, c.wantWS)
		}
		if route.MemoryType != c.wantMemType {
			t.Errorf("ResolveMemoryRoute(%s, %s, %s) MemoryType = %q, want %q", c.scope, c.mode, c.keep, route.MemoryType, c.wantMemType)
		}
		if len(route.Tags) != len(c.wantTags) {
			t.Errorf("ResolveMemoryRoute(%s, %s, %s) tags = %v, want %v", c.scope, c.mode, c.keep, route.Tags, c.wantTags)
		} else {
			for i := range route.Tags {
				if route.Tags[i] != c.wantTags[i] {
					t.Errorf("ResolveMemoryRoute(%s, %s, %s) tags[%d] = %q, want %q", c.scope, c.mode, c.keep, i, route.Tags[i], c.wantTags[i])
				}
			}
		}
	}
}

func TestResolveParams_InvalidCombination(t *testing.T) {
	_, err := ResolveMemoryRoute("garbage", "garbage", "garbage", "ws-1")
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
		_, err := provider.Update(ctx, MemoryUpdateArgs{Content: content, Scope: "workspace", Mode: "on_demand", Keep: "permanent"})
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
	provider.Update(ctx, MemoryUpdateArgs{Content: "workspace fact", Scope: "workspace", Mode: "on_demand", Keep: "permanent"})
	provider.Update(ctx, MemoryUpdateArgs{Content: "user fact", Scope: "user", Mode: "on_demand", Keep: "permanent"})

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
	args := MemoryUpdateArgs{
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
	args := MemoryUpdateArgs{
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

	args := MemoryUpdateArgs{
		Content: "exact duplicate content",
		Scope:   memory.ScopeWorkspace, Mode: memory.ModeOnDemand, Keep: memory.KeepPermanent,
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

	createArgs := MemoryUpdateArgs{
		Content: "database port is 5433",
		Scope:   memory.ScopeWorkspace, Mode: memory.ModeOnDemand, Keep: memory.KeepPermanent,
	}

	result, err := provider.Update(ctx, createArgs)
	if err != nil {
		t.Fatalf("create Update failed: %v", err)
	}
	if !strings.Contains(result.(string), "saved to memory") {
		t.Errorf("expected 'saved to memory', got: %s", result)
	}

	updateArgs := MemoryUpdateArgs{
		Content: "database port is 5433 (updated)",
		Scope:   memory.ScopeWorkspace, Mode: memory.ModeOnDemand, Keep: memory.KeepPermanent,
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

	createArgs := MemoryUpdateArgs{
		Content: "database port is 5433",
		Scope:   memory.ScopeWorkspace, Mode: memory.ModeOnDemand, Keep: memory.KeepPermanent,
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

	args := MemoryUpdateArgs{
		Content: "Step 1 done",
		Scope:   memory.ScopeWorkspace, Mode: memory.ModeOnDemand, Keep: memory.KeepPermanent,
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

	createArgs := MemoryUpdateArgs{
		Content: "this is entry one",
		Scope:   memory.ScopeWorkspace, Mode: memory.ModeOnDemand, Keep: memory.KeepPermanent,
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

	args := MemoryUpdateArgs{
		Content: "Smoke test executed successfully",
		Scope:   memory.ScopeWorkspace, Mode: memory.ModeOnDemand, Keep: memory.KeepPermanent,
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

	args := MemoryUpdateArgs{
		Content: "this is entry one",
		Scope:   memory.ScopeWorkspace, Mode: memory.ModeOnDemand, Keep: memory.KeepPermanent,
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

	args := MemoryUpdateArgs{
		Content: "TypeScript 6.0.3 is installed",
		Scope:   memory.ScopeWorkspace, Mode: memory.ModeOnDemand, Keep: memory.KeepPermanent,
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

type updateArgs = MemoryUpdateArgs

func firstEntry(t *testing.T, store *memory.Store, memType memory.MemoryType) memory.MemoryEntry {
	t.Helper()
	entries, err := store.List(context.Background(), "ws-1", memType, 10, 0)
	if err != nil || len(entries) != 1 {
		t.Fatalf("List(%s) = %d entries, err %v; want exactly 1", memType, len(entries), err)
	}
	return entries[0]
}

// Unattended runs fill the store with nobody watching. A bare memory_update
// (no explicit routing) must therefore land as a session entry tagged with the
// run id, so the retention reaper can clear it and the operator can trace it.
func TestMemoryUpdate_UnattendedRunDefaultsToSessionWithRunSource(t *testing.T) {
	store := newRealTestStore(t)
	provider := NewMemoryToolProvider(store)
	ctx := models.WithUnattendedRun(models.WithRunID(models.WithWorkspaceID(context.Background(), "ws-1"), "20260901T174541Z_abc"))

	if _, err := provider.Update(ctx, updateArgs{Content: "TypeScript version is 6.0.3"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got := firstEntry(t, store, memory.Session)
	if got.Source != "run:20260901T174541Z_abc" {
		t.Errorf("source = %q, want run:<run id>", got.Source)
	}
}

func TestMemoryUpdate_AttendedDefaultStaysPermanentFromAgent(t *testing.T) {
	store := newRealTestStore(t)
	provider := NewMemoryToolProvider(store)
	ctx := models.WithWorkspaceID(context.Background(), "ws-1")

	if _, err := provider.Update(ctx, updateArgs{Content: "user likes concise answers"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got := firstEntry(t, store, memory.LongTerm)
	if got.Source != "agent" {
		t.Errorf("source = %q, want agent", got.Source)
	}
}

// A model that explicitly asks for permanent memory is making a deliberate
// choice; only the unspecified default changes for unattended runs.
func TestMemoryUpdate_UnattendedExplicitPermanentIsHonoured(t *testing.T) {
	store := newRealTestStore(t)
	provider := NewMemoryToolProvider(store)
	ctx := models.WithUnattendedRun(models.WithRunID(models.WithWorkspaceID(context.Background(), "ws-1"), "r1"))

	args := updateArgs{Content: "deploy target is staging", Scope: memory.ScopeWorkspace, Mode: memory.ModeOnDemand, Keep: memory.KeepPermanent}
	if _, err := provider.Update(ctx, args); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got := firstEntry(t, store, memory.LongTerm); got.Source != "run:r1" {
		t.Errorf("source = %q, want run:r1 (still traceable to the run)", got.Source)
	}
}

// Each routing field defaults on its own: a model that states scope or mode but
// leaves keep out must not have what it stated thrown away.
func TestMemoryUpdate_OmittedFieldsDefaultIndependently(t *testing.T) {
	attended := models.WithWorkspaceID(context.Background(), "ws-1")
	unattended := models.WithUnattendedRun(models.WithRunID(attended, "r1"))

	tests := []struct {
		name        string
		ctx         context.Context
		args        updateArgs
		wantWS      string
		wantType    memory.MemoryType
		wantHot     bool
		wantErrPart string
	}{
		{name: "user scope kept without keep", ctx: attended, args: updateArgs{Content: "I prefer short answers", Scope: memory.ScopeUser}, wantWS: "global", wantType: memory.UserProfile},
		{name: "user hot kept without keep", ctx: attended, args: updateArgs{Content: "I prefer short answers", Scope: memory.ScopeUser, Mode: memory.ModeAlways}, wantWS: "global", wantType: memory.UserProfile, wantHot: true},
		{name: "mode alone keeps workspace scope", ctx: attended, args: updateArgs{Content: "deploys go through staging", Mode: memory.ModeAlways}, wantWS: "ws-1", wantType: memory.LongTerm, wantHot: true},
		{name: "keep alone keeps workspace on_demand", ctx: attended, args: updateArgs{Content: "scratch note for now", Keep: memory.KeepSession}, wantWS: "ws-1", wantType: memory.Session},
		{name: "unattended user scope defaults keep to session", ctx: unattended, args: updateArgs{Content: "I prefer short answers", Scope: memory.ScopeUser}, wantWS: "global", wantType: memory.UserProfile},
		{name: "unattended hot without keep is refused, not silently demoted", ctx: unattended, args: updateArgs{Content: "I prefer short answers", Scope: memory.ScopeUser, Mode: memory.ModeAlways}, wantErrPart: "invalid memory parameters"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newRealTestStore(t)
			provider := NewMemoryToolProvider(store)
			_, err := provider.Update(tt.ctx, tt.args)
			if tt.wantErrPart != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrPart) {
					t.Fatalf("err = %v, want it to contain %q", err, tt.wantErrPart)
				}
				return
			}
			if err != nil {
				t.Fatalf("Update: %v", err)
			}
			entries, err := store.List(context.Background(), tt.wantWS, tt.wantType, 10, 0)
			if err != nil || len(entries) != 1 {
				t.Fatalf("List(%s, %s) = %d entries, err %v; want exactly 1", tt.wantWS, tt.wantType, len(entries), err)
			}
			if got := slices.Contains(entries[0].Tags, "hot"); got != tt.wantHot {
				t.Errorf("hot tag = %v, want %v (tags %v)", got, tt.wantHot, entries[0].Tags)
			}
		})
	}
}

const (
	deployVertex = "We always deploy through the vertex host."
	deployNew    = "We always deploy through the new host."
)

// A fact whose topic overlaps a stored one used to be concatenated onto it, which kept a contradiction in one entry
// ("…vertex host.\n…new host."). The tool now reports the similar fact, so the model chooses: replace it (old_text) or
// keep both (separate).
func TestMemoryUpdateTool_OverlapReportsTheExistingFactInsteadOfMerging(t *testing.T) {
	store := newRealTestStore(t)
	provider := NewMemoryToolProvider(store)
	ctx := models.WithWorkspaceID(context.Background(), "ws-1")
	save := func(args MemoryUpdateArgs) string {
		got, err := provider.Update(ctx, args)
		if err != nil {
			t.Fatalf("Update: %v", err)
		}
		return got.(string)
	}
	route := MemoryUpdateArgs{Scope: memory.ScopeWorkspace, Mode: memory.ModeOnDemand, Keep: memory.KeepPermanent}

	first := route
	first.Content = deployVertex
	save(first)

	second := route
	second.Content = deployNew
	got := save(second)
	for _, want := range []string{"already exists", deployVertex, "old_text", "separate"} {
		if !strings.Contains(got, want) {
			t.Errorf("the conflict message lacks %q: %s", want, got)
		}
	}
	entries, _ := store.List(ctx, "ws-1", "", 10, 0)
	if len(entries) != 1 || entries[0].Content != deployVertex {
		t.Fatalf("the stored fact must be untouched and nothing added, got %+v", entries)
	}

	second.Separate = true
	if got := save(second); !strings.Contains(got, "saved to memory") {
		t.Errorf("separate: true must save it, got: %s", got)
	}
	if entries, _ = store.List(ctx, "ws-1", "", 10, 0); len(entries) != 2 {
		t.Errorf("both facts should be stored separately, got %d entries", len(entries))
	}
}

// Explicit capture and the per-chat review cannot retry with old_text, so they replace the similar fact in place:
// "remember that we now deploy through X" must neither be dropped nor concatenated onto the old fact.
func TestSaveFact_OverlapReplacesTheSimilarFact(t *testing.T) {
	store := newRealTestStore(t)
	provider := NewMemoryToolProvider(store)
	ctx := models.WithWorkspaceID(context.Background(), "ws-1")
	fact := func(content string) Fact {
		return Fact{Content: content, Scope: memory.ScopeWorkspace, Mode: memory.ModeOnDemand, Keep: memory.KeepPermanent}
	}
	if _, err := provider.SaveFact(ctx, "ws-1", fact(deployVertex)); err != nil {
		t.Fatal(err)
	}
	res, err := provider.SaveFact(ctx, "ws-1", fact(deployNew))
	if err != nil || res.Outcome != SaveUpdated {
		t.Fatalf("SaveFact = %+v, %v; want the similar fact updated", res, err)
	}
	entries, _ := store.List(ctx, "ws-1", "", 10, 0)
	if len(entries) != 1 || entries[0].Content != deployNew {
		t.Errorf("the old fact should be replaced, not concatenated or duplicated: %+v", entries)
	}
}

// old_text used to look only in the scope the new fact goes to, so "that actually applies to all my projects" could not
// find the workspace fact it meant and left a second copy behind. It now finds the fact in either scope and moves it.
func TestMemoryUpdateTool_OldTextMovesAFactBetweenScopes(t *testing.T) {
	cases := []struct {
		name               string
		from, to           memory.Scope
		fromWS, toWS       string
		oldContent, newTxt string
	}{
		{"workspace to user", memory.ScopeWorkspace, memory.ScopeUser, "ws-1", "global", "We prefer tabs in Go files.", "I prefer tabs in Go files."},
		{"user to workspace", memory.ScopeUser, memory.ScopeWorkspace, "global", "ws-1", "I deploy through the vertex host.", "We deploy through the vertex host."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newRealTestStore(t)
			provider := NewMemoryToolProvider(store)
			ctx := models.WithWorkspaceID(context.Background(), "ws-1")
			if _, err := provider.Update(ctx, MemoryUpdateArgs{Content: tc.oldContent, Scope: tc.from, Mode: memory.ModeOnDemand, Keep: memory.KeepPermanent}); err != nil {
				t.Fatal(err)
			}
			got, err := provider.Update(ctx, MemoryUpdateArgs{Content: tc.newTxt, Scope: tc.to, Mode: memory.ModeOnDemand, Keep: memory.KeepPermanent, OldText: tc.oldContent[:12]})
			if err != nil || !strings.Contains(got.(string), "moved") {
				t.Fatalf("Update = %v, %v; want a result saying the fact moved", got, err)
			}
			if entries, _ := store.List(context.Background(), tc.fromWS, "", 10, 0); len(entries) != 0 {
				t.Errorf("the old copy was left in %s: %+v", tc.fromWS, entries)
			}
			entries, _ := store.List(context.Background(), tc.toWS, "", 10, 0)
			if len(entries) != 1 || entries[0].Content != tc.newTxt {
				t.Errorf("the fact should now be in %s with the new text, got %+v", tc.toWS, entries)
			}
		})
	}
}

// memory_forget deletes a wrong or unwanted fact, but only on an unambiguous match, only in the operator's own chat,
// and it echoes what it deleted so the model (and the transcript) can show it.
func TestMemoryForgetTool(t *testing.T) {
	seed := func(t *testing.T) (*memory.Store, *MemoryToolProvider) {
		store := newRealTestStore(t)
		provider := NewMemoryToolProvider(store)
		ctx := models.WithWorkspaceID(context.Background(), "ws-1")
		for _, a := range []MemoryUpdateArgs{
			{Content: "We deploy through the vertex host.", Scope: memory.ScopeWorkspace, Mode: memory.ModeOnDemand, Keep: memory.KeepPermanent},
			{Content: "The staging database runs on port 5433.", Scope: memory.ScopeWorkspace, Mode: memory.ModeOnDemand, Keep: memory.KeepPermanent},
			{Content: "I prefer tabs in Go files.", Scope: memory.ScopeUser, Mode: memory.ModeOnDemand, Keep: memory.KeepPermanent},
		} {
			if _, err := provider.Update(ctx, a); err != nil {
				t.Fatal(err)
			}
		}
		return store, provider
	}
	count := func(store *memory.Store) int {
		a, _ := store.List(context.Background(), "ws-1", "", 20, 0)
		b, _ := store.List(context.Background(), "global", "", 20, 0)
		return len(a) + len(b)
	}
	chat := models.WithWorkspaceID(context.Background(), "ws-1")

	tests := []struct {
		name      string
		ctx       context.Context
		text      string
		wantInMsg string
		wantLeft  int
	}{
		{"deletes the one match in the workspace", chat, "vertex host", "forgot", 2},
		{"deletes the one match in the user scope", chat, "tabs", "forgot", 2},
		{"says so when nothing matches", chat, "no such thing", "no saved fact", 3},
		{"asks for more when several match", chat, "the", "more than one", 3},
		{"needs real text", chat, "  ", "required", 3},
		{"refused in an automation", models.WithUnattendedRun(chat), "vertex host", "operator", 3},
		{"refused for a connector chat", models.WithOriginConnector(chat, "telegram"), "vertex host", "operator", 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store, provider := seed(t)
			got, err := provider.Forget(tc.ctx, MemoryForgetArgs{Text: tc.text})
			if err != nil {
				t.Fatalf("Forget: %v", err)
			}
			if msg, _ := got.(string); !strings.Contains(strings.ToLower(msg), tc.wantInMsg) {
				t.Errorf("result %q lacks %q", got, tc.wantInMsg)
			}
			if left := count(store); left != tc.wantLeft {
				t.Errorf("facts left = %d, want %d", left, tc.wantLeft)
			}
		})
	}
	t.Run("echoes the deleted text", func(t *testing.T) {
		_, provider := seed(t)
		got, _ := provider.Forget(chat, MemoryForgetArgs{Text: "port 5433"})
		if msg, _ := got.(string); !strings.Contains(msg, "The staging database runs on port 5433.") {
			t.Errorf("the result must echo the deleted fact: %v", got)
		}
	})
}

// Keyword search finds a fact by the words it was saved with. When a question is phrased differently nothing matches,
// and a model cannot invent the right synonym, but it can recognise a fact by meaning when shown the candidates: a miss
// therefore lists the latest saved facts (bounded) and says to check them by meaning. No word list is involved.
func TestMemorySearchTool_AMissShowsTheLatestFactsToCheckByMeaning(t *testing.T) {
	ctx := models.WithWorkspaceID(context.Background(), "ws-1")
	seed := func(t *testing.T, contents ...string) *MemoryToolProvider {
		provider := NewMemoryToolProvider(newRealTestStore(t))
		for i, c := range contents {
			scope := memory.ScopeWorkspace
			if i%2 == 1 {
				scope = memory.ScopeUser
			}
			if _, err := provider.Update(ctx, MemoryUpdateArgs{Content: c, Scope: scope, Mode: memory.ModeOnDemand, Keep: memory.KeepPermanent}); err != nil {
				t.Fatal(err)
			}
		}
		return provider
	}
	search := func(provider *MemoryToolProvider) string {
		got, err := provider.Search(ctx, searchArgs("zzqxj"))
		if err != nil {
			t.Fatal(err)
		}
		return got.(string)
	}

	t.Run("lists the saved facts of both scopes and says to check by meaning", func(t *testing.T) {
		got := search(seed(t, "Alpha lanterns glow at dusk.", "Bravo orchards ripen in autumn."))
		for _, want := range []string{"by meaning", "Alpha lanterns glow at dusk.", "Bravo orchards ripen in autumn."} {
			if !strings.Contains(got, want) {
				t.Errorf("the miss result lacks %q:\n%s", want, got)
			}
		}
	})
	t.Run("is bounded", func(t *testing.T) {
		var facts []string
		for i := 0; i < searchMissFactLimit+4; i++ {
			facts = append(facts, fmt.Sprintf("Distinct%c%c subject number %d stays unique.", 'a'+i, 'k'+i, i))
		}
		got := search(seed(t, facts...))
		if lines := strings.Count(got, "\n- "); lines != searchMissFactLimit {
			t.Errorf("listed %d facts, want exactly %d:\n%s", lines, searchMissFactLimit, got)
		}
	})
	t.Run("an outside sender in a connector chat is never shown the owner's facts", func(t *testing.T) {
		provider := seed(t, "Alpha lanterns glow at dusk.")
		got, err := provider.Search(models.WithOriginConnector(ctx, "telegram"), searchArgs("zzqxj"))
		if err != nil || got.(string) != "no memories found matching that query" {
			t.Errorf("a connector chat must get the plain miss, got %v, %v", got, err)
		}
	})
	t.Run("an empty memory just says nothing matched", func(t *testing.T) {
		if got := search(seed(t)); got != "no memories found matching that query" {
			t.Errorf("got %q", got)
		}
	})
}

// ── Typed facts and expiry ───────────────────────────────────────────────

// typedProvider is a provider with a fixed clock (Wednesday 14 October 2026, noon UTC) and a 48-hour session lifetime.
func typedProvider(t *testing.T) (*MemoryToolProvider, *memory.Store, context.Context) {
	t.Helper()
	store := newRealTestStore(t)
	p := NewMemoryToolProvider(store).WithSessionRetention(func() time.Duration { return 48 * time.Hour })
	p.now = func() time.Time { return time.Date(2026, time.October, 14, 12, 0, 0, 0, time.UTC) }
	ctx := models.WithChatTimezone(models.WithWorkspaceID(context.Background(), "ws-1"), "UTC")
	return p, store, ctx
}

func onlyFact(t *testing.T, store *memory.Store, ws string) memory.MemoryEntry {
	t.Helper()
	entries, _ := store.List(context.Background(), ws, "", 10, 0)
	if len(entries) != 1 {
		t.Fatalf("want exactly one fact in %s, got %d: %+v", ws, len(entries), entries)
	}
	return entries[0]
}

func TestMemoryUpdateTool_StoresKindWhyAndAnExpiryDate(t *testing.T) {
	p, store, ctx := typedProvider(t)
	got, err := p.Update(ctx, MemoryUpdateArgs{Content: "Answer review requests within a day.", Scope: memory.ScopeWorkspace, Mode: memory.ModeOnDemand, Keep: memory.KeepPermanent,
		Kind: memory.KindFeedback, Why: "the team wants faster turnaround", Expires: "2026-11-01"})
	if err != nil || !strings.Contains(got.(string), "saved") {
		t.Fatalf("Update = %v, %v", got, err)
	}
	e := onlyFact(t, store, "ws-1")
	// the fact lives through the whole of 1 November: it expires when 2 November starts
	if e.Kind != memory.KindFeedback || e.Why != "the team wants faster turnaround" || e.ExpiresAt != "2026-11-02T00:00:00Z" {
		t.Errorf("typing not stored: %+v", e)
	}
}

func TestMemoryUpdateTool_RefusesMalformedTypingWithoutSaving(t *testing.T) {
	for name, args := range map[string]MemoryUpdateArgs{
		"unknown kind":          {Kind: "opinion"},
		"multi-line why":        {Why: "one\ntwo"},
		"over-long why":         {Why: strings.Repeat("w", memory.MaxWhyChars+1)},
		"a secret in the why":   {Why: "the password is hunter2"},
		"an unreadable expiry":  {Expires: "whenever"},
		"an expiry in the past": {Expires: "2026-10-01"},
	} {
		t.Run(name, func(t *testing.T) {
			p, store, ctx := typedProvider(t)
			args.Content, args.Scope, args.Mode, args.Keep = "A fact worth keeping.", memory.ScopeWorkspace, memory.ModeOnDemand, memory.KeepPermanent
			got, err := p.Update(ctx, args)
			if err != nil || !strings.Contains(got.(string), "not saved") {
				t.Errorf("Update = %v, %v; want a refusal that says it was not saved", got, err)
			}
			if entries, _ := store.List(context.Background(), "ws-1", "", 10, 0); len(entries) != 0 {
				t.Errorf("nothing may be stored on a refusal: %+v", entries)
			}
		})
	}
}

// A saved fact is rewritten from "tomorrow" to the date it meant, so it is still right next month; a day such as
// "next Friday" works as an expiry too.
func TestMemoryUpdateTool_WritesAbsoluteDates(t *testing.T) {
	p, store, ctx := typedProvider(t)
	_, err := p.Update(ctx, MemoryUpdateArgs{Content: "The demo is tomorrow.", Scope: memory.ScopeWorkspace, Mode: memory.ModeOnDemand, Keep: memory.KeepPermanent, Expires: "next Friday"})
	if err != nil {
		t.Fatal(err)
	}
	e := onlyFact(t, store, "ws-1")
	if e.Content != "The demo is 2026-10-15 (Thursday)." {
		t.Errorf("the relative date must be written out: %q", e.Content)
	}
	if e.ExpiresAt != "2026-10-17T00:00:00Z" { // through Friday 16 October
		t.Errorf("expiry = %q, want the end of Friday 2026-10-16", e.ExpiresAt)
	}
}

// keep=session now means an expiry for both scopes, so the reaper reaches a user-scope session fact too.
func TestMemoryUpdateTool_SessionFactsExpireInEveryScope(t *testing.T) {
	for _, tc := range []struct {
		scope memory.Scope
		ws    string
	}{{memory.ScopeWorkspace, "ws-1"}, {memory.ScopeUser, "global"}} {
		t.Run(string(tc.scope), func(t *testing.T) {
			p, store, ctx := typedProvider(t)
			if _, err := p.Update(ctx, MemoryUpdateArgs{Content: "A note for now.", Scope: tc.scope, Mode: memory.ModeOnDemand, Keep: memory.KeepSession}); err != nil {
				t.Fatal(err)
			}
			if got := onlyFact(t, store, tc.ws).ExpiresAt; got != "2026-10-16T12:00:00Z" {
				t.Errorf("a session fact should expire after the 48h lifetime, got %q", got)
			}
		})
	}
	t.Run("a permanent fact has none", func(t *testing.T) {
		p, store, ctx := typedProvider(t)
		p.Update(ctx, MemoryUpdateArgs{Content: "Lasts.", Scope: memory.ScopeWorkspace, Mode: memory.ModeOnDemand, Keep: memory.KeepPermanent})
		if got := onlyFact(t, store, "ws-1").ExpiresAt; got != "" {
			t.Errorf("a permanent fact must not expire, got %q", got)
		}
	})
}

// Replacing a fact keeps its kind and reason unless the model gives new ones, and follows the new keep.
func TestMemoryUpdateTool_OldTextKeepsTypingUnlessChanged(t *testing.T) {
	p, store, ctx := typedProvider(t)
	route := MemoryUpdateArgs{Scope: memory.ScopeWorkspace, Mode: memory.ModeOnDemand}
	first := route
	first.Content, first.Keep, first.Kind, first.Why = "Cache entries live for ten minutes.", memory.KeepSession, memory.KindProject, "keeps the dashboard fresh"
	if _, err := p.Update(ctx, first); err != nil {
		t.Fatal(err)
	}
	second := route
	second.Content, second.Keep, second.OldText = "Cache entries live for five minutes.", memory.KeepPermanent, "ten minutes"
	if _, err := p.Update(ctx, second); err != nil {
		t.Fatal(err)
	}
	e := onlyFact(t, store, "ws-1")
	if e.Content != "Cache entries live for five minutes." || e.Kind != memory.KindProject || e.Why != "keeps the dashboard fresh" {
		t.Errorf("kind and why must survive a replacement that does not restate them: %+v", e)
	}
	if e.ExpiresAt != "" {
		t.Errorf("replacing with keep=permanent clears the session expiry, got %q", e.ExpiresAt)
	}
	third := route
	third.Content, third.Keep, third.OldText, third.Why = "Cache entries live for five minutes.", memory.KeepPermanent, "five minutes", "measured by the dashboard team"
	p.Update(ctx, third)
	if got := onlyFact(t, store, "ws-1").Why; got != "measured by the dashboard team" {
		t.Errorf("a restated why replaces the old one, got %q", got)
	}
}

func TestMemorySearchTool_ShowsKindWhyAndExpiry(t *testing.T) {
	p, _, ctx := typedProvider(t)
	p.Update(ctx, MemoryUpdateArgs{Content: "Releases are cut on Thursdays.", Scope: memory.ScopeWorkspace, Mode: memory.ModeOnDemand, Keep: memory.KeepPermanent,
		Kind: memory.KindReference, Why: "the release calendar lives in the wiki", Expires: "2027-01-01"})
	got, err := p.Search(ctx, searchArgs("releases"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"[reference]", "why: the release calendar lives in the wiki", "expires 2027-01-01"} {
		if !strings.Contains(got.(string), want) {
			t.Errorf("the search result lacks %q:\n%s", want, got)
		}
	}
}

// Capture and review save through SaveFact: it types and dates a fact the same way, and HasFact compares the dated text.
func TestSaveFact_TypesAndDatesLikeTheTool(t *testing.T) {
	p, store, ctx := typedProvider(t)
	res, err := p.SaveFact(ctx, "ws-1", Fact{Content: "The audit is tomorrow.", Scope: memory.ScopeWorkspace, Mode: memory.ModeOnDemand, Keep: memory.KeepPermanent,
		Kind: memory.KindProject, Why: "the auditors set the date"})
	if err != nil || res.Outcome != SaveCreated {
		t.Fatalf("SaveFact = %+v, %v", res, err)
	}
	e := onlyFact(t, store, "ws-1")
	if e.Content != "The audit is 2026-10-15 (Thursday)." || e.Kind != memory.KindProject || e.Why != "the auditors set the date" {
		t.Errorf("SaveFact lost the typing or the date: %+v", e)
	}
	if !p.HasFact(ctx, "ws-1", "The audit is tomorrow.") {
		t.Error("HasFact must compare the dated text, or capture saves the same fact twice")
	}
	if _, err := p.SaveFact(ctx, "ws-1", Fact{Content: "x fact", Scope: memory.ScopeWorkspace, Mode: memory.ModeOnDemand, Keep: memory.KeepPermanent, Kind: "bogus"}); err == nil {
		t.Error("SaveFact accepted a bad kind")
	}
}

// A secret must never be stored, whatever the model decides: the prompt says so, but only code enforces it (a small
// model saved "User's password is hunter2." on 2026-10-16 when the save reminder was widened).
func TestMemorySavePath_RefusesSecrets(t *testing.T) {
	secrets := []string{
		"User's password is hunter2.",
		"The staging API key is sk-abcdefghijklmnopqrstuvwxyz0123456789ABCD.",
		"My ssh key passphrase is correct-horse.",
		"The deploy token is ghp_abcdefghijklmnop1234.",
	}
	ctx := models.WithWorkspaceID(context.Background(), "ws-1")
	for _, content := range secrets {
		t.Run(content, func(t *testing.T) {
			store := newRealTestStore(t)
			provider := NewMemoryToolProvider(store)
			got, err := provider.Update(ctx, updateArgs{Content: content, Scope: memory.ScopeUser, Mode: memory.ModeOnDemand, Keep: memory.KeepPermanent})
			if err != nil || !strings.Contains(got.(string), "not saved") {
				t.Errorf("Update = %v, %v; want a refusal that says it was not saved", got, err)
			}
			if _, err := provider.SaveFact(ctx, "ws-1", Fact{Content: content, Scope: memory.ScopeWorkspace, Mode: memory.ModeOnDemand, Keep: memory.KeepPermanent}); !errors.Is(err, ErrSecretContent) {
				t.Errorf("SaveFact err = %v, want ErrSecretContent", err)
			}
			for _, ws := range []string{"ws-1", "global"} {
				if entries, _ := store.List(context.Background(), ws, "", 10, 0); len(entries) != 0 {
					t.Errorf("a secret was stored in %s: %+v", ws, entries)
				}
			}
		})
	}
	// Tokens, keys and secrets are everywhere in legitimate project facts: only a stated credential value is refused.
	for _, content := range []string{
		"We deploy through the vertex host.",
		"The context window is 16K tokens.",
		"max_tokens is 8192 for this model.",
		"The token budget is 4000 per run.",
		"We rotate the staging API key every month.",
		"Secrets live in secrets.json, encrypted.",
	} {
		t.Run("an ordinary fact still saves: "+content, func(t *testing.T) {
			provider := NewMemoryToolProvider(newRealTestStore(t))
			if _, err := provider.SaveFact(ctx, "ws-1", Fact{Content: content, Scope: memory.ScopeWorkspace, Mode: memory.ModeOnDemand, Keep: memory.KeepPermanent}); err != nil {
				t.Errorf("SaveFact refused an ordinary fact: %v", err)
			}
		})
	}
}

func searchArgs(query string) struct {
	Query interface{}  `json:"query"`
	Limit int          `json:"limit"`
	Scope memory.Scope `json:"scope"`
	Tags  []string     `json:"tags"`
} {
	return struct {
		Query interface{}  `json:"query"`
		Limit int          `json:"limit"`
		Scope memory.Scope `json:"scope"`
		Tags  []string     `json:"tags"`
	}{Query: query}
}

// Only what memory_search actually returned to the model counts as used.
func TestMemorySearch_RecordsTheFactsItReturned(t *testing.T) {
	store := newRealTestStore(t)
	ctx := models.WithWorkspaceID(context.Background(), "ws-1")
	hit, _ := store.Insert(ctx, memory.NewEntry{WorkspaceID: "ws-1", MemoryType: memory.LongTerm, Title: "tsc", Content: "TypeScript version 6.0.3", Tags: nil, Source: "agent"})
	miss, _ := store.Insert(ctx, memory.NewEntry{WorkspaceID: "ws-1", MemoryType: memory.LongTerm, Title: "tea", Content: "likes green tea", Tags: nil, Source: "agent"})
	provider := NewMemoryToolProvider(store)

	if _, err := provider.Search(ctx, searchArgs("TypeScript")); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Search(ctx, searchArgs("zzzz-nothing-matches")); err != nil {
		t.Fatal(err)
	}
	if err := store.FlushUsage(context.Background()); err != nil {
		t.Fatal(err)
	}

	gotHit, _ := store.Get(ctx, "ws-1", hit)
	gotMiss, _ := store.Get(ctx, "ws-1", miss)
	if gotHit.SearchedCount != 1 || gotMiss.SearchedCount != 0 {
		t.Errorf("searched counts: hit=%d miss=%d, want 1 and 0", gotHit.SearchedCount, gotMiss.SearchedCount)
	}
	if gotHit.InjectedCount != 0 {
		t.Error("a search is not an injection")
	}
}

// SaveFact is the one save path: the tool and the chat capture both go through it, so dedup and routing are shared.
func TestMemoryToolProvider_SaveFact(t *testing.T) {
	store := newRealTestStore(t)
	provider := NewMemoryToolProvider(store)
	ctx := context.Background()
	fact := Fact{Content: "the staging DB runs on port 5433", Scope: memory.ScopeWorkspace, Mode: memory.ModeOnDemand, Keep: memory.KeepPermanent, Source: "capture"}

	res, err := provider.SaveFact(ctx, "ws-1", fact)
	if err != nil || res.Outcome != SaveCreated || res.ID == 0 {
		t.Fatalf("first save = %+v, %v; want created", res, err)
	}
	res, err = provider.SaveFact(ctx, "ws-1", fact)
	if err != nil || res.Outcome != SaveDuplicate {
		t.Fatalf("second save = %+v, %v; want duplicate", res, err)
	}

	always := Fact{Content: "answer in short sentences", Scope: memory.ScopeWorkspace, Mode: memory.ModeAlways, Keep: memory.KeepPermanent, Source: "capture"}
	if _, err := provider.SaveFact(ctx, "ws-1", always); err != nil {
		t.Fatal(err)
	}
	entries, _ := store.List(ctx, "ws-1", "", 10, 0)
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	for _, e := range entries {
		if e.Source != "capture" {
			t.Errorf("entry %q source = %q, want the caller's source", e.Content, e.Source)
		}
		if e.Content == always.Content && !slices.Contains(e.Tags, memory.HotTag) {
			t.Errorf("an always fact must carry the hot tag, got %v", e.Tags)
		}
	}

	if _, err := provider.SaveFact(ctx, "ws-1", Fact{Content: "x", Scope: "garbage", Mode: memory.ModeOnDemand, Keep: memory.KeepPermanent}); err == nil {
		t.Error("an invalid scope must be rejected")
	}
	if _, err := NewMemoryToolProvider(nil).SaveFact(ctx, "ws-1", fact); err == nil {
		t.Error("saving without a store must be an error, not a panic")
	}
}

func TestMemoryToolProvider_HasFact(t *testing.T) {
	store := newRealTestStore(t)
	provider := NewMemoryToolProvider(store)
	ctx := context.Background()
	if provider.HasFact(ctx, "ws-1", "the staging DB runs on port 5433") {
		t.Fatal("an empty store has no facts")
	}
	_, _ = provider.SaveFact(ctx, "ws-1", Fact{Content: "the staging DB runs on port 5433", Scope: memory.ScopeWorkspace, Mode: memory.ModeOnDemand, Keep: memory.KeepPermanent})
	if !provider.HasFact(ctx, "ws-1", "the staging DB runs on port 5433") {
		t.Error("a saved fact must be found")
	}
	if provider.HasFact(ctx, "ws-2", "the staging DB runs on port 5433") {
		t.Error("another workspace's fact must not count")
	}
}

// Both "already saved" results tell the model it is done, so a small model does not call again.
func TestSaveResultMessage_DuplicatesSayNothingMoreIsNeeded(t *testing.T) {
	for name, res := range map[string]SaveResult{
		"matching entry":    {Outcome: SaveDuplicate, ID: 7, Type: memory.LongTerm},
		"duplicate content": {Outcome: SaveDuplicate, Type: memory.LongTerm},
	} {
		got := res.message()
		if !strings.Contains(got, "already saved") || !strings.Contains(got, "do not call memory_update again") {
			t.Errorf("%s: %q", name, got)
		}
	}
}
