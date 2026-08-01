package automation

import (
	"context"
	"errors"
	"fmt"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type mockExecutor struct{}

func (e *mockExecutor) Execute(ctx context.Context, req ExecuteRequest) (*ExecuteResponse, error) {
	return &ExecuteResponse{State: req.State}, nil
}

func (e *mockExecutor) ShellPGID(ctx context.Context, workspaceID string) (int, error) {
	return 0, nil
}

func TestDispatcher_Start_CleanupStaleState(t *testing.T) {
	// 1. Setup temporary test environment
	tmpRoot, err := os.MkdirTemp("", "dispatcher-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpRoot)

	wsDir := filepath.Join(tmpRoot, "workspaces")
	os.MkdirAll(wsDir, 0755)

	resolver := storage.NewPathResolver(tmpRoot, wsDir, wsDir)
	manager := persistence.NewWorkspaceManager(resolver)
	logger := logging.NewNopLogger()

	// 2. Create a workspace with STALE "IsRunning" state
	wsID := "stale-ws"
	wsPath := resolver.WorkspaceDir(wsID)
	os.MkdirAll(filepath.Join(wsPath, ".internal"), 0755)

	staleState := &models.AgentState{
		LastOutput: "previous run output",
	}
	staleState.SetRunning("some-automation")
	if err := manager.WriteState(wsID, staleState); err != nil {
		t.Fatalf("failed to write stale state: %v", err)
	}

	// 3. Initialize Dispatcher
	d, err := NewDispatcher(manager, &mockExecutor{}, logger)
	if err != nil {
		t.Fatalf("failed to create dispatcher: %v", err)
	}

	// 4. Start Dispatcher (this should trigger cleanup)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// We don't need it to run for long, just long enough for Start to process workspaces
	go func() {
		if err := d.Start(ctx); err != nil {
			// Start returns error when context is cancelled, which is fine
		}
	}()

	// Give it a moment to process
	deadline := 10
	for i := 0; i < deadline; i++ {
		state, err := manager.ReadState(wsID)
		if err == nil && !state.IsRunning() {
			// Success! State was cleaned up.
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Errorf("Dispatcher.Start() failed to cleanup stale execution state for workspace %s", wsID)
}

// pgidMockExecutor returns a fixed PGID for ShellPGID queries.
type pgidMockExecutor struct {
	executeResp *ExecuteResponse
	pgid         int
	pgidErr      error
}

func (e *pgidMockExecutor) Execute(ctx context.Context, req ExecuteRequest) (*ExecuteResponse, error) {
	if e.executeResp != nil {
		return e.executeResp, nil
	}
	return &ExecuteResponse{State: req.State}, nil
}

func (e *pgidMockExecutor) ShellPGID(ctx context.Context, workspaceID string) (int, error) {
	return e.pgid, e.pgidErr
}

func TestStopAutomation_ForceKillUsesPGID(t *testing.T) {
	d := &Dispatcher{
		logger:     logging.NewNopLogger(),
		activeRuns: make(map[string]*activeRun),
		executor:   &pgidMockExecutor{pgid: -12345},
	}

	_, cancel := context.WithCancel(context.Background())
	d.activeRuns["test-ws"] = &activeRun{cancel: cancel, pgid: -12345}

	err := d.StopAutomation("test-ws")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The goroutine will fire in 30s — the run is still in activeRuns
	// with a valid pgid. We can't assert syscall.Kill in a unit test
	// without a real process, but we can verify the run is still tracked.
	d.runMu.Lock()
	r, ok := d.activeRuns["test-ws"]
	d.runMu.Unlock()
	if !ok {
		t.Fatal("run removed prematurely")
	}
	if r.pgid != -12345 {
		t.Errorf("pgid changed: got %d, want -12345", r.pgid)
	}
}

func TestStopAutomation_NoShellGraceful(t *testing.T) {
	d := &Dispatcher{
		logger:     logging.NewNopLogger(),
		activeRuns: make(map[string]*activeRun),
		executor:   &pgidMockExecutor{pgidErr: fmt.Errorf("no shell")},
	}

	_, cancel := context.WithCancel(context.Background())
	d.activeRuns["test-ws"] = &activeRun{cancel: cancel}

	err := d.StopAutomation("test-ws")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// PGID should be 0 — graceful degradation
	d.runMu.Lock()
	r, ok := d.activeRuns["test-ws"]
	d.runMu.Unlock()
	if !ok {
		t.Fatal("run removed prematurely")
	}
	if r.pgid != 0 {
		t.Errorf("expected pgid=0 (no shell), got %d", r.pgid)
	}
}

func TestStopAutomation_NoActiveRun(t *testing.T) {
	d := &Dispatcher{
		logger:     logging.NewNopLogger(),
		activeRuns: make(map[string]*activeRun),
	}

	err := d.StopAutomation("nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent run")
	}
}

func TestStopAutomation_ReplacedRunNoStaleKill(t *testing.T) {
	prevDelay := stopDiagnosticDelay
	stopDiagnosticDelay = 0
	defer func() { stopDiagnosticDelay = prevDelay }()

	d := &Dispatcher{
		logger:     logging.NewNopLogger(),
		activeRuns: make(map[string]*activeRun),
	}
	d.events = NewEventBus()

	_, cancelA := context.WithCancel(context.Background())
	runA := &activeRun{cancel: cancelA, pgid: -11111}
	d.activeRuns["ws"] = runA

	_, cancelB := context.WithCancel(context.Background())
	runB := &activeRun{cancel: cancelB, pgid: -22222}

	err := d.StopAutomation("ws")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Replace runA with runB before diagnostic goroutine checks.
	d.runMu.Lock()
	d.activeRuns["ws"] = runB
	d.runMu.Unlock()

	// Give goroutine time to fire (delay=0).
	time.Sleep(50 * time.Millisecond)

	d.runMu.Lock()
	current := d.activeRuns["ws"]
	d.runMu.Unlock()
	if current != runB {
		t.Error("replacement run must not be deleted or replaced by stale diagnostic goroutine")
	}

	cancelA()
	cancelB()
}

func TestDefaultTaskExecutor_ShellPGID(t *testing.T) {
	e := &DefaultTaskExecutor{}
	pgid, err := e.ShellPGID(context.Background(), "ws")
	if err == nil {
		t.Error("expected error")
	}
	if !errors.Is(err, ErrShellPGIDNotAvailable) {
		t.Errorf("expected ErrShellPGIDNotAvailable, got %v", err)
	}
	if pgid != 0 {
		t.Errorf("expected 0, got %d", pgid)
	}
}
