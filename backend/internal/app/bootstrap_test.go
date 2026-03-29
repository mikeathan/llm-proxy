package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"llm-proxy/internal/config"
	"llm-proxy/internal/mocks"
	"llm-proxy/internal/proxy"
	"llm-proxy/internal/testutils"
	"llm-proxy/models"
)

func TestBuildAppServices_UsesRuntimeProvider(t *testing.T) {
	testutils.SetRequiredEnv(t)

	logger := &mocks.MockLogger{}
	cfgMgr := minimalConfig(t)

	container := bootstrap(cfgMgr, logger)
	services := container.BuildAppServices()

	if _, ok := services.ClientProvider().(*proxy.RuntimeClientProvider); !ok {
		t.Fatalf("expected RuntimeClientProvider")
	}
}

func minimalConfig(t *testing.T) *config.ConfigManager {
	cfg := &models.Config{
		Server: models.ServerConfig{
			Bind:            ":0",
			ModelHost:       "http://localhost",
			IdleTimeoutSecs: 10,
		},
		Models:   []models.ModelConfig{},
		ModelDir: "",
		Metrics: models.MetricsConfig{
			GPU: models.GPUConfig{
				Provider: "none",
			},
		},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	mgr := config.NewConfigManager(path)

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if err := mgr.Load(); err != nil {
		t.Fatalf("load config: %v", err)
	}
	return mgr
}
