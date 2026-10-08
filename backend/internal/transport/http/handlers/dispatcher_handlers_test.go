package handlers

import (
	"encoding/json"
	"errors"
	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/automation"
	"llm-proxy/internal/core/eventbus"
	"llm-proxy/internal/core/runlane"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// Minimal hand-rolled mock for dispatcher satisfying the full interface
type testDispatcher struct {
	mgr           *persistence.WorkspaceManager
	stopCalled    map[string]bool
	triggerResult automation.TriggerResult
	triggerErr    error
	cancelErr     error
	cancelCalled  map[string]bool
	laneSnapshot  runlane.Snapshot
	heartbeat     models.HeartbeatState
	registered    []string // "ws/name" for every Register call
	unregistered  []string // "ws/name" for every Unregister call
}

func (t *testDispatcher) Persistence() *persistence.WorkspaceManager { return t.mgr }
func (t *testDispatcher) Register(ws string, a *models.Automation) error {
	t.registered = append(t.registered, ws+"/"+a.Name)
	return nil
}
func (t *testDispatcher) Unregister(ws, name string) error {
	t.unregistered = append(t.unregistered, ws+"/"+name)
	return nil
}
func (t *testDispatcher) HeartbeatState(string) (models.HeartbeatState, error) {
	return t.heartbeat, nil
}
func (t *testDispatcher) ListAll() []*automation.AutomationEntry { return nil }
func (t *testDispatcher) Trigger(ws, name, _ string) (automation.TriggerResult, error) {
	return t.triggerResult, t.triggerErr
}
func (t *testDispatcher) CancelQueued(ws, name string) error {
	if t.cancelCalled == nil {
		t.cancelCalled = make(map[string]bool)
	}
	t.cancelCalled[ws+"/"+name] = true
	return t.cancelErr
}
func (t *testDispatcher) LaneSnapshot() runlane.Snapshot { return t.laneSnapshot }
func (t *testDispatcher) StopAutomation(ws string) error {
	if t.stopCalled != nil {
		t.stopCalled[ws] = true
	}
	return nil
}
func (t *testDispatcher) Metrics() *automation.DispatcherMetrics {
	return &automation.DispatcherMetrics{}
}
func (t *testDispatcher) Events() *eventbus.Bus         { return nil }
func (t *testDispatcher) UnregisterWorkspace(ws string) {}

func TestValidateAutomation_LoopStrategy(t *testing.T) {
	dispatcher := &testDispatcher{}
	handlers := NewDispatcherHandlers(dispatcher, NewWorkspaceService(nil), logging.NewNopLogger())

	cases := []struct {
		name         string
		loopStrategy models.LoopStrategy
		wantErr      bool
	}{
		{"empty passes (model config default)", "", false},
		{"react passes", models.LoopStrategyReact, false},
		{"plan_execute passes", models.LoopStrategyPlanExecute, false},
		{"evaluator_optimizer passes", models.LoopStrategyEvaluatorOptimizer, false},
		{"unknown rejected", "map_reduce", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := handlers.validateAutomation(&models.Automation{
				Name:         "ok-name",
				TaskFile:     "task.md",
				LoopStrategy: tc.loopStrategy,
			})
			if tc.wantErr && err == nil {
				t.Fatal("expected error for invalid loop_strategy")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if tc.wantErr && !strings.Contains(err.Error(), "react") {
				t.Errorf("expected valid-values hint listing react, got %q", err.Error())
			}
		})
	}
}

func TestCreateWorkspace_Isolation(t *testing.T) {
	tmpWorkspaces := t.TempDir()
	tmpMetadata := t.TempDir()
	resolver := storage.NewPathResolver(tmpWorkspaces, tmpWorkspaces, tmpMetadata)
	mgr := persistence.NewWorkspaceManager(resolver)

	dispatcher := &testDispatcher{mgr: mgr}
	wsSvc := NewWorkspaceService(mgr)
	handlers := NewDispatcherHandlers(dispatcher, wsSvc, logging.NewNopLogger())

	workspaceID := "secure-project"
	reqBody := `{"id": "` + workspaceID + `"}`
	req := httptest.NewRequest("POST", "/admin/api/dispatcher/workspaces", strings.NewReader(reqBody))
	rr := httptest.NewRecorder()

	handlers.CreateWorkspace(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("handler returned unexpected status: %v", rr.Body.String())
	}

	// VERIFY ISOLATION

	// 1. Config MUST NOT exist in root
	rootConfig := filepath.Join(tmpWorkspaces, workspaceID, models.ConfigFilename)
	if _, err := os.Stat(rootConfig); err == nil {
		t.Error("SECURITY VIOLATION: config.yaml found in workspace root")
	}

	// 3. New workspaces should NOT have .internal folder in root
	internalDirInRoot := filepath.Join(tmpWorkspaces, workspaceID, models.InternalDirName)
	if _, err := os.Stat(internalDirInRoot); err == nil {
		t.Error(".internal directory found in workspace root (should be moved out)")
	}

	// 4. Root should only contain task files and NO hidden files
	entries, _ := os.ReadDir(filepath.Join(tmpWorkspaces, workspaceID))
	for _, entry := range entries {
		name := entry.Name()
		if name[0] == '.' && name != "." && name != ".." {
			t.Errorf("Hidden file/directory %s found in workspace root (should be clean)", name)
		}
		if name == models.ConfigFilename || name == models.StateFilename {
			t.Errorf("Forbidden internal file %s found in workspace root", name)
		}
	}
}

func TestCreateWorkspace_SeedsAgentsFile(t *testing.T) {
	tmp := t.TempDir()
	resolver := storage.NewPathResolver(tmp, tmp, tmp)
	mgr := persistence.NewWorkspaceManager(resolver)
	handlers := NewDispatcherHandlers(&testDispatcher{mgr: mgr}, NewWorkspaceService(mgr), logging.NewNopLogger())

	workspaceID := "seed-check"
	req := httptest.NewRequest("POST", "/admin/api/dispatcher/workspaces", strings.NewReader(`{"id": "`+workspaceID+`"}`))
	rr := httptest.NewRecorder()
	handlers.CreateWorkspace(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %v", rr.Body.String())
	}

	// AGENTS.md must be seeded with the default content.
	got, err := mgr.ReadTaskFile(workspaceID, models.RulesFilename)
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	if got != prompts.DefaultAgentsMD {
		t.Errorf("AGENTS.md not seeded with DefaultAgentsMD content")
	}

	// Legacy agent.md must NOT be seeded anymore.
	if _, err := os.Stat(filepath.Join(tmp, workspaceID, "agent.md")); err == nil {
		t.Error("legacy agent.md should no longer be seeded")
	}

	// Heartbeat file is still seeded.
	if _, err := os.Stat(filepath.Join(tmp, workspaceID, models.HeartbeatFilename)); err != nil {
		t.Errorf("heartbeat file should be seeded: %v", err)
	}
}

func TestDispatcherHandlers_Validation(t *testing.T) {
	tmp := t.TempDir()
	resolver := storage.NewPathResolver(tmp, tmp, tmp)
	mgr := persistence.NewWorkspaceManager(resolver)
	dispatcher := &testDispatcher{mgr: mgr}
	wsSvc := NewWorkspaceService(mgr)
	handlers := NewDispatcherHandlers(dispatcher, wsSvc, logging.NewNopLogger())

	tests := []struct {
		name           string
		workspaceID    string
		automationName string
		wantStatus     int
	}{
		{
			name:        "Valid IDs work",
			workspaceID: "valid-ws",
			wantStatus:  http.StatusOK,
		},
		{
			name:        "Invalid workspace ID (injection)",
			workspaceID: "../outside",
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:        "Invalid workspace ID (special chars)",
			workspaceID: "invalid$id",
			wantStatus:  http.StatusBadRequest,
		},
		{
			name:           "Invalid automation name",
			workspaceID:    "valid-ws",
			automationName: "bad/auto",
			wantStatus:     http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/workspaces/"+tt.workspaceID, nil)
			req.SetPathValue(models.WorkspaceIDParam, tt.workspaceID)
			if tt.automationName != "" {
				req.SetPathValue("automation", tt.automationName)
			}

			rr := httptest.NewRecorder()

			if tt.automationName != "" {
				handlers.TriggerAutomation(rr, req)
			} else {
				handlers.GetWorkspaceState(rr, req)
			}

			if rr.Code != tt.wantStatus {
				t.Errorf("%s: expected status %d, got %d. Body: %s", tt.name, tt.wantStatus, rr.Code, rr.Body.String())
			}
		})
	}
}

