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

func (m *WorkspaceManager) openLockFile(workspaceID string) (*os.File, error) {
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
	return f, nil
}

// AcquireLock acquires an exclusive flock on the workspace.
func (m *WorkspaceManager) AcquireLock(workspaceID string) (*os.File, error) {
	f, err := m.openLockFile(workspaceID)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, fmt.Errorf("failed to acquire flock: %w", err)
	}
	return f, nil
}

// TryAcquireLock attempts a non-blocking exclusive flock.
func (m *WorkspaceManager) TryAcquireLock(workspaceID string) (*os.File, error) {
	f, err := m.openLockFile(workspaceID)
	if err != nil {
		return nil, err
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
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode state: %w", err)
	}
	return storage.WriteAtomic(m.resolver.State(workspaceID), "state-*.json.tmp", data)
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
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to encode config: %w", err)
	}
	return storage.WriteAtomic(m.resolver.Config(workspaceID), "config-*.yaml.tmp", data)
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
	return storage.WriteAtomic(m.resolver.Heartbeat(workspaceID), "heartbeat-*.md.tmp", []byte(content))
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
	return storage.WriteAtomic(m.resolver.TaskFile(workspaceID, filename), fmt.Sprintf("%s-*.tmp", filename), []byte(content))
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
	if err := os.RemoveAll(m.resolver.WorkspaceDir(workspaceID)); err != nil {
		return err
	}
	return os.RemoveAll(m.resolver.SessionsDir(workspaceID))
}

// ============================================================================
// Assistant Sessions
// ============================================================================

// sessionOldDir returns the legacy sessions path inside the workspace directory.
func (m *WorkspaceManager) sessionOldDir(workspaceID string) string {
	return filepath.Join(m.resolver.WorkspaceDir(workspaceID), "sessions")
}

// ReadSession reads a specific assistant session JSON file.
// Falls back to the legacy workspace-located sessions directory if not found in the metadata dir.
func (m *WorkspaceManager) ReadSession(workspaceID, sessionID string) (*models.AssistantSession, error) {
	path := filepath.Join(m.resolver.SessionsDir(workspaceID), sessionID+".json")
	data, err := os.ReadFile(path)
	if err == nil {
		session := &models.AssistantSession{}
		if err := json.Unmarshal(data, session); err != nil {
			return nil, fmt.Errorf("failed to decode session %s: %w", sessionID, err)
		}
		return session, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to read session %s: %w", sessionID, err)
	}

	// Fallback: check legacy path inside workspace
	oldPath := filepath.Join(m.sessionOldDir(workspaceID), sessionID+".json")
	data, err = os.ReadFile(oldPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read session %s: %w", sessionID, err)
	}
	session := &models.AssistantSession{}
	if err := json.Unmarshal(data, session); err != nil {
		return nil, fmt.Errorf("failed to decode session %s: %w", sessionID, err)
	}
	return session, nil
}

// migrateSessionDir moves a session file from the legacy workspace location to the metadata directory.
func (m *WorkspaceManager) migrateSessionDir(workspaceID, sessionID string) error {
	oldPath := filepath.Join(m.sessionOldDir(workspaceID), sessionID+".json")
	newDir := m.resolver.SessionsDir(workspaceID)
	newPath := filepath.Join(newDir, sessionID+".json")

	if err := os.MkdirAll(newDir, 0755); err != nil {
		return fmt.Errorf("failed to create sessions dir: %w", err)
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		return fmt.Errorf("failed to migrate session %s: %w", sessionID, err)
	}
	return nil
}

// WriteSession writes an assistant session JSON file atomically.
func (m *WorkspaceManager) WriteSession(workspaceID string, session *models.AssistantSession) error {
	session.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode session: %w", err)
	}
	destPath := filepath.Join(m.resolver.SessionsDir(workspaceID), session.ID+".json")
	return storage.WriteAtomic(destPath, "session-*.json.tmp", data)
}

// DeleteSession deletes an assistant session JSON file from both locations.
func (m *WorkspaceManager) DeleteSession(workspaceID, sessionID string) error {
	path := filepath.Join(m.resolver.SessionsDir(workspaceID), sessionID+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	oldPath := filepath.Join(m.sessionOldDir(workspaceID), sessionID+".json")
	os.Remove(oldPath)
	return nil
}

// DeleteAllSessions removes all assistant session files for a workspace.
func (m *WorkspaceManager) DeleteAllSessions(workspaceID string) error {
	dir := m.resolver.SessionsDir(workspaceID)
	if err := os.RemoveAll(dir); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	oldDir := m.sessionOldDir(workspaceID)
	if err := os.RemoveAll(oldDir); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ListSessions returns a list of session summaries for a workspace.
// Checks both the metadata directory and the legacy workspace directory.
func (m *WorkspaceManager) ListSessions(workspaceID string) ([]models.SessionBrief, error) {
	// Collect unique session IDs from both new and legacy locations
	seen := map[string]bool{}
	var sessionIDs []string

	collectIDs := func(dir string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
				continue
			}
			id := strings.TrimSuffix(entry.Name(), ".json")
			if !seen[id] {
				seen[id] = true
				sessionIDs = append(sessionIDs, id)
			}
		}
	}

	collectIDs(m.resolver.SessionsDir(workspaceID))
	collectIDs(m.sessionOldDir(workspaceID))

	var briefs []models.SessionBrief
	for _, sessionID := range sessionIDs {
		session, err := m.ReadSession(workspaceID, sessionID)
		if err != nil || session == nil {
			continue
		}

		snippet := firstUserSnippet(session.History)

		briefs = append(briefs, models.SessionBrief{
			ID:        session.ID,
			Snippet:   snippet,
			UpdatedAt: session.UpdatedAt,
			Source:    models.SessionSource(session.ID),
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

// firstUserSnippet returns a short preview of the session's first user message
// for use as a stable conversation title. It falls back to the first non-empty
// content when no user message exists (e.g. system-only sessions).
func firstUserSnippet(history []models.Message) string {
	for _, m := range history {
		if m.Role == models.UserRole && m.Content != "" {
			s := m.Content
			if len(s) > 80 {
				s = s[:77] + "..."
			}
			return s
		}
	}
	for _, m := range history {
		if m.Content != "" {
			s := m.Content
			if len(s) > 80 {
				s = s[:77] + "..."
			}
			return s
		}
	}
	return ""
}

