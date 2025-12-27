package app

import (
	"llm-proxy/internal/api"
	"llm-proxy/internal/logging"
	"llm-proxy/internal/proxy"
	"llm-proxy/internal/ratelimiter"
	"llm-proxy/models"
	"llm-proxy/utils"
	"net/http"
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
	provider := buildDeviceContextProvider(clock)

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

	router := api.NewRouter()
	jsonMethodNotAllowed := api.WithMethodNotAllowed(http.HandlerFunc(api.MethodNotAllowedJSON))
	textMethodNotAllowed := api.WithMethodNotAllowed(http.HandlerFunc(api.MethodNotAllowedText))

	// Admin
	router.Get("/admin/api/state", admin.AdminStateHandler, textMethodNotAllowed)
	router.Post("/admin/api/start", admin.AdminStartHandler, jsonMethodNotAllowed)
	router.Post("/admin/api/stop", admin.AdminStopHandler, textMethodNotAllowed)
	router.Post("/admin/api/models", admin.AdminAddModelHandler, jsonMethodNotAllowed)
	router.Put("/admin/api/models", admin.AdminUpdateModelHandler, jsonMethodNotAllowed)
	router.Delete("/admin/api/models", admin.AdminDeleteModelHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/config", admin.AdminConfigHandler, jsonMethodNotAllowed)
	router.Put("/admin/api/config", admin.AdminConfigUpdateHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/logs", admin.AdminLogsHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/metrics", admin.AdminMetricsHandler, jsonMethodNotAllowed)
	router.Get("/admin", admin.AdminPageHandler, textMethodNotAllowed)

	// Proxy
	router.Any("/v1/chat/completions", http.HandlerFunc(proxyHandlers.ChatHandler))

	// Conversation API
	router.Any("/api/conversation/message", assistant)

	return &App{
		Server: &http.Server{
			Addr:    cfg.Server.Bind,
			Handler: router,
		},
	}, nil
}
