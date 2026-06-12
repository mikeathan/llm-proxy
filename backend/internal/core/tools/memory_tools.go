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

// ── Three-tier route resolution ───────────────────────────────────────────

// MemoryRoute holds the store-level primitives that map from the model-facing
// (scope, mode, keep) triple.  Only store primitives — the handler translates,
// the store persists.
type MemoryRoute struct {
	WorkspaceID string
	MemoryType  string
	Tags        []string
}

// RouteStrategy is a function that resolves a (scope, mode, keep) triple to a
// MemoryRoute for the given workspace ID.
type RouteStrategy func(wsID string) MemoryRoute

// routeStrategies maps the composite key "scope_mode_keep" to its strategy.
// Open/Closed: adding a new combination is a one-line registration here,
// not a new case in branching logic.
var routeStrategies = map[string]RouteStrategy{
	"user_always_permanent":      func(wsID string) MemoryRoute { return MemoryRoute{"global", "user_profile", []string{"hot"}} },
	"user_on_demand_permanent":   func(wsID string) MemoryRoute { return MemoryRoute{"global", "user_profile", nil} },
	"user_on_demand_session":     func(wsID string) MemoryRoute { return MemoryRoute{"global", "user_profile", nil} },
	"workspace_always_permanent": func(wsID string) MemoryRoute { return MemoryRoute{wsID, "long_term", []string{"hot"}} },
	"workspace_on_demand_permanent": func(wsID string) MemoryRoute { return MemoryRoute{wsID, "long_term", nil} },
	"workspace_on_demand_session":   func(wsID string) MemoryRoute { return MemoryRoute{wsID, "session", nil} },
}

func resolveParams(scope memory.Scope, mode memory.Mode, keep memory.Keep, wsID string) (MemoryRoute, error) {
	key := string(scope) + "_" + string(mode) + "_" + string(keep)
	s, ok := routeStrategies[key]
	if !ok {
		return MemoryRoute{}, fmt.Errorf("invalid memory parameter combination: %s", key)
	}
	return s(wsID), nil
}

// ── Search ────────────────────────────────────────────────────────────────

// Search runs a full-text query against stored memories and returns results
// prefixed with Entry N/M so the model can always determine the exact count.
//
// Query is interface{} rather than string because the model occasionally emits
// query: {} (invalid JSON for a string field). The type assertion silently
// tolerates null, "", or any non-string value rather than crashing the tool call.
// See docs/PLANS/llamacpp-grammar-constraint.md for the planned proper fix.
//
// Scope is optional: "user" searches global user facts, "workspace" searches
// project facts, omitted searches both.
func (m *MemoryToolProvider) Search(ctx context.Context, args struct {
	Query interface{} `json:"query"`
	Limit int         `json:"limit"`
	Scope memory.Scope `json:"scope"`
	Tags  []string     `json:"tags"`
}) (any, error) {
	if m.store == nil {
		return "memory is not available", nil
	}

	query, _ := args.Query.(string)
	if args.Limit <= 0 || args.Limit > 20 {
		args.Limit = 5
	}

	wsID := models.GetWorkspaceID(ctx)
	var entries []memory.MemoryEntry
	var err error

	if strings.TrimSpace(query) == "" && len(args.Tags) == 0 {
		entries, err = m.listAllMemories(ctx, wsID, args.Scope, args.Limit)
	} else {
		opt := memory.SearchOption{Tags: args.Tags}
		switch args.Scope {
		case memory.ScopeUser:
			opt.WorkspaceID = "global"
		case memory.ScopeWorkspace:
			opt.WorkspaceID = wsID
		default:
			opt.SearchAllWorkspaces = true
		}
		entries, err = m.store.Search(ctx, wsID, query, args.Limit, opt)
		if err != nil && strings.Contains(err.Error(), "fts5: syntax error") {
			return "Invalid search query. Use plain words separated by spaces (e.g. 'birthday TypeScript'). No special characters, wildcards, or operators.", nil
		}
	}
	if err != nil {
		return "", fmt.Errorf("memory search failed: %w", err)
	}
	if len(entries) == 0 {
		return "no memories found matching that query", nil
	}

	var b strings.Builder
	for i, e := range entries {
		fmt.Fprintf(&b, "Entry %d/%d — **%s**", i+1, len(entries), e.Title)
		if e.MemoryType != memory.LongTerm {
			fmt.Fprintf(&b, " [%s]", e.MemoryType)
		}
		b.WriteString("\n")
		b.WriteString(e.Content)
		b.WriteString("\n")
	}
	return b.String(), nil
}

// listAllMemories returns all entries for the given scope without an FTS5 query.
func (m *MemoryToolProvider) listAllMemories(ctx context.Context, wsID string, scope memory.Scope, limit int) ([]memory.MemoryEntry, error) {
	switch scope {
	case memory.ScopeUser:
		return m.store.List(ctx, "global", "", limit, 0)
	case memory.ScopeWorkspace:
		return m.store.List(ctx, wsID, "", limit, 0)
	default:
		wsEntries, err := m.store.List(ctx, wsID, "", limit, 0)
		if err != nil {
			return nil, err
		}
		globalEntries, err := m.store.List(ctx, "global", "", limit, 0)
		if err != nil {
			return nil, err
		}
		return mergeAndCap(wsEntries, globalEntries, limit), nil
	}
}

// mergeAndCap merges two entry slices by ID (primary first), deduplicates, and caps.
func mergeAndCap(a, b []memory.MemoryEntry, limit int) []memory.MemoryEntry {
	seen := map[int64]bool{}
	result := make([]memory.MemoryEntry, 0, limit)

	for _, e := range a {
		seen[e.ID] = true
		result = append(result, e)
	}
	for _, e := range b {
		if len(result) >= limit {
			break
		}
		if !seen[e.ID] {
			result = append(result, e)
			seen[e.ID] = true
		}
	}
	return result
}

