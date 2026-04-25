package automation

import (
	"context"
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
		IsRunning:  true,
		LastOutput: "previous run output",
	}
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
		if err == nil && !state.IsRunning {
			// Success! State was cleaned up.
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Errorf("Dispatcher.Start() failed to cleanup stale execution state for workspace %s", wsID)
}
