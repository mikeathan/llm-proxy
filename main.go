package main

import (
	"fmt"
	"llm-proxy/internal/api"
	"llm-proxy/internal/proxy"
	"llm-proxy/utils"
	"log"
	"net/http"
)

func main() {

	cfg, err := utils.LoadConfig("config/config.json")
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		return
	}

	mgr := proxy.NewManagerFromConfig(cfg)
	server := proxy.NewServer(mgr, cfg, "config/config.json")
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

	log.Printf("LLM proxy listening on %s", cfg.Server.Bind)
	log.Fatal(http.ListenAndServe(cfg.Server.Bind, nil))
}
