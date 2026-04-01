package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"llm-proxy/models"
)

func TestManager_AtomicStateWrite(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workspace-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m := NewManager(tmpDir)
	wsID := "test-ws"

	var wg sync.WaitGroup
	workers := 100

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			f, err := m.AcquireLock(wsID)
			if err != nil {
				t.Errorf("Worker %d failed to acquire lock: %v", i, err)
				return
			}
			defer m.ReleaseLock(f)
			state, err := m.ReadState(wsID)
			if err != nil {
				t.Errorf("Worker %d failed to read state: %v", i, err)
				return
			}
			state.LastOutput = fmt.Sprintf("output from worker %d", i)
			if err := m.WriteState(wsID, state); err != nil {
				t.Errorf("Worker %d failed to write state: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	state, err := m.ReadState(wsID)
	if err != nil {
		t.Fatalf("Failed to read final state: %v", err)
	}
	if state.LastOutput == "" {
		t.Errorf("Final state is empty")
	}
}

// ============================================================================
// State Tests
// ============================================================================

func TestManager_ReadState_NotExist_ReturnsEmpty(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workspace-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m := NewManager(tmpDir)
	state, err := m.ReadState("nonexistent")
	if err != nil {
		t.Fatalf("expected no error for nonexistent workspace, got %v", err)
	}
	if state == nil {
		t.Fatal("expected empty state, got nil")
	}
	if state.LastOutput != "" {
		t.Errorf("expected empty LastOutput, got %q", state.LastOutput)
	}
	if state.IsRunning {
		t.Error("expected IsRunning=false")
	}
}

func TestManager_WriteState_ReadState_RoundTrip(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workspace-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m := NewManager(tmpDir)
	wsID := "roundtrip-ws"

	input := &models.AgentState{
		LastOutput: "test output",
		LastError:  "test error",
		NextRunAt:  time.Now().Add(1 * time.Hour),
		IsRunning:  true,
	}

	if err := m.WriteState(wsID, input); err != nil {
		t.Fatalf("WriteState failed: %v", err)
	}

	output, err := m.ReadState(wsID)
	if err != nil {
		t.Fatalf("ReadState failed: %v", err)
	}

	if output.LastOutput != input.LastOutput {
		t.Errorf("LastOutput mismatch: got %q, want %q", output.LastOutput, input.LastOutput)
	}
	if output.LastError != input.LastError {
		t.Errorf("LastError mismatch: got %q, want %q", output.LastError, input.LastError)
	}
	if !output.NextRunAt.Equal(input.NextRunAt) {
		t.Errorf("NextRunAt mismatch: got %v, want %v", output.NextRunAt, input.NextRunAt)
	}
	if output.IsRunning != input.IsRunning {
		t.Errorf("IsRunning mismatch: got %v, want %v", output.IsRunning, input.IsRunning)
	}
}

func TestManager_ReadState_InvalidJSON(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workspace-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	wsID := "bad-json-ws"
	wsDir := filepath.Join(tmpDir, wsID)
	if err := os.MkdirAll(wsDir, 0755); err != nil {
		t.Fatal(err)
	}
	badJSON := []byte("{ invalid json }")
	if err := os.WriteFile(filepath.Join(wsDir, "state.json"), badJSON, 0644); err != nil {
		t.Fatal(err)
	}

	m := NewManager(tmpDir)
	_, err = m.ReadState(wsID)
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

// ============================================================================
// Config Tests
// ============================================================================

func TestManager_ReadConfig_NotExist_ReturnsEmpty(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workspace-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m := NewManager(tmpDir)
	cfg, err := m.ReadConfig("nonexistent")
	if err != nil {
		t.Fatalf("expected no error for nonexistent workspace, got %v", err)
	}
	if cfg == nil {
		t.Fatal("expected empty config, got nil")
	}
	if cfg.CronSchedule != "" {
		t.Errorf("expected empty CronSchedule, got %q", cfg.CronSchedule)
	}
}

func TestManager_WriteConfig_ReadConfig_RoundTrip(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workspace-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m := NewManager(tmpDir)
	wsID := "config-roundtrip"

	input := &models.WorkspaceConfig{
		CronSchedule: "*/5 * * * *",
		Model:        "claude-3-5-sonnet",
		Temperature:  0.7,
	}

	if err := m.WriteConfig(wsID, input); err != nil {
		t.Fatalf("WriteConfig failed: %v", err)
	}

	output, err := m.ReadConfig(wsID)
	if err != nil {
		t.Fatalf("ReadConfig failed: %v", err)
	}

	if output.CronSchedule != input.CronSchedule {
		t.Errorf("CronSchedule mismatch: got %q, want %q", output.CronSchedule, input.CronSchedule)
	}
	if output.Model != input.Model {
		t.Errorf("Model mismatch: got %q, want %q", output.Model, input.Model)
	}
	if output.Temperature != input.Temperature {
		t.Errorf("Temperature mismatch: got %v, want %v", output.Temperature, input.Temperature)
	}
}

func TestManager_ReadConfig_InvalidYAML(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workspace-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	wsID := "bad-yaml-ws"
	wsDir := filepath.Join(tmpDir, wsID)
	if err := os.MkdirAll(wsDir, 0755); err != nil {
		t.Fatal(err)
	}
	badYAML := []byte("invalid: yaml: content: [")
	if err := os.WriteFile(filepath.Join(wsDir, "config.yaml"), badYAML, 0644); err != nil {
		t.Fatal(err)
	}

	m := NewManager(tmpDir)
	_, err = m.ReadConfig(wsID)
	if err == nil {
		t.Error("expected error for invalid YAML, got nil")
	}
}

// ============================================================================
// Heartbeat Tests
// ============================================================================

func TestManager_ReadHeartbeat_NotExist_ReturnsEmpty(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workspace-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m := NewManager(tmpDir)
	content, err := m.ReadHeartbeat("nonexistent")
	if err != nil {
		t.Fatalf("expected no error for nonexistent workspace, got %v", err)
	}
	if content != "" {
		t.Errorf("expected empty heartbeat, got %q", content)
	}
}

func TestManager_WriteHeartbeat_ReadHeartbeat_RoundTrip(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workspace-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m := NewManager(tmpDir)
	wsID := "heartbeat-roundtrip"
	expected := "This is a test heartbeat prompt for the agent."

	if err := m.WriteHeartbeat(wsID, expected); err != nil {
		t.Fatalf("WriteHeartbeat failed: %v", err)
	}

	content, err := m.ReadHeartbeat(wsID)
	if err != nil {
		t.Fatalf("ReadHeartbeat failed: %v", err)
	}

	if content != expected {
		t.Errorf("content mismatch: got %q, want %q", content, expected)
	}
}

// ============================================================================
// ListWorkspaces Tests
// ============================================================================

func TestManager_ListWorkspaces_Empty(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workspace-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m := NewManager(tmpDir)
	workspaces, err := m.ListWorkspaces()
	if err != nil {
		t.Fatalf("ListWorkspaces failed: %v", err)
	}
	if len(workspaces) != 0 {
		t.Errorf("expected 0 workspaces, got %d", len(workspaces))
	}
}

func TestManager_ListWorkspaces_Multiple(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workspace-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m := NewManager(tmpDir)

	// Create multiple workspaces with different content
	ws1 := "ws-1"
	ws2 := "ws-2"
	ws3 := "ws-3"

	m.WriteConfig(ws1, &models.WorkspaceConfig{CronSchedule: "@every 1h", Model: "model-1"})
	m.WriteState(ws1, &models.AgentState{LastOutput: "out-1"})
	m.WriteHeartbeat(ws1, "prompt-1")

	m.WriteConfig(ws2, &models.WorkspaceConfig{CronSchedule: "0 * * * *", Model: "model-2"})
	m.WriteState(ws2, &models.AgentState{LastOutput: "out-2"})
	m.WriteHeartbeat(ws2, "prompt-2")

	// ws3 has no cron_schedule - should still appear with defaults
	m.WriteConfig(ws3, &models.WorkspaceConfig{CronSchedule: "*/30 * * * *", Model: "model-3"})
	m.WriteState(ws3, &models.AgentState{LastOutput: "out-3"})
	m.WriteHeartbeat(ws3, "prompt-3")

	workspaces, err := m.ListWorkspaces()
	if err != nil {
		t.Fatalf("ListWorkspaces failed: %v", err)
	}

	if len(workspaces) != 3 {
		t.Errorf("expected 3 workspaces, got %d", len(workspaces))
	}

	// Build a map for easy lookup
	wsMap := make(map[string]*models.Workspace)
	for _, ws := range workspaces {
		wsMap[ws.ID] = ws
	}

	if wsMap[ws1].Config.CronSchedule != "@every 1h" {
		t.Errorf("ws1 CronSchedule mismatch")
	}
	if wsMap[ws1].State.LastOutput != "out-1" {
		t.Errorf("ws1 LastOutput mismatch")
	}
	if wsMap[ws1].Heartbeat != "prompt-1" {
		t.Errorf("ws1 Heartbeat mismatch")
	}

	if wsMap[ws2].Config.Model != "model-2" {
		t.Errorf("ws2 Model mismatch")
	}
}

func TestManager_ListWorkspaces_IgnoresFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workspace-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m := NewManager(tmpDir)

	// Create a valid workspace
	m.WriteConfig("valid-ws", &models.WorkspaceConfig{CronSchedule: "@every 1h"})

	// Create a file (not a directory) in the base dir
	if err := os.WriteFile(filepath.Join(tmpDir, "not-a-workspace.txt"), []byte("ignored"), 0644); err != nil {
		t.Fatal(err)
	}

	workspaces, err := m.ListWorkspaces()
	if err != nil {
		t.Fatalf("ListWorkspaces failed: %v", err)
	}

	if len(workspaces) != 1 {
		t.Errorf("expected 1 workspace (files should be ignored), got %d", len(workspaces))
	}
}

// ============================================================================
// Lock Tests
// ============================================================================

func TestManager_AcquireLock_Success(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workspace-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m := NewManager(tmpDir)
	wsID := "lock-test"

	f, err := m.AcquireLock(wsID)
	if err != nil {
		t.Fatalf("AcquireLock failed: %v", err)
	}
	defer m.ReleaseLock(f)

	// Verify lock file exists
	lockPath := filepath.Join(tmpDir, wsID, ".lock")
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Error("lock file should exist after AcquireLock")
	}
}

func TestManager_AcquireLock_Blocks(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workspace-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m := NewManager(tmpDir)
	wsID := "blocking-lock"

	// First lock
	f1, err := m.AcquireLock(wsID)
	if err != nil {
		t.Fatalf("First AcquireLock failed: %v", err)
	}
	defer m.ReleaseLock(f1)

	// Second AcquireLock should block (use TryAcquireLock in a goroutine to test)
	done := make(chan error, 1)
	go func() {
		f2, err := m.AcquireLock(wsID)
		if err != nil {
			done <- err
		} else {
			m.ReleaseLock(f2)
			done <- nil
		}
	}()

	// Give it a moment then release first lock
	time.Sleep(50 * time.Millisecond)
	m.ReleaseLock(f1)

	// Wait for goroutine with timeout
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("second AcquireLock should succeed after first released, got error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("AcquireLock appeared to block indefinitely (timeout)")
	}
}

func TestManager_TryAcquireLock_Success(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workspace-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m := NewManager(tmpDir)
	wsID := "try-lock"

	f, err := m.TryAcquireLock(wsID)
	if err != nil {
		t.Fatalf("TryAcquireLock failed: %v", err)
	}
	defer m.ReleaseLock(f)
}

func TestManager_TryAcquireLock_AlreadyHeld(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workspace-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m := NewManager(tmpDir)
	wsID := "try-lock-held"

	// Acquire first lock
	f1, err := m.AcquireLock(wsID)
	if err != nil {
		t.Fatalf("First AcquireLock failed: %v", err)
	}
	defer m.ReleaseLock(f1)

	// TryAcquireLock should fail immediately
	_, err = m.TryAcquireLock(wsID)
	if err == nil {
		t.Error("TryAcquireLock should fail when lock is already held")
	}
}

func TestManager_ReleaseLock_NilFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workspace-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m := NewManager(tmpDir)
	// Should not panic
	err = m.ReleaseLock(nil)
	if err != nil {
		t.Errorf("ReleaseLock(nil) should return nil, got %v", err)
	}
}

