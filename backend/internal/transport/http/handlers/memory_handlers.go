package handlers

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/orchestrator"
	"llm-proxy/internal/core/tools"
	"llm-proxy/internal/platform/memory"
	"llm-proxy/models"
)

const (
	// maxMemoryContentChars bounds an operator-written fact. A hot fact rides in
	// every prompt, so an unbounded one would only get clipped by the budget.
	maxMemoryContentChars = 2000
	// maxMemoryTitleChars matches the title length the agent tool derives.
	maxMemoryTitleChars = 60
	// memorySourceOperator attributes entries created from the UI.
	memorySourceOperator = "operator"
	// memorySourceImport attributes facts that arrived through a markdown import.
	memorySourceImport = "import"
	// maxImportFacts bounds one import; maxExportEntries bounds one export side.
	maxImportFacts   = 200
	maxExportEntries = 5000
	// globalMemoryWorkspace is the reserved workspace of user-scope facts; it is
	// readable and editable through the same endpoints but is never a target for
	// creating workspace facts.
	globalMemoryWorkspace = "global"
)

// ModelConfigResolver looks up a registered model's config by name.
type ModelConfigResolver func(name string) (models.ModelConfig, bool)

type MemoryHandlers struct {
	store        *memory.Store
	resolveModel ModelConfigResolver
}

func NewMemoryHandlers(store *memory.Store) *MemoryHandlers {
	return &MemoryHandlers{store: store}
}

// WithModelResolver enables the injection preview to size its block from a
// chosen model's context budget.
func (h *MemoryHandlers) WithModelResolver(resolve ModelConfigResolver) *MemoryHandlers {
	h.resolveModel = resolve
	return h
}

func (h *MemoryHandlers) ListMemories(w http.ResponseWriter, r *http.Request) {
	vals, ok := requirePathParams(w, r, "workspace")
	if !ok {
		return
	}
	wsID := vals[0]
	if r.URL.Query().Get("hot") == "true" {
		// The facts every prompt carries: the injection query itself (workspace +
		// global), uncapped, so a hot fact can never hide behind the page limit.
		h.respondEntries(w, r, wsID)
		return
	}
	memType := memory.MemoryType(r.URL.Query().Get("type"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}

	var entries []memory.MemoryEntry
	var err error
	if r.URL.Query().Get("unused") == "true" {
		// Never sent to a model and never returned by search.
		entries, err = h.store.ListUnused(r.Context(), wsID, limit, offset)
	} else {
		entries, err = h.store.List(r.Context(), wsID, memType, limit, offset)
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("list failed: %v", err))
		return
	}
	if entries == nil {
		entries = []memory.MemoryEntry{}
	}
	respondJSON(w, entries)
}

func (h *MemoryHandlers) SearchMemories(w http.ResponseWriter, r *http.Request) {
	vals, ok := requirePathParams(w, r, "workspace")
	if !ok {
		return
	}
	wsID := vals[0]

	var req struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Query == "" {
		writeJSONError(w, http.StatusBadRequest, "query is required")
		return
	}
	if req.Limit <= 0 || req.Limit > 100 {
		req.Limit = 20
	}

	entries, err := h.store.Search(r.Context(), wsID, req.Query, req.Limit)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("search failed: %v", err))
		return
	}
	if entries == nil {
		entries = []memory.MemoryEntry{}
	}
	respondJSON(w, entries)
}

func (h *MemoryHandlers) GetMemory(w http.ResponseWriter, r *http.Request) {
	vals, ok := requirePathParamsMsg(w, r, "workspace and id are required", "workspace", "id")
	if !ok {
		return
	}
	wsID, idStr := vals[0], vals[1]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid id")
		return
	}

	entry, err := h.store.Get(r.Context(), wsID, id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("get failed: %v", err))
		return
	}
	if entry == nil {
		writeJSONError(w, http.StatusNotFound, "memory not found")
		return
	}
	respondJSON(w, entry)
}

