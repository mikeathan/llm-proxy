package handlers

import (
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
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
