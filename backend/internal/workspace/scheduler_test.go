package workspace

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"llm-proxy/models"
)

type mockExecutor struct {
	called int
}

func (m *mockExecutor) Execute(ctx context.Context, prompt string, state *models.AgentState) (string, error) {
	m.called++
	return "mock response", nil
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
