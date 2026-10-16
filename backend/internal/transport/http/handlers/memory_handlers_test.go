package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/platform/db"
	"llm-proxy/internal/platform/memory"
	"llm-proxy/models"
)

func newTestMemoryStore(t *testing.T) *memory.Store {
	t.Helper()
	f, err := os.CreateTemp("", "memory-handler-test-*.db")
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

func seedMemories(t *testing.T, store *memory.Store) {
	t.Helper()
	ctx := context.Background()
	store.Insert(ctx, memory.NewEntry{WorkspaceID: "ws-1", MemoryType: memory.LongTerm, Title: "topic-a", Content: "content a", Tags: nil, Source: "agent"})
	store.Insert(ctx, memory.NewEntry{WorkspaceID: "ws-1", MemoryType: memory.Daily, Title: "topic-b", Content: "content b", Tags: nil, Source: "agent"})
	store.Insert(ctx, memory.NewEntry{WorkspaceID: "ws-2", MemoryType: memory.LongTerm, Title: "topic-c", Content: "content c", Tags: nil, Source: "agent"})
}

func TestClearWorkspace_All(t *testing.T) {
	store := newTestMemoryStore(t)
	seedMemories(t, store)
	handlers := NewMemoryHandlers(store)

	req := httptest.NewRequest("DELETE", "/admin/api/memory/ws-1", nil)
	req.SetPathValue("workspace", "ws-1")
	rr := httptest.NewRecorder()
	handlers.ClearWorkspace(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	entries, _ := store.List(context.Background(), "ws-1", "", 10, 0)
	if len(entries) != 0 {
		t.Errorf("expected 0 entries in ws-1 after clear, got %d", len(entries))
	}

	ws2Entries, _ := store.List(context.Background(), "ws-2", "", 10, 0)
	if len(ws2Entries) != 1 {
		t.Errorf("expected ws-2 untouched (1 entry), got %d", len(ws2Entries))
	}
}

func TestClearWorkspace_ByType(t *testing.T) {
	store := newTestMemoryStore(t)
	seedMemories(t, store)
	handlers := NewMemoryHandlers(store)

	req := httptest.NewRequest("DELETE", "/admin/api/memory/ws-1?type=daily", nil)
	req.SetPathValue("workspace", "ws-1")
	rr := httptest.NewRecorder()
	handlers.ClearWorkspace(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	entries, _ := store.List(context.Background(), "ws-1", "", 10, 0)
	if len(entries) != 1 {
		t.Fatalf("expected 1 remaining entry in ws-1, got %d", len(entries))
	}
	if entries[0].MemoryType != memory.LongTerm {
		t.Errorf("expected remaining entry to be long_term, got %s", entries[0].MemoryType)
	}
}

func TestClearWorkspace_MissingWorkspace(t *testing.T) {
	store := newTestMemoryStore(t)
	handlers := NewMemoryHandlers(store)

	req := httptest.NewRequest("DELETE", "/admin/api/memory/", nil)
	req.SetPathValue("workspace", "")
	rr := httptest.NewRecorder()
	handlers.ClearWorkspace(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing workspace, got %d", rr.Code)
	}
}

func putMemory(t *testing.T, h *MemoryHandlers, ws string, id int64, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("PUT", fmt.Sprintf("/admin/api/memory/%s/%d", ws, id), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("workspace", ws)
	req.SetPathValue("id", fmt.Sprint(id))
	rr := httptest.NewRecorder()
	h.UpdateMemory(rr, req)
	return rr
}

// Editing a fact's wording must not change whether the model sees it: the
// "hot" tag is what puts an entry in every prompt, and the editor never sends
// tags. Before this fix an edit silently demoted a hot fact to on-demand.
func TestUpdateMemory_EditingTextKeepsTags(t *testing.T) {
	store := newTestMemoryStore(t)
	id, err := store.Insert(context.Background(), memory.NewEntry{WorkspaceID: "ws-1", MemoryType: memory.LongTerm, Title: "build", Content: "run go build", Tags: []string{"hot", "ci"}, Source: "agent"})
	if err != nil {
		t.Fatal(err)
	}

	rr := putMemory(t, NewMemoryHandlers(store), "ws-1", id, `{"title":"build","content":"run go build ./..."}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	got, _ := store.Get(context.Background(), "ws-1", id)
	if got.Content != "run go build ./..." {
		t.Errorf("content not updated: %q", got.Content)
	}
	if !slices.Contains(got.Tags, "hot") || !slices.Contains(got.Tags, "ci") {
		t.Errorf("tags = %v, want hot and ci preserved", got.Tags)
	}
}

func TestUpdateMemory_HotFlagTogglesOnlyTheHotTag(t *testing.T) {
	cases := []struct {
		name     string
		start    []string
		body     string
		wantTags []string
	}{
		{"promote to hot", []string{"ci"}, `{"title":"t","content":"c","hot":true}`, []string{"ci", "hot"}},
		{"demote from hot", []string{"hot", "ci"}, `{"title":"t","content":"c","hot":false}`, []string{"ci"}},
		{"promote is idempotent", []string{"hot"}, `{"title":"t","content":"c","hot":true}`, []string{"hot"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newTestMemoryStore(t)
			id, _ := store.Insert(context.Background(), memory.NewEntry{WorkspaceID: "ws-1", MemoryType: memory.LongTerm, Title: "t", Content: "c", Tags: tc.start, Source: "agent"})
			if rr := putMemory(t, NewMemoryHandlers(store), "ws-1", id, tc.body); rr.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
			}
			got, _ := store.Get(context.Background(), "ws-1", id)
			slices.Sort(got.Tags)
			if !slices.Equal(got.Tags, tc.wantTags) {
				t.Errorf("tags = %v, want %v", got.Tags, tc.wantTags)
			}
		})
	}
}

func TestUpdateMemory_MissingEntryIs404(t *testing.T) {
	rr := putMemory(t, NewMemoryHandlers(newTestMemoryStore(t)), "ws-1", 999, `{"title":"t","content":"c"}`)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func seedHot(t *testing.T, store *memory.Store, ws string, n int) {
	t.Helper()
	for i := range n {
		if _, err := store.Insert(context.Background(), memory.NewEntry{WorkspaceID: ws, MemoryType: memory.LongTerm, Title: fmt.Sprintf("fact-%02d", i), Content: strings.Repeat("x", 150), Tags: []string{memory.HotTag}, Source: "agent"}); err != nil {
			t.Fatal(err)
		}
	}
}

func modelResolver(cfgs ...models.ModelConfig) func(string) (models.ModelConfig, bool) {
	return func(name string) (models.ModelConfig, bool) {
		for _, c := range cfgs {
			if c.Name == name {
				return c, true
			}
		}
		return models.ModelConfig{}, false
	}
}

func getPreview(h *MemoryHandlers, ws, query string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "/admin/api/memory/"+ws+"/injection-preview"+query, nil)
	req.SetPathValue("workspace", ws)
	rr := httptest.NewRecorder()
	h.InjectionPreview(rr, req)
	return rr
}

// previewBody is the handler's own response type, so the test decodes exactly what is served.
type previewBody = injectionPreviewResponse

func decodePreview(t *testing.T, rr *httptest.ResponseRecorder) previewBody {
	t.Helper()
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var p previewBody
	if err := json.Unmarshal(rr.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return p
}

// The preview must use the model's own window: an 8K and a 16K model see
// different blocks from the same facts.
func TestInjectionPreview_UsesTheChosenModelsBudget(t *testing.T) {
	store := newTestMemoryStore(t)
	seedHot(t, store, "ws-1", 60)
	h := NewMemoryHandlers(store).WithModelResolver(modelResolver(
		models.ModelConfig{Name: "small", ContextBudget: 21848, WorkloadClass: models.WorkloadLocal},
		models.ModelConfig{Name: "big", ContextBudget: 57344, WorkloadClass: models.WorkloadLocal},
	))

	small := decodePreview(t, getPreview(h, "ws-1", "?model=small"))
	big := decodePreview(t, getPreview(h, "ws-1", "?model=big"))

	if !small.BudgetResolved || small.Model != "small" || small.ContextBudgetChars != 21848 {
		t.Errorf("small model not resolved: %+v", small)
	}
	if big.Chars <= small.Chars || len(big.Included) <= len(small.Included) {
		t.Errorf("a 16K window must admit more: small %d chars/%d facts, big %d chars/%d facts",
			small.Chars, len(small.Included), big.Chars, len(big.Included))
	}
	if len(small.Cut) == 0 {
		t.Error("60 facts cannot all fit an 8K window; the preview must list what was cut")
	}
}

func TestInjectionPreview_UnknownModelIs404AndNoModelSaysUnresolved(t *testing.T) {
	store := newTestMemoryStore(t)
	seedHot(t, store, "ws-1", 2)
	h := NewMemoryHandlers(store).WithModelResolver(modelResolver())

	if rr := getPreview(h, "ws-1", "?model=ghost"); rr.Code != http.StatusNotFound {
		t.Errorf("unknown model: expected 404, got %d", rr.Code)
	}
	p := decodePreview(t, getPreview(h, "ws-1", ""))
	if p.BudgetResolved {
		t.Error("no model chosen: the budget is a fallback and must say so")
	}
	if len(p.Included) != 2 {
		t.Errorf("both facts fit the fallback budget, got %d", len(p.Included))
	}
}

// User-profile facts live under workspace "global" and are injected into every
// workspace, so the preview must show them.
func TestInjectionPreview_IncludesGlobalUserFacts(t *testing.T) {
	store := newTestMemoryStore(t)
	seedHot(t, store, "ws-1", 1)
	if _, err := store.Insert(context.Background(), memory.NewEntry{WorkspaceID: "global", MemoryType: memory.UserProfile, Title: "name", Content: "Alice", Tags: []string{memory.HotTag}, Source: "agent"}); err != nil {
		t.Fatal(err)
	}
	p := decodePreview(t, getPreview(NewMemoryHandlers(store), "ws-1", ""))
	if !strings.Contains(p.Block, "Alice") || len(p.Included) != 2 {
		t.Errorf("global hot fact missing from preview: %+v", p.Block)
	}
}

// GET /{workspace}/{id} already exists; the literal preview path must win.
func TestInjectionPreview_RouteBeatsMemoryIDRoute(t *testing.T) {
	store := newTestMemoryStore(t)
	h := NewMemoryHandlers(store)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin/api/memory/{workspace}/{id}", h.GetMemory)
	mux.HandleFunc("GET /admin/api/memory/{workspace}/injection-preview", h.InjectionPreview)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("GET", "/admin/api/memory/ws-1/injection-preview", nil))
	if rr.Code != http.StatusOK || strings.Contains(rr.Body.String(), "invalid id") {
		t.Errorf("preview request reached GetMemory: %d %s", rr.Code, rr.Body.String())
	}
}

func postMemory(h *MemoryHandlers, ws, contentType, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/admin/api/memory/"+ws, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.SetPathValue("workspace", ws)
	rr := httptest.NewRecorder()
	h.CreateMemory(rr, req)
	return rr
}

func TestCreateMemory_RoutesLikeTheAgentTool(t *testing.T) {
	cases := []struct {
		name       string
		body       string
		wantWS     string
		wantType   memory.MemoryType
		wantHot    bool
		wantSource string
	}{
		{"defaults: workspace, on demand, permanent", `{"content":"deploys go through staging"}`, "ws-1", memory.LongTerm, false, "operator"},
		{"always = hot", `{"content":"use tabs","mode":"always"}`, "ws-1", memory.LongTerm, true, "operator"},
		{"this conversation only", `{"content":"scratch","keep":"session"}`, "ws-1", memory.Session, false, "operator"},
		{"user scope lives in global", `{"content":"likes tea","scope":"user","mode":"always"}`, "global", memory.UserProfile, true, "operator"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newTestMemoryStore(t)
			rr := postMemory(NewMemoryHandlers(store), "ws-1", "application/json", tc.body)
			if rr.Code != http.StatusCreated {
				t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
			}
			var created memory.MemoryEntry
			if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
				t.Fatal(err)
			}
			got, _ := store.Get(context.Background(), tc.wantWS, created.ID)
			if got == nil || got.MemoryType != tc.wantType || got.IsHot() != tc.wantHot || got.Source != tc.wantSource {
				t.Errorf("stored %+v, want ws=%s type=%s hot=%v source=%s", got, tc.wantWS, tc.wantType, tc.wantHot, tc.wantSource)
			}
		})
	}
}

func TestCreateMemory_RespondsWithJSONContentType(t *testing.T) {
	rr := postMemory(NewMemoryHandlers(newTestMemoryStore(t)), "ws-1", "application/json", `{"content":"a fact"}`)
	if got := rr.Result().Header.Get("Content-Type"); got != "application/json" { // Result() snapshots at WriteHeader like a real server
		t.Errorf("Content-Type = %q, want application/json", got)
	}
}

func TestCreateMemory_Validation(t *testing.T) {
	long := `{"content":"` + strings.Repeat("x", maxMemoryContentChars+1) + `"}`
	cases := []struct {
		name        string
		ws          string
		contentType string
		body        string
		want        int
	}{
		{"empty content", "ws-1", "application/json", `{"content":"  "}`, http.StatusBadRequest},
		{"too long", "ws-1", "application/json", long, http.StatusBadRequest},
		{"bad combination", "ws-1", "application/json", `{"content":"x","scope":"user","mode":"always","keep":"session"}`, http.StatusBadRequest},
		{"unknown scope", "ws-1", "application/json", `{"content":"x","scope":"galaxy"}`, http.StatusBadRequest},
		{"global is not a workspace", "global", "application/json", `{"content":"x"}`, http.StatusBadRequest},
		{"bad workspace id", "../etc", "application/json", `{"content":"x"}`, http.StatusBadRequest},
		// A cross-site form can send text/plain without a CORS preflight; refusing
		// it stops a web page planting a fact that goes into every prompt.
		{"non-JSON content type", "ws-1", "text/plain", `{"content":"x"}`, http.StatusUnsupportedMediaType},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := postMemory(NewMemoryHandlers(newTestMemoryStore(t)), tc.ws, tc.contentType, tc.body)
			if rr.Code != tc.want {
				t.Errorf("expected %d, got %d: %s", tc.want, rr.Code, rr.Body.String())
			}
		})
	}
}

func TestCreateMemory_DuplicateIs409(t *testing.T) {
	h := NewMemoryHandlers(newTestMemoryStore(t))
	body := `{"content":"same fact"}`
	if rr := postMemory(h, "ws-1", "application/json", body); rr.Code != http.StatusCreated {
		t.Fatalf("first create: %d", rr.Code)
	}
	if rr := postMemory(h, "ws-1", "application/json", body); rr.Code != http.StatusConflict {
		t.Errorf("duplicate: expected 409, got %d", rr.Code)
	}
}

// The hot filter must come from the injection query, not from the capped
// list: a hot fact beyond the first 50 entries still has to show up.
func TestListMemories_HotFilterUsesTheInjectionQuery(t *testing.T) {
	store := newTestMemoryStore(t)
	ctx := context.Background()
	seedHot(t, store, "ws-1", 1)
	store.Insert(ctx, memory.NewEntry{WorkspaceID: "global", MemoryType: memory.UserProfile, Title: "name", Content: "Alice", Tags: []string{memory.HotTag}, Source: "agent"})
	for i := range 60 {
		store.Insert(ctx, memory.NewEntry{WorkspaceID: "ws-1", MemoryType: memory.LongTerm, Title: fmt.Sprintf("cold-%d", i), Content: "not hot", Tags: nil, Source: "agent"})
	}

	req := httptest.NewRequest("GET", "/admin/api/memory/ws-1?hot=true", nil)
	req.SetPathValue("workspace", "ws-1")
	rr := httptest.NewRecorder()
	NewMemoryHandlers(store).ListMemories(rr, req)

	var got []memory.MemoryEntry
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("hot=true must return exactly the 2 injected facts (workspace + global), got %d", len(got))
	}
}

// ── Operator notes (MEMORY.md) ──────────────────────────────────────────────

func withNotes(t *testing.T, store *memory.Store) {
	t.Helper()
	root := t.TempDir()
	store.SetNotesLocations(filepath.Join(root, "MEMORY.md"), func(ws string) string { return filepath.Join(root, "meta", ws, "MEMORY.md") })
}

func notesRequest(h *MemoryHandlers, method, ws, contentType, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/admin/api/memory/"+ws+"/notes", strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.SetPathValue("workspace", ws)
	rr := httptest.NewRecorder()
	if method == "GET" {
		h.GetNotes(rr, req)
	} else {
		h.PutNotes(rr, req)
	}
	return rr
}

type notesBody struct {
	Global    string `json:"global"`
	Workspace string `json:"workspace"`
	MaxChars  int    `json:"max_chars"`
}

func TestNotes_SaveThenReadBackPerScope(t *testing.T) {
	store := newTestMemoryStore(t)
	withNotes(t, store)
	h := NewMemoryHandlers(store)

	if rr := notesRequest(h, "PUT", "ws-1", "application/json", `{"scope":"workspace","content":"Always use tabs."}`); rr.Code != http.StatusOK {
		t.Fatalf("save workspace notes: %d %s", rr.Code, rr.Body.String())
	}
	if rr := notesRequest(h, "PUT", "ws-1", "application/json", `{"scope":"global","content":"British English."}`); rr.Code != http.StatusOK {
		t.Fatalf("save global notes: %d %s", rr.Code, rr.Body.String())
	}

	rr := notesRequest(h, "GET", "ws-1", "", "")
	var got notesBody
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil || rr.Code != http.StatusOK {
		t.Fatalf("read notes: %d %v", rr.Code, err)
	}
	if got.Workspace != "Always use tabs." || got.Global != "British English." || got.MaxChars != memory.MaxOperatorNotesChars {
		t.Errorf("notes = %+v", got)
	}
	var other notesBody
	json.Unmarshal(notesRequest(h, "GET", "ws-2", "", "").Body.Bytes(), &other)
	if other.Workspace != "" || other.Global != "British English." {
		t.Errorf("workspace notes are per workspace, global is shared: %+v", other)
	}
}

func TestNotes_Validation(t *testing.T) {
	long := `{"scope":"workspace","content":"` + strings.Repeat("x", memory.MaxOperatorNotesChars+1) + `"}`
	cases := []struct {
		name, ws, contentType, body string
		want                        int
	}{
		{"too long", "ws-1", "application/json", long, http.StatusBadRequest},
		{"unknown scope", "ws-1", "application/json", `{"scope":"galaxy","content":"x"}`, http.StatusBadRequest},
		{"bad workspace id", "../etc", "application/json", `{"scope":"workspace","content":"x"}`, http.StatusBadRequest},
		// A cross-site form can send text/plain without a preflight; refusing it
		// stops a web page rewriting what goes into every prompt.
		{"non-JSON content type", "ws-1", "text/plain", `{"scope":"workspace","content":"x"}`, http.StatusUnsupportedMediaType},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newTestMemoryStore(t)
			withNotes(t, store)
			if rr := notesRequest(NewMemoryHandlers(store), "PUT", tc.ws, tc.contentType, tc.body); rr.Code != tc.want {
				t.Errorf("expected %d, got %d: %s", tc.want, rr.Code, rr.Body.String())
			}
		})
	}
}

func TestNotes_UnavailableIs503(t *testing.T) {
	rr := notesRequest(NewMemoryHandlers(newTestMemoryStore(t)), "PUT", "ws-1", "application/json", `{"scope":"workspace","content":"x"}`)
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when notes are not configured, got %d", rr.Code)
	}
}

// Notes ride in the preview exactly as in a run, share the model's memory budget with the saved facts, and the preview
// says how much of them did not fit.
func TestInjectionPreview_NotesShareTheBudgetAndReportWhatWasCut(t *testing.T) {
	store := newTestMemoryStore(t)
	withNotes(t, store)
	seedHot(t, store, "ws-1", 3)
	big := strings.Repeat("A standing rule that goes on a little.\n", 100) // ~3,900 chars against a ~1,700-char budget
	if err := store.SetOperatorNotes(context.Background(), memory.NotesWorkspace, "ws-1", big); err != nil {
		t.Fatal(err)
	}
	h := NewMemoryHandlers(store).WithModelResolver(modelResolver(models.ModelConfig{Name: "small", ContextBudget: 21848, WorkloadClass: models.WorkloadLocal}))

	p := decodePreview(t, getPreview(h, "ws-1", "?model=small"))
	if p.OperatorChars != len(strings.TrimSpace(big)) || !strings.Contains(p.Block, "A standing rule that goes on a little.") {
		t.Errorf("notes missing from the preview: operator_chars=%d", p.OperatorChars)
	}
	if p.NotesCutChars <= 0 || p.NotesCutChars >= p.OperatorChars {
		t.Errorf("notes_cut_chars must report a partial cut: %d of %d", p.NotesCutChars, p.OperatorChars)
	}
	if p.Chars > p.BudgetChars || len(p.Included) == 0 {
		t.Errorf("the block stays within the budget and keeps room for facts: %d of %d chars, %d facts", p.Chars, p.BudgetChars, len(p.Included))
	}
}

// GET /{workspace}/{id} exists; the literal notes path must win.
func TestNotes_RouteBeatsMemoryIDRoute(t *testing.T) {
	store := newTestMemoryStore(t)
	withNotes(t, store)
	h := NewMemoryHandlers(store)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin/api/memory/{workspace}/{id}", h.GetMemory)
	mux.HandleFunc("GET /admin/api/memory/{workspace}/notes", h.GetNotes)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("GET", "/admin/api/memory/ws-1/notes", nil))
	if rr.Code != http.StatusOK || strings.Contains(rr.Body.String(), "invalid id") {
		t.Errorf("notes request reached GetMemory: %d %s", rr.Code, rr.Body.String())
	}
}

// ── Priority ────────────────────────────────────────────────────────────────

func TestUpdateMemory_SetsPriorityAndEditsKeepIt(t *testing.T) {
	store := newTestMemoryStore(t)
	id, _ := store.Insert(context.Background(), memory.NewEntry{WorkspaceID: "ws-1", MemoryType: memory.LongTerm, Title: "t", Content: "c", Tags: []string{memory.HotTag}, Source: "agent"})
	h := NewMemoryHandlers(store)

	if rr := putMemory(t, h, "ws-1", id, `{"title":"t","content":"c","priority":2}`); rr.Code != http.StatusOK {
		t.Fatalf("set priority: %d %s", rr.Code, rr.Body.String())
	}
	if got, _ := store.Get(context.Background(), "ws-1", id); got.Priority != memory.PriorityHigh {
		t.Errorf("priority = %d, want high", got.Priority)
	}
	// A text-only edit (no priority in the body) must leave it alone.
	if rr := putMemory(t, h, "ws-1", id, `{"title":"t","content":"reworded"}`); rr.Code != http.StatusOK {
		t.Fatalf("edit: %d", rr.Code)
	}
	if got, _ := store.Get(context.Background(), "ws-1", id); got.Priority != memory.PriorityHigh || got.Content != "reworded" {
		t.Errorf("edit changed priority or lost the text: %+v", got)
	}
}

func TestUpdateMemory_InvalidPriorityIs400AndChangesNothing(t *testing.T) {
	store := newTestMemoryStore(t)
	id, _ := store.Insert(context.Background(), memory.NewEntry{WorkspaceID: "ws-1", MemoryType: memory.LongTerm, Title: "t", Content: "c", Tags: nil, Source: "agent"})
	rr := putMemory(t, NewMemoryHandlers(store), "ws-1", id, `{"title":"t","content":"changed","priority":7}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
	if got, _ := store.Get(context.Background(), "ws-1", id); got.Content != "c" {
		t.Errorf("a rejected update must not apply its text either, got %q", got.Content)
	}
}

func TestCreateMemory_AcceptsPriority(t *testing.T) {
	store := newTestMemoryStore(t)
	h := NewMemoryHandlers(store)

	rr := postMemory(h, "ws-1", "application/json", `{"content":"must survive","mode":"always","priority":2}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var created memory.MemoryEntry
	json.Unmarshal(rr.Body.Bytes(), &created)
	if created.Priority != memory.PriorityHigh {
		t.Errorf("created priority = %d, want high", created.Priority)
	}
	if rr := postMemory(h, "ws-1", "application/json", `{"content":"other","priority":9}`); rr.Code != http.StatusBadRequest {
		t.Errorf("invalid priority: expected 400, got %d", rr.Code)
	}
	rr = postMemory(h, "ws-1", "application/json", `{"content":"plain"}`)
	json.Unmarshal(rr.Body.Bytes(), &created)
	if created.Priority != memory.PriorityNormal {
		t.Errorf("default priority = %d, want normal", created.Priority)
	}
}

// ── Usage counters ──────────────────────────────────────────────────────────

func TestListMemories_UnusedFilterAndCountersInTheResponse(t *testing.T) {
	store := newTestMemoryStore(t)
	ctx := context.Background()
	sent, _ := store.Insert(ctx, memory.NewEntry{WorkspaceID: "ws-1", MemoryType: memory.LongTerm, Title: "sent", Content: "x", Tags: []string{memory.HotTag}, Source: "agent"})
	store.Insert(ctx, memory.NewEntry{WorkspaceID: "ws-1", MemoryType: memory.LongTerm, Title: "never", Content: "y", Tags: nil, Source: "agent"})
	e, _ := store.Get(ctx, "ws-1", sent)
	store.RecordInjected([]memory.MemoryEntry{*e})
	store.FlushUsage(ctx)

	list := func(query string) []memory.MemoryEntry {
		req := httptest.NewRequest("GET", "/admin/api/memory/ws-1"+query, nil)
		req.SetPathValue("workspace", "ws-1")
		rr := httptest.NewRecorder()
		NewMemoryHandlers(store).ListMemories(rr, req)
		var got []memory.MemoryEntry
		if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		return got
	}

	unused := list("?unused=true")
	if len(unused) != 1 || unused[0].Title != "never" {
		t.Errorf("unused = %+v, want only 'never'", unused)
	}
	for _, e := range list("") {
		if e.Title == "sent" && (e.InjectedCount != 1 || e.LastUsedAt == "") {
			t.Errorf("counters must be in the list response: %+v", e)
		}
	}
}

// Looking at memory in the UI is not using it.
func TestOperatorBrowsingDoesNotCountAsUse(t *testing.T) {
	store := newTestMemoryStore(t)
	ctx := context.Background()
	seedHot(t, store, "ws-1", 2)
	h := NewMemoryHandlers(store)

	getPreview(h, "ws-1", "")
	req := httptest.NewRequest("GET", "/admin/api/memory/ws-1?hot=true", nil)
	req.SetPathValue("workspace", "ws-1")
	h.ListMemories(httptest.NewRecorder(), req)
	store.FlushUsage(ctx)

	all, _ := store.List(ctx, "ws-1", "", 10, 0)
	for _, e := range all {
		if e.InjectedCount != 0 || e.SearchedCount != 0 {
			t.Errorf("browsing counted as use: %+v", e)
		}
	}
}

// ── Markdown export / import ────────────────────────────────────────────────

func exportMemory(h *MemoryHandlers, ws string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "/admin/api/memory/"+ws+"/export", nil)
	req.SetPathValue("workspace", ws)
	rr := httptest.NewRecorder()
	h.ExportMemory(rr, req)
	return rr
}

func importMemory(h *MemoryHandlers, ws, contentType, markdown string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"markdown": markdown})
	req := httptest.NewRequest("POST", "/admin/api/memory/"+ws+"/import", strings.NewReader(string(body)))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.SetPathValue("workspace", ws)
	rr := httptest.NewRecorder()
	h.ImportMemory(rr, req)
	return rr
}

func TestExportMemory_DownloadsWorkspaceAndUserFactsAsMarkdown(t *testing.T) {
	store := newTestMemoryStore(t)
	ctx := context.Background()
	store.Insert(ctx, memory.NewEntry{WorkspaceID: "ws-1", MemoryType: memory.LongTerm, Title: "build", Content: "run go build", Tags: []string{memory.HotTag}, Source: "agent"})
	store.Insert(ctx, memory.NewEntry{WorkspaceID: "global", MemoryType: memory.UserProfile, Title: "name", Content: "Alice", Tags: nil, Source: "agent"})
	store.Insert(ctx, memory.NewEntry{WorkspaceID: "ws-2", MemoryType: memory.LongTerm, Title: "other", Content: "not mine", Tags: nil, Source: "agent"})

	rr := exportMemory(NewMemoryHandlers(store), "ws-1")
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	res := rr.Result()
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/markdown") {
		t.Errorf("Content-Type = %q", ct)
	}
	if cd := res.Header.Get("Content-Disposition"); !strings.Contains(cd, "attachment") || !strings.Contains(cd, "memory-ws-1.md") {
		t.Errorf("Content-Disposition = %q", cd)
	}
	body := rr.Body.String()
	for _, want := range []string{"### build", "scope=workspace mode=always", "### name", "scope=user"} {
		if !strings.Contains(body, want) {
			t.Errorf("export missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "not mine") {
		t.Error("another workspace's facts must not be exported")
	}
}

func TestExportMemory_RejectsAnUnsafeWorkspaceName(t *testing.T) {
	if rr := exportMemory(NewMemoryHandlers(newTestMemoryStore(t)), "../etc"); rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestImportMemory_CreatesFactsRoutedLikeTheAgentToolWould(t *testing.T) {
	store := newTestMemoryStore(t)
	doc := `### Tabs
<!-- scope=workspace mode=always keep=permanent priority=high -->
Use tabs.

### Tea
<!-- scope=user mode=always -->
Alice likes tea.

### Scratch
<!-- keep=session -->
temporary
`
	rr := importMemory(NewMemoryHandlers(store), "ws-1", "application/json", doc)
	var got importResult
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil || rr.Code != http.StatusOK {
		t.Fatalf("import: %d %s", rr.Code, rr.Body.String())
	}
	if got.Created != 3 || got.Skipped != 0 || len(got.Issues) != 0 {
		t.Fatalf("result = %+v", got)
	}

	ctx := context.Background()
	tabs, _ := store.List(ctx, "ws-1", memory.LongTerm, 10, 0)
	if len(tabs) != 1 || !tabs[0].IsHot() || tabs[0].Priority != memory.PriorityHigh || tabs[0].Source != "import" {
		t.Errorf("tabs = %+v", tabs)
	}
	if tea, _ := store.List(ctx, "global", memory.UserProfile, 10, 0); len(tea) != 1 || !tea[0].IsHot() {
		t.Errorf("user-scope fact must be stored under global: %+v", tea)
	}
	if scratch, _ := store.List(ctx, "ws-1", memory.Session, 10, 0); len(scratch) != 1 {
		t.Errorf("session fact missing: %+v", scratch)
	}
}

func TestImportMemory_SkipsDuplicatesAndReportsBadEntries(t *testing.T) {
	store := newTestMemoryStore(t)
	store.Insert(context.Background(), memory.NewEntry{WorkspaceID: "ws-1", MemoryType: memory.LongTerm, Title: "have", Content: "already here", Tags: nil, Source: "agent"})
	doc := "### have\nalready here\n\n### bad\n<!-- priority=urgent -->\ntext\n\n### good\nnew fact\n\n### huge\n" + strings.Repeat("x", maxMemoryContentChars+1) + "\n"

	rr := importMemory(NewMemoryHandlers(store), "ws-1", "application/json", doc)
	var got importResult
	json.Unmarshal(rr.Body.Bytes(), &got)
	if rr.Code != http.StatusOK || got.Created != 1 || got.Skipped != 1 || len(got.Issues) != 2 {
		t.Fatalf("result = %+v (code %d)", got, rr.Code)
	}
	if got.Issues[0].Line == 0 || got.Issues[0].Message == "" {
		t.Errorf("issues must say where and why: %+v", got.Issues)
	}
}

func TestImportMemory_Validation(t *testing.T) {
	var many strings.Builder
	for i := range maxImportFacts + 1 {
		fmt.Fprintf(&many, "### f%d\ncontent %d\n\n", i, i)
	}
	cases := []struct {
		name, ws, contentType, doc string
		want                       int
	}{
		{"no facts in the file", "ws-1", "application/json", "just some prose", http.StatusBadRequest},
		{"too many facts", "ws-1", "application/json", many.String(), http.StatusBadRequest},
		{"global is not a workspace", "global", "application/json", "### a\nb\n", http.StatusBadRequest},
		{"bad workspace id", "../x", "application/json", "### a\nb\n", http.StatusBadRequest},
		// A cross-site form can send text/plain without a preflight; refusing it
		// stops a web page planting facts that go into every prompt.
		{"non-JSON content type", "ws-1", "text/plain", "### a\nb\n", http.StatusUnsupportedMediaType},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if rr := importMemory(NewMemoryHandlers(newTestMemoryStore(t)), tc.ws, tc.contentType, tc.doc); rr.Code != tc.want {
				t.Errorf("expected %d, got %d: %s", tc.want, rr.Code, rr.Body.String())
			}
		})
	}
}

// Export from one workspace and import into another loses nothing; importing
// the same file again adds nothing.
func TestMemoryMarkdown_ExportThenImportRoundTrips(t *testing.T) {
	store := newTestMemoryStore(t)
	ctx := context.Background()
	store.Insert(ctx, memory.NewEntry{WorkspaceID: "ws-1", MemoryType: memory.LongTerm, Title: "build", Content: "run go build\n### tricky line\n---", Tags: []string{memory.HotTag}, Source: "agent"})
	id, _ := store.Insert(ctx, memory.NewEntry{WorkspaceID: "ws-1", MemoryType: memory.LongTerm, Title: "must", Content: "never force-push", Tags: []string{memory.HotTag}, Source: "agent"})
	store.SetPriority(ctx, "ws-1", id, memory.PriorityHigh)
	h := NewMemoryHandlers(store)

	exported := exportMemory(h, "ws-1").Body.String()
	var first importResult
	json.Unmarshal(importMemory(h, "ws-2", "application/json", exported).Body.Bytes(), &first)
	if first.Created != 2 || len(first.Issues) != 0 {
		t.Fatalf("first import = %+v", first)
	}
	got, _ := store.List(ctx, "ws-2", memory.LongTerm, 10, 0)
	byTitle := map[string]memory.MemoryEntry{}
	for _, e := range got {
		byTitle[e.Title] = e
	}
	if byTitle["build"].Content != "run go build\n### tricky line\n---" || !byTitle["build"].IsHot() {
		t.Errorf("build fact changed in transit: %+v", byTitle["build"])
	}
	if byTitle["must"].Priority != memory.PriorityHigh {
		t.Errorf("priority lost in transit: %+v", byTitle["must"])
	}

	var second importResult
	json.Unmarshal(importMemory(h, "ws-2", "application/json", exported).Body.Bytes(), &second)
	if second.Created != 0 || second.Skipped != 2 {
		t.Errorf("re-import must add nothing: %+v", second)
	}
}

func TestMemoryMarkdown_RoutesBeatTheIDRoute(t *testing.T) {
	store := newTestMemoryStore(t)
	h := NewMemoryHandlers(store)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin/api/memory/{workspace}/{id}", h.GetMemory)
	mux.HandleFunc("GET /admin/api/memory/{workspace}/export", h.ExportMemory)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest("GET", "/admin/api/memory/ws-1/export", nil))
	if rr.Code != http.StatusOK || strings.Contains(rr.Body.String(), "invalid id") {
		t.Errorf("export request reached GetMemory: %d", rr.Code)
	}
}

// ?message= adds what that chat message would recall; without it the response carries no recall section.
func TestInjectionPreview_MessageShowsTheRecall(t *testing.T) {
	store := newTestMemoryStore(t)
	id, err := store.Insert(context.Background(), memory.NewEntry{WorkspaceID: "ws-1", MemoryType: memory.LongTerm, Title: "codename", Content: "The codename is BLUEHERON.", Tags: nil, Source: "operator"})
	if err != nil {
		t.Fatal(err)
	}
	h := NewMemoryHandlers(store).WithModelResolver(modelResolver())

	p := decodePreview(t, getPreview(h, "ws-1", "?message=What%20is%20the%20codename%3F"))
	if p.Recall == nil || len(p.Recall.IDs) != 1 || p.Recall.IDs[0] != id || !strings.Contains(p.Recall.Block, "BLUEHERON") {
		t.Fatalf("recall preview = %+v, want the codename fact", p.Recall)
	}
	if none := decodePreview(t, getPreview(h, "ws-1", "")); none.Recall != nil {
		t.Errorf("no message, no recall section: %+v", none.Recall)
	}
}

// ── Typed facts ─────────────────────────────────────────────────────────────

// Rewording a fact in the editor must not wipe what the model recorded about it: the editor sends kind, why or expires
// only when the operator changes them.
func TestUpdateMemory_KeepsTypingUnlessTheBodyNamesIt(t *testing.T) {
	store := newTestMemoryStore(t)
	ctx := context.Background()
	id, err := store.Insert(ctx, memory.NewEntry{WorkspaceID: "ws-1", MemoryType: memory.LongTerm, Title: "t", Content: "c", Source: "agent",
		Kind: memory.KindProject, Why: "the launch depends on it", ExpiresAt: time.Date(2030, 1, 2, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	h := NewMemoryHandlers(store)

	if rr := putMemory(t, h, "ws-1", id, `{"title":"t","content":"reworded"}`); rr.Code != http.StatusOK {
		t.Fatalf("edit: %d %s", rr.Code, rr.Body.String())
	}
	got, _ := store.Get(ctx, "ws-1", id)
	if got.Kind != memory.KindProject || got.Why != "the launch depends on it" || got.ExpiresAt != "2030-01-02T00:00:00Z" {
		t.Errorf("a text-only edit changed the typing: %+v", got)
	}

	if rr := putMemory(t, h, "ws-1", id, `{"title":"t","content":"reworded","kind":"reference","why":"","expires":"2031-05-06"}`); rr.Code != http.StatusOK {
		t.Fatalf("retype: %d %s", rr.Code, rr.Body.String())
	}
	got, _ = store.Get(ctx, "ws-1", id)
	if got.Kind != memory.KindReference || got.Why != "" || got.ExpiresAt != "2031-05-07T00:00:00Z" {
		t.Errorf("named fields must replace the old ones (a date lasts through its day): %+v", got)
	}

	if rr := putMemory(t, h, "ws-1", id, `{"title":"t","content":"reworded","expires":""}`); rr.Code != http.StatusOK {
		t.Fatalf("clear expiry: %d", rr.Code)
	}
	if got, _ = store.Get(ctx, "ws-1", id); got.ExpiresAt != "" || got.Kind != memory.KindReference {
		t.Errorf("an empty expires clears only the expiry: %+v", got)
	}
}

func TestMemoryHandlers_RefuseBadTypingAndChangeNothing(t *testing.T) {
	store := newTestMemoryStore(t)
	ctx := context.Background()
	id, _ := store.Insert(ctx, memory.NewEntry{WorkspaceID: "ws-1", MemoryType: memory.LongTerm, Title: "t", Content: "c", Source: "agent"})
	h := NewMemoryHandlers(store)
	for name, fields := range map[string]string{
		"unknown kind":     `"kind":"hobby"`,
		"multi-line why":   `"why":"a\nb"`,
		"bad date":         `"expires":"soon"`,
		"date in the past": `"expires":"2001-01-01"`,
	} {
		t.Run(name, func(t *testing.T) {
			if rr := putMemory(t, h, "ws-1", id, `{"title":"t","content":"changed",`+fields+`}`); rr.Code != http.StatusBadRequest {
				t.Errorf("update: expected 400, got %d: %s", rr.Code, rr.Body.String())
			}
			if rr := postMemory(h, "ws-1", "application/json", `{"content":"new fact",`+fields+`}`); rr.Code != http.StatusBadRequest {
				t.Errorf("create: expected 400, got %d: %s", rr.Code, rr.Body.String())
			}
		})
	}
	if got, _ := store.Get(ctx, "ws-1", id); got.Content != "c" {
		t.Errorf("a refused update must not apply its text, got %q", got.Content)
	}
}

func TestCreateMemory_StoresKindWhyAndExpiry(t *testing.T) {
	store := newTestMemoryStore(t)
	rr := postMemory(NewMemoryHandlers(store), "ws-1", "application/json", `{"content":"Wiki holds the runbook","kind":"reference","why":"on-call asks weekly","expires":"2031-05-06"}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var created memory.MemoryEntry
	json.Unmarshal(rr.Body.Bytes(), &created)
	if created.Kind != memory.KindReference || created.Why != "on-call asks weekly" || created.ExpiresAt != "2031-05-07T00:00:00Z" {
		t.Errorf("typing not stored: %+v", created)
	}
}

// A session fact added by hand must expire like one the model saves, or nothing would ever remove it.
func TestCreateMemory_SessionFactsGetTheSessionLifetime(t *testing.T) {
	store := newTestMemoryStore(t)
	h := NewMemoryHandlers(store).WithSessionRetention(func() time.Duration { return 48 * time.Hour })
	before := time.Now()
	rr := postMemory(h, "ws-1", "application/json", `{"content":"only for this run","keep":"session"}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var created memory.MemoryEntry
	json.Unmarshal(rr.Body.Bytes(), &created)
	exp, err := time.Parse(time.RFC3339, created.ExpiresAt)
	if err != nil || exp.Before(before.Add(47*time.Hour)) || exp.After(time.Now().Add(49*time.Hour)) {
		t.Errorf("a session fact should expire about 48h out, got %q", created.ExpiresAt)
	}
	rr = postMemory(h, "ws-1", "application/json", `{"content":"permanent one"}`)
	json.Unmarshal(rr.Body.Bytes(), &created)
	if created.ExpiresAt != "" {
		t.Errorf("a permanent fact never expires, got %q", created.ExpiresAt)
	}
}
