package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"llm-proxy/internal/platform/storage"
	"llm-proxy/internal/testing/mocks"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/testing/utils"
	"llm-proxy/models"
	realutils "llm-proxy/utils"
	"llm-proxy/internal/buildinfo"
)

func TestBuildAppServices_UsesRuntimeProvider(t *testing.T) {
	utils.SetRequiredEnv(t)

	logger := &mocks.MockLogger{}
	dataMgr := minimalDataManager(t)

	container := bootstrap(dataMgr, logger, false, false)
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

func TestLimiter_IsSingleton(t *testing.T) {
	utils.SetRequiredEnv(t)
	logger := &mocks.MockLogger{}
	clock := realutils.NewRealClock()
	
	dir := t.TempDir()
	dataMgr, _ := storage.NewDataManager(dir)
	
	appCtx := NewServer(nil, dataMgr)
	
	// Enable sandboxing so bootstrap doesn't fatal
	settings := appCtx.HostSettings()
	settings.Sandboxing.Enabled = true
	_ = appCtx.UpdateHostSettings(settings)
	
	c := &Container{
		Core: Core{
			AppCtx: appCtx,
		},
		Infra: Infra{
			Logger: logger,
			Clock:  clock,
		},
	}

	services := c.BuildAppServices()

	l1 := services.Limiter()
	l2 := services.Limiter()

	if l1 == nil {
		t.Fatal("Limiter should not be nil")
	}

	if l1 != l2 {
		t.Errorf("Limiter should be a singleton, but got different instances: %p vs %p", l1, l2)
	}
}

func TestApp_ServerTimeouts(t *testing.T) {
	utils.SetRequiredEnv(t)

	dataMgr := minimalDataManager(t)
	
	// Enable sandboxing for bootstrap
	appCtx := NewServer(nil, dataMgr)
	settings := appCtx.HostSettings()
	settings.Sandboxing.Enabled = true
	_ = appCtx.UpdateHostSettings(settings)

	a := New(context.Background(), dataMgr, &mocks.MockLogger{}, &buildinfo.Info{}, false, false)

	if a.server.ReadTimeout == 0 {
		t.Error("expected ReadTimeout to be set")
	}
	if a.server.WriteTimeout == 0 {
		t.Error("expected WriteTimeout to be set")
	}
	if a.server.IdleTimeout == 0 {
		t.Error("expected IdleTimeout to be set")
	}
	if a.server.ReadHeaderTimeout == 0 {
		t.Error("expected ReadHeaderTimeout to be set")
	}
}

func TestApp_ShutdownCancelsBaseContext(t *testing.T) {
	utils.SetRequiredEnv(t)

	dataMgr := minimalDataManager(t)
	appCtx := NewServer(nil, dataMgr)
	settings := appCtx.HostSettings()
	settings.Sandboxing.Enabled = true
	_ = appCtx.UpdateHostSettings(settings)

	a := New(context.Background(), dataMgr, &mocks.MockLogger{}, &buildinfo.Info{}, false, false)

	// BaseContext must be set
	baseCtx := a.server.BaseContext(nil)
	if baseCtx == nil {
		t.Fatal("BaseContext returned nil")
	}
	if baseCtx.Err() != nil {
		t.Error("BaseContext should not be cancelled before Shutdown")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	a.Shutdown(shutdownCtx)

	if baseCtx.Err() == nil {
		t.Error("BaseContext should be cancelled after Shutdown")
	}
}