// listDispatcher returns a fixed automation entry so ListAutomations can be
// exercised through the handler.
type listDispatcher struct {
	*testDispatcher
	entries []*automation.AutomationEntry
}

func (l *listDispatcher) ListAll() []*automation.AutomationEntry { return l.entries }

// TestListAutomations_NoStaleOutputAfterDelete verifies that an automation's
// output is sourced only from its own latest run (LastRuns), and that deleting
// all its runs removes the output entirely rather than surfacing stale data.
func TestListAutomations_NoStaleOutputAfterDelete(t *testing.T) {
	tmp := t.TempDir()
	resolver := storage.NewPathResolver(tmp, tmp, tmp)
	mgr := persistence.NewWorkspaceManager(resolver)
	wsID := "auto-output"
	automationName := "task-a"

	entry := &automation.AutomationEntry{
		ID:        wsID + "/" + automationName,
		Workspace: wsID,
		Name:      automationName,
		TaskFile:  "task.md",
		Model:     "model-1",
	}
	entry.Trigger, _ = automation.New(models.TriggerConfig{Type: models.TriggerManual})
	entry.Strategy = &automation.IsolatedStrategy{}

	handlers := NewDispatcherHandlers(
		&listDispatcher{testDispatcher: &testDispatcher{mgr: mgr}, entries: []*automation.AutomationEntry{entry}},
		NewWorkspaceService(mgr),
		logging.NewNopLogger(),
	)

	last := &models.AutomationRun{
		ID:             "run_1",
		WorkspaceID:    wsID,
		AutomationName: automationName,
		Model:          "model-1",
		Output:         "latest summary",
	}
	if err := mgr.WriteState(wsID, &models.AgentState{LastRuns: map[string]*models.AutomationRun{automationName: last}}); err != nil {
		t.Fatalf("WriteState: %v", err)
	}

	req := httptest.NewRequest("GET", "/admin/api/dispatcher/automations", nil)
	rr := httptest.NewRecorder()
	handlers.ListAutomations(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("ListAutomations status: %d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "latest summary") {
		t.Fatalf("expected automation output from LastRuns, got: %s", rr.Body.String())
	}

	// Delete all runs for the automation; its output must disappear.
	if err := mgr.DeleteAutomationRuns(wsID, automationName); err != nil {
		t.Fatalf("DeleteAutomationRuns: %v", err)
	}
	rr = httptest.NewRecorder()
	handlers.ListAutomations(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("ListAutomations status after delete: %d", rr.Code)
	}
	if strings.Contains(rr.Body.String(), "latest summary") {
		t.Errorf("stale automation output surfaced after deleting all runs: %s", rr.Body.String())
	}
}

// TestDeleteRun_NotFound returns 404 when the run ID has no history entry, so a
// double-click or already-deleted run does not surface a spurious server error.
func TestDeleteRun_NotFound(t *testing.T) {
	tmp := t.TempDir()
	resolver := storage.NewPathResolver(tmp, tmp, tmp)
	mgr := persistence.NewWorkspaceManager(resolver)
	handlers := NewDispatcherHandlers(&testDispatcher{mgr: mgr}, NewWorkspaceService(mgr), logging.NewNopLogger())

	req := httptest.NewRequest("DELETE", "/admin/api/dispatcher/runs/ws/run/run_999", nil)
	req.SetPathValue(models.WorkspaceIDParam, "ws")
	req.SetPathValue("run", "run_999")
	rr := httptest.NewRecorder()
	handlers.DeleteRun(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown run, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestDeleteWorkspace_RemovesAllLocationsAndStopsRuns verifies the DELETE
// workspace endpoint stops any in-flight automation and removes the entire
// on-disk footprint: the user content dir, the metadata dir (config.yaml,
// state.json, .lock, process.log, sessions/), and the runs tree.
func TestDeleteWorkspace_RemovesAllLocationsAndStopsRuns(t *testing.T) {
	tmp := t.TempDir()
	resolver := storage.NewPathResolver(tmp, tmp, tmp)
	mgr := persistence.NewWorkspaceManager(resolver)
	dispatcher := &testDispatcher{mgr: mgr, stopCalled: map[string]bool{}}
	handlers := NewDispatcherHandlers(dispatcher, NewWorkspaceService(mgr), logging.NewNopLogger())
	wsID := "delete-ws"

	if err := mgr.WriteTaskFile(wsID, "notes.txt", "hello"); err != nil {
		t.Fatalf("WriteTaskFile: %v", err)
	}
	if err := mgr.WriteConfig(wsID, &models.WorkspaceConfig{Model: "m"}); err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}
	if err := mgr.WriteState(wsID, &models.AgentState{}); err != nil {
		t.Fatalf("WriteState: %v", err)
	}
	if err := mgr.WriteSession(wsID, &models.AssistantSession{
		ID: "s1", WorkspaceID: wsID, History: []models.Message{{Role: models.UserRole, Content: "hi"}},
	}); err != nil {
		t.Fatalf("WriteSession: %v", err)
	}
	if err := os.WriteFile(resolver.Lock(wsID), nil, 0600); err != nil {
		t.Fatalf("write .lock: %v", err)
	}
	if err := os.WriteFile(resolver.ProcessLog(wsID), []byte("log"), 0600); err != nil {
		t.Fatalf("write process.log: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(resolver.WorkspaceRunsDir(wsID), "m", "task", "r1"), 0755); err != nil {
		t.Fatalf("seed run dir: %v", err)
	}

	req := httptest.NewRequest("DELETE", "/admin/api/dispatcher/workspaces/"+wsID, nil)
	req.SetPathValue(models.WorkspaceIDParam, wsID)
	rr := httptest.NewRecorder()
	handlers.DeleteWorkspace(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("DeleteWorkspace status: %d %s", rr.Code, rr.Body.String())
	}

	if !dispatcher.stopCalled[wsID] {
		t.Error("expected StopAutomation to be called for the workspace")
	}

	for _, path := range []string{
		resolver.WorkspaceDir(wsID),
		resolver.InternalDir(wsID),
		resolver.WorkspaceRunsDir(wsID),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed, err=%v", path, err)
		}
	}
}

func TestValidateAutomation_TaskFile(t *testing.T) {
	dispatcher := &testDispatcher{}
	handlers := NewDispatcherHandlers(dispatcher, NewWorkspaceService(nil), logging.NewNopLogger())

	cases := []struct {
		name     string
		taskFile string
		wantErr  bool
	}{
		{"valid relative file", "task.md", false},
		{"nested task file accepted", "jobs/nightly.md", false},
		{"empty rejected", "", true},
		{"absolute path rejected", "/etc/passwd", true},
		{"parent traversal rejected", "../secret.txt", true},
		{"nested traversal rejected", "a/../b.md", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := handlers.validateAutomation(&models.Automation{
				Name:     "ok-name",
				TaskFile: tc.taskFile,
			})
			if tc.wantErr && err == nil {
				t.Fatal("expected error for invalid task_file")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}

func TestIsUnsafeFileParam(t *testing.T) {
	cases := []struct {
		value  string
		unsafe bool
	}{
		{"task.md", false},
		{"v1..2.md", false}, // dots inside a name are fine; only traversal is blocked
		{"", true},
		{".", true},
		{"..", true},
		{"a/../b", true},
		{"sub/task.md", false}, // nested paths are allowed (plan Phase 3)
		{"../secret.md", true},
		{"sub/../../secret.md", true},
		{"/etc/passwd", true},
	}
	for _, tc := range cases {
		if got := isUnsafeFileParam(tc.value); got != tc.unsafe {
			t.Errorf("isUnsafeFileParam(%q) = %v, want %v", tc.value, got, tc.unsafe)
		}
	}
}

// network_grant (per-run scope override, sandboxing plan §4.4) is fail-fast
// validated: empty = inherit passes; explicit none/lan/internet pass; anything
// else is rejected with the valid-values hint.
func TestValidateAutomation_NetworkGrant(t *testing.T) {
	handlers := NewDispatcherHandlers(&testDispatcher{}, NewWorkspaceService(nil), logging.NewNopLogger())

	cases := []struct {
		name    string
		grant   models.NetworkScope
		wantErr bool
	}{
		{"empty passes (inherit workspace scope)", "", false},
		{"none passes", models.NetworkScopeNone, false},
		{"lan passes", models.NetworkScopeLan, false},
		{"internet passes", models.NetworkScopeInternet, false},
		{"unknown rejected", "wifi", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := handlers.validateAutomation(&models.Automation{
				Name:         "ok-name",
				TaskFile:     "task.md",
				NetworkGrant: tc.grant,
			})
			if tc.wantErr && err == nil {
				t.Fatal("expected error for invalid network_grant")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if tc.wantErr && !strings.Contains(err.Error(), "network_grant") {
				t.Errorf("expected network_grant hint, got %q", err.Error())
			}
		})
	}
}

func TestTriggerAutomation_ReportsLaneDisposition(t *testing.T) {
	handlers := NewDispatcherHandlers(&testDispatcher{
		triggerResult: automation.TriggerResult{Status: automation.TriggerQueued, Position: 2},
	}, NewWorkspaceService(nil), logging.NewNopLogger())

	req := httptest.NewRequest(http.MethodPost, "/admin/api/dispatcher/trigger/ws/a", nil)
	req.SetPathValue(models.WorkspaceIDParam, "ws")
	req.SetPathValue("automation", "a")
	rec := httptest.NewRecorder()

	handlers.TriggerAutomation(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for a queued trigger, got %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Status   string `json:"status"`
		Position int    `json:"position"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Status != automation.TriggerQueued || body.Position != 2 {
		t.Fatalf("body = %+v, want queued at position 2", body)
	}
}

func TestTriggerAutomation_ReportsStartedDisposition(t *testing.T) {
	handlers := NewDispatcherHandlers(&testDispatcher{
		triggerResult: automation.TriggerResult{Status: automation.TriggerStarted},
	}, NewWorkspaceService(nil), logging.NewNopLogger())

	req := httptest.NewRequest(http.MethodPost, "/admin/api/dispatcher/trigger/ws/a", nil)
	req.SetPathValue(models.WorkspaceIDParam, "ws")
	req.SetPathValue("automation", "a")
	rec := httptest.NewRecorder()

	handlers.TriggerAutomation(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for a started trigger, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestTriggerAutomation_MapsAdmissionErrorToNotFound(t *testing.T) {
	handlers := NewDispatcherHandlers(&testDispatcher{triggerErr: errors.New("boom")}, NewWorkspaceService(nil), logging.NewNopLogger())

	req := httptest.NewRequest(http.MethodPost, "/admin/api/dispatcher/trigger/ws/a", nil)
	req.SetPathValue(models.WorkspaceIDParam, "ws")
	req.SetPathValue("automation", "a")
	rec := httptest.NewRecorder()

	handlers.TriggerAutomation(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for an admission error, got %d", rec.Code)
	}
}

func TestCancelQueuedHandler_RecordsCancellation(t *testing.T) {
	dispatcher := &testDispatcher{}
	handlers := NewDispatcherHandlers(dispatcher, NewWorkspaceService(nil), logging.NewNopLogger())

	req := httptest.NewRequest(http.MethodDelete, "/admin/api/dispatcher/queue/ws/a", nil)
	req.SetPathValue(models.WorkspaceIDParam, "ws")
	req.SetPathValue("automation", "a")
	rec := httptest.NewRecorder()

	handlers.CancelQueuedHandler(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
	if !dispatcher.cancelCalled["ws/a"] {
		t.Fatal("CancelQueued was not invoked for ws/a")
	}
}

func TestCancelQueuedHandler_NotQueued(t *testing.T) {
	handlers := NewDispatcherHandlers(&testDispatcher{cancelErr: automation.ErrNoQueuedRun}, NewWorkspaceService(nil), logging.NewNopLogger())

	req := httptest.NewRequest(http.MethodDelete, "/admin/api/dispatcher/queue/ws/a", nil)
	req.SetPathValue(models.WorkspaceIDParam, "ws")
	req.SetPathValue("automation", "a")
	rec := httptest.NewRecorder()

	handlers.CancelQueuedHandler(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when nothing is queued, got %d", rec.Code)
	}
}

func TestListAutomations_ReportsQueuedPosition(t *testing.T) {
	entry := &automation.AutomationEntry{ID: "ws/queued", Workspace: "ws", Name: "queued", TaskFile: "task.md"}
	entry.Trigger, _ = automation.New(models.TriggerConfig{Type: models.TriggerManual})
	entry.Strategy = &automation.IsolatedStrategy{}
	ld := &listDispatcher{
		testDispatcher: &testDispatcher{laneSnapshot: runlane.Snapshot{Lanes: []runlane.LaneState{{
			Lane:  runlane.LaneLocal,
			Limit: 1,
			Queued: []runlane.Entry{{
				Key: "ws/queued", Lane: runlane.LaneLocal, WorkspaceID: "ws",
				Automation: "queued", Position: 3,
			}},
		}}}},
		entries: []*automation.AutomationEntry{entry},
	}
	tmp := t.TempDir()
	handlers := NewDispatcherHandlers(ld, NewWorkspaceService(persistence.NewWorkspaceManager(storage.NewPathResolver(tmp, tmp, tmp))), logging.NewNopLogger())

	req := httptest.NewRequest(http.MethodGet, "/admin/api/dispatcher/automations", nil)
	rec := httptest.NewRecorder()

	handlers.ListAutomations(rec, req)

	var infos []AutomationInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &infos); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(infos) != 1 || !infos[0].Queued || infos[0].QueuePosition != 3 {
		t.Fatalf("infos = %+v, want the queued automation at position 3", infos)
	}

	// The JSON key is the frontend contract: the panel reads `queued` (and
	// `queue_position`), so a struct-level assertion alone would not catch a
	// tag drift like `is_queued`.
	var raw []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode raw response: %v", err)
	}
	if _, ok := raw[0]["queued"]; !ok {
		t.Fatalf("wire payload missing \"queued\": %s", rec.Body.String())
	}
	if _, ok := raw[0]["is_queued"]; ok {
		t.Fatalf("wire payload must use \"queued\", not \"is_queued\": %s", rec.Body.String())
	}
}

// fileRoutes mounts the workspace file handlers on a real ServeMux with the
// production patterns, so the {file...} wildcard and %2F decoding are exercised.
func fileRoutes(h *DispatcherHandlers) *http.ServeMux {
	const ws = "/ws/{" + models.WorkspaceIDParam + "}"
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+ws+"/tree", h.ListWorkspaceTree)
	mux.HandleFunc("GET "+ws+"/files/{file...}", h.ReadWorkspaceFile)
	mux.HandleFunc("PUT "+ws+"/files/{file...}", h.WriteWorkspaceFile)
	mux.HandleFunc("DELETE "+ws+"/files/{file...}", h.DeleteWorkspaceFile)
	return mux
}

func serveFile(mux *http.ServeMux, method, target, body string) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(method, target, strings.NewReader(body)))
	return rr
}

// Nested workspace files over HTTP: write, read (plain and %2F-encoded), list,
// delete; traversal and symlink escapes are refused and touch nothing outside.
func TestWorkspaceFileHandlers_NestedAndContained(t *testing.T) {
	tmp := t.TempDir()
	resolver := storage.NewPathResolver(tmp, tmp, t.TempDir())
	mgr := persistence.NewWorkspaceManager(resolver)
	mux := fileRoutes(NewDispatcherHandlers(&testDispatcher{mgr: mgr}, NewWorkspaceService(mgr), logging.NewNopLogger()))
	const wsID = "files-ws"

	if rr := serveFile(mux, "PUT", "/ws/"+wsID+"/files/docs/plan.md", `{"content":"nested"}`); rr.Code != http.StatusOK {
		t.Fatalf("PUT nested: %d %s", rr.Code, rr.Body.String())
	}
	for _, target := range []string{"/ws/" + wsID + "/files/docs/plan.md", "/ws/" + wsID + "/files/docs%2Fplan.md"} {
		rr := serveFile(mux, "GET", target, "")
		if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"nested"`) {
			t.Fatalf("GET %s: %d %s", target, rr.Code, rr.Body.String())
		}
	}
	rr := serveFile(mux, "GET", "/ws/"+wsID+"/tree", "")
	var tree models.WorkspaceTree
	if err := json.Unmarshal(rr.Body.Bytes(), &tree); err != nil || rr.Code != http.StatusOK {
		t.Fatalf("GET tree: %d %s (%v)", rr.Code, rr.Body.String(), err)
	}
	if !slices.Contains(tree.Entries, models.TreeEntry{Path: "docs/plan.md", Type: models.TreeEntryFile}) {
		t.Fatalf("tree lacks docs/plan.md: %+v", tree.Entries)
	}
	if rr := serveFile(mux, "DELETE", "/ws/"+wsID+"/files/docs/plan.md", ""); rr.Code != http.StatusOK {
		t.Fatalf("DELETE nested: %d %s", rr.Code, rr.Body.String())
	}

	// A folder with content: a plain DELETE refuses it, ?recursive=true removes it.
	if rr := serveFile(mux, "PUT", "/ws/"+wsID+"/files/docs/deep/a.md", `{"content":"x"}`); rr.Code != http.StatusOK {
		t.Fatalf("PUT deep: %d %s", rr.Code, rr.Body.String())
	}
	if rr := serveFile(mux, "DELETE", "/ws/"+wsID+"/files/docs", ""); rr.Code == http.StatusOK {
		t.Fatalf("plain DELETE of a non-empty folder succeeded: %s", rr.Body.String())
	}
	if rr := serveFile(mux, "DELETE", "/ws/"+wsID+"/files/docs?recursive=true", ""); rr.Code != http.StatusOK {
		t.Fatalf("recursive DELETE: %d %s", rr.Code, rr.Body.String())
	}
	if _, err := os.Stat(filepath.Join(resolver.WorkspaceDir(wsID), "docs")); !os.IsNotExist(err) {
		t.Fatalf("folder still present after recursive DELETE: %v", err)
	}
	for _, root := range []string{".", "%2E", ""} {
		if rr := serveFile(mux, "DELETE", "/ws/"+wsID+"/files/"+root+"?recursive=true", ""); rr.Code == http.StatusOK {
			t.Fatalf("recursive DELETE of the workspace root %q succeeded", root)
		}
	}
	if _, err := os.Stat(resolver.WorkspaceDir(wsID)); err != nil {
		t.Fatalf("workspace root removed: %v", err)
	}
	if rr := serveFile(mux, "DELETE", "/ws/"+wsID+"/files/docs?recursive=maybe", ""); rr.Code != http.StatusBadRequest {
		t.Fatalf("recursive=maybe: %d — want 400", rr.Code)
	}

	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.md")
	if err := os.WriteFile(secret, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(resolver.WorkspaceDir(wsID), "out")); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ method, file, body string }{
		{"GET", "..%2Fescape.md", ""},
		{"GET", "out/secret.md", ""},
		{"PUT", "out/new.md", `{"content":"x"}`},
		{"DELETE", "out/secret.md", ""},
		{"DELETE", "out/secret.md?recursive=true", ""},
		{"DELETE", "..%2F" + wsID + "?recursive=true", ""},
	} {
		rr := serveFile(mux, tc.method, "/ws/"+wsID+"/files/"+tc.file, tc.body)
		if rr.Code == http.StatusOK || strings.Contains(rr.Body.String(), "secret\"") {
			t.Errorf("%s %s: status %d body %s — want refused", tc.method, tc.file, rr.Code, rr.Body.String())
		}
	}
	if data, err := os.ReadFile(secret); err != nil || string(data) != "secret" {
		t.Fatalf("outside file changed: %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(outside, "new.md")); !os.IsNotExist(err) {
		t.Fatal("a file was written outside the workspace")
	}
}

// memory_mode is fail-fast validated like loop_strategy: empty (inherit), on and
// off pass; anything else, including the pre-rename "hot", is rejected with the valid-values hint.
func TestValidateAutomation_MemoryMode(t *testing.T) {
	handlers := NewDispatcherHandlers(&testDispatcher{}, NewWorkspaceService(nil), logging.NewNopLogger())

	cases := []struct {
		name    string
		mode    models.MemoryMode
		wantErr bool
	}{
		{"empty passes (inherit)", "", false},
		{"off passes", models.MemoryModeOff, false},
		{"on passes", models.MemoryModeOn, false},
		{"pre-rename hot rejected", "hot", true},
		{"unknown rejected", "hot+hints", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := handlers.validateAutomation(&models.Automation{Name: "ok-name", TaskFile: "task.md", MemoryMode: tc.mode})
			if tc.wantErr && (err == nil || !strings.Contains(err.Error(), "memory_mode")) {
				t.Fatalf("expected memory_mode error, got %v", err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}

// The heartbeat is configured in the workspace's Heartbeat section, so its automation name is reserved: a
// hand-made automation with that name would collide with the compiled one.
func TestValidateAutomation_ReservedHeartbeatName(t *testing.T) {
	handlers := NewDispatcherHandlers(&testDispatcher{}, NewWorkspaceService(nil), logging.NewNopLogger())
	err := handlers.validateAutomation(&models.Automation{Name: models.HeartbeatAutomationName, TaskFile: "task.md"})
	if err == nil || !strings.Contains(err.Error(), "Heartbeat") {
		t.Fatalf("expected a reserved-name error pointing at the Heartbeat section, got %v", err)
	}
}

func TestUpdateWorkspaceConfig_ValidatesHeartbeat(t *testing.T) {
	tmp := t.TempDir()
	resolver := storage.NewPathResolver(tmp, tmp, tmp)
	mgr := persistence.NewWorkspaceManager(resolver)
	handlers := NewDispatcherHandlers(&testDispatcher{mgr: mgr}, NewWorkspaceService(mgr), logging.NewNopLogger())

	for _, tc := range []struct {
		name string
		body string
		want int
	}{
		{"enabled with an interval", `{"heartbeat":{"enabled":true,"every":"15m"}}`, http.StatusOK},
		{"disabled needs no interval", `{"heartbeat":{"enabled":false}}`, http.StatusOK},
		{"too frequent", `{"heartbeat":{"enabled":true,"every":"10s"}}`, http.StatusBadRequest},
		{"not a duration", `{"heartbeat":{"enabled":true,"every":"often"}}`, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("PUT", "/admin/api/dispatcher/workspaces/ws/config", strings.NewReader(tc.body))
			req.SetPathValue(models.WorkspaceIDParam, "ws")
			rr := httptest.NewRecorder()
			handlers.UpdateWorkspaceConfig(rr, req)
			if rr.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rr.Code, tc.want, rr.Body.String())
			}
			if tc.want == http.StatusBadRequest && !strings.Contains(rr.Body.String(), "heartbeat.every") {
				t.Errorf("error should name the field: %s", rr.Body.String())
			}
		})
	}
}

// assistant_memory is the workspace-level override of the same switch and is validated at the same boundary,
// before anything is persisted.
func TestUpdateWorkspaceConfig_RejectsBadAssistantMemory(t *testing.T) {
	tmp := t.TempDir()
	resolver := storage.NewPathResolver(tmp, tmp, tmp)
	mgr := persistence.NewWorkspaceManager(resolver)
	handlers := NewDispatcherHandlers(&testDispatcher{mgr: mgr}, NewWorkspaceService(mgr), logging.NewNopLogger())

	for _, tc := range []struct {
		value string
		want  int
	}{
		{`"on"`, http.StatusOK},
		{`"off"`, http.StatusOK},
		{`""`, http.StatusOK},
		{`"hot"`, http.StatusBadRequest},
		{`"sometimes"`, http.StatusBadRequest},
	} {
		t.Run(tc.value, func(t *testing.T) {
			req := httptest.NewRequest("PUT", "/admin/api/dispatcher/workspaces/ws/config", strings.NewReader(`{"assistant_memory":`+tc.value+`}`))
			req.SetPathValue(models.WorkspaceIDParam, "ws")
			rr := httptest.NewRecorder()
			handlers.UpdateWorkspaceConfig(rr, req)
			if rr.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rr.Code, tc.want, rr.Body.String())
			}
			if tc.want == http.StatusBadRequest && !strings.Contains(rr.Body.String(), "assistant_memory") {
				t.Errorf("error should name the field: %s", rr.Body.String())
			}
		})
	}
}

// notify is fail-fast validated: a delivery block must name a connector and
// carry a sane retention.
func TestValidateAutomation_Notify(t *testing.T) {
	handlers := NewDispatcherHandlers(&testDispatcher{}, NewWorkspaceService(nil), logging.NewNopLogger())

	cases := []struct {
		name    string
		notify  *models.NotifyConfig
		wantErr bool
	}{
		{"absent passes", nil, false},
		{"connector only passes", &models.NotifyConfig{Connector: "my-telegram"}, false},
		{"dedup with retention passes", &models.NotifyConfig{Connector: "tg", Dedup: true, DedupDays: 30}, false},
		{"missing connector rejected", &models.NotifyConfig{Dedup: true}, true},
		{"negative retention rejected", &models.NotifyConfig{Connector: "tg", DedupDays: -1}, true},
		{"absurd retention rejected", &models.NotifyConfig{Connector: "tg", DedupDays: models.MaxDedupDays + 1}, true},
		{"maximum retention passes", &models.NotifyConfig{Connector: "tg", DedupDays: models.MaxDedupDays}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := handlers.validateAutomation(&models.Automation{Name: "ok-name", TaskFile: "task.md", Notify: tc.notify})
			if tc.wantErr && (err == nil || !strings.Contains(err.Error(), "notify")) {
				t.Fatalf("expected notify error, got %v", err)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}

func TestAutomationJournalHandlers(t *testing.T) {
	tmp := t.TempDir()
	mgr := persistence.NewWorkspaceManager(storage.NewPathResolver(tmp, tmp, tmp))
	h := NewDispatcherHandlers(&testDispatcher{mgr: mgr}, NewWorkspaceService(mgr), logging.NewNopLogger())
	call := func(handler http.HandlerFunc, method, ws, name string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/admin/api/dispatcher/workspaces/"+ws+"/automations/"+name+"/journal", nil)
		req.SetPathValue(models.WorkspaceIDParam, ws)
		req.SetPathValue("automation", name)
		rr := httptest.NewRecorder()
		handler(rr, req)
		return rr
	}

	t.Run("reads the stored journal", func(t *testing.T) {
		if err := mgr.WriteJournal("ws", "nightly", "- query A"); err != nil {
			t.Fatal(err)
		}
		rr := call(h.GetAutomationJournal, "GET", "ws", "nightly")
		if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"journal":"- query A"`) {
			t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("a missing journal reads as empty", func(t *testing.T) {
		rr := call(h.GetAutomationJournal, "GET", "ws", "never-ran")
		if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"journal":""`) {
			t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
		}
	})

	t.Run("clearing removes the journal", func(t *testing.T) {
		rr := call(h.ClearAutomationJournal, "DELETE", "ws", "nightly")
		if rr.Code != http.StatusOK {
			t.Fatalf("status %d body %s", rr.Code, rr.Body.String())
		}
		if got, _ := mgr.ReadJournal("ws", "nightly"); got != "" {
			t.Errorf("journal survived the clear: %q", got)
		}
	})

	t.Run("rejects unsafe identifiers", func(t *testing.T) {
		for _, handler := range []http.HandlerFunc{h.GetAutomationJournal, h.ClearAutomationJournal} {
			if rr := call(handler, "GET", "ws", ".."); rr.Code != http.StatusBadRequest {
				t.Errorf("status %d for a traversing automation name, want 400", rr.Code)
			}
		}
	})
}

func TestListAutomations_ExposesJournalFlag(t *testing.T) {
	tmp := t.TempDir()
	mgr := persistence.NewWorkspaceManager(storage.NewPathResolver(tmp, tmp, tmp))
	entry := &automation.AutomationEntry{ID: "ws/a", Workspace: "ws", Name: "a", TaskFile: "t.md", Journal: true}
	entry.Trigger, _ = automation.New(models.TriggerConfig{Type: models.TriggerManual})
	entry.Strategy = &automation.IsolatedStrategy{}
	h := NewDispatcherHandlers(&listDispatcher{testDispatcher: &testDispatcher{mgr: mgr}, entries: []*automation.AutomationEntry{entry}}, NewWorkspaceService(mgr), logging.NewNopLogger())

	rr := httptest.NewRecorder()
	h.ListAutomations(rr, httptest.NewRequest("GET", "/admin/api/dispatcher/automations", nil))
	if !strings.Contains(rr.Body.String(), `"journal":true`) {
		t.Errorf("journal flag missing from the automation list: %s", rr.Body.String())
	}
}

// newActivityTestHandlers seeds one workspace's persisted run history and
// returns handlers backed by a real WorkspaceManager, so the activity feed is
// read exactly as in production.
func newActivityTestHandlers(t *testing.T, ws string, history []models.AutomationRun) *DispatcherHandlers {
	t.Helper()
	tmp := t.TempDir()
	resolver := storage.NewPathResolver(tmp, filepath.Join(tmp, "workspaces"), filepath.Join(tmp, "meta"))
	mgr := persistence.NewWorkspaceManager(resolver)
	if err := os.MkdirAll(resolver.WorkspaceDir(ws), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := mgr.WriteState(ws, &models.AgentState{History: history}); err != nil {
		t.Fatalf("WriteState: %v", err)
	}
	return NewDispatcherHandlers(&testDispatcher{mgr: mgr}, NewWorkspaceService(mgr), logging.NewNopLogger())
}

func activityIDs(t *testing.T, h *DispatcherHandlers) []string {
	t.Helper()
	rr := httptest.NewRecorder()
	h.GetGlobalActivity(rr, httptest.NewRequest("GET", "/admin/api/dispatcher/activity", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GetGlobalActivity: %d %s", rr.Code, rr.Body.String())
	}
	var runs []models.AutomationRun
	if err := json.Unmarshal(rr.Body.Bytes(), &runs); err != nil {
		t.Fatalf("decode activity %q: %v", rr.Body.String(), err)
	}
	if runs == nil {
		t.Fatalf("activity must be a JSON array, got %s", rr.Body.String())
	}
	ids := make([]string, 0, len(runs))
	for _, run := range runs {
		ids = append(ids, run.ID)
	}
	return ids
}

// TestGlobalActivity_ReflectsDeletes guards the "deleted runs still listed on
// the main page" bug: the activity feed reads the same persisted history the
// delete endpoints purge, so a deleted run is gone from it immediately.
func TestGlobalActivity_ReflectsDeletes(t *testing.T) {
	history := []models.AutomationRun{
		{ID: "run_1", WorkspaceID: "ws", AutomationName: "a", Timestamp: time.Unix(1, 0)},
		{ID: "run_2", WorkspaceID: "ws", AutomationName: "a", Timestamp: time.Unix(2, 0)},
		{ID: "run_3", WorkspaceID: "ws", AutomationName: "b", Timestamp: time.Unix(3, 0)},
	}

	t.Run("single run", func(t *testing.T) {
		h := newActivityTestHandlers(t, "ws", history)
		req := httptest.NewRequest("DELETE", "/admin/api/dispatcher/runs/ws/run/run_1", nil)
		req.SetPathValue(models.WorkspaceIDParam, "ws")
		req.SetPathValue("run", "run_1")
		rr := httptest.NewRecorder()
		h.DeleteRun(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("DeleteRun: %d %s", rr.Code, rr.Body.String())
		}
		if got := strings.Join(activityIDs(t, h), ","); got != "run_2,run_3" {
			t.Fatalf("want run_2,run_3 after deleting run_1, got %s", got)
		}
	})

	t.Run("all runs of an automation", func(t *testing.T) {
		h := newActivityTestHandlers(t, "ws", history)
		req := httptest.NewRequest("DELETE", "/admin/api/dispatcher/runs/ws/a", nil)
		req.SetPathValue(models.WorkspaceIDParam, "ws")
		req.SetPathValue("automation", "a")
		rr := httptest.NewRecorder()
		h.DeleteAutomationRuns(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("DeleteAutomationRuns: %d %s", rr.Code, rr.Body.String())
		}
		if got := strings.Join(activityIDs(t, h), ","); got != "run_3" {
			t.Fatalf("want only run_3 after clearing automation a, got %s", got)
		}
	})

	t.Run("no runs is an empty array", func(t *testing.T) {
		h := newActivityTestHandlers(t, "ws", nil)
		if ids := activityIDs(t, h); len(ids) != 0 {
			t.Fatalf("want no runs, got %v", ids)
		}
	})
}

// A duplicate name must be rejected exactly once: one 409, no second response
// body, no registry mutation, and the persisted definition stays untouched
// (the closure used to respond and fall through to Register).
func TestCreateAutomation_DuplicateNameConflicts(t *testing.T) {
	tmp := t.TempDir()
	mgr := persistence.NewWorkspaceManager(storage.NewPathResolver(tmp, tmp, tmp))
	if err := mgr.WriteConfig("ws", &models.WorkspaceConfig{}); err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}
	td := &testDispatcher{mgr: mgr}
	handlers := NewDispatcherHandlers(td, NewWorkspaceService(mgr), logging.NewNopLogger())

	create := func(taskFile string) *httptest.ResponseRecorder {
		body := `{"name":"a","task_file":"` + taskFile + `"}`
		req := httptest.NewRequest("POST", "/admin/api/dispatcher/workspaces/ws/automations", strings.NewReader(body))
		req.SetPathValue(models.WorkspaceIDParam, "ws")
		rr := httptest.NewRecorder()
		handlers.CreateAutomation(rr, req)
		return rr
	}

	if rr := create("task1.md"); rr.Code != http.StatusOK {
		t.Fatalf("first create status = %d: %s", rr.Code, rr.Body.String())
	}
	rr := create("task2.md")

	if rr.Code != http.StatusConflict {
		t.Fatalf("duplicate status = %d, want 409: %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "created") {
		t.Errorf("duplicate response must not report success: %s", rr.Body.String())
	}
	if n := strings.Count(rr.Body.String(), "{"); n != 1 {
		t.Errorf("expected a single JSON response, got %d objects: %s", n, rr.Body.String())
	}
	if !slices.Equal(td.registered, []string{"ws/a"}) {
		t.Errorf("registered = %v, want only the original", td.registered)
	}
	cfg, err := mgr.ReadConfig("ws")
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}
	if len(cfg.Automations) != 1 || cfg.Automations[0].TaskFile != "task1.md" {
		t.Errorf("persisted automations = %+v, want the original task1.md only", cfg.Automations)
	}
}

// Updating an automation that is not in config is a single 404 and never
// registers a definition with the dispatcher.
func TestUpdateAutomation_UnknownNameReturnsNotFound(t *testing.T) {
	tmp := t.TempDir()
	mgr := persistence.NewWorkspaceManager(storage.NewPathResolver(tmp, tmp, tmp))
	if err := mgr.WriteConfig("ws", &models.WorkspaceConfig{}); err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}
	td := &testDispatcher{mgr: mgr}
	handlers := NewDispatcherHandlers(td, NewWorkspaceService(mgr), logging.NewNopLogger())

	req := httptest.NewRequest("PUT", "/admin/api/dispatcher/workspaces/ws/automations/ghost", strings.NewReader(`{"name":"ghost","task_file":"task.md"}`))
	req.SetPathValue(models.WorkspaceIDParam, "ws")
	req.SetPathValue("automation", "ghost")
	rr := httptest.NewRecorder()
	handlers.UpdateAutomation(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rr.Code, rr.Body.String())
	}
	if len(td.registered) != 0 {
		t.Errorf("registered = %v, want none", td.registered)
	}
}
