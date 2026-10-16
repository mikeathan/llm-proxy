// Package tools implements agent-accessible tool providers for filesystem, terminal,
// network, memory, and search operations. Each provider wraps a backend and exposes
// handler functions registered by name in LocalToolRegistry.
package tools

import (
	"context"
	"errors"
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

// ErrSecretContent is returned when a fact that looks like a credential is offered for saving.
var ErrSecretContent = errors.New("memory: content looks like a secret and is never stored")

// memorySecretRefusal is what memory_update tells the model when it offers a secret.
const memorySecretRefusal = "not saved: this looks like a password, key or token, and secrets are never stored in memory"

// isSecretContent is the code-level screen every save passes: secret-shaped strings, and statements of a
// credential's value. The prompt also says "never save secrets", but only this enforces it. It is narrower than
// capture's sensitive-word list on purpose: the agent saves project facts, which mention tokens and keys constantly.
func isSecretContent(content string) bool {
	return ContainsSecret(content) || ContainsCredentialAssignment(content)
}

// MemoryRoute holds the store-level primitives that map from the model-facing
// (scope, mode, keep) triple.  Only store primitives — the handler translates,
// the store persists.
type MemoryRoute struct {
	WorkspaceID string
	MemoryType  string
	Tags        []string
}

const (
	// searchMissFactLimit bounds the saved facts a search that matched nothing shows for the model to check by meaning.
	searchMissFactLimit = 8

	// globalWorkspaceID is where user-scope facts live.
	globalWorkspaceID = "global"

	memorySourceAgent     = "agent"
	memorySourceRunPrefix = "run:"
)

// RouteStrategy is a function that resolves a (scope, mode, keep) triple to a
// MemoryRoute for the given workspace ID.
type RouteStrategy func(wsID string) MemoryRoute

// routeStrategies maps the composite key "scope_mode_keep" to its strategy.
// Open/Closed: adding a new combination is a one-line registration here,
// not a new case in branching logic.
var routeStrategies = map[string]RouteStrategy{
	"user_always_permanent":         func(wsID string) MemoryRoute { return MemoryRoute{"global", "user_profile", []string{"hot"}} },
	"user_on_demand_permanent":      func(wsID string) MemoryRoute { return MemoryRoute{"global", "user_profile", nil} },
	"user_on_demand_session":        func(wsID string) MemoryRoute { return MemoryRoute{"global", "user_profile", nil} },
	"workspace_always_permanent":    func(wsID string) MemoryRoute { return MemoryRoute{wsID, "long_term", []string{"hot"}} },
	"workspace_on_demand_permanent": func(wsID string) MemoryRoute { return MemoryRoute{wsID, "long_term", nil} },
	"workspace_on_demand_session":   func(wsID string) MemoryRoute { return MemoryRoute{wsID, "session", nil} },
}

// ResolveMemoryRoute translates the (scope, mode, keep) triple into store primitives.
// Exported so the operator UI and the agent tool share one routing table.
func ResolveMemoryRoute(scope memory.Scope, mode memory.Mode, keep memory.Keep, wsID string) (MemoryRoute, error) {
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
	Query interface{}  `json:"query"`
	Limit int          `json:"limit"`
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
		return m.missMessage(ctx, wsID, args.Scope), nil
	}
	// Used = returned to the model. In-memory; the usage flusher writes it later.
	m.store.RecordSearched(entries)

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

const (
	searchMissNothing  = "no memories found matching that query"
	searchMissIntro    = "no saved fact contains those words. These are the latest saved facts; if the user phrased the question differently, one of them may answer it by meaning:"
	searchMissFactClip = 160
)

// missMessage is the result of a search that matched nothing. Keyword search finds a fact by the words it was saved with,
// and a model cannot always guess the words the fact used, but it can recognise a fact by meaning when shown candidates,
// so the latest saved facts (both scopes, bounded) follow, in the owner's own chats only. An empty memory, a failed
// listing or a connector chat just says nothing matched.
func (m *MemoryToolProvider) missMessage(ctx context.Context, wsID string, scope memory.Scope) string {
	if models.OriginConnector(ctx) != "" {
		return searchMissNothing // an outside sender is never shown the owner's facts, as with recall
	}
	facts, err := m.listAllMemories(ctx, wsID, scope, searchMissFactLimit)
	if err != nil || len(facts) == 0 {
		return searchMissNothing
	}
	var b strings.Builder
	b.WriteString(searchMissIntro)
	for _, e := range facts {
		fmt.Fprintf(&b, "\n- %s", clipFact(e.Content, searchMissFactClip))
	}
	return b.String()
}

// clipFact shortens a fact to one line of at most limit characters.
func clipFact(content string, limit int) string {
	line := []rune(strings.Join(strings.Fields(content), " "))
	if len(line) > limit {
		return string(line[:limit]) + "…"
	}
	return string(line)
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

// MemoryUpdateArgs are the model-facing arguments of memory_update (docs: manifests/memory.json).
type MemoryUpdateArgs struct {
	Content string       `json:"content"`
	Scope   memory.Scope `json:"scope"`
	Mode    memory.Mode  `json:"mode"`
	Keep    memory.Keep  `json:"keep"`
	OldText string       `json:"old_text"`
	// Separate saves the fact as its own entry even when a similar one exists (otherwise the tool reports the similar fact).
	Separate bool `json:"separate"`
}

// Update saves a new memory or updates an existing one. Uses scope, mode, and keep
// to control where the fact goes and how long it lasts. When old_text is provided the
// tool first tries to find a matching entry by content substring; if none is found it
// falls through to create a new entry rather than erroring.
func (m *MemoryToolProvider) Update(ctx context.Context, args MemoryUpdateArgs) (any, error) {
	if m.store == nil {
		return "memory is not available", nil
	}
	if args.Content == "" {
		return "content is required", nil
	}
	if isSecretContent(args.Content) {
		logging.Info("memory_update result", "action", "refused", "reason", "secret")
		return memorySecretRefusal, nil
	}
	args.Scope, args.Mode, args.Keep = withRoutingDefaults(ctx, args.Scope, args.Mode, args.Keep)

	wsID := models.GetWorkspaceID(ctx)
	route, err := ResolveMemoryRoute(args.Scope, args.Mode, args.Keep, wsID)
	if err != nil {
		return "", fmt.Errorf("invalid memory parameters — provide scope ('user'/'workspace'), mode ('always'/'on_demand'), keep ('permanent'/'session'): %w", err)
	}
	ctx = models.WithWorkspaceID(ctx, route.WorkspaceID)
	saveWS := route.WorkspaceID

	if args.OldText != "" {
		if msg, replaced, err := m.replaceByOldText(ctx, route, wsID, args); err != nil || replaced {
			return msg, err
		}
	}

	policy := overlapReport
	if args.Separate {
		policy = overlapSeparate
	}
	res, err := m.insertFact(ctx, saveRequest{workspaceID: saveWS, content: args.Content, tags: route.Tags, memType: memory.MemoryType(route.MemoryType), onOverlap: policy})
	if err != nil {
		return "", err
	}
	return res.message(), nil
}

// replaceByOldText replaces the fact that contains args.OldText with the new content. It looks in the scope the new
// fact goes to first; failing that, in the other scope (the workspace's and the user's), and then moves the fact: a
// fact "that actually applies to all my projects" must not leave a second copy behind. replaced is false when no fact
// matches, so the caller saves the content as a new fact.
func (m *MemoryToolProvider) replaceByOldText(ctx context.Context, route MemoryRoute, workspaceID string, args MemoryUpdateArgs) (msg string, replaced bool, err error) {
	title := deriveTitle(args.Content)
	existing, err := m.store.FindByContentSubstring(ctx, route.WorkspaceID, args.OldText)
	if err == nil {
		if err := m.store.Update(ctx, route.WorkspaceID, existing.ID, title, args.Content, route.Tags, memory.ReplaceTags); err != nil {
			return "", false, fmt.Errorf("memory update failed: %w", err)
		}
		logging.Info("memory_update result", "action", "updated", "existing_id", existing.ID, "match", "old_text")
		return fmt.Sprintf("updated memory entry %d (type: %s)", existing.ID, route.MemoryType), true, nil
	}
	if !strings.Contains(err.Error(), "no memory entry matching") {
		return "", false, fmt.Errorf("memory update failed: %w", err)
	}

	otherWS := globalWorkspaceID
	if route.WorkspaceID == globalWorkspaceID {
		otherWS = workspaceID
	}
	old, err := m.store.FindByContentSubstring(ctx, otherWS, args.OldText)
	if err != nil {
		return "", false, nil // no match in either scope: the caller saves a new fact
	}
	id, err := m.store.Insert(ctx, route.WorkspaceID, memory.MemoryType(route.MemoryType), title, args.Content, route.Tags, memorySource(ctx))
	if err != nil {
		return "", false, fmt.Errorf("memory update failed: %w", err)
	}
	if err := m.store.Delete(ctx, otherWS, old.ID); err != nil {
		return "", false, fmt.Errorf("memory update failed: %w", err)
	}
	logging.Info("memory_update result", "action", "moved", "from_id", old.ID, "to_id", id, "match", "old_text")
	return fmt.Sprintf("moved and updated memory entry %d to %d (type: %s)", old.ID, id, route.MemoryType), true, nil
}

// MemoryForgetArgs are the model-facing arguments of memory_forget.
type MemoryForgetArgs struct {
	// Text is a unique part of the saved fact to delete.
	Text string `json:"text"`
}

const (
	forgetMinTextLen  = 3 // shorter text would match almost any fact
	forgetScanLimit   = 500
	forgetCandidates  = 5 // how many matches to show when the text is ambiguous
	forgetOperatorMsg = "memory_forget is only available in the operator's own chat, not in automations or connector chats"
)

// Forget deletes the one saved fact that contains args.Text, in the workspace's or the user's scope. It deletes only
// on an unambiguous match, only in the operator's own chat (an automation or an outside sender must not erase what the
// owner saved), and says what it deleted.
func (m *MemoryToolProvider) Forget(ctx context.Context, args MemoryForgetArgs) (any, error) {
	if m.store == nil {
		return "memory is not available", nil
	}
	if models.IsUnattendedRun(ctx) || models.OriginConnector(ctx) != "" {
		return forgetOperatorMsg, nil
	}
	text := strings.ToLower(strings.TrimSpace(args.Text))
	if len(text) < forgetMinTextLen {
		return fmt.Sprintf("text is required: give at least %d characters from the fact to forget", forgetMinTextLen), nil
	}
	matches, err := m.forgetMatches(ctx, text)
	if err != nil {
		return "", fmt.Errorf("memory forget failed: %w", err)
	}
	switch len(matches) {
	case 0:
		return "no saved fact contains that text; nothing was forgotten", nil
	case 1:
		if err := m.store.Delete(ctx, matches[0].WorkspaceID, matches[0].ID); err != nil {
			return "", fmt.Errorf("memory forget failed: %w", err)
		}
		logging.Info("memory_forget result", "action", "deleted", "id", matches[0].ID, "workspace", matches[0].WorkspaceID)
		return fmt.Sprintf("forgot memory entry %d: %q", matches[0].ID, matches[0].Content), nil
	}
	return ambiguousForgetMessage(matches), nil
}

// forgetMatches lists the facts of the workspace and of the user whose text contains the lower-cased text.
func (m *MemoryToolProvider) forgetMatches(ctx context.Context, text string) ([]memory.MemoryEntry, error) {
	var matches []memory.MemoryEntry
	for _, ws := range []string{models.GetWorkspaceID(ctx), globalWorkspaceID} {
		entries, err := m.store.List(ctx, ws, "", forgetScanLimit, 0)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if strings.Contains(strings.ToLower(e.Content), text) {
				matches = append(matches, e)
			}
		}
	}
	return matches, nil
}

// ambiguousForgetMessage asks the model for more of the fact's own words, showing a few candidates.
func ambiguousForgetMessage(matches []memory.MemoryEntry) string {
	var b strings.Builder
	b.WriteString("more than one saved fact contains that text; nothing was forgotten. Call memory_forget again with more of the fact's own words:")
	for i, e := range matches {
		if i == forgetCandidates {
			break
		}
		fmt.Fprintf(&b, "\n- id %d: %q", e.ID, e.Content)
	}
	return b.String()
}

// SaveOutcome is what saving a fact did.
type SaveOutcome int

const (
	SaveCreated SaveOutcome = iota + 1
	SaveUpdated
	SaveDuplicate
	// SaveConflict: nothing was written because a similar fact exists; the model chooses to replace or keep both.
	SaveConflict
)

// overlapPolicy is what saving does when a similar fact already exists.
type overlapPolicy int

const (
	overlapReport   overlapPolicy = iota // the tool reports the similar fact and writes nothing
	overlapSeparate                      // the new fact is saved as its own entry
	overlapReplace                       // the similar fact is replaced in place (capture and review cannot retry)
)

// SaveResult describes a save: what happened, which entry it touched (0 for an exact-content duplicate) and its type.
type SaveResult struct {
	Outcome SaveOutcome
	ID      int64
	Type    memory.MemoryType
	// Existing is the similar fact's text, set for SaveConflict.
	Existing string
}

// message renders the result in the wording memory_update has always returned to the model.
func (r SaveResult) message() string {
	switch {
	case r.Outcome == SaveDuplicate && r.ID != 0:
		return fmt.Sprintf("already saved — matching entry found (id: %d); do not call memory_update again", r.ID)
	case r.Outcome == SaveDuplicate:
		return fmt.Sprintf("already saved — duplicate content (type: %s); do not call memory_update again", r.Type)
	case r.Outcome == SaveUpdated:
		return fmt.Sprintf("updated memory entry %d (type: %s)", r.ID, r.Type)
	case r.Outcome == SaveConflict:
		return fmt.Sprintf("not saved: a similar fact already exists (id: %d): %q. If the new content replaces it, call %s again with old_text set to a unique part of that fact. If it is a different fact, call %s again with the same content and separate set to true.",
			r.ID, r.Existing, models.ToolMemoryUpdate, models.ToolMemoryUpdate)
	default:
		return fmt.Sprintf("saved to memory (id: %d, type: %s)", r.ID, r.Type)
	}
}

// Fact is one fact to save through the shared path (the agent tool and the chat capture).
type Fact struct {
	Content string
	Scope   memory.Scope
	Mode    memory.Mode
	Keep    memory.Keep
	// Source attributes the entry; empty takes the tool's rule (memorySource).
	Source string
}

// SaveFact saves a fact with the same routing and dedup as memory_update.
func (m *MemoryToolProvider) SaveFact(ctx context.Context, workspaceID string, f Fact) (SaveResult, error) {
	if m.store == nil {
		return SaveResult{}, fmt.Errorf("memory is not available")
	}
	if isSecretContent(f.Content) {
		return SaveResult{}, ErrSecretContent
	}
	route, err := ResolveMemoryRoute(f.Scope, f.Mode, f.Keep, workspaceID)
	if err != nil {
		return SaveResult{}, err
	}
	ctx = models.WithWorkspaceID(ctx, route.WorkspaceID)
	return m.insertFact(ctx, saveRequest{workspaceID: route.WorkspaceID, content: f.Content, tags: route.Tags, memType: memory.MemoryType(route.MemoryType), source: f.Source, onOverlap: overlapReplace})
}

// HasFact reports whether the workspace already holds this fact, exactly or as a near-duplicate.
func (m *MemoryToolProvider) HasFact(ctx context.Context, workspaceID, content string) bool {
	if m.store == nil {
		return false
	}
	if m.findOverlappingEntry(ctx, workspaceID, deriveTitle(content), content) != nil {
		return true
	}
	exists, err := m.store.Exists(ctx, workspaceID, content)
	return err == nil && exists
}

type saveRequest struct {
	workspaceID string
	content     string
	tags        []string
	memType     memory.MemoryType
	source      string
	onOverlap   overlapPolicy
}

// insertFact saves content with Jaccard similarity dedup and exact content match; req.onOverlap says what a similar fact does.
// The title is auto-derived from content so the model doesn't need to provide one.
func (m *MemoryToolProvider) insertFact(ctx context.Context, req saveRequest) (SaveResult, error) {
	title := deriveTitle(req.content)

	existing := m.findOverlappingEntry(ctx, req.workspaceID, title, req.content)
	if existing != nil {
		if existing.Content == req.content {
			logging.Info("memory_update result", "action", "already_saved", "existing_id", existing.ID, "match", "jaccard")
			return SaveResult{Outcome: SaveDuplicate, ID: existing.ID, Type: req.memType}, nil
		}
		switch req.onOverlap {
		case overlapReport:
			logging.Info("memory_update result", "action", "conflict", "existing_id", existing.ID, "match", "jaccard")
			return SaveResult{Outcome: SaveConflict, ID: existing.ID, Type: req.memType, Existing: existing.Content}, nil
		case overlapReplace:
			if err := m.store.Update(ctx, req.workspaceID, existing.ID, title, req.content, req.tags, memory.MergeTags); err != nil {
				return SaveResult{}, fmt.Errorf("memory update failed: %w", err)
			}
			logging.Info("memory_update result", "action", "replaced", "existing_id", existing.ID, "match", "jaccard")
			return SaveResult{Outcome: SaveUpdated, ID: existing.ID, Type: req.memType}, nil
		}
	}

	exists, err := m.store.Exists(ctx, req.workspaceID, req.content)
	if err != nil {
		return SaveResult{}, fmt.Errorf("memory update failed: %w", err)
	}
	if exists {
		logging.Info("memory_update result", "action", "already_saved", "match", "content_dup")
		return SaveResult{Outcome: SaveDuplicate, Type: req.memType}, nil
	}
	source := req.source
	if source == "" {
		source = memorySource(ctx)
	}
	id, err := m.store.Insert(ctx, req.workspaceID, req.memType, title, req.content, req.tags, source)
	if err != nil {
		return SaveResult{}, fmt.Errorf("memory update failed: %w", err)
	}
	logging.Info("memory_update result", "action", "created", "id", id, "memory_type", string(req.memType))
	return SaveResult{Outcome: SaveCreated, ID: id, Type: req.memType}, nil
}

// withRoutingDefaults fills each omitted routing field on its own; what the model did state is kept.
func withRoutingDefaults(ctx context.Context, scope memory.Scope, mode memory.Mode, keep memory.Keep) (memory.Scope, memory.Mode, memory.Keep) {
	if scope == "" {
		scope = memory.ScopeWorkspace
	}
	if mode == "" {
		mode = memory.ModeOnDemand
	}
	if keep == "" {
		keep = defaultKeep(ctx)
	}
	return scope, mode, keep
}

// defaultKeep is the retention an unspecified memory_update gets. Unattended
// (automation) runs write with nobody watching, so their default is session
// scope — the retention reaper clears it — while a deliberate keep=permanent
// is still honoured.
func defaultKeep(ctx context.Context) memory.Keep {
	if models.IsUnattendedRun(ctx) {
		return memory.KeepSession
	}
	return memory.KeepPermanent
}

// memorySource attributes a saved entry: "run:<id>" for unattended runs so the
// operator can trace where a fact came from, "agent" otherwise.
func memorySource(ctx context.Context) string {
	if id := models.GetRunID(ctx); id != "" && models.IsUnattendedRun(ctx) {
		return memorySourceRunPrefix + id
	}
	return memorySourceAgent
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
