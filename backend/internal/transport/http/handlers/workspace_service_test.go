package handlers

import (
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
	"os"
	"path/filepath"
	"testing"
)

func setupWS(t *testing.T) (WorkspaceService, string) {
	tmp := t.TempDir()
	resolver := storage.NewPathResolver(tmp, tmp, tmp)
	mgr := persistence.NewWorkspaceManager(resolver)
	return NewWorkspaceService(mgr), tmp
}

func TestWorkspaceService_GetConfig_SaveConfig(t *testing.T) {
	svc, _ := setupWS(t)
	wsID := "test-ws"

	cfg, err := svc.GetConfig(wsID)
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}

	cfg.Temperature = 0.5
	cfg.Model = "test-model"
	if err := svc.SaveConfig(wsID, cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	loaded, _ := svc.GetConfig(wsID)
	if loaded.Temperature != 0.5 || loaded.Model != "test-model" {
		t.Errorf("config round-trip failed: got temp=%v model=%v", loaded.Temperature, loaded.Model)
	}
}

func TestWorkspaceService_MutateConfig(t *testing.T) {
	svc, _ := setupWS(t)
	wsID := "mutate-test"

	if err := svc.MutateConfig(wsID, func(cfg *models.WorkspaceConfig) {
		cfg.Temperature = 0.3
	}); err != nil {
		t.Fatalf("MutateConfig: %v", err)
	}

	cfg, _ := svc.GetConfig(wsID)
	if cfg.Temperature != 0.3 {
		t.Errorf("expected Temperature=0.3, got %v", cfg.Temperature)
	}
}

func TestWorkspaceService_CreateWorkspace(t *testing.T) {
	svc, tmp := setupWS(t)
	wsID := "create-test"

	files := map[string]string{
		"README.md": "# Workspace",
		"task.md":   "Do something",
	}
	if err := svc.CreateWorkspace(wsID, &models.WorkspaceConfig{Temperature: 0.7}, files); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	wsPath := filepath.Join(tmp, wsID)
	if _, err := filepath.Glob(wsPath); err != nil {
		t.Errorf("workspace dir not created: %v", err)
	}

	for name := range files {
		content, err := svc.ReadTaskFile(wsID, name)
		if err != nil {
			t.Errorf("ReadTaskFile(%s): %v", name, err)
		}
		if content != files[name] {
			t.Errorf("%s: expected %q, got %q", name, files[name], content)
		}
	}

	cfg, _ := svc.GetConfig(wsID)
	if cfg.Temperature != 0.7 {
		t.Errorf("expected Temperature=0.7, got %v", cfg.Temperature)
	}
}

func TestWorkspaceService_DeleteRun_PurgesHistory(t *testing.T) {
	tmp := t.TempDir()
	resolver := storage.NewPathResolver(tmp, tmp, tmp)
	mgr := persistence.NewWorkspaceManager(resolver)
	svc := NewWorkspaceService(mgr)
	wsID := "purge-test"

	matching := models.AutomationRun{
		ID:             "run_1",
		WorkspaceID:    wsID,
		AutomationName: "task-a",
		Model:          "model-1",
		RunDirName:     "20260814T120000Z_abcdef01",
	}
	otherTask := models.AutomationRun{
		ID:             "run_2",
		WorkspaceID:    wsID,
		AutomationName: "task-b",
		Model:          "model-1",
		RunDirName:     "20260814T120000Z_abcdef02",
	}
	otherDir := models.AutomationRun{
		ID:             "run_3",
		WorkspaceID:    wsID,
		AutomationName: "task-a",
		Model:          "model-1",
		RunDirName:     "20260814T130000Z_abcdef03",
	}
	state := &models.AgentState{
		History:  []models.AutomationRun{matching, otherTask, otherDir},
		LastRuns: map[string]*models.AutomationRun{"task-a": &matching},
	}
	if err := mgr.WriteState(wsID, state); err != nil {
		t.Fatalf("WriteState: %v", err)
	}

	if err := svc.DeleteRunByID(wsID, matching.ID); err != nil {
		t.Fatalf("DeleteRunByID: %v", err)
	}

	loaded, err := mgr.ReadState(wsID)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if len(loaded.History) != 2 {
		t.Fatalf("expected 2 history entries, got %d", len(loaded.History))
	}
	for _, run := range loaded.History {
		if run.ID == matching.ID {
			t.Errorf("matching run was not purged from history: %+v", run)
		}
	}
	if _, ok := loaded.LastRuns["task-a"]; ok {
		t.Error("LastRuns entry for purged run was not removed")
	}
}

