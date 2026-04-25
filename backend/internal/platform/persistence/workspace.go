package persistence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"

	"gopkg.in/yaml.v3"
)

var workspaceFiles = []string{
	models.StateFilename,
	models.ConfigFilename,
}

const sessionsDir = "sessions"

// WorkspaceManager handles atomic file I/O for workspaces with flock locking.
type WorkspaceManager struct {
	resolver *storage.PathResolver
	mu       sync.RWMutex
}

// NewWorkspaceManager creates a WorkspaceManager with the given path resolver.
func NewWorkspaceManager(r *storage.PathResolver) *WorkspaceManager {
	os.MkdirAll(r.WorkspacesRoot(), 0755)
	os.MkdirAll(r.MetadataRoot(), 0755)
	return &WorkspaceManager{resolver: r}
}

// ============================================================================
// Locking
// ============================================================================

// AcquireLock acquires an exclusive flock on the workspace.
func (m *WorkspaceManager) AcquireLock(workspaceID string) (*os.File, error) {
	dirPath := m.resolver.WorkspaceDir(workspaceID)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create workspace dir: %w", err)
	}
	if err := os.MkdirAll(m.resolver.InternalDir(workspaceID), 0755); err != nil {
		return nil, fmt.Errorf("failed to create internal dir: %w", err)
	}
	lockPath := m.resolver.Lock(workspaceID)
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
	dirPath := m.resolver.WorkspaceDir(workspaceID)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create workspace dir: %w", err)
	}
	if err := os.MkdirAll(m.resolver.InternalDir(workspaceID), 0755); err != nil {
		return nil, fmt.Errorf("failed to create internal dir: %w", err)
	}
	lockPath := m.resolver.Lock(workspaceID)
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
	state := &models.AgentState{}
	path := m.resolver.State(workspaceID)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return state, nil
		}
		return state, fmt.Errorf("failed to read state.json: %w", err)
	}
	if err := json.Unmarshal(data, state); err != nil {
		fmt.Printf("FAILED TO DECODE %s: %v\n", path, err)
		return state, fmt.Errorf("failed to decode state.json: %w", err)
	}
	return state, nil
}

// WriteState writes state.json atomically (temp file + rename + sync).
func (m *WorkspaceManager) WriteState(workspaceID string, state *models.AgentState) error {
	dirPath := m.resolver.InternalDir(workspaceID)
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

	destPath := m.resolver.State(workspaceID)
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
	cfg := &models.WorkspaceConfig{}
	path := m.resolver.Config(workspaceID)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("failed to read config.yaml: %w", err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		fmt.Printf("FAILED TO DECODE %s: %v\n", path, err)
		return cfg, fmt.Errorf("failed to decode config.yaml: %w", err)
	}
	return cfg, nil
}

// WriteConfig writes config.yaml atomically.
func (m *WorkspaceManager) WriteConfig(workspaceID string, cfg *models.WorkspaceConfig) error {
	dirPath := m.resolver.InternalDir(workspaceID)
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

	destPath := m.resolver.Config(workspaceID)
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
	path := m.resolver.Heartbeat(workspaceID)
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
	dirPath := m.resolver.WorkspaceDir(workspaceID)
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

	destPath := m.resolver.Heartbeat(workspaceID)
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
	path := m.resolver.TaskFile(workspaceID, filename)
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
	dirPath := m.resolver.WorkspaceDir(workspaceID)
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

	destPath := m.resolver.TaskFile(workspaceID, filename)
	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("failed to rename temp file to %s: %w", filename, err)
	}
	return nil
}

func (m *WorkspaceManager) ListWorkspaces() ([]*models.Workspace, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entries, err := os.ReadDir(m.resolver.WorkspacesRoot())
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

func (m *WorkspaceManager) BaseDir() string {
	return m.resolver.WorkspacesRoot()
}

// GetRelativeWorkspacePath returns the relative path from the current working directory to the base workspace directory.
func (m *WorkspaceManager) GetRelativeWorkspacePath() string {
	cwd, _ := os.Getwd()
	relWs, err := filepath.Rel(cwd, m.resolver.WorkspacesRoot())
	if err != nil {
		return m.resolver.WorkspacesRoot()
	}
	return filepath.Clean(relWs)
}

func (m *WorkspaceManager) LastModified(workspaceID string) (time.Time, error) {
	path := m.resolver.State(workspaceID)
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return time.Time{}, nil
		}
		return time.Time{}, err
	}
	return info.ModTime(), nil
}

