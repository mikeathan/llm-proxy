package app_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"llm-proxy/internal/api"
	"llm-proxy/internal/app"
	"llm-proxy/internal/llm"
	"llm-proxy/internal/logging"
	"llm-proxy/internal/ratelimiter"
	"llm-proxy/models"
	"llm-proxy/utils"
)

func TestAppBoots(t *testing.T) {
	t.Setenv("DEVICE_CONTEXT_BASE_URL", "http://mock-device-context")

	cfg := minimalTestConfig()
	server, err := newTestServer(cfg)
	if err != nil {
		t.Fatalf("failed to boot app: %v", err)
	}
	if server == nil {
		t.Fatal("server not initialized")
	}
}

func TestRoutesExist(t *testing.T) {
	t.Setenv("DEVICE_CONTEXT_BASE_URL", "http://mock-device-context")

	cfg := minimalTestConfig()
	server, _ := newTestServer(cfg)

	tests := []string{
		"/admin",
		"/admin/api/state",
		"/v1/chat/completions",
		"/api/conversation/message",
	}

	for _, path := range tests {
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()
		server.Handler.ServeHTTP(rec, req)
		if rec.Code == http.StatusNotFound {
			t.Fatalf("route missing: %s", path)
		}
	}
}

func TestMethodEnforcement(t *testing.T) {
	t.Setenv("DEVICE_CONTEXT_BASE_URL", "http://mock-device-context")

	cfg := minimalTestConfig()
	server, _ := newTestServer(cfg)

	req := httptest.NewRequest("PUT", "/admin/api/state", nil)
	rec := httptest.NewRecorder()
	server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func newTestServer(cfg *models.Config) (*http.Server, error) {
	logger, err := logging.NewFileLogger(logging.Options{
		Stdout: true,
		Level:  logging.LevelInfo,
	})
	if err != nil {
		return nil, err
	}

	clock := utils.NewRealClock()
	provider := app.BuildDeviceContextProvider(clock)

	manager := llm.NewManagerFromConfig(cfg)
	srv := app.NewServer(manager, cfg, "config/config.json")
	runtime := srv.Manager()

	assistant := api.NewAssistantMessageHandler(
		provider,
		ratelimiter.NewLimiter(clock),
		logger,
	)

	adminHandlers := api.NewAdminHandlers(runtime, srv)
	proxyHandlers := api.NewProxyHandlers(runtime)

	router := api.NewRouter()
	jsonMethodNotAllowed := api.WithMethodNotAllowed(http.HandlerFunc(api.MethodNotAllowedJSON))
	textMethodNotAllowed := api.WithMethodNotAllowed(http.HandlerFunc(api.MethodNotAllowedText))

	// Admin
	router.Get("/admin/api/state", adminHandlers.AdminStateHandler, textMethodNotAllowed)
	router.Post("/admin/api/start", adminHandlers.AdminStartHandler, jsonMethodNotAllowed)
	router.Post("/admin/api/stop", adminHandlers.AdminStopHandler, textMethodNotAllowed)
	router.Post("/admin/api/models", adminHandlers.AdminAddModelHandler, jsonMethodNotAllowed)
	router.Put("/admin/api/models", adminHandlers.AdminUpdateModelHandler, jsonMethodNotAllowed)
	router.Delete("/admin/api/models", adminHandlers.AdminDeleteModelHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/config", adminHandlers.AdminConfigHandler, jsonMethodNotAllowed)
	router.Put("/admin/api/config", adminHandlers.AdminConfigUpdateHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/logs", adminHandlers.AdminLogsHandler, textMethodNotAllowed)
	router.Get("/admin/api/metrics", adminHandlers.AdminMetricsHandler, jsonMethodNotAllowed)
	router.Get("/admin", adminHandlers.AdminPageHandler, textMethodNotAllowed)

	// Proxy
	router.Any("/v1/chat/completions", http.HandlerFunc(proxyHandlers.ChatHandler))

	// Conversation API
	router.Any("/api/conversation/message", assistant)

	return &http.Server{
		Addr:    cfg.Server.Bind,
		Handler: router,
	}, nil
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
