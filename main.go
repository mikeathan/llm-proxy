package main

import (
	"fmt"
	"llm-proxy/api"
	"llm-proxy/models"
)

// Example usage in IoT backend
func exampleIoTBackendUsage() {
	// Initialize client
	client := api.NewLLMProxyClient(
		"http://localhost:9000", // Management API
		"http://localhost:9001", // Proxy API
	)

	// Example 1: Direct approach (for frequent queries)
	messages := []models.Message{
		{Role: "user", Content: "What was the lowest temperature this week?"},
	}

	response, err := client.QueryLLMDirect("small-tooling", messages)
	if err != nil {
		fmt.Printf("Query failed: %v\n", err)
		return
	}

	fmt.Printf("Response: %s\n", response.Choices[0].Message.Content)

	// Example 2: Proxy approach (for occasional queries)
	response2, err := client.QueryLLMProxy("small-tooling", messages)
	if err != nil {
		fmt.Printf("Query failed: %v\n", err)
		return
	}

	fmt.Printf("Response: %s\n", response2.Choices[0].Message.Content)

	// Example 3: Check status of all models
	status, err := client.GetModelStatus()
	if err == nil {
		fmt.Printf("Running models: %+v\n", status)
	}

	// Example 4: Switch to larger model for complex query
	complexMessages := []models.Message{
		{Role: "user", Content: "Analyze temperature patterns and explain why there were fluctuations"},
	}

	response3, err := client.QueryLLMDirect("large-reasoning", complexMessages)
	if err != nil {
		fmt.Printf("Query failed: %v\n", err)
		return
	}

	fmt.Printf("Complex analysis: %s\n", response3.Choices[0].Message.Content)
}

// Example integration with IoT automation
type IoTBackend struct {
	llmClient *api.LLMProxyClient
}

func (iot *IoTBackend) HandleUserQuery(query string) (string, error) {
	// Use small model for tooling queries
	messages := []models.Message{
		{
			Role:    "system",
			Content: "You are a helpful assistant for home automation queries.",
		},
		{
			Role:    "user",
			Content: query,
		},
	}

	response, err := iot.llmClient.QueryLLMDirect("small-tooling", messages)
	if err != nil {
		return "", err
	}

	return response.Choices[0].Message.Content, nil
}

func (iot *IoTBackend) HandleComplexAnalysis(query string) (string, error) {
	// Use large model for complex reasoning
	messages := []models.Message{
		{Role: "user", Content: query},
	}

	response, err := iot.llmClient.QueryLLMDirect("large-reasoning", messages)
	if err != nil {
		return "", err
	}

	return response.Choices[0].Message.Content, nil
}
