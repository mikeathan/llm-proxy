package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"llm-proxy/models"
	"net/http"
	"time"
)

type LLMProxyClient struct {
	proxyURL   string
	httpClient *http.Client
}

func NewLLMProxyClient(proxyURL string) *LLMProxyClient {
	return &LLMProxyClient{
		proxyURL: proxyURL,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

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