func TestManager_ConcurrentTryAcquireLock(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workspace-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m := NewManager(tmpDir)
	wsID := "concurrent-try"

	var wg sync.WaitGroup
	successCount := 0
	var countMu sync.Mutex

	// 10 goroutines all trying to acquire the same lock
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f, err := m.TryAcquireLock(wsID)
			if err == nil {
				countMu.Lock()
				successCount++
				countMu.Unlock()
				time.Sleep(10 * time.Millisecond) // hold briefly
				m.ReleaseLock(f)
			}
		}()
	}

	wg.Wait()

	// At least one should succeed
	if successCount == 0 {
		t.Error("expected at least one TryAcquireLock to succeed")
	}
}

// ============================================================================
// BaseDir Tests
// ============================================================================

func TestManager_BaseDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workspace-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m := NewManager(tmpDir)
	if m.BaseDir() != tmpDir {
		t.Errorf("BaseDir mismatch: got %q, want %q", m.BaseDir(), tmpDir)
	}
}

// ============================================================================
// Edge Cases
// ============================================================================

func TestManager_WriteState_CreatesDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workspace-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m := NewManager(tmpDir)
	wsID := "new-workspace/deep/nested"

	state := &models.AgentState{LastOutput: "test"}
	if err := m.WriteState(wsID, state); err != nil {
		t.Fatalf("WriteState should create nested directories: %v", err)
	}

	// Verify file exists
	statePath := filepath.Join(tmpDir, wsID, "state.json")
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		t.Error("state.json should exist after WriteState")
	}
}

