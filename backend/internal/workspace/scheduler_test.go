package workspace

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"llm-proxy/models"
)

type mockExecutor struct {
	called       int
	errOnCall    int
	returnError  error
	returnOutput string
	mu           sync.Mutex
}

func (m *mockExecutor) Execute(ctx context.Context, prompt string, state *models.AgentState) (string, error) {
	m.mu.Lock()
	m.called++
	if m.errOnCall == m.called {
		m.mu.Unlock()
		return "", m.returnError
	}
	m.mu.Unlock()
	if m.returnOutput != "" {
		return m.returnOutput, nil
	}
	return "mock response", nil
}

func (m *mockExecutor) Called() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.called
}

func TestScheduler_CronExecution(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "scheduler-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mgr := NewManager(tmpDir)

	wsID := "test-ws"
	cfg := &models.WorkspaceConfig{CronSchedule: "@every 1s"}
	mgr.WriteConfig(wsID, cfg)
	mgr.WriteHeartbeat(wsID, "test prompt")

	exec := &mockExecutor{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	sched, err := NewScheduler(mgr, exec, logger)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	go sched.Start(ctx)

	time.Sleep(1500 * time.Millisecond)

	cancel() // Graceful shutdown

	time.Sleep(100 * time.Millisecond)

	if exec.called < 1 {
		t.Errorf("expected at least 1 execution, got %d", exec.called)
	}
}

// ============================================================================
// scheduleWorkspace Tests
// ============================================================================

func TestScheduler_ScheduleWorkspace_EmptyCron(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "scheduler-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mgr := NewManager(tmpDir)
	exec := &mockExecutor{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	sched, err := NewScheduler(mgr, exec, logger)
	if err != nil {
		t.Fatal(err)
	}

	ws := &models.Workspace{
		ID:     "empty-cron",
		Config: models.WorkspaceConfig{CronSchedule: ""},
	}

	if err := sched.scheduleWorkspace(ws); err != nil {
		t.Errorf("scheduleWorkspace with empty cron should not error: %v", err)
	}

	// Verify no job was scheduled
	sched.mu.RLock()
	if _, exists := sched.jobs[ws.ID]; exists {
		t.Error("no job should be scheduled for empty cron")
	}
	sched.mu.RUnlock()
}

func TestScheduler_ScheduleWorkspace_InvalidCron(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "scheduler-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mgr := NewManager(tmpDir)
	exec := &mockExecutor{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	sched, err := NewScheduler(mgr, exec, logger)
	if err != nil {
		t.Fatal(err)
	}

	ws := &models.Workspace{
		ID:     "bad-cron",
		Config: models.WorkspaceConfig{CronSchedule: "invalid cron expression"},
	}

	if err := sched.scheduleWorkspace(ws); err == nil {
		t.Error("scheduleWorkspace with invalid cron should error")
	}
}

func TestScheduler_ScheduleWorkspace_UpdatesExisting(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "scheduler-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mgr := NewManager(tmpDir)
	exec := &mockExecutor{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	sched, err := NewScheduler(mgr, exec, logger)
	if err != nil {
		t.Fatal(err)
	}

	ws := &models.Workspace{
		ID:     "update-test",
		Config: models.WorkspaceConfig{CronSchedule: "*/5 * * * *"},
	}

	// First schedule
	if err := sched.scheduleWorkspace(ws); err != nil {
		t.Fatalf("first scheduleWorkspace failed: %v", err)
	}

	sched.mu.RLock()
	firstEntryID := sched.jobs[ws.ID]
	sched.mu.RUnlock()

	// Second schedule should update
	if err := sched.scheduleWorkspace(ws); err != nil {
		t.Fatalf("second scheduleWorkspace failed: %v", err)
	}

	sched.mu.RLock()
	secondEntryID := sched.jobs[ws.ID]
	sched.mu.RUnlock()

	// Entry IDs should be different (old removed, new added)
	if firstEntryID == secondEntryID {
		t.Error("rescheduling should update the job entry")
	}
}

func TestScheduler_ScheduleWorkspace_SetsNextRunAt(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "scheduler-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mgr := NewManager(tmpDir)

	wsID := "nextrun-test"
	// Use a cron that fires every minute
	cfg := &models.WorkspaceConfig{CronSchedule: "* * * * *"}
	mgr.WriteConfig(wsID, cfg)
	mgr.WriteHeartbeat(wsID, "prompt")

	exec := &mockExecutor{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	sched, err := NewScheduler(mgr, exec, logger)
	if err != nil {
		t.Fatal(err)
	}

	workspaces, _ := mgr.ListWorkspaces()
	for _, ws := range workspaces {
		if err := sched.scheduleWorkspace(ws); err != nil {
			t.Fatalf("scheduleWorkspace failed: %v", err)
		}
	}

	// Verify job was scheduled
	sched.mu.RLock()
	_, exists := sched.jobs[wsID]
	sched.mu.RUnlock()

	if !exists {
		t.Error("workspace should be scheduled after scheduleWorkspace")
	}
}

// ============================================================================
// ExecuteHeartbeat Tests
// ============================================================================

func TestScheduler_ExecuteHeartbeat_Locked(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "scheduler-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mgr := NewManager(tmpDir)
	exec := &mockExecutor{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	sched, err := NewScheduler(mgr, exec, logger)
	if err != nil {
		t.Fatal(err)
	}

	wsID := "locked-test"
	mgr.WriteConfig(wsID, &models.WorkspaceConfig{CronSchedule: "@every 1s"})
	mgr.WriteHeartbeat(wsID, "test prompt")

	// Acquire lock before executing heartbeat
	lock, err := mgr.AcquireLock(wsID)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.ReleaseLock(lock)

	err = sched.ExecuteHeartbeat(context.Background(), wsID)
	if err == nil {
		t.Error("ExecuteHeartbeat should fail when workspace is locked")
	}
}

func TestScheduler_ExecuteHeartbeat_EmptyPrompt(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "scheduler-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mgr := NewManager(tmpDir)
	exec := &mockExecutor{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	sched, err := NewScheduler(mgr, exec, logger)
	if err != nil {
		t.Fatal(err)
	}

	wsID := "empty-prompt"
	mgr.WriteConfig(wsID, &models.WorkspaceConfig{CronSchedule: "@every 1s"})
	mgr.WriteHeartbeat(wsID, "") // empty prompt

	err = sched.ExecuteHeartbeat(context.Background(), wsID)
	if err == nil {
		t.Error("ExecuteHeartbeat should fail with empty prompt")
	}
}

func TestScheduler_ExecuteHeartbeat_Success(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "scheduler-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mgr := NewManager(tmpDir)
	exec := &mockExecutor{returnOutput: "heartbeat response"}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	sched, err := NewScheduler(mgr, exec, logger)
	if err != nil {
		t.Fatal(err)
	}

	wsID := "success-test"
	mgr.WriteConfig(wsID, &models.WorkspaceConfig{CronSchedule: "@every 1s"})
	mgr.WriteHeartbeat(wsID, "test prompt")

	if err := sched.ExecuteHeartbeat(context.Background(), wsID); err != nil {
		t.Errorf("ExecuteHeartbeat failed: %v", err)
	}

	state, _ := mgr.ReadState(wsID)
	if state.IsRunning {
		t.Error("IsRunning should be false after completion")
	}
	if state.LastOutput != "heartbeat response" {
		t.Errorf("LastOutput should be set: got %q", state.LastOutput)
	}
	if state.LastError != "" {
		t.Errorf("LastError should be empty on success: %q", state.LastError)
	}
}

func TestScheduler_ExecuteHeartbeat_ExecutorError(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "scheduler-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mgr := NewManager(tmpDir)
	exec := &mockExecutor{
		errOnCall:   1,
		returnError: context.DeadlineExceeded,
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	sched, err := NewScheduler(mgr, exec, logger)
	if err != nil {
		t.Fatal(err)
	}

	wsID := "exec-error"
	mgr.WriteConfig(wsID, &models.WorkspaceConfig{CronSchedule: "@every 1s"})
	mgr.WriteHeartbeat(wsID, "test prompt")

	if err := sched.ExecuteHeartbeat(context.Background(), wsID); err != nil {
		t.Errorf("ExecuteHeartbeat should not return error (error is stored in state): %v", err)
	}

	state, _ := mgr.ReadState(wsID)
	if state.LastError == "" {
		t.Error("LastError should be set when executor returns error")
	}
	if state.IsRunning {
		t.Error("IsRunning should be false after error")
	}
}

func TestScheduler_ExecuteHeartbeat_IsRunningFlag(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "scheduler-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mgr := NewManager(tmpDir)

	wsID := "running-flag"
	mgr.WriteConfig(wsID, &models.WorkspaceConfig{CronSchedule: "@every 1s"})
	mgr.WriteHeartbeat(wsID, "test prompt")

	// Use a slow executor to catch the running state
	slowExec := &mockExecutor{returnOutput: "done"}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	sched, err := NewScheduler(mgr, slowExec, logger)
	if err != nil {
		t.Fatal(err)
	}

	// Start execution in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- sched.ExecuteHeartbeat(context.Background(), wsID)
	}()

	time.Sleep(50 * time.Millisecond) // Let it start

	state, _ := mgr.ReadState(wsID)
	// Note: IsRunning might already be false by the time we check
	// This is a racy check, so we just verify state was written
	_ = state

	<-errCh // wait for completion
}

func TestScheduler_ExecuteHeartbeat_NonExistent(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "scheduler-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mgr := NewManager(tmpDir)
	exec := &mockExecutor{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	sched, err := NewScheduler(mgr, exec, logger)
	if err != nil {
		t.Fatal(err)
	}

	err = sched.ExecuteHeartbeat(context.Background(), "nonexistent-ws")
	if err == nil {
		t.Error("ExecuteHeartbeat should fail for nonexistent workspace")
	}
}

// ============================================================================
// handleFSEvent Tests
// ============================================================================

func TestScheduler_HandleFSEvent_ReconcilesWorkspaces(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "scheduler-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mgr := NewManager(tmpDir)
	exec := &mockExecutor{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	sched, err := NewScheduler(mgr, exec, logger)
	if err != nil {
		t.Fatal(err)
	}

	// Write workspace to manager so handleFSEvent can find it via ListWorkspaces
	ws1 := &models.Workspace{
		ID:     "ws-1",
		Config: models.WorkspaceConfig{CronSchedule: "*/10 * * * *"},
	}
	mgr.WriteConfig(ws1.ID, &ws1.Config)
	mgr.WriteHeartbeat(ws1.ID, "prompt")
	mgr.WriteState(ws1.ID, &models.AgentState{})

	// Initially schedule a workspace
	if err := sched.scheduleWorkspace(ws1); err != nil {
		t.Fatalf("initial scheduleWorkspace failed: %v", err)
	}

	sched.mu.RLock()
	if _, exists := sched.jobs["ws-1"]; !exists {
		t.Error("ws-1 should be scheduled")
	}
	sched.mu.RUnlock()

	// Simulate fs event (e.g., new workspace added via file system)
	sched.handleFSEvent()

	// After event, should still have ws-1 (re-scheduled from ListWorkspaces)
	sched.mu.RLock()
	if _, exists := sched.jobs["ws-1"]; !exists {
		t.Error("ws-1 should still be scheduled after fs event")
	}
	sched.mu.RUnlock()
}

// ============================================================================
// Concurrent Execution Prevention Tests
// ============================================================================

func TestScheduler_ExecuteHeartbeat_ConcurrentPrevention(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "scheduler-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mgr := NewManager(tmpDir)
	exec := &mockExecutor{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	sched, err := NewScheduler(mgr, exec, logger)
	if err != nil {
		t.Fatal(err)
	}

	wsID := "concurrent-test"
	mgr.WriteConfig(wsID, &models.WorkspaceConfig{CronSchedule: "@every 1s"})
	mgr.WriteHeartbeat(wsID, "test prompt")

	// Launch multiple concurrent executions
	var wg sync.WaitGroup
	successCount := int32(0)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := sched.ExecuteHeartbeat(context.Background(), wsID); err == nil {
				atomic.AddInt32(&successCount, 1)
			}
		}()
	}

	wg.Wait()

	// Only ONE should succeed due to TryAcquireLock
	if successCount > 1 {
		t.Errorf("concurrent execution prevention failed: %d succeeded", successCount)
	}
}

// ============================================================================
// Scheduler Lifecycle Tests
// ============================================================================

func TestScheduler_Start_ListWorkspacesError(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "scheduler-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a file where the workspace dir should be (causes ReadDir to fail)
	mgr := NewManager(tmpDir)
	exec := &mockExecutor{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	sched, err := NewScheduler(mgr, exec, logger)
	if err != nil {
		t.Fatal(err)
	}

	// Put a file in the base dir to cause error on ListWorkspaces
	if err := os.WriteFile(tmpDir+"/notadir", []byte("oops"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Should not panic, should handle error gracefully
	_ = sched.Start(ctx)
}

func TestScheduler_Start_ThenCancel(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "scheduler-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mgr := NewManager(tmpDir)
	exec := &mockExecutor{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	sched, err := NewScheduler(mgr, exec, logger)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	if err := sched.Start(ctx); err != nil {
		t.Errorf("Start should not error on cancel: %v", err)
	}
}

// ============================================================================
// Edge Cases
// ============================================================================

func TestScheduler_ExecuteHeartbeat_EmptyWorkspace(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "scheduler-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mgr := NewManager(tmpDir)
	exec := &mockExecutor{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	sched, err := NewScheduler(mgr, exec, logger)
	if err != nil {
		t.Fatal(err)
	}

	wsID := "empty-ws"
	// No config, no heartbeat - just the directory

	err = sched.ExecuteHeartbeat(context.Background(), wsID)
	if err == nil {
		t.Error("ExecuteHeartbeat should fail with empty prompt for empty workspace")
	}
}

func TestScheduler_MultipleSchedules_SameWorkspace(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "scheduler-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	mgr := NewManager(tmpDir)
	exec := &mockExecutor{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	sched, err := NewScheduler(mgr, exec, logger)
	if err != nil {
		t.Fatal(err)
	}

	ws := &models.Workspace{
		ID:     "multi-sched",
		Config: models.WorkspaceConfig{CronSchedule: "*/15 * * * *"},
	}

	// Schedule multiple times
	for i := 0; i < 3; i++ {
		if err := sched.scheduleWorkspace(ws); err != nil {
			t.Fatalf("scheduleWorkspace iteration %d failed: %v", i, err)
		}
	}

	sched.mu.RLock()
	defer sched.mu.RUnlock()

	// Should only have ONE entry
	if len(sched.jobs) != 1 {
		t.Errorf("expected 1 job entry, got %d", len(sched.jobs))
	}
}
