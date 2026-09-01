package handlers

import (
	"context"
	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/automation"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Minimal hand-rolled mock for dispatcher satisfying the full interface
type testDispatcher struct {
	mgr        *persistence.WorkspaceManager
	stopCalled map[string]bool
}

func (t *testDispatcher) Persistence() *persistence.WorkspaceManager { return t.mgr }
func (t *testDispatcher) Register(ws string, a *models.Automation) error { return nil }
func (t *testDispatcher) Unregister(ws, name string) error { return nil }
func (t *testDispatcher) ListAll() []*automation.AutomationEntry { return nil }
func (t *testDispatcher) Trigger(ctx context.Context, ws, name, _ string) error { return nil }
func (t *testDispatcher) StopAutomation(ws string) error {
	if t.stopCalled != nil {
		t.stopCalled[ws] = true
	}
	return nil
}
func (t *testDispatcher) Metrics() *automation.DispatcherMetrics { return &automation.DispatcherMetrics{} }
func (t *testDispatcher) Events() *automation.EventBus { return nil }
func (t *testDispatcher) GlobalActivity() []models.AutomationRun { return nil }
func (t *testDispatcher) UnregisterWorkspace(ws string) {}
func (t *testDispatcher) ClearWorkspaceHistory(ws string) {}

func TestValidateAutomation_LoopStrategy(t *testing.T) {
	dispatcher := &testDispatcher{}
	handlers := NewDispatcherHandlers(dispatcher, NewWorkspaceService(nil), logging.NewNopLogger())

	cases := []struct {
		name       string
		loopStrategy models.LoopStrategy
		wantErr    bool
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

func TestCreateWorkspace_Isolation(t *testing.T) {	tmpWorkspaces := t.TempDir()
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
		value string
		unsafe bool
	}{
		{"task.md", false},
		{"v1..2.md", false}, // dots inside a name are fine; only traversal is blocked
		{"", true},
		{".", true},
		{"..", true},
		{"a/../b", true},
	}
	for _, tc := range cases {
		if got := isUnsafeFileParam(tc.value); got != tc.unsafe {
			t.Errorf("isUnsafeFileParam(%q) = %v, want %v", tc.value, got, tc.unsafe)
		}
	}
}