func TestWorkspaceService_DeleteRun_NonExistentIDErrors(t *testing.T) {
	tmp := t.TempDir()
	resolver := storage.NewPathResolver(tmp, tmp, tmp)
	mgr := persistence.NewWorkspaceManager(resolver)
	svc := NewWorkspaceService(mgr)
	wsID := "purge-nomatch"

	run := models.AutomationRun{
		ID:             "run_1",
		WorkspaceID:    wsID,
		AutomationName: "task-a",
		Model:          "model-1",
		RunDirName:     "20260814T120000Z_abcdef01",
	}
	if err := mgr.WriteState(wsID, &models.AgentState{History: []models.AutomationRun{run}}); err != nil {
		t.Fatalf("WriteState: %v", err)
	}

	// Deleting a run ID that is not in history must fail and leave it intact.
	if err := svc.DeleteRunByID(wsID, "run_does_not_exist"); err == nil {
		t.Fatal("expected error deleting a non-existent run ID")
	}

	loaded, err := mgr.ReadState(wsID)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if len(loaded.History) != 1 {
		t.Fatalf("expected 1 history entry preserved, got %d", len(loaded.History))
	}
}

func TestWorkspaceService_DeleteRun_MissingStateErrors(t *testing.T) {
	tmp := t.TempDir()
	resolver := storage.NewPathResolver(tmp, tmp, tmp)
	mgr := persistence.NewWorkspaceManager(resolver)
	svc := NewWorkspaceService(mgr)

	if err := svc.DeleteRunByID("no-state-ws", "run_1"); err == nil {
		t.Fatal("expected error deleting a run from a workspace with no state")
	}
}

func TestWorkspaceService_DeleteRun_DoesNotCreateWorkspaceDirs(t *testing.T) {
	tmp := t.TempDir()
	resolver := storage.NewPathResolver(tmp, tmp, tmp)
	mgr := persistence.NewWorkspaceManager(resolver)
	svc := NewWorkspaceService(mgr)
	wsID := "ghost-ws"

	// Pruning a run from a workspace that never existed must not create the
	// workspace/internal directories (the not-found check runs before the lock,
	// whose AcquireLock has a mkdir side effect).
	if err := svc.DeleteRunByID(wsID, "run_x"); err == nil {
		t.Fatal("expected error deleting from a nonexistent workspace")
	}
	if _, err := os.Stat(filepath.Join(tmp, wsID)); !os.IsNotExist(err) {
		t.Errorf("workspace dir was created for a nonexistent workspace: %v", err)
	}
	if _, err := os.Stat(resolver.State(wsID)); !os.IsNotExist(err) {
		t.Errorf("state file was created for a nonexistent workspace: %v", err)
	}
}

func TestWorkspaceService_Lifecycle(t *testing.T) {
	svc, _ := setupWS(t)
	wsID := "lifecycle-test"

	if err := svc.CreateWorkspace(wsID, &models.WorkspaceConfig{}, nil); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	if err := svc.WriteTaskFile(wsID, "notes.txt", "hello"); err != nil {
		t.Fatalf("WriteTaskFile: %v", err)
	}

	content, err := svc.ReadTaskFile(wsID, "notes.txt")
	if err != nil {
		t.Fatalf("ReadTaskFile: %v", err)
	}
	if content != "hello" {
		t.Errorf("expected 'hello', got %q", content)
	}

	files, err := svc.ListFiles(wsID)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(files) == 0 {
		t.Error("expected at least one file")
	}

	if err := svc.DeleteTaskFile(wsID, "notes.txt"); err != nil {
		t.Fatalf("DeleteTaskFile: %v", err)
	}

	after, _ := svc.ReadTaskFile(wsID, "notes.txt")
	if after != "" {
		t.Error("expected empty content after delete")
	}

	workspaces, err := svc.ListWorkspaces()
	if err != nil {
		t.Fatalf("ListWorkspaces: %v", err)
	}
	found := false
	for _, w := range workspaces {
		if w.ID == wsID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("workspace %s not in ListWorkspaces", wsID)
	}

	if err := svc.DeleteWorkspace(wsID); err != nil {
		t.Fatalf("DeleteWorkspace: %v", err)
	}
}

