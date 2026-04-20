package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"llm-proxy/internal/platform/storage"
	"llm-proxy/internal/testing/mocks"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/testing/utils"
	"llm-proxy/models"
)

func TestBuildAppServices_UsesRuntimeProvider(t *testing.T) {
	utils.SetRequiredEnv(t)

	logger := &mocks.MockLogger{}
	dataMgr := minimalDataManager(t)

	container := bootstrap(dataMgr, logger)
	services := container.BuildAppServices()

	if _, ok := services.ClientProvider().(*proxy.RuntimeClientProvider); !ok {
		t.Fatalf("expected RuntimeClientProvider")
	}
}

func minimalDataManager(t *testing.T) *storage.DataManager {
	dir := t.TempDir()
	
	// Pre-create config.json so NewDataManager doesn't fail if it expects it
	cfg := &models.Config{
		Server: models.ServerConfig{
			Bind:            ":0",
			ModelHost:       "http://localhost",
			IdleTimeoutSecs: 10,
		},
		Models:   []models.ModelConfig{},
	}
	
	data, _ := json.Marshal(cfg)
	_ = os.WriteFile(filepath.Join(dir, "config.json"), data, 0644)

	mgr, err := storage.NewDataManager(dir)
	if err != nil {
		t.Fatalf("NewDataManager: %v", err)
	}

	if err := mgr.LoadAll(); err != nil {
		// If files don't exist, LoadAll might fail, but NewDataManager should have created them 
		// if they follow the "create if not exist" pattern.
		// Actually, storage.Store.Load() usually returns error if file not found.
		t.Logf("LoadAll failed (expected if empty): %v", err)
	}
	
	return mgr
}
