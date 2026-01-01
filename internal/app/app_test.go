package app_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"llm-proxy/internal/app"
	"llm-proxy/models"
)

func TestAppBoots(t *testing.T) {
	t.Setenv("DEVICE_CONTEXT_BASE_URL", "http://mock-device-context")

	cfg := minimalTestConfig()
	a := app.New(cfg, "", "", "")

	if a == nil {
		t.Fatal("app not initialized")
	}
}

func TestRoutesExist(t *testing.T) {
	t.Setenv("DEVICE_CONTEXT_BASE_URL", "http://mock-device-context")

	cfg := minimalTestConfig()
	a := app.New(cfg, "", "", "")

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
	t.Setenv("DEVICE_CONTEXT_BASE_URL", "http://mock-device-context")

	cfg := minimalTestConfig()
	a := app.New(cfg, "", "", "")

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
 