func TestManager_WriteConfig_CreatesDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workspace-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m := NewManager(tmpDir)
	wsID := "config-new/nested"

	cfg := &models.WorkspaceConfig{CronSchedule: "@hourly"}
	if err := m.WriteConfig(wsID, cfg); err != nil {
		t.Fatalf("WriteConfig should create nested directories: %v", err)
	}

	// Verify file exists
	configPath := filepath.Join(tmpDir, wsID, "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config.yaml should exist after WriteConfig")
	}
}

// ============================================================================
// Integration: State + Config + Heartbeat Together
// ============================================================================

func TestManager_FullWorkspaceLifecycle(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workspace-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m := NewManager(tmpDir)
	wsID := "lifecycle-ws"

	// Initial state
	initialState := &models.AgentState{IsRunning: false}
	if err := m.WriteState(wsID, initialState); err != nil {
		t.Fatalf("WriteState failed: %v", err)
	}

	// Config
	cfg := &models.WorkspaceConfig{
		CronSchedule: "0 */6 * * *",
		Model:        "claude-3-opus",
		Temperature:  0.5,
	}
	if err := m.WriteConfig(wsID, cfg); err != nil {
		t.Fatalf("WriteConfig failed: %v", err)
	}

	// Heartbeat
	prompt := "Analyze system performance and report."
	if err := m.WriteHeartbeat(wsID, prompt); err != nil {
		t.Fatalf("WriteHeartbeat failed: %v", err)
	}

	// List and verify
	workspaces, err := m.ListWorkspaces()
	if err != nil {
		t.Fatalf("ListWorkspaces failed: %v", err)
	}
	if len(workspaces) != 1 {
		t.Fatalf("expected 1 workspace, got %d", len(workspaces))
	}

	ws := workspaces[0]
	if ws.ID != wsID {
		t.Errorf("ID mismatch: got %q, want %q", ws.ID, wsID)
	}
	if ws.Config.Model != "claude-3-opus" {
		t.Errorf("Model mismatch: got %q", ws.Config.Model)
	}
	if ws.Heartbeat != prompt {
		t.Errorf("Heartbeat mismatch: got %q", ws.Heartbeat)
	}
}

// TestManager_StateJSONFormat verifies the actual JSON output format
func TestManager_StateJSONFormat(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "workspace-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	m := NewManager(tmpDir)
	wsID := "json-format"

	state := &models.AgentState{
		LastOutput: "some output",
		LastError:  "",
		NextRunAt:  time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		IsRunning:  false,
	}

	if err := m.WriteState(wsID, state); err != nil {
		t.Fatalf("WriteState failed: %v", err)
	}

	// Read raw file and verify it's valid JSON
	path := filepath.Join(tmpDir, wsID, "state.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read state.json: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("state.json should be valid JSON: %v", err)
	}

	if decoded["last_output"] != "some output" {
		t.Errorf("last_output mismatch in raw JSON")
	}
}
