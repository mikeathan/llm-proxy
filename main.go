package main

import (
	"fmt"
	"llm-proxy/internal/app"
	"llm-proxy/utils"
	"log"
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

	app, err := app.New(cfg)
	if err != nil {
		log.Fatalf("Failed to create app: %v", err)
	}

	log.Printf("LLM proxy listening on %s", cfg.Server.Bind)
	log.Fatal(app.Server.ListenAndServe())
}
