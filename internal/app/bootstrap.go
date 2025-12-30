package app

import (
	"llm-proxy/internal/api"
	"llm-proxy/internal/device_context"
	"llm-proxy/internal/llm"
	"llm-proxy/internal/logging"
	"llm-proxy/internal/proxy"
	"llm-proxy/internal/ratelimiter"
	"llm-proxy/models"
	"llm-proxy/utils"
	"log"
	"net/http"
	"os"
)

type Core struct {
	AppCtx  *AppContext
	Runtime llm.RuntimeManager
}

type Infra struct {
	Logger    logging.Logger
	Clock     utils.Clock
	DeviceCtx device_context.DeviceContextProvider
}

type Container struct {
	Core  Core
	Infra Infra
}

type AssistantService interface {
	DeviceContextProvider() device_context.DeviceContextProvider
	ClientProvider() proxy.LLMClientProvider
	Limiter() ratelimiter.Limiter
	Logger() logging.Logger
	DefaultModel() (string, error)
}

func (c *Container) BuildAppServices() AppServices {
	s := AppServices{
		Runtime:   c.Core.Runtime,
		AppCtx:    c.Core.AppCtx,
		deviceCtx: c.Infra.DeviceCtx,
		logger:    c.Infra.Logger,
		Clock:     c.Infra.Clock,
	}

	factory := func(baseURL string) proxy.Client {
		return proxy.NewLLMClient(baseURL, nil)
	}

	if baseURL := os.Getenv("LLM_PROXY_DEV_BASE_URL"); baseURL != "" {
		s.clientProvider = proxy.NewStaticClientProvider(factory(baseURL))
		return s
	}

	s.clientProvider = proxy.NewRuntimeClientProvider(s, c.Core.Runtime, factory)

	return s
}

type AppServices struct {
	Runtime        llm.RuntimeManager
	AppCtx         *AppContext
	deviceCtx      device_context.DeviceContextProvider
	clientProvider proxy.LLMClientProvider
	logger         logging.Logger
	Clock          utils.Clock
}

func (s AppServices) DeviceContextProvider() device_context.DeviceContextProvider {
	return s.deviceCtx
}

func (s AppServices) ClientProvider() proxy.LLMClientProvider {
	return s.clientProvider
}

func (s AppServices) Logger() logging.Logger {
	return s.logger
}

func (s AppServices) Limiter() ratelimiter.Limiter {
	return ratelimiter.NewLimiter(s.Clock)
}

func (s AppServices) DefaultModel() (string, error) {
	return s.AppCtx.DefaultModel()
}

func bootstrap(cfg *models.Config) *Container {
	logger := initLogger()
	clock := utils.NewRealClock()

	deviceProvider := BuildDeviceContextProvider(clock)

	manager := llm.NewManagerFromConfig(cfg)
	appCtx := NewServer(manager, cfg, "config/config.json")
	runtime := appCtx.Manager()

	return &Container{

		Core: Core{
			AppCtx:  appCtx,
			Runtime: runtime,
		},
		Infra: Infra{
			Logger:    logger,
			Clock:     clock,
			DeviceCtx: deviceProvider,
		},
	}
}

func initLogger() logging.Logger {
	logger, err := logging.NewFileLogger(logging.Options{
		Stdout: true,
		Level:  logging.LevelInfo,
	})
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}
	return logger
}

func buildHTTP(s AppServices) http.Handler {
	assistant := api.NewAssistantMessageHandler(s)

	adminHandlers := api.NewAdminHandlers(s.Runtime, s.AppCtx)
	proxyHandlers := api.NewProxyHandlers(s.Runtime)

	return buildRouter(adminHandlers, proxyHandlers, assistant)
}

func buildRouter(
	admin *api.AdminHandlers,
	proxyHandlers *api.ProxyHandlers,
	assistant http.Handler,
) http.Handler {

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
	router.Any("/v1/chat/completions", http.HandlerFunc(proxyHandlers.EnsureModelProxyHandler))

	// Conversation API
	router.Any("/api/conversation/message", assistant)

	return router
}
