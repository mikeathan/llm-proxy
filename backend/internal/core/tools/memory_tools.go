// Package tools implements agent-accessible tool providers for filesystem, terminal,
// network, memory, and search operations. Each provider wraps a backend and exposes
// handler functions registered by name in LocalToolRegistry.
package tools

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/memory"
	"llm-proxy/models"
)

// MemoryToolProvider wraps a memory.Store so the agent can persist and retrieve facts
// across conversations via the memory_search and memory_update tools.
type MemoryToolProvider struct {
	store *memory.Store
}

func NewMemoryToolProvider(store *memory.Store) *MemoryToolProvider {
	return &MemoryToolProvider{store: store}
}

// Search runs a full-text query against stored memories and returns results formatted
// as markdown with triple-dash separators between entries.
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

// Update saves a new memory or updates an existing one. When old_text is provided the
// tool first tries to find a matching entry by content substring; if none is found it
// falls through to create a new entry rather than erroring. This lets agents use old_text
// as an upsert key without needing to track entry IDs.
func (m *MemoryToolProvider) Update(ctx context.Context, args struct {
	Topic      string `json:"topic"`
	Content    string `json:"content"`
	MemoryType string `json:"memory_type"`
	OldText    string `json:"old_text"`
	Target     string `json:"target"`
}) (any, error) {
	if m.store == nil {
		return "memory is not available", nil
	}
	if args.Topic == "" || args.Content == "" {
		return "both topic and content are required", nil
	}

	wsID := models.GetWorkspaceID(ctx)

	if args.OldText != "" {
		existing, err := m.store.FindByContentSubstring(ctx, wsID, args.OldText)
		if err == nil {
			mt := resolveMemType(args.Target, args.MemoryType)
			if err := m.store.Update(ctx, wsID, existing.ID, args.Topic, args.Content); err != nil {
				return "", fmt.Errorf("memory update failed: %w", err)
			}
			logging.Info("memory_update result", "topic", args.Topic, "action", "updated", "existing_id", existing.ID, "match", "old_text")
			return fmt.Sprintf("updated memory entry %d (type: %s)", existing.ID, mt), nil
		}
		// Distinguish "not found" from ambiguous/multiple matches — only the former should
		// fall through to create a new entry; ambiguous matches are a real error.
		if !strings.Contains(err.Error(), "no memory entry matching") {
			return "", fmt.Errorf("memory update failed: %w", err)
		}
	}

	return m.insertEntry(ctx, wsID, args.Topic, args.Content, args.Target, args.MemoryType)
}

// resolveMemType maps the user-facing target and memory_type strings to the internal
// MemoryType constant. target="user" takes precedence and maps to UserProfile regardless
// of memory_type.
func resolveMemType(target, memoryType string) memory.MemoryType {
	if target == "user" {
		return memory.UserProfile
	}
	switch memoryType {
	case "daily":
		return memory.Daily
	case "session":
		return memory.Session
	default:
		return memory.LongTerm
	}
}

// insertEntry checks for topic-based duplicates before inserting. If an entry with the
// same topic already exists it is updated in-place, ensuring one entry per unique topic.
// Falls back to FTS4 content search + word-overlap dedup, then exact content match.
func (m *MemoryToolProvider) insertEntry(ctx context.Context, wsID, topic, content, target, memoryType string) (any, error) {
	existing, err := m.store.FindByTitle(ctx, wsID, topic)
	if err != nil {
		return "", fmt.Errorf("memory update failed: %w", err)
	}
	if existing != nil {
		if err := m.store.Update(ctx, wsID, existing.ID, topic, content); err != nil {
			return "", fmt.Errorf("memory update failed: %w", err)
		}
		mt := resolveMemType(target, memoryType)
		logging.Info("memory_update result", "topic", topic, "action", "updated", "existing_id", existing.ID, "match", "title")
		return fmt.Sprintf("updated memory entry %d (type: %s)", existing.ID, mt), nil
	}

	existing = m.findOverlappingEntry(ctx, wsID, topic, content)
	if existing != nil {
		if existing.Content == content {
			logging.Info("memory_update result", "topic", topic, "action", "already_saved", "existing_id", existing.ID, "match", "jaccard")
			return fmt.Sprintf("already saved — matching entry found for topic %q (id: %d)", existing.Title, existing.ID), nil
		}
		if err := m.store.Update(ctx, wsID, existing.ID, topic, content); err != nil {
			return "", fmt.Errorf("memory update failed: %w", err)
		}
		mt := resolveMemType(target, memoryType)
		logging.Info("memory_update result", "topic", topic, "action", "updated", "existing_id", existing.ID, "match", "jaccard")
		return fmt.Sprintf("updated memory entry %d (type: %s)", existing.ID, mt), nil
	}

	if target == "user" {
		exists, err := m.store.Exists(ctx, wsID, content)
		if err != nil {
			return "", fmt.Errorf("memory update failed: %w", err)
		}
		if exists {
			logging.Info("memory_update result", "topic", topic, "action", "already_saved", "match", "user_content_dup")
			return fmt.Sprintf("already saved — duplicate content for topic %q", topic), nil
		}
		id, err := m.store.Insert(ctx, wsID, memory.UserProfile, topic, content, "agent")
		if err != nil {
			return "", fmt.Errorf("memory update failed: %w", err)
		}
		logging.Info("memory_update result", "topic", topic, "action", "created", "id", id, "memory_type", "user_profile")
		return fmt.Sprintf("saved to user profile (id: %d)", id), nil
	}

	mt := resolveMemType(target, memoryType)
	exists, err := m.store.Exists(ctx, wsID, content)
	if err != nil {
		return "", fmt.Errorf("memory update failed: %w", err)
	}
	if exists {
		logging.Info("memory_update result", "topic", topic, "action", "already_saved", "match", "content_dup")
		return fmt.Sprintf("already saved — duplicate content for topic %q", topic), nil
	}
	id, err := m.store.Insert(ctx, wsID, mt, topic, content, "agent")
	if err != nil {
		return "", fmt.Errorf("memory update failed: %w", err)
	}
	logging.Info("memory_update result", "topic", topic, "action", "created", "id", id, "memory_type", string(mt))
	return fmt.Sprintf("saved to memory (id: %d, type: %s)", id, mt), nil
}