func (h *MemoryHandlers) UpdateMemory(w http.ResponseWriter, r *http.Request) {
	vals, ok := requirePathParamsMsg(w, r, "workspace and id are required", "workspace", "id")
	if !ok {
		return
	}
	wsID, idStr := vals[0], vals[1]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid id")
		return
	}

	var req struct {
		Title   string `json:"title"`
		Content string `json:"content"`
		// Hot, when present, promotes (true) or demotes (false) the entry in the
		// every-prompt set. Absent leaves it as it was: an edit of the wording
		// must never change whether the model sees the fact.
		Hot *bool `json:"hot"`
		// Priority (0 low, 1 normal, 2 high), when present, sets how long the
		// fact survives when the budget cuts the tail. Validated before any write.
		Priority *int `json:"priority"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Priority != nil && !memory.ValidPriority(*req.Priority) {
		writeJSONError(w, http.StatusBadRequest, memory.ErrInvalidPriority.Error())
		return
	}

	existing, err := h.store.Get(r.Context(), wsID, id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("update failed: %v", err))
		return
	}
	if existing == nil {
		writeJSONError(w, http.StatusNotFound, "memory not found")
		return
	}
	tags := existing.Tags
	if req.Hot != nil {
		tags = memory.WithHot(tags, *req.Hot)
	}

	if err := h.store.Update(r.Context(), wsID, id, req.Title, req.Content, tags, memory.ReplaceTags); err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("update failed: %v", err))
		return
	}
	if req.Priority != nil {
		if err := h.store.SetPriority(r.Context(), wsID, id, *req.Priority); err != nil {
			writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("update failed: %v", err))
			return
		}
	}
	respondJSON(w, map[string]string{"status": "updated"})
}

func (h *MemoryHandlers) DeleteMemory(w http.ResponseWriter, r *http.Request) {
	vals, ok := requirePathParamsMsg(w, r, "workspace and id are required", "workspace", "id")
	if !ok {
		return
	}
	wsID, idStr := vals[0], vals[1]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid id")
		return
	}

	if err := h.store.Delete(r.Context(), wsID, id); err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("delete failed: %v", err))
		return
	}
	respondJSON(w, map[string]string{"status": "deleted"})
}

func (h *MemoryHandlers) ClearWorkspace(w http.ResponseWriter, r *http.Request) {
	vals, ok := requirePathParams(w, r, "workspace")
	if !ok {
		return
	}
	wsID := vals[0]
	memType := memory.MemoryType(r.URL.Query().Get("type"))

	n, err := h.store.DeleteAllByWorkspace(r.Context(), wsID, memType)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("clear failed: %v", err))
		return
	}
	respondJSON(w, map[string]any{"status": "cleared", "deleted": n})
}

// respondEntries writes the hot (always-injected) entries visible to a workspace.
func (h *MemoryHandlers) respondEntries(w http.ResponseWriter, r *http.Request, wsID string) {
	entries, err := h.store.SearchHot(r.Context(), wsID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("list failed: %v", err))
		return
	}
	if entries == nil {
		entries = []memory.MemoryEntry{}
	}
	respondJSON(w, entries)
}

// injectionPreviewResponse is HotMemoryPreview plus how the model was resolved.
type injectionPreviewResponse struct {
	assistant.HotMemoryPreview
	Model          string `json:"model"`
	BudgetResolved bool   `json:"budget_resolved"`
}

// InjectionPreview returns exactly the <memory> block a run of the chosen model
// would receive — same query, same builder (assistant.PreviewHotMemory), same
// budget derivation as the agent — and which facts were cut. With no model the
// budget is the unresolved fallback, and the response says so.
func (h *MemoryHandlers) InjectionPreview(w http.ResponseWriter, r *http.Request) {
	vals, ok := requirePathParams(w, r, "workspace")
	if !ok {
		return
	}
	wsID := vals[0]

	var opts assistant.AgentOptions
	resp := injectionPreviewResponse{Model: r.URL.Query().Get("model")}
	if resp.Model != "" {
		cfg, found := h.lookupModel(resp.Model)
		if !found {
			writeJSONError(w, http.StatusNotFound, fmt.Sprintf("unknown model %q", resp.Model))
			return
		}
		if cfg.ContextBudget == 0 {
			orchestrator.ApplyMetadataDefaults(&cfg)
		}
		opts.ApplyModelConfig(cfg)
		resp.BudgetResolved = opts.ContextBudget > 0
	}

	entries, err := h.store.SearchHot(r.Context(), wsID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("preview failed: %v", err))
		return
	}
	notes, err := h.store.OperatorNotes(r.Context(), wsID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("preview failed: %v", err))
		return
	}
	resp.HotMemoryPreview = assistant.PreviewHotMemory(entries, notes, opts)
	respondJSON(w, resp)
}

func (h *MemoryHandlers) lookupModel(name string) (models.ModelConfig, bool) {
	if h.resolveModel == nil {
		return models.ModelConfig{}, false
	}
	return h.resolveModel(name)
}

// newMemoryRequest is the body of POST /admin/api/memory/{workspace}.
type newMemoryRequest struct {
	Title   string       `json:"title"`
	Content string       `json:"content"`
	Scope   memory.Scope `json:"scope"`
	Mode    memory.Mode  `json:"mode"`
	Keep    memory.Keep  `json:"keep"`
	// Priority is optional (default normal); see memory.Priority*.
	Priority *int `json:"priority"`
	// source attributes the stored fact; empty means the operator typed it in.
	// Unexported, so a request body can never set it.
	source string
}

// route validates the request and resolves it with the agent tool's own table,
// so a fact added here is stored exactly as if the agent had saved it with the
// same choices. Unset choices default to workspace / on demand / permanent.
func (q *newMemoryRequest) route(wsID string) (tools.MemoryRoute, error) {
	q.Content = strings.TrimSpace(q.Content)
	if q.Content == "" || len(q.Content) > maxMemoryContentChars {
		return tools.MemoryRoute{}, fmt.Errorf("content is required and at most %d characters", maxMemoryContentChars)
	}
	if q.Priority != nil && !memory.ValidPriority(*q.Priority) {
		return tools.MemoryRoute{}, memory.ErrInvalidPriority
	}
	if q.Scope == "" {
		q.Scope = memory.ScopeWorkspace
	}
	if q.Mode == "" {
		q.Mode = memory.ModeOnDemand
	}
	if q.Keep == "" {
		q.Keep = memory.KeepPermanent
	}
	return tools.ResolveMemoryRoute(q.Scope, q.Mode, q.Keep, wsID)
}

// derivedTitle is the title used when the operator gives none: the start of the fact.
func (q *newMemoryRequest) derivedTitle() string {
	if title := strings.TrimSpace(q.Title); title != "" {
		return title
	}
	return q.Content[:min(len(q.Content), maxMemoryTitleChars)]
}

// saveNew persists a validated operator fact and returns the stored entry; on
// failure the int is the HTTP status to report.
func (h *MemoryHandlers) saveNew(ctx context.Context, route tools.MemoryRoute, req *newMemoryRequest) (*memory.MemoryEntry, int, error) {
	exists, err := h.store.Exists(ctx, route.WorkspaceID, req.Content)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("create failed: %w", err)
	}
	if exists {
		return nil, http.StatusConflict, errors.New("that fact is already saved")
	}
	id, err := h.store.Insert(ctx, route.WorkspaceID, memory.MemoryType(route.MemoryType), req.derivedTitle(), req.Content, route.Tags, cmp.Or(req.source, memorySourceOperator))
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("create failed: %w", err)
	}
	if req.Priority != nil && *req.Priority != memory.PriorityNormal {
		if err := h.store.SetPriority(ctx, route.WorkspaceID, id, *req.Priority); err != nil {
			return nil, http.StatusInternalServerError, fmt.Errorf("create failed: %w", err)
		}
	}
	entry, err := h.store.Get(ctx, route.WorkspaceID, id)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("create failed: %w", err)
	}
	return entry, http.StatusCreated, nil
}

// CreateMemory saves an operator-written fact (201 with the stored entry).
func (h *MemoryHandlers) CreateMemory(w http.ResponseWriter, r *http.Request) {
	vals, ok := requirePathParams(w, r, "workspace")
	if !ok {
		return
	}
	wsID := vals[0]
	if !validateID(wsID) || wsID == globalMemoryWorkspace {
		writeJSONError(w, http.StatusBadRequest, "invalid workspace")
		return
	}

	var req newMemoryRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	route, err := req.route(wsID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	entry, status, err := h.saveNew(r.Context(), route, &req)
	if err != nil {
		writeJSONError(w, status, err.Error())
		return
	}
	// Content-Type must be set before WriteHeader, or it is silently dropped.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(entry)
}

// notesResponse is the operator's MEMORY.md for one workspace plus the global one.
type notesResponse struct {
	memory.OperatorNotes
	MaxChars int `json:"max_chars"`
}

// GetNotes returns the global and the workspace operator notes (MEMORY.md).
func (h *MemoryHandlers) GetNotes(w http.ResponseWriter, r *http.Request) {
	vals, ok := requirePathParams(w, r, "workspace")
	if !ok {
		return
	}
	if !validateID(vals[0]) {
		writeJSONError(w, http.StatusBadRequest, "invalid workspace")
		return
	}
	notes, err := h.store.OperatorNotes(r.Context(), vals[0])
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("read notes failed: %v", err))
		return
	}
	respondJSON(w, notesResponse{OperatorNotes: notes, MaxChars: memory.MaxOperatorNotesChars})
}

// PutNotes replaces one notes file: {"scope":"workspace"|"global","content":"…"}.
// Blank content removes the file. Notes go at the top of every run's memory
// block, so the body must be JSON (a cross-site form cannot send that without a
// preflight) and is bounded.
func (h *MemoryHandlers) PutNotes(w http.ResponseWriter, r *http.Request) {
	vals, ok := requirePathParams(w, r, "workspace")
	if !ok {
		return
	}
	wsID := vals[0]
	if !validateID(wsID) {
		writeJSONError(w, http.StatusBadRequest, "invalid workspace")
		return
	}
	var req struct {
		Scope   memory.NotesScope `json:"scope"`
		Content string            `json:"content"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Scope != memory.NotesWorkspace && req.Scope != memory.NotesGlobal {
		writeJSONError(w, http.StatusBadRequest, `scope must be "workspace" or "global"`)
		return
	}

	switch err := h.store.SetOperatorNotes(r.Context(), req.Scope, wsID, req.Content); {
	case err == nil:
		respondJSON(w, map[string]string{"status": "saved"})
	case errors.Is(err, memory.ErrNotesTooLong), errors.Is(err, memory.ErrInvalidNotesWorkspace):
		writeJSONError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, memory.ErrNotesUnavailable):
		writeJSONError(w, http.StatusServiceUnavailable, err.Error())
	default:
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("save notes failed: %v", err))
	}
}