func (m *WorkspaceManager) ListFiles(workspaceID string) ([]string, error) {
	dirPath := m.resolver.WorkspaceDir(workspaceID)
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	var files []string
	for _, entry := range entries {

		name := filepath.Base(entry.Name())
		isWorkspaceFile := false
		// Skip if it's a directory, empty, or hidden
		if entry.IsDir() || name == "" || name[0] == '.' {
			continue
		}

		// exclude known workspace files to only list task files
		if slices.Contains(workspaceFiles, name) {
			isWorkspaceFile = true
		}

		if !isWorkspaceFile {
			files = append(files, name)
		}
	}
	return files, nil
}

func (m *WorkspaceManager) DeleteTaskFile(workspaceID, filename string) error {
	path := m.resolver.TaskFile(workspaceID, filename)
	return os.Remove(path)
}

func (m *WorkspaceManager) DeleteWorkspace(workspaceID string) error {
	path := m.resolver.WorkspaceDir(workspaceID)
	return os.RemoveAll(path)
}

// ============================================================================
// Assistant Sessions
// ============================================================================

// ReadSession reads a specific assistant session JSON file.
func (m *WorkspaceManager) ReadSession(workspaceID, sessionID string) (*models.AssistantSession, error) {
	path := filepath.Join(m.resolver.WorkspaceDir(workspaceID), sessionsDir, sessionID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Not found is not an error
		}
		return nil, fmt.Errorf("failed to read session %s: %w", sessionID, err)
	}

	session := &models.AssistantSession{}
	if err := json.Unmarshal(data, session); err != nil {
		return nil, fmt.Errorf("failed to decode session %s: %w", sessionID, err)
	}
	return session, nil
}

// WriteSession writes an assistant session JSON file atomically.
func (m *WorkspaceManager) WriteSession(workspaceID string, session *models.AssistantSession) error {
	dirPath := filepath.Join(m.resolver.WorkspaceDir(workspaceID), sessionsDir)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return fmt.Errorf("failed to create sessions dir: %w", err)
	}

	filename := session.ID + ".json"
	tmpFile, err := os.CreateTemp(dirPath, "session-*.json.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp session file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	session.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to encode session: %w", err)
	}

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to write temp session file: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to sync temp session file: %w", err)
	}
	tmpFile.Close()

	destPath := filepath.Join(dirPath, filename)
	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("failed to rename temp session file: %w", err)
	}
	return nil
}

// DeleteSession deletes an assistant session JSON file.
func (m *WorkspaceManager) DeleteSession(workspaceID, sessionID string) error {
	path := filepath.Join(m.resolver.WorkspaceDir(workspaceID), sessionsDir, sessionID+".json")
	return os.Remove(path)
}

// ListSessions returns a list of session summaries for a workspace.
func (m *WorkspaceManager) ListSessions(workspaceID string) ([]models.SessionBrief, error) {
	dirPath := filepath.Join(m.resolver.WorkspaceDir(workspaceID), sessionsDir)
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []models.SessionBrief{}, nil
		}
		return nil, err
	}

	var briefs []models.SessionBrief
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		sessionID := strings.TrimSuffix(entry.Name(), ".json")
		// We could optimize by only reading the first few bytes, but sessions are usually small.
		session, err := m.ReadSession(workspaceID, sessionID)
		if err != nil || session == nil {
			continue
		}

		snippet := ""
		if len(session.History) > 0 {
			// Find the last user message or assistant reply for the snippet
			for i := len(session.History) - 1; i >= 0; i-- {
				if session.History[i].Content != "" {
					snippet = session.History[i].Content
					if len(snippet) > 80 {
						snippet = snippet[:77] + "..."
					}
					break
				}
			}
		}

		briefs = append(briefs, models.SessionBrief{
			ID:        session.ID,
			Snippet:   snippet,
			UpdatedAt: session.UpdatedAt,
		})
	}

	// Sort by updated at descending
	slices.SortFunc(briefs, func(a, b models.SessionBrief) int {
		if b.UpdatedAt.After(a.UpdatedAt) {
			return 1
		}
		if a.UpdatedAt.After(b.UpdatedAt) {
			return -1
		}
		return 0
	})

	return briefs, nil
}
