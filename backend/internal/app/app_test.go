package app_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"llm-proxy/internal/app"
	"llm-proxy/internal/buildinfo"
	"llm-proxy/internal/platform/config"
	"llm-proxy/internal/testing/mocks"
	"llm-proxy/internal/testing/utils"
	"llm-proxy/models"
)

func TestAppBoots(t *testing.T) {
	utils.SetRequiredEnv(t)

	cfg := minimalTestConfig(t)
	a := app.New(cfg, &mocks.MockLogger{}, &buildinfo.Info{})

	if a == nil {
		t.Fatal("app not initialized")
	}
}

func TestRoutesExist(t *testing.T) {
	utils.SetRequiredEnv(t)

	cfg := minimalTestConfig(t)
	a := app.New(cfg, &mocks.MockLogger{}, &buildinfo.Info{})

	tests := []string{
		"/admin",
		"/admin/api/state",
		"/v1/chat/completions",
		"/admin/api/conversation/message",
	}

	for _, path := range tests {
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()

		a.Handler().ServeHTTP(rec, req)

		if rec.Code == http.StatusNotFound {
			t.Fatalf("route missing: %s", path)
		}
	}
}

func TestMethodEnforcement(t *testing.T) {
	utils.SetRequiredEnv(t)

	cfg := minimalTestConfig(t)
	a := app.New(cfg, &mocks.MockLogger{}, &buildinfo.Info{})

	req := httptest.NewRequest("PUT", "/admin/api/state", nil)
	rec := httptest.NewRecorder()

	a.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func minimalTestConfig(t *testing.T) *config.ConfigManager {
	cfg := &models.Config{
		Server: models.ServerConfig{
			Bind:            ":0",
			ModelHost:       "http://localhost",
			IdleTimeoutSecs: 10,
		},
		Models:   []models.ModelConfig{},
		Metrics: models.MetricsConfig{
			GPU: models.GPUConfig{
				Provider: "none",
			},
		},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	// Create manager
	mgr := config.NewConfigManager(path)

	// Write initial config
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
