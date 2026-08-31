package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"llm-proxy/internal/platform/paths"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/internal/testing/mocks"
	"llm-proxy/models"
)

// TestRefreshMetricsService_WiresGPUMetricsConfig guards the P0 wiring
// (docs/PLANS/gpu-performance.md): refreshMetricsService must rebuild the
// metrics service with the FULL system metrics config (GPU provider + sample
// interval + smoothing alpha). Before the fix only GPU{...} was passed, so
// the documented background sampler never started (interval 0 → Start()
// no-ops) and every metrics snapshot ran an on-demand provider sample
// (e.g. an ioreg subprocess on macOS) instead of reading the background cache.
func TestRefreshMetricsService_WiresGPUMetricsConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LLM_PROXY_HOME", dir)

	// Seed the merged settings.yml (the AppConfig store behind System()) with
	// GPU metrics already configured so the constructor-time rebuild picks them
	// up. Keys use the store's on-disk YAML naming: MetricsConfig carries only
	// json tags, so the yaml store persists field-name-derived keys
	// ("gpusampleintervalsec"), not the json snake_case. JSON is valid YAML,
	// matching the createTestServer convention.
	appCfg := map[string]any{
		"server": map[string]any{
			"bind":              ":0",
			"model_host":        "http://localhost",
			"idle_timeout_secs": 10,
		},
		"metrics": map[string]any{
			"gpusampleintervalsec": 7,
			"gpusmoothingalpha":    0.15,
		},
	}
	cfgData, err := json.Marshal(appCfg)
	if err != nil {
		t.Fatalf("marshal settings.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.yml"), cfgData, 0o644); err != nil {
		t.Fatalf("write settings.yml: %v", err)
	}

	p := paths.Paths{ConfigDir: dir, DataDir: dir}
	if err := p.SeedDefaults(); err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}
	dataMgr, err := storage.NewDataManager(p)
	if err != nil {
		t.Fatalf("NewDataManager: %v", err)
	}
	if err := dataMgr.LoadAll(); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	srv := NewServer(mocks.NewMockManager(), dataMgr)
	defer srv.metrics.Stop()

	if got := srv.metrics.SampleInterval(); got != 7*time.Second {
		t.Fatalf("constructor wiring: expected sample interval 7s, got %v", got)
	}
	if got := srv.metrics.EffectiveSmoothingAlpha(); got != 0.15 {
		t.Fatalf("constructor wiring: expected smoothing alpha 0.15, got %v", got)
	}

	// Live re-wiring: an admin system update flows through System().OnChange →
	// SetGPUConfig → refreshMetricsService, so a new cadence applies without a
	// backend restart.
	if err := srv.ApplySystemUpdate(context.Background(), models.SystemUpdatePayload{
		GPUSampleIntervalSec: 9,
		GPUSmoothingAlpha:    0.2,
	}); err != nil {
		t.Fatalf("ApplySystemUpdate: %v", err)
	}
	if got := srv.metrics.SampleInterval(); got != 9*time.Second {
		t.Fatalf("live wiring: expected sample interval 9s, got %v", got)
	}
	if got := srv.metrics.EffectiveSmoothingAlpha(); got != 0.2 {
		t.Fatalf("live wiring: expected smoothing alpha 0.2, got %v", got)
	}
}
