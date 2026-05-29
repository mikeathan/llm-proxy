package tools

import (
	"context"
	"fmt"
	"strings"

	"llm-proxy/internal/platform/memory"
	"llm-proxy/models"
)

// MemoryToolProvider wraps the memory store for agent tool execution.
type MemoryToolProvider struct {
	store *memory.Store
}

func NewMemoryToolProvider(store *memory.Store) *MemoryToolProvider {
	return &MemoryToolProvider{store: store}
}

func (m *MemoryToolProvider) Search(ctx context.Context, args struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}) (any, error) {
	if m.store == nil {
		return "memory is not available", nil
	}
	if args.Query == "" {
		return "please provide a search query", nil
	}
	if args.Limit <= 0 || args.Limit > 20 {
		args.Limit = 5
	}

	wsID := models.GetWorkspaceID(ctx)
	entries, err := m.store.Search(ctx, wsID, args.Query, args.Limit)
	if err != nil {
		return "", fmt.Errorf("memory search failed: %w", err)
	}
	if len(entries) == 0 {
		return "no memories found matching that query", nil
	}

	var b strings.Builder
	for i, e := range entries {
		if i > 0 {
			b.WriteString("\n---\n")
		}
		fmt.Fprintf(&b, "**%s**", e.Title)
		if e.MemoryType != memory.LongTerm {
			fmt.Fprintf(&b, " [%s]", e.MemoryType)
		}
		b.WriteString("\n")
		b.WriteString(e.Content)
	}
	return b.String(), nil
}

func (m *MemoryToolProvider) Update(ctx context.Context, args struct {
	Topic      string `json:"topic"`
	Content    string `json:"content"`
	MemoryType string `json:"memory_type"`
}) (any, error) {
	if m.store == nil {
		return "memory is not available", nil
	}
	if args.Topic == "" || args.Content == "" {
		return "both topic and content are required", nil
	}

	mt := memory.LongTerm
	switch args.MemoryType {
	case "daily":
		mt = memory.Daily
	case "session":
		mt = memory.Session
	}

	wsID := models.GetWorkspaceID(ctx)
	id, err := m.store.Insert(ctx, wsID, mt, args.Topic, args.Content, "agent")
	if err != nil {
		return "", fmt.Errorf("memory update failed: %w", err)
	}
	return fmt.Sprintf("saved to memory (id: %d, type: %s)", id, mt), nil
}

func (m *MemoryToolProvider) ValidateWorkspace(ctx context.Context) error {
	wsID := models.GetWorkspaceID(ctx)
	if wsID == "" {
		return fmt.Errorf("no workspace in context")
	}
	return nil
}