// Update saves a new memory or updates an existing one. Uses scope, mode, and keep
// to control where the fact goes and how long it lasts. When old_text is provided the
// tool first tries to find a matching entry by content substring; if none is found it
// falls through to create a new entry rather than erroring.
func (m *MemoryToolProvider) Update(ctx context.Context, args struct {
	Content string        `json:"content"`
	Scope   memory.Scope  `json:"scope"`
	Mode    memory.Mode   `json:"mode"`
	Keep    memory.Keep   `json:"keep"`
	OldText string        `json:"old_text"`
}) (any, error) {
	if m.store == nil {
		return "memory is not available", nil
	}
	if args.Content == "" {
		return "content is required", nil
	}
	if args.Scope == "" || args.Mode == "" || args.Keep == "" {
		args.Scope = memory.ScopeWorkspace
		args.Mode = memory.ModeOnDemand
		args.Keep = memory.KeepPermanent
	}

	wsID := models.GetWorkspaceID(ctx)
	route, err := resolveParams(args.Scope, args.Mode, args.Keep, wsID)
	if err != nil {
		return "", fmt.Errorf("invalid memory parameters — provide scope ('user'/'workspace'), mode ('always'/'on_demand'), keep ('permanent'/'session'): %w", err)
	}
	ctx = models.WithWorkspaceID(ctx, route.WorkspaceID)
	saveWS := route.WorkspaceID

	if args.OldText != "" {
		existing, err := m.store.FindByContentSubstring(ctx, saveWS, args.OldText)
		if err == nil {
			title := deriveTitle(args.Content)
			if err := m.store.Update(ctx, saveWS, existing.ID, title, args.Content, route.Tags, memory.ReplaceTags); err != nil {
				return "", fmt.Errorf("memory update failed: %w", err)
			}
			logging.Info("memory_update result", "action", "updated", "existing_id", existing.ID, "match", "old_text")
			return fmt.Sprintf("updated memory entry %d (type: %s)", existing.ID, route.MemoryType), nil
		}
		if !strings.Contains(err.Error(), "no memory entry matching") {
			return "", fmt.Errorf("memory update failed: %w", err)
		}
	}

	return m.insertEntry(ctx, saveWS, args.Content, route.Tags, memory.MemoryType(route.MemoryType))
}

// insertEntry saves content with Jaccard similarity dedup and exact content match.
// The title is auto-derived from content so the model doesn't need to provide one.
func (m *MemoryToolProvider) insertEntry(ctx context.Context, wsID, content string, tags []string, memType memory.MemoryType) (any, error) {
	title := deriveTitle(content)

	existing := m.findOverlappingEntry(ctx, wsID, title, content)
	if existing != nil {
		if existing.Content == content {
			logging.Info("memory_update result", "action", "already_saved", "existing_id", existing.ID, "match", "jaccard")
			return fmt.Sprintf("already saved — matching entry found (id: %d)", existing.ID), nil
		}
		combined := existing.Content + "\n" + content
		if err := m.store.Update(ctx, wsID, existing.ID, title, combined, tags, memory.MergeTags); err != nil {
			return "", fmt.Errorf("memory update failed: %w", err)
		}
		logging.Info("memory_update result", "action", "updated", "existing_id", existing.ID, "match", "jaccard")
		return fmt.Sprintf("updated memory entry %d (type: %s)", existing.ID, memType), nil
	}

	exists, err := m.store.Exists(ctx, wsID, content)
	if err != nil {
		return "", fmt.Errorf("memory update failed: %w", err)
	}
	if exists {
		logging.Info("memory_update result", "action", "already_saved", "match", "content_dup")
		return fmt.Sprintf("already saved — duplicate content (type: %s)", memType), nil
	}
	id, err := m.store.Insert(ctx, wsID, memType, title, content, tags, "agent")
	if err != nil {
		return "", fmt.Errorf("memory update failed: %w", err)
	}
	logging.Info("memory_update result", "action", "created", "id", id, "memory_type", string(memType))
	return fmt.Sprintf("saved to memory (id: %d, type: %s)", id, memType), nil
}

// deriveTitle creates a short title from the first 60 characters of content.
func deriveTitle(content string) string {
	if len(content) > 60 {
		return content[:60]
	}
	return content
}

// findOverlappingEntry searches existing memories using the new topic + content
// as an FTS5 query, then computes Jaccard similarity on normalized topic words
// (threshold 0.70) and content words (threshold 0.90). Topic match catches
// near-synonym entries like "smoke-test-status" vs "llm-smoke-test-results" (J=0.75).
// Content match catches the same fact stored under different topics — for example,
// "tool_versions" (content: "TypeScript 6.0.3") vs "typescript_version" (same content).
func (m *MemoryToolProvider) findOverlappingEntry(ctx context.Context, wsID, topic, content string) *memory.MemoryEntry {
	newWords := topicWords(topic)
	if len(newWords) < 2 {
		return nil
	}
	newContentWords := topicWords(content)

	entries, err := m.store.Search(ctx, wsID, content+" "+topic, 5)
	if err != nil || len(entries) == 0 {
		return nil
	}

	for _, existing := range entries {
		oldWords := topicWords(existing.Title)
		if topicJaccard(newWords, oldWords) >= 0.70 {
			return &existing
		}
		if len(newContentWords) >= 2 {
			oldContentWords := topicWords(existing.Content)
			if topicJaccard(newContentWords, oldContentWords) >= 0.90 {
				return &existing
			}
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
