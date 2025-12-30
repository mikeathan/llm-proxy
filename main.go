package main

import (
	"fmt"
	"log"

	"llm-proxy/internal/app"
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

	proxyApp := app.New(cfg)

	log.Printf("LLM proxy listening on %s", cfg.Server.Bind)

	log.Fatal(proxyApp.ListenAndServe())
}
