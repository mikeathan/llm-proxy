package main

import (
	"fmt"
	"log"
	"net/http"

	"llm-proxy/internal/api"
	"llm-proxy/internal/app"
	"llm-proxy/internal/llm"
	"llm-proxy/internal/logging"
	"llm-proxy/internal/proxy"
	"llm-proxy/internal/ratelimiter"
	"llm-proxy/utils"
)

func main() {

	// Load configuration
	cfg, err := utils.LoadConfig("config/config.json")
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		return
	}
	// Load environment variables
	utils.LoadEnv()

	logger, err := logging.NewFileLogger(logging.Options{
		Stdout: true,
		Level:  logging.LevelInfo,
	})
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
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
	router.Get("/admin/api/logs", adminHandlers.AdminLogsHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/metrics", adminHandlers.AdminMetricsHandler, jsonMethodNotAllowed)
	router.Get("/admin", adminHandlers.AdminPageHandler, textMethodNotAllowed)

	// Proxy
	router.Any("/v1/chat/completions", http.HandlerFunc(proxyHandlers.ChatHandler))

	// Conversation API
	router.Any("/api/conversation/message", assistant)

	log.Printf("LLM proxy listening on %s", cfg.Server.Bind)
	log.Fatal((&http.Server{
		Addr:    cfg.Server.Bind,
		Handler: router,
	}).ListenAndServe())
}
