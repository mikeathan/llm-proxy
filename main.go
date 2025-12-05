package main

import (
	"encoding/json"
	"fmt"
	"llm-proxy/internal/api"
	"llm-proxy/models"
	"os"
)

func main() {
	// Initialize client
	client := api.NewLLMProxyClient(
		"http://localhost:9000",
	)

	cfg, err := LoadConfig("config/config.json")
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		return
	}

	messages := []models.Message{
		{Role: "user", Content: "What was the lowest temperature this week?"},
	}

	response, err := client.Query("small-tooling", messages)
	if err != nil {
		fmt.Printf("Query failed: %v\n", err)
		return
	}

	fmt.Printf("Response: %s\n", response.Choices[0].Message.Content)
}

func LoadConfig(path string) (*models.Config, error) {
	f, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg models.Config
	if err := json.Unmarshal(f, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
