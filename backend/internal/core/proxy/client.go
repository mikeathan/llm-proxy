package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"llm-proxy/utils"
	"net/http"
	"strings"
	"time"
)

type Client interface {
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
}

type LLMClient struct {
	httpClient         *http.Client
	chatCompletionsURL string
	headers            http.Header
	model              string
}

func NewLLMClient(baseURL string, model string, httpClient *http.Client, headers http.Header) Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 180 * time.Second}
	}
	chatURL := utils.SanitiseUrl(baseURL)
	if !strings.HasSuffix(chatURL, "/v1/chat/completions") && !strings.HasSuffix(chatURL, "/chat/completions") {
		chatURL += "/v1/chat/completions"
	}
	return &LLMClient{chatCompletionsURL: chatURL, model: model, httpClient: httpClient, headers: headers}
}

func (c *LLMClient) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if req.Model == "" {
		req.Model = c.model
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("LLM chat serialisation error: %s", err.Error())
	}

	httpReq, _ := http.NewRequestWithContext(ctx, "POST", c.chatCompletionsURL, bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	for k, vv := range c.headers {
		for _, v := range vv {
			httpReq.Header.Add(k, v)
		}
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("LLM chat error %d: %s", resp.StatusCode, string(b))
	}

	var out ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}
