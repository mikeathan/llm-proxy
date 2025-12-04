package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"llm-proxy/models"
	"net/http"
	"time"
)

type LLMProxyClient struct {
	managementURL string
	proxyURL      string
	httpClient    *http.Client
}

func NewLLMProxyClient(managementURL, proxyURL string) *LLMProxyClient {
	return &LLMProxyClient{
		managementURL: managementURL,
		proxyURL:      proxyURL,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (c *LLMProxyClient) EnsureModel(modelName string) (*models.EnsureModelResponse, error) {
	reqBody, _ := json.Marshal(map[string]string{
		"model": modelName,
	})

	resp, err := c.httpClient.Post(c.managementURL+"/models/ensure", "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to ensure model: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ensure model failed: %s", string(body))
	}

	var result models.EnsureModelResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if result.Status == "starting" {
		if err := c.waitForModel(modelName, 30*time.Second); err != nil {
			return nil, err
		}
		result.Status = "ready"
	}

	return &result, nil
}

func (c *LLMProxyClient) waitForModel(modelName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		status, err := c.GetModelStatus()
		if err == nil {
			if modelStatus, exists := status[modelName]; exists {
				if modelStatus["port"] != nil {
					return nil
				}
			}
		}
		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("timeout waiting for model %s", modelName)
}

func (c *LLMProxyClient) RecordActivity(modelName string) error {
	reqBody, _ := json.Marshal(map[string]string{
		"model": modelName,
	})

	resp, err := c.httpClient.Post(c.managementURL+"/models/activity", "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

func (c *LLMProxyClient) GetModelStatus() (map[string]map[string]interface{}, error) {
	resp, err := c.httpClient.Get(c.managementURL + "/models/status")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var status map[string]map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, err
	}

	return status, nil
}

// func (c *LLMProxyClient) QueryLLMDirect(modelName string, messages []models.Message) (*models.CompletionResponse, error) {
// 	modelInfo, err := c.EnsureModel(modelName)
// 	if err != nil {
// 		return nil, err
// 	}

// 	// Query llamacpp directly
// 	reqBody, _ := json.Marshal(map[string]interface{}{
// 		"messages":    messages,
// 		"temperature": 0.7,
// 	})

// 	resp, err := c.httpClient.Post(modelInfo.Endpoint+"/v1/chat/completions", "application/json", bytes.NewBuffer(reqBody))
// 	if err != nil {
// 		return nil, err
// 	}
// 	defer resp.Body.Close()

// 	var result models.CompletionResponse
// 	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
// 		return nil, err
// 	}

// 	go c.RecordActivity(modelName)

// 	return &result, nil
// }

func (c *LLMProxyClient) Query(modelName string, messages []models.Message) (*models.CompletionResponse, error) {
	body, _ := json.Marshal(map[string]interface{}{
		"messages": messages,
	})

	req, _ := http.NewRequest("POST", c.proxyURL+"/v1/chat/completions", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Model-Name", modelName)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		buf := new(bytes.Buffer)
		buf.ReadFrom(resp.Body)
		return nil, fmt.Errorf("proxy request error: %s", buf.String())
	}

	var out models.CompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}

	return &out, nil
}
