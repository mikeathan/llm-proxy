package app_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"llm-proxy/internal/app"
	"llm-proxy/internal/buildinfo"
	"llm-proxy/internal/mocks"
	"llm-proxy/internal/testutils"
	"llm-proxy/models"
)

func TestAppBoots(t *testing.T) {
	testutils.SetRequiredEnv(t)

	cfg := minimalTestConfig()
	a := app.New(cfg, &mocks.MockLogger{}, &buildinfo.Info{})

	if a == nil {
		t.Fatal("app not initialized")
	}
}

func TestRoutesExist(t *testing.T) {
	testutils.SetRequiredEnv(t)

	cfg := minimalTestConfig()
	a := app.New(cfg, &mocks.MockLogger{}, &buildinfo.Info{})

	tests := []string{
		"/admin",
		"/admin/api/state",
		"/v1/chat/completions",
		"/api/conversation/message",
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
	testutils.SetRequiredEnv(t)

	cfg := minimalTestConfig()
	a := app.New(cfg, &mocks.MockLogger{}, &buildinfo.Info{})

	req := httptest.NewRequest("PUT", "/admin/api/state", nil)
	rec := httptest.NewRecorder()

	a.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func minimalTestConfig() *models.Config {
	return &models.Config{
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
}
