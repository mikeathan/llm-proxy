package main

import (
	"fmt"
	"llm-proxy/internal/api"
	"llm-proxy/internal/device_context"
	"llm-proxy/internal/logging"
	"llm-proxy/internal/proxy"
	"llm-proxy/internal/ratelimiter"
	"llm-proxy/utils"
	"log"
	"net/http"
	"time"
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
	base_url := utils.Require("DEVICE_CONTEXT_BASE_URL")

	logger, err := logging.NewFileLogger(logging.Options{
		Stdout: true,
		Level:  logging.LevelInfo,
	})
	if err != nil {
		log.Fatalf("Failed to create logger: %v", err)
	}

	clock := utils.NewRealClock()
	//
	fetcher := device_context.NewHttpDeviceContextFetcher(base_url, &http.Client{})
	provider := device_context.NewDeviceContextProvider(fetcher, device_context.NewDeviceContextCache(1*time.Minute, clock))

	mgr := proxy.NewManagerFromConfig(cfg)
	server := proxy.NewServer(mgr, cfg, "config/config.json")
	assistant := api.NewAssistantMessageHandler(provider, ratelimiter.NewLimiter(clock), logger)
	
	admin := api.NewAdminHandlers(server)
	proxyHandlers := api.NewProxyHandlers(server)

	http.HandleFunc("/admin/api/state", admin.AdminStateHandler)
	http.HandleFunc("/admin/api/start", admin.AdminStartHandler)
	http.HandleFunc("/admin/api/stop", admin.AdminStopHandler)
	http.HandleFunc("/admin/api/models", admin.AdminAddModelHandler)
	http.HandleFunc("/admin/api/config", admin.AdminConfigHandler)
	http.HandleFunc("/admin/api/logs", admin.AdminLogsHandler)
	http.HandleFunc("/admin/api/metrics", admin.AdminMetricsHandler)
	http.HandleFunc("/admin", admin.AdminPageHandler)
	http.HandleFunc("/v1/chat/completions", proxyHandlers.ChatHandler)

	//http.HandleFunc(" /api/conversation/message", assistant)

	log.Printf("LLM proxy listening on %s", cfg.Server.Bind)
	log.Fatal(http.ListenAndServe(cfg.Server.Bind, nil))
}