// TestWorkspaceService_DeleteAutomationRuns_RemovesTreeAndPurgesHistory
// verifies the bulk "clear all runs" path: it must remove every per-model run
// directory for the automation AND purge matching history/LastRuns so deleted
// runs do not resurface in the UI.
func TestWorkspaceService_DeleteAutomationRuns_RemovesTreeAndPurgesHistory(t *testing.T) {
	tmp := t.TempDir()
	resolver := storage.NewPathResolver(tmp, tmp, tmp)
	mgr := persistence.NewWorkspaceManager(resolver)
	svc := NewWorkspaceService(mgr)
	wsID := "bulk-test"
	automation := "task-a"

	for _, model := range []string{"model-1", "model-2"} {
		dir := resolver.WorkspaceAutomationRunsDirModel(wsID, model, automation)
		if err := os.MkdirAll(filepath.Join(dir, "20260814T120000Z_aaaaaa01"), 0755); err != nil {
			t.Fatalf("seed run dir: %v", err)
		}
	}
	keepDir := resolver.WorkspaceAutomationRunsDirModel(wsID, "model-1", "other-task")
	if err := os.MkdirAll(filepath.Join(keepDir, "20260814T120000Z_bbbbbb02"), 0755); err != nil {
		t.Fatalf("seed keep dir: %v", err)
	}

	run := models.AutomationRun{
		ID:             "run_1",
		WorkspaceID:    wsID,
		AutomationName: automation,
		Model:          "model-1",
		RunDirName:     "20260814T120000Z_aaaaaa01",
	}
	other := models.AutomationRun{
		ID:             "run_2",
		WorkspaceID:    wsID,
		AutomationName: "other-task",
		Model:          "model-1",
		RunDirName:     "20260814T120000Z_bbbbbb02",
	}
	state := &models.AgentState{
		History:  []models.AutomationRun{run, other},
		LastRuns: map[string]*models.AutomationRun{automation: &run, "other-task": &other},
	}
	if err := mgr.WriteState(wsID, state); err != nil {
		t.Fatalf("WriteState: %v", err)
	}

	if err := svc.DeleteAutomationRuns(wsID, automation); err != nil {
		t.Fatalf("DeleteAutomationRuns: %v", err)
	}

	for _, model := range []string{"model-1", "model-2"} {
		if _, err := os.Stat(resolver.WorkspaceAutomationRunsDirModel(wsID, model, automation)); !os.IsNotExist(err) {
			t.Errorf("automation run dir for %s still exists", model)
		}
	}
	if _, err := os.Stat(keepDir); os.IsNotExist(err) {
		t.Error("unrelated automation run dir was removed")
	}

	loaded, err := mgr.ReadState(wsID)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if len(loaded.History) != 1 || loaded.History[0].AutomationName != "other-task" {
		t.Errorf("expected only other-task in history, got %+v", loaded.History)
	}
	if _, ok := loaded.LastRuns[automation]; ok {
		t.Error("LastRuns entry for cleared automation was not removed")
	}
	if _, ok := loaded.LastRuns["other-task"]; !ok {
		t.Error("LastRuns entry for other-task was incorrectly removed")
	}

	// The bulk delete removes the automation's dir under each model; empty model
	// parents that no longer hold any run should also be pruned. model-2 only
	// held this automation's runs, so it must be gone; model-1 still holds
	// other-task, so it must remain.
	if _, err := os.Stat(resolver.WorkspaceRunsDir(wsID)); os.IsNotExist(err) {
		t.Error("runs root was removed even though other-task remains")
	}
	if _, err := os.Stat(resolver.WorkspaceAutomationRunsDirModel(wsID, "model-2", automation)); !os.IsNotExist(err) {
		t.Error("model-2 dir was not pruned after its last automation run was removed")
	}
}

