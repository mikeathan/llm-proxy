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
	searchFn func(ctx context.Context, workspaceID, query string, limit int) ([]memory.MemoryEntry, error)
}

func (m *mockMemoryStore) Search(ctx context.Context, workspaceID, query string, limit int) ([]memory.MemoryEntry, error) {
	if m.searchFn != nil {
		return m.searchFn(ctx, workspaceID, query, limit)
	}
	return m.Store.Search(ctx, workspaceID, query, limit)
}

func TestMemorySearchTool(t *testing.T) {
	store := &mockMemoryStore{
		searchFn: func(ctx context.Context, workspaceID, query string, limit int) ([]memory.MemoryEntry, error) {
			return []memory.MemoryEntry{
				{Title: "port", Content: "test DB port is 5433", MemoryType: memory.LongTerm},
			}, nil
		},
	}
	provider := NewMemoryToolProvider(store.Store)
	args := struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
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
		Query string `json:"query"`
		Limit int    `json:"limit"`
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

func TestMemoryUpdateTool(t *testing.T) {
	store := &mockMemoryStore{}
	provider := NewMemoryToolProvider(store.Store)
	args := struct {
		Topic      string `json:"topic"`
		Content    string `json:"content"`
		MemoryType string `json:"memory_type"`
		OldText    string `json:"old_text"`
		Target     string `json:"target"`
	}{
		Topic:      "test key",
		Content:    "test content",
		MemoryType: "long_term",
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
		Topic      string `json:"topic"`
		Content    string `json:"content"`
		MemoryType string `json:"memory_type"`
		OldText    string `json:"old_text"`
		Target     string `json:"target"`
	}{
		Topic:   "",
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
		Topic      string `json:"topic"`
		Content    string `json:"content"`
		MemoryType string `json:"memory_type"`
		OldText    string `json:"old_text"`
		Target     string `json:"target"`
	}{
		Topic:      "my topic",
		Content:    "exact duplicate content",
		MemoryType: "long_term",
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
		Topic      string `json:"topic"`
		Content    string `json:"content"`
		MemoryType string `json:"memory_type"`
		OldText    string `json:"old_text"`
		Target     string `json:"target"`
	}{
		Topic:      "port",
		Content:    "database port is 5433",
		MemoryType: "long_term",
	}

	result, err := provider.Update(ctx, createArgs)
	if err != nil {
		t.Fatalf("create Update failed: %v", err)
	}
	if !strings.Contains(result.(string), "saved to memory") {
		t.Errorf("expected 'saved to memory', got: %s", result)
	}

	updateArgs := struct {
		Topic      string `json:"topic"`
		Content    string `json:"content"`
		MemoryType string `json:"memory_type"`
		OldText    string `json:"old_text"`
		Target     string `json:"target"`
	}{
		Topic:      "port",
		Content:    "database port is 5433 (updated)",
		MemoryType: "long_term",
		OldText:    "5433",
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
		Topic      string `json:"topic"`
		Content    string `json:"content"`
		MemoryType string `json:"memory_type"`
		OldText    string `json:"old_text"`
		Target     string `json:"target"`
	}{
		Topic:      "port",
		Content:    "database port is 5433",
		MemoryType: "long_term",
		OldText:    "nonexistent text",
	}

	ctx := models.WithWorkspaceID(context.Background(), "ws-1")
	_, err := provider.Update(ctx, createArgs)
	if err == nil {
		t.Fatal("expected error when old_text matches nothing")
	}
}

func TestMemoryUpdateTool_OldTextBackwardCompat(t *testing.T) {
	store := newRealTestStore(t)
	provider := NewMemoryToolProvider(store)
	ctx := models.WithWorkspaceID(context.Background(), "ws-1")

	args := struct {
		Topic      string `json:"topic"`
		Content    string `json:"content"`
		MemoryType string `json:"memory_type"`
		OldText    string `json:"old_text"`
		Target     string `json:"target"`
	}{
		Topic:      "first entry",
		Content:    "this is entry one",
		MemoryType: "long_term",
	}

	result, err := provider.Update(ctx, args)
	if err != nil {
		t.Fatalf("first Update failed: %v", err)
	}
	if !strings.Contains(result.(string), "saved to memory") {
		t.Errorf("expected 'saved to memory', got: %s", result)
	}

	args.Topic = "second entry"
	args.Content = "this is entry two"

	result, err = provider.Update(ctx, args)
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
		t.Errorf("expected 2 entries (no old_text means insert), got %d", len(entries))
	}
}
