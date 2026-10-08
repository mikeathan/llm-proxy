package automation

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
)

func TestSeedMetricsFromHistory(t *testing.T) {
	base := t.TempDir()
	resolver := storage.NewPathResolver(base, filepath.Join(base, "workspaces"), filepath.Join(base, "meta"))
	mgr := persistence.NewWorkspaceManager(resolver)
	if err := os.MkdirAll(resolver.WorkspaceDir("ws"), 0o755); err != nil {
		t.Fatal(err)
	}
	history := []models.AutomationRun{
		{ID: "ok", DurationMs: 1000, Timestamp: time.Unix(1, 0)},
		{ID: "failed", DurationMs: 500, Error: "boom", Timestamp: time.Unix(2, 0)},
	}
	if err := mgr.WriteState("ws", &models.AgentState{History: history}); err != nil {
		t.Fatal(err)
	}
	d := &Dispatcher{persistence: mgr, metrics: &DispatcherMetrics{}, logger: logging.NewNopLogger()}

	d.seedMetricsFromHistory(context.Background())

	got := d.metrics.Snapshot()
	if got.TotalExecutions != 2 || got.SuccessfulExecutions != 1 || got.FailedExecutions != 1 {
		t.Fatalf("want 2 total / 1 ok / 1 failed, got %+v", got)
	}
	if got.TotalLatency != 1500*time.Millisecond {
		t.Fatalf("want 1.5s total latency, got %v", got.TotalLatency)
	}
}
