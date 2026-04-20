package api

import (
	"context"
	"llm-proxy/internal/core/automation"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/storage"
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
	tmpDir := t.TempDir()
	resolver := storage.NewPathResolver(tmpDir)
	mgr := persistence.NewWorkspaceManager(resolver)
	
	dispatcher := &testDispatcher{mgr: mgr}
	handlers := &DispatcherHandlers{dispatcher: dispatcher}

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
	rootConfig := filepath.Join(tmpDir, workspaceID, models.ConfigFilename)
	if _, err := os.Stat(rootConfig); err == nil {
		t.Error("SECURITY VIOLATION: config.yaml found in workspace root")
	}

	// 2. Config MUST exist in .internal
	internalConfig := resolver.Config(workspaceID)
	if _, err := os.Stat(internalConfig); os.IsNotExist(err) {
		t.Errorf("Config missing from required isolated path: %s", internalConfig)
	}

	// 3. New workspaces should have .internal folder
	internalDir := resolver.InternalDir(workspaceID)
	if _, err := os.Stat(internalDir); os.IsNotExist(err) {
		t.Error(".internal directory was not created during workspace initialization")
	}

	// 4. Root should only contain task files
	entries, _ := os.ReadDir(filepath.Join(tmpDir, workspaceID))
	for _, entry := range entries {
		name := entry.Name()
		// .internal is a directory and allowed
		if entry.IsDir() && name == models.InternalDirName {
			continue
		}
		if name == models.ConfigFilename || name == models.StateFilename {
			t.Errorf("Forbidden file %s found in workspace root", name)
		}
	}
}
