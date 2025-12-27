package app

import (
	"llm-proxy/internal/api"
	"llm-proxy/internal/device_context"
	"llm-proxy/internal/logging"
	"llm-proxy/internal/proxy"
	"llm-proxy/internal/ratelimiter"
	"llm-proxy/models"
	"llm-proxy/utils"
	"net/http"
	"time"
)

type App struct {
	Server *http.Server
}

func New(cfg *models.Config) (*App, error) {
	logger, err := logging.NewFileLogger(logging.Options{
		Stdout: true,
		Level:  logging.LevelInfo,
	})
	if err != nil {
		return nil, err
	}

	clock := utils.NewRealClock()

	// Device context stack
	httpClient := &http.Client{Timeout: 10 * time.Second}
	fetcher := device_context.NewHttpDeviceContextFetcher(
		utils.Require("DEVICE_CONTEXT_BASE_URL"),
		httpClient,
	)

	cache := device_context.NewDeviceContextCache(1*time.Minute, clock)
	provider := device_context.NewDeviceContextProvider(fetcher, cache)

	// Proxy stack
	manager := proxy.NewManagerFromConfig(cfg)
	server := proxy.NewServer(manager, cfg, "config/config.json")

	// Handlers
	assistant := api.NewAssistantMessageHandler(
		provider,
		ratelimiter.NewLimiter(clock),
		logger,
	)

	admin := api.NewAdminHandlers(server)
	proxyHandlers := api.NewProxyHandlers(server)

	mux := http.NewServeMux()

	// Admin
	mux.HandleFunc("/admin/api/state", admin.AdminStateHandler)
	mux.HandleFunc("/admin/api/start", admin.AdminStartHandler)
	mux.HandleFunc("/admin/api/stop", admin.AdminStopHandler)
	mux.HandleFunc("/admin/api/models", admin.AdminAddModelHandler)
	mux.HandleFunc("/admin/api/config", admin.AdminConfigHandler)
	mux.HandleFunc("/admin/api/logs", admin.AdminLogsHandler)
	mux.HandleFunc("/admin/api/metrics", admin.AdminMetricsHandler)
	mux.HandleFunc("/admin", admin.AdminPageHandler)

	// Proxy
	mux.HandleFunc("/v1/chat/completions", proxyHandlers.ChatHandler)

	// Conversation API
	mux.Handle("/api/conversation/message", assistant)

	return &App{
		Server: &http.Server{
			Addr:    cfg.Server.Bind,
			Handler: mux,
		},
	}, nil
}
