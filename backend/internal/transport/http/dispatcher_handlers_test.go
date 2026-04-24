package api

import (
	"context"
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
	mgr *persistence.WorkspaceManager
}

func (t *testDispatcher) Persistence() *persistence.WorkspaceManager { return t.mgr }
func (t *testDispatcher) Register(ws string, a *models.Automation) error { return nil }
func (t *testDispatcher) Unregister(ws, name string) error { return nil }
func (t *testDispatcher) ListAll() []*automation.AutomationEntry { return nil }
func (t *testDispatcher) Trigger(ctx context.Context, ws, name string) error { return nil }
func (t *testDispatcher) StopAutomation(ws string) error { return nil }
func (t *testDispatcher) Metrics() *automation.DispatcherMetrics { return &automation.DispatcherMetrics{} }
func (t *testDispatcher) Events() *automation.EventBus { return nil }
func (t *testDispatcher) GlobalActivity() []models.AutomationRun { return nil }
func (t *testDispatcher) UnregisterWorkspace(ws string) {}
func (t *testDispatcher) ClearWorkspaceHistory(ws string) {}

func TestCreateWorkspace_Isolation(t *testing.T) {
	tmpWorkspaces := t.TempDir()
	tmpMetadata := t.TempDir()
	resolver := storage.NewPathResolver(tmpWorkspaces, tmpWorkspaces, tmpMetadata)
	mgr := persistence.NewWorkspaceManager(resolver)
	
	dispatcher := &testDispatcher{mgr: mgr}
	handlers := NewDispatcherHandlers(dispatcher, logging.NewNopLogger())

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

func TestDispatcherHandlers_Validation(t *testing.T) {
	tmp := t.TempDir()
	resolver := storage.NewPathResolver(tmp, tmp, tmp)
	mgr := persistence.NewWorkspaceManager(resolver)
	dispatcher := &testDispatcher{mgr: mgr}
	handlers := NewDispatcherHandlers(dispatcher, logging.NewNopLogger())

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