// findOverlappingEntry searches existing memories using the new topic + content
// as an FTS5 query, then computes Jaccard similarity on normalized topic words.
// If any existing entry's topic has Jaccard ≥ 0.70 vs the new topic, the entry
// is returned for in-place update. This catches near-duplicate entries with
// different topic names — for example, "smoke-test-status" (["smoke", "test", "status"])
// vs "llm-smoke-test-results" (["llm", "smoke", "test", "results"]) → J = 3/4 = 0.75 → match.
func (m *MemoryToolProvider) findOverlappingEntry(ctx context.Context, wsID, topic, content string) *memory.MemoryEntry {
	newWords := topicWords(topic)
	if len(newWords) < 2 {
		return nil
	}

	entries, err := m.store.Search(ctx, wsID, content+" "+topic, 3)
	if err != nil || len(entries) == 0 {
		return nil
	}

	for _, existing := range entries {
		oldWords := topicWords(existing.Title)
		if topicJaccard(newWords, oldWords) >= 0.70 {
			return &existing
		}
	}
	return nil
}

// topicJaccard returns the Jaccard similarity coefficient between two word slices.
// J(A,B) = |A∩B| / |A∪B|. Returns 0 if either slice is empty.
func topicJaccard(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	setA := make(map[string]bool, len(a))
	for _, w := range a {
		setA[w] = true
	}
	intersect := 0
	for _, w := range b {
		if setA[w] {
			intersect++
			setA[w] = false
		}
	}
	union := len(a) + len(b) - intersect
	return float64(intersect) / float64(union)
}

// topicWords extracts alphabetic words ≥3 chars from s, excluding common English
// stop words. Used for comparing topic names across entries.
func topicWords(s string) []string {
	var words []string
	var buf []rune
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) {
			buf = append(buf, r)
		} else {
			if len(buf) >= 3 && !stopWords[string(buf)] {
				words = append(words, string(buf))
			}
			buf = buf[:0]
		}
	}
	if len(buf) >= 3 && !stopWords[string(buf)] {
		words = append(words, string(buf))
	}
	return words
}

// stopWords contains common English words that carry little semantic value for
// topic deduplication. Words shorter than 3 chars are filtered by topicWords.
var stopWords = map[string]bool{
	"the": true, "and": true, "for": true, "are": true, "but": true, "not": true,
	"you": true, "all": true, "can": true, "her": true, "was": true,
	"one": true, "our": true, "out": true, "has": true, "had": true,
	"his": true, "him": true, "how": true, "now": true, "its": true,
	"any": true, "may": true, "see": true, "way": true, "who": true, "get": true,
	"two": true, "use": true, "let": true, "say": true, "too": true,
	"off": true, "old": true, "own": true, "put": true, "set": true,
	"end": true, "men": true, "ago": true, "ask": true, "few": true, "got": true,
	"yet": true, "top": true, "try": true, "ran": true, "did": true,
	"run": true, "big": true, "far": true, "via": true, "per": true, "new": true,
	"this": true, "entry": true, "than": true, "also": true, "more": true,
}

// ValidateWorkspace checks that the context carries a non-empty workspace ID, used as a
// preregistration guard before tool execution.
func (m *MemoryToolProvider) ValidateWorkspace(ctx context.Context) error {
	wsID := models.GetWorkspaceID(ctx)
	if wsID == "" {
		return fmt.Errorf("no workspace in context")
	}
	return nil
}
