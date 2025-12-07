package main

import (
	"fmt"
	"llm-proxy/internal/proxy"
	"llm-proxy/utils"
	"log"
	"net/http"
)

func main() {
	// Initialize client
	// client := api.NewLLMProxyClient(
	// 	"http://localhost:9000",
	// )

	cfg, err := utils.LoadConfig("config/config.json")
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		return
	}

	mgr := proxy.NewManagerFromConfig(cfg)
	server := proxy.NewServer(mgr)

	http.HandleFunc("/admin/api/state", server.AdminStateHandler)
	http.HandleFunc("/admin/api/start", server.AdminStartHandler)
	http.HandleFunc("/admin/api/stop", server.AdminStopHandler)
	http.HandleFunc("/admin", server.AdminPageHandler)
	http.HandleFunc("/admin/", server.AdminPageHandler)
	http.HandleFunc("/v1/chat/completions", server.ChatHandler)

	log.Printf("LLM proxy listening on %s", cfg.Server.Bind)
	log.Fatal(http.ListenAndServe(cfg.Server.Bind, nil))

	// messages := []models.Message{
	// 	{Role: "user", Content: "What was the lowest temperature this week?"},
	// }

	// response, err := client.Query("small-tooling", messages)
	// if err != nil {
	// 	fmt.Printf("Query failed: %v\n", err)
	// 	return
	// }

	// fmt.Printf("Response: %s\n", response.Choices[0].Message.Content)

}
