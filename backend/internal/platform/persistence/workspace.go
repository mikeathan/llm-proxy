package persistence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"llm-proxy/models"

	"gopkg.in/yaml.v3"
)

// WorkspaceManager handles atomic file I/O for workspaces with flock locking.
type WorkspaceManager struct {
	baseDir string
	mu      sync.RWMutex
}

// NewWorkspaceManager creates a WorkspaceManager at the given base directory.
func NewWorkspaceManager(baseDir string) *WorkspaceManager {
	os.MkdirAll(baseDir, 0755)
	return &WorkspaceManager{baseDir: baseDir}
}

// ============================================================================
// Locking
// ============================================================================

// AcquireLock acquires an exclusive flock on the workspace.
func (m *WorkspaceManager) AcquireLock(workspaceID string) (*os.File, error) {
	dirPath := filepath.Join(m.baseDir, workspaceID)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create workspace dir: %w", err)
	}
	lockPath := filepath.Join(dirPath, ".lock")
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open lock file: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, fmt.Errorf("failed to acquire flock: %w", err)
	}
	return f, nil
}

// TryAcquireLock attempts a non-blocking exclusive flock.
func (m *WorkspaceManager) TryAcquireLock(workspaceID string) (*os.File, error) {
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

// ReleaseLock releases an exclusive flock.
func (m *WorkspaceManager) ReleaseLock(f *os.File) error {
	if f == nil {
		return nil
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); err != nil {
		return fmt.Errorf("failed to release flock: %w", err)
	}
	return nil
}

// ============================================================================
// State
// ============================================================================

// ReadState reads state.json for a workspace. Returns empty state if absent.
func (m *WorkspaceManager) ReadState(workspaceID string) (*models.AgentState, error) {
	path := filepath.Join(m.baseDir, workspaceID, "state.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &models.AgentState{}, nil
		}
		return nil, fmt.Errorf("failed to read state.json: %w", err)
	}
	var state models.AgentState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to decode state.json: %w", err)
	}
	return &state, nil
}

// WriteState writes state.json atomically (temp file + rename + sync).
func (m *WorkspaceManager) WriteState(workspaceID string, state *models.AgentState) error {
	dirPath := filepath.Join(m.baseDir, workspaceID)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return fmt.Errorf("failed to create workspace dir: %w", err)
	}
	tmpFile, err := os.CreateTemp(dirPath, "state-*.json.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp state file: %w", err)
	}
	tmpPath := tmpFile.Name()
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
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to sync temp state file: %w", err)
	}
	tmpFile.Close()

	destPath := filepath.Join(dirPath, "state.json")
	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("failed to rename temp state file to state.json: %w", err)
	}
	return nil
}

// ============================================================================
// Config
// ============================================================================

// ReadConfig reads config.yaml for a workspace. Returns empty config if absent.
func (m *WorkspaceManager) ReadConfig(workspaceID string) (*models.WorkspaceConfig, error) {
	path := filepath.Join(m.baseDir, workspaceID, "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &models.WorkspaceConfig{}, nil
		}
		return nil, fmt.Errorf("failed to read config.yaml: %w", err)
	}
	var cfg models.WorkspaceConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to decode config.yaml: %w", err)
	}
	return &cfg, nil
}

// WriteConfig writes config.yaml atomically.
func (m *WorkspaceManager) WriteConfig(workspaceID string, cfg *models.WorkspaceConfig) error {
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

// ============================================================================
// Heartbeat
// ============================================================================

// ReadHeartbeat reads heartbeat.md. Returns empty string if absent.
func (m *WorkspaceManager) ReadHeartbeat(workspaceID string) (string, error) {
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

// WriteHeartbeat writes heartbeat.md atomically.
func (m *WorkspaceManager) WriteHeartbeat(workspaceID string, content string) error {
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

// ============================================================================
// Task Files
// ============================================================================

// ReadTaskFile reads an arbitrary task file from the workspace.
func (m *WorkspaceManager) ReadTaskFile(workspaceID, filename string) (string, error) {
	path := filepath.Join(m.baseDir, workspaceID, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to read %s: %w", filename, err)
	}
	return string(data), nil
}

// WriteTaskFile writes an arbitrary task file atomically.
func (m *WorkspaceManager) WriteTaskFile(workspaceID, filename, content string) error {
	dirPath := filepath.Join(m.baseDir, workspaceID)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return fmt.Errorf("failed to create workspace dir: %w", err)
	}
	tmpFile, err := os.CreateTemp(dirPath, fmt.Sprintf("%s-*.tmp", filename))
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.WriteString(content); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to sync temp file: %w", err)
	}
	tmpFile.Close()

	destPath := filepath.Join(dirPath, filename)
	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("failed to rename temp file to %s: %w", filename, err)
	}
	return nil
}

// ============================================================================
// Listing
// ============================================================================

// ListWorkspaces returns all workspaces under baseDir.
func (m *WorkspaceManager) ListWorkspaces() ([]*models.Workspace, error) {
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

// BaseDir returns the base directory path.
func (m *WorkspaceManager) BaseDir() string {
	return m.baseDir
}

// LastModified returns the last modification time of a workspace's state.json.
func (m *WorkspaceManager) LastModified(workspaceID string) (time.Time, error) {
	path := filepath.Join(m.baseDir, workspaceID, "state.json")
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return time.Time{}, nil
		}
		return time.Time{}, err
	}
	return info.ModTime(), nil
}

// ListFiles lists all non-hidden files in a workspace directory.
func (m *WorkspaceManager) ListFiles(workspaceID string) ([]string, error) {
	dirPath := filepath.Join(m.baseDir, workspaceID)
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	var files []string
	for _, entry := range entries {
		if !entry.IsDir() && entry.Name() != "" && entry.Name()[0] != '.' {
			files = append(files, entry.Name())
		}
	}
	return files, nil
}

func (m *WorkspaceManager) DeleteTaskFile(workspaceID, filename string) error {
	path := filepath.Join(m.baseDir, workspaceID, filename)
	return os.Remove(path)
}

func (m *WorkspaceManager) DeleteWorkspace(workspaceID string) error {
	path := filepath.Join(m.baseDir, workspaceID)
	return os.RemoveAll(path)
}
