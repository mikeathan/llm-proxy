package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"llm-proxy/models"

	"gopkg.in/yaml.v3"
)

type Manager struct {
	baseDir string
	mu      sync.RWMutex
}

func NewManager(baseDir string) *Manager {
	os.MkdirAll(baseDir, 0755)
	return &Manager{baseDir: baseDir}
}

func (m *Manager) AcquireLock(workspaceID string) (*os.File, error) {
	dirPath := filepath.Join(m.baseDir, workspaceID)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create workspace dir: %w", err)
	}

	lockPath := filepath.Join(dirPath, ".lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open lock file: %w", err)
	}

	// Request an exclusive lock; this will block until acquired
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, fmt.Errorf("failed to acquire flock: %w", err)
	}

	return f, nil
}

func (m *Manager) TryAcquireLock(workspaceID string) (*os.File, error) {
	dirPath := filepath.Join(m.baseDir, workspaceID)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create workspace dir: %w", err)
	}

	lockPath := filepath.Join(dirPath, ".lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open lock file: %w", err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("lock already held or unavailable: %w", err)
	}

	return f, nil
}

// ReleaseLock drops the OS-level flock.
func (m *Manager) ReleaseLock(f *os.File) error {
	if f == nil {
		return nil
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); err != nil {
		return fmt.Errorf("failed to release flock: %w", err)
	}
	return nil
}

// ReadState reads the current state.json for a workspace.
func (m *Manager) ReadState(workspaceID string) (*models.AgentState, error) {
	path := filepath.Join(m.baseDir, workspaceID, "state.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &models.AgentState{}, nil // Return empty state if none exists yet
		}
		return nil, fmt.Errorf("failed to read state.json: %w", err)
	}

	var state models.AgentState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to decode state.json: %w", err)
	}

	return &state, nil
}

// WriteState writes state.json using an atomic Write-Rename pattern to prevent data corruption.
func (m *Manager) WriteState(workspaceID string, state *models.AgentState) error {
	dirPath := filepath.Join(m.baseDir, workspaceID)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return fmt.Errorf("failed to create workspace dir: %w", err)
	}

	tmpFile, err := os.CreateTemp(dirPath, "state-*.json.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp state file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Clean up temp file on error
	defer os.Remove(tmpPath)

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to encode state: %w", err)
	}

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write temp state file: %w", err)
	}

	// Flush to disk
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to sync temp state file: %w", err)
	}

	tmpFile.Close()

	// Atomic rename to replace the actual file
	destPath := filepath.Join(dirPath, "state.json")
	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("failed to rename temp state file to state.json: %w", err)
	}

	return nil
}

// ReadConfig reads the current config.yaml for a workspace.
func (m *Manager) ReadConfig(workspaceID string) (*models.WorkspaceConfig, error) {
	path := filepath.Join(m.baseDir, workspaceID, "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &models.WorkspaceConfig{}, nil
		}
		return nil, fmt.Errorf("failed to read config.yaml: %w", err)
	}

	var config models.WorkspaceConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to decode config.yaml: %w", err)
	}

	return &config, nil
}

func (m *Manager) WriteConfig(workspaceID string, cfg *models.WorkspaceConfig) error {
	dirPath := filepath.Join(m.baseDir, workspaceID)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return fmt.Errorf("failed to create workspace dir: %w", err)
	}

	tmpFile, err := os.CreateTemp(dirPath, "config-*.yaml.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp config file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	data, err := yaml.Marshal(cfg)
	if err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to encode config: %w", err)
	}

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write temp config file: %w", err)
	}

	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to sync temp config file: %w", err)
	}
	tmpFile.Close()

	destPath := filepath.Join(dirPath, "config.yaml")
	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("failed to rename temp config file to config.yaml: %w", err)
	}

	return nil
}

func (m *Manager) ReadHeartbeat(workspaceID string) (string, error) {
	path := filepath.Join(m.baseDir, workspaceID, "heartbeat.md")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to read heartbeat.md: %w", err)
	}
	return string(data), nil
}

func (m *Manager) WriteHeartbeat(workspaceID string, content string) error {
	dirPath := filepath.Join(m.baseDir, workspaceID)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return fmt.Errorf("failed to create workspace dir: %w", err)
	}

	tmpFile, err := os.CreateTemp(dirPath, "heartbeat-*.md.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp heartbeat file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.WriteString(content); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write temp heartbeat file: %w", err)
	}

	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to sync temp heartbeat file: %w", err)
	}
	tmpFile.Close()

	destPath := filepath.Join(dirPath, "heartbeat.md")
	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("failed to rename temp heartbeat file to heartbeat.md: %w", err)
	}

	return nil
}

func (m *Manager) ListWorkspaces() ([]*models.Workspace, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entries, err := os.ReadDir(m.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*models.Workspace{}, nil
		}
		return nil, fmt.Errorf("failed to read workspace directory: %w", err)
	}

	var workspaces []*models.Workspace

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()

		cfg, _ := m.ReadConfig(id)
		state, _ := m.ReadState(id)
		heartbeat, _ := m.ReadHeartbeat(id)

		ws := &models.Workspace{
			ID:        id,
			Config:    *cfg,
			State:     *state,
			Heartbeat: heartbeat,
		}
		workspaces = append(workspaces, ws)
	}

	return workspaces, nil
}

func (m *Manager) BaseDir() string {
	return m.baseDir
}