// TestWorkspaceService_DeleteRun_PrunesEmptyParents verifies that deleting the
// last run of an automation prunes the now-empty {model}/{automation} and
// {model} parent dirs, while deleting one of several runs keeps the parents.
func TestWorkspaceService_DeleteRun_PrunesEmptyParents(t *testing.T) {
	tmp := t.TempDir()
	resolver := storage.NewPathResolver(tmp, tmp, tmp)
	mgr := persistence.NewWorkspaceManager(resolver)
	svc := NewWorkspaceService(mgr)
	wsID := "prune-parents"
	model := "model-1"
	automation := "task-a"

	seedRun := func(runDir string) {
		if err := os.MkdirAll(filepath.Join(resolver.RunDir(wsID, model, automation, runDir)), 0755); err != nil {
			t.Fatalf("seed run dir: %v", err)
		}
	}
	seedRun("20260814T120000Z_aaaaaa01")
	seedRun("20260814T120000Z_aaaaaa02")

	state := &models.AgentState{
		History: []models.AutomationRun{
			{ID: "run_1", WorkspaceID: wsID, AutomationName: automation, Model: model, RunDirName: "20260814T120000Z_aaaaaa01"},
			{ID: "run_2", WorkspaceID: wsID, AutomationName: automation, Model: model, RunDirName: "20260814T120000Z_aaaaaa02"},
		},
	}
	if err := mgr.WriteState(wsID, state); err != nil {
		t.Fatalf("WriteState: %v", err)
	}

	// Delete one of two runs: parents must remain.
	if err := svc.DeleteRunByID(wsID, "run_1"); err != nil {
		t.Fatalf("DeleteRunByID: %v", err)
	}
	if _, err := os.Stat(resolver.WorkspaceAutomationRunsDirModel(wsID, model, automation)); err != nil {
		t.Fatalf("automation parent should remain after deleting one of two runs: %v", err)
	}

	// Delete the last run: parents must be pruned.
	if err := svc.DeleteRunByID(wsID, "run_2"); err != nil {
		t.Fatalf("DeleteRunByID: %v", err)
	}
	if _, err := os.Stat(resolver.WorkspaceAutomationRunsDirModel(wsID, model, automation)); !os.IsNotExist(err) {
		t.Errorf("automation parent dir was not pruned after deleting last run")
	}
	if _, err := os.Stat(resolver.WorkspaceRunsDir(wsID)); os.IsNotExist(err) {
		t.Error("runs root should remain after deleting last run")
	}
}

// TestWorkspaceService_DeleteRunByID_RemovesRunDir verifies that deleting a run
// by ID also removes its on-disk run directory (when a run_dir_name is known).
func TestWorkspaceService_DeleteRunByID_RemovesRunDir(t *testing.T) {
	tmp := t.TempDir()
	resolver := storage.NewPathResolver(tmp, tmp, tmp)
	mgr := persistence.NewWorkspaceManager(resolver)
	svc := NewWorkspaceService(mgr)
	wsID := "dir-removal"
	model := "model-1"
	automation := "task-a"
	runDir := "20260814T120000Z_aaaaaa01"

	runPath := resolver.RunDir(wsID, model, automation, runDir)
	if err := os.MkdirAll(runPath, 0755); err != nil {
		t.Fatalf("seed run dir: %v", err)
	}
	if err := mgr.WriteState(wsID, &models.AgentState{
		History: []models.AutomationRun{
			{ID: "run_1", WorkspaceID: wsID, AutomationName: automation, Model: model, RunDirName: runDir},
		},
	}); err != nil {
		t.Fatalf("WriteState: %v", err)
	}

	if err := svc.DeleteRunByID(wsID, "run_1"); err != nil {
		t.Fatalf("DeleteRunByID: %v", err)
	}
	if _, err := os.Stat(runPath); !os.IsNotExist(err) {
		t.Error("run directory was not removed on delete by ID")
	}
}

// TestWorkspaceService_DeleteRunByID_LegacyRunPurgesHistory verifies that a run
// recorded before run_dir_name was persisted (no on-disk dir) is still purged
// from history by ID without error.
func TestWorkspaceService_DeleteRunByID_LegacyRunPurgesHistory(t *testing.T) {
	tmp := t.TempDir()
	resolver := storage.NewPathResolver(tmp, tmp, tmp)
	mgr := persistence.NewWorkspaceManager(resolver)
	svc := NewWorkspaceService(mgr)
	wsID := "legacy-run"

	if err := mgr.WriteState(wsID, &models.AgentState{
		History: []models.AutomationRun{
			{ID: "run_1", WorkspaceID: wsID, AutomationName: "task-a", Model: "model-1"},
		},
	}); err != nil {
		t.Fatalf("WriteState: %v", err)
	}

	if err := svc.DeleteRunByID(wsID, "run_1"); err != nil {
		t.Fatalf("DeleteRunByID for legacy run should not error: %v", err)
	}
	loaded, err := mgr.ReadState(wsID)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if len(loaded.History) != 0 {
		t.Errorf("expected legacy run to be purged from history, got %d entries", len(loaded.History))
	}
}

