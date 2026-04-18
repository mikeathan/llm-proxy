package storage

import (
	"llm-proxy/models"
	"path/filepath"
	"testing"
)

func TestPathResolver(t *testing.T) {
	workspacesDir := "/tmp/workspaces"
	resolver := NewPathResolver(workspacesDir)

	t.Run("WorkspacesRoot", func(t *testing.T) {
		if got := resolver.WorkspacesRoot(); got != workspacesDir {
			t.Errorf("WorkspacesRoot() = %v, want %v", got, workspacesDir)
		}
	})

	t.Run("WorkspaceDir", func(t *testing.T) {
		id := "test-ws"
		want := filepath.Join(workspacesDir, id)
		if got := resolver.WorkspaceDir(id); got != want {
			t.Errorf("WorkspaceDir() = %v, want %v", got, want)
		}
	})

	t.Run("Config", func(t *testing.T) {
		id := "test-ws"
		want := filepath.Join(workspacesDir, id, models.InternalDirName, models.ConfigFilename)
		if got := resolver.Config(id); got != want {
			t.Errorf("Config() = %v, want %v", got, want)
		}
	})

	t.Run("State", func(t *testing.T) {
		id := "test-ws"
		want := filepath.Join(workspacesDir, id, models.InternalDirName, models.StateFilename)
		if got := resolver.State(id); got != want {
			t.Errorf("State() = %v, want %v", got, want)
		}
	})

	t.Run("InternalDir", func(t *testing.T) {
		id := "test-ws"
		want := filepath.Join(workspacesDir, id, models.InternalDirName)
		if got := resolver.InternalDir(id); got != want {
			t.Errorf("InternalDir() = %v, want %v", got, want)
		}
	})

	t.Run("ProcessLog", func(t *testing.T) {
		id := "test-ws"
		want := filepath.Join(workspacesDir, id, models.InternalDirName, models.ProcessLogFilename)
		if got := resolver.ProcessLog(id); got != want {
			t.Errorf("ProcessLog() = %v, want %v", got, want)
		}
	})

	t.Run("Lock", func(t *testing.T) {
		id := "test-ws"
		want := filepath.Join(workspacesDir, id, models.LockFilename)
		if got := resolver.Lock(id); got != want {
			t.Errorf("Lock() = %v, want %v", got, want)
		}
	})

	t.Run("TaskFile", func(t *testing.T) {
		id := "test-ws"
		filename := "custom.md"
		want := filepath.Join(workspacesDir, id, filename)
		if got := resolver.TaskFile(id, filename); got != want {
			t.Errorf("TaskFile() = %v, want %v", got, want)
		}
	})
}