// ExportMemory downloads the workspace's facts and the user-wide facts as one
// markdown file (oldest first, so a re-import keeps the recency order).
func (h *MemoryHandlers) ExportMemory(w http.ResponseWriter, r *http.Request) {
	vals, ok := requirePathParams(w, r, "workspace")
	if !ok {
		return
	}
	wsID := vals[0]
	if !validateID(wsID) || wsID == globalMemoryWorkspace {
		writeJSONError(w, http.StatusBadRequest, "invalid workspace")
		return
	}
	var entries []memory.MemoryEntry
	for _, scope := range []string{wsID, globalMemoryWorkspace} {
		listed, err := h.store.List(r.Context(), scope, "", maxExportEntries, 0)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("export failed: %v", err))
			return
		}
		slices.Reverse(listed) // List is newest-first
		entries = append(entries, listed...)
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="memory-%s.md"`, wsID))
	_, _ = w.Write([]byte(memory.FormatMarkdown(wsID, entries)))
}

// importResult reports what an import did; bad entries are skipped, not fatal.
type importResult struct {
	Created int                 `json:"created"`
	Skipped int                 `json:"skipped"` // already saved
	Issues  []memory.ParseIssue `json:"issues"`
}

// ImportMemory adds the facts of a markdown file (see memory.ParseMarkdown).
// Each fact is validated and routed exactly like a fact added by hand; duplicates
// are skipped and unusable entries are reported with their line. The body must
// be JSON — a cross-site form cannot send that without a preflight — so a web
// page cannot plant facts that ride in every prompt.
func (h *MemoryHandlers) ImportMemory(w http.ResponseWriter, r *http.Request) {
	vals, ok := requirePathParams(w, r, "workspace")
	if !ok {
		return
	}
	wsID := vals[0]
	if !validateID(wsID) || wsID == globalMemoryWorkspace {
		writeJSONError(w, http.StatusBadRequest, "invalid workspace")
		return
	}
	var req struct {
		Markdown string `json:"markdown"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	facts, issues := memory.ParseMarkdown(req.Markdown)
	switch {
	case len(facts)+len(issues) == 0:
		writeJSONError(w, http.StatusBadRequest, "no facts found: each fact starts with a \"### Title\" heading")
		return
	case len(facts)+len(issues) > maxImportFacts:
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("too many facts: at most %d per import", maxImportFacts))
		return
	}
	result := h.importFacts(r.Context(), wsID, facts)
	result.Issues = append(issues, result.Issues...)
	respondJSON(w, result)
}

func (h *MemoryHandlers) importFacts(ctx context.Context, wsID string, facts []memory.Fact) importResult {
	result := importResult{Issues: []memory.ParseIssue{}}
	for _, f := range facts {
		priority := f.Priority
		req := newMemoryRequest{Title: f.Title, Content: f.Content, Scope: f.Scope, Mode: f.Mode, Keep: f.Keep, Priority: &priority, source: memorySourceImport}
		route, err := req.route(wsID)
		if err != nil {
			result.Issues = append(result.Issues, memory.ParseIssue{Line: f.Line, Message: err.Error()})
			continue
		}
		switch _, status, err := h.saveNew(ctx, route, &req); {
		case err == nil:
			result.Created++
		case status == http.StatusConflict:
			result.Skipped++
		default:
			result.Issues = append(result.Issues, memory.ParseIssue{Line: f.Line, Message: err.Error()})
		}
	}
	return result
}
