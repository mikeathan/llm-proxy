package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/utils"
	"net/http"
	"strings"
	"time"
)

type Client interface {
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	Stream(ctx context.Context, req ChatRequest) (<-chan *ChatResponse, error)
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
func (c *LLMClient) Stream(ctx context.Context, req ChatRequest) (<-chan *ChatResponse, error) {
	if req.Model == "" {
		req.Model = c.model
	}
	req.Stream = true
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("LLM stream serialisation error: %s", err.Error())
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

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("LLM stream error %d: %s", resp.StatusCode, string(b))
	}

	ch := make(chan *ChatResponse, 100)
	go func() {
		defer resp.Body.Close()
		defer close(ch)

		reader := io.Reader(resp.Body)
		for {
			line, err := readSSELine(reader)
			if err != nil {
				if err != io.EOF {
					logging.Error("LLM stream read error", "error", err)
				}
				return
			}

			if line == "" || line == "data: [DONE]" {
				if line == "data: [DONE]" {
					return
				}
				continue
			}

			if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")
				var out ChatResponse
				if err := json.Unmarshal([]byte(data), &out); err != nil {
					logging.Error("LLM stream unmarshal error", "error", err)
					continue
				}
				ch <- &out
			}
		}
	}()

	return ch, nil
}

func readSSELine(r io.Reader) (string, error) {
	var buf []byte
	b := make([]byte, 1)
	for {
		n, err := r.Read(b)
		if n > 0 {
			if b[0] == '\n' {
				return string(buf), nil
			}
			buf = append(buf, b[0])
		}
		if err != nil {
			return string(buf), err
		}
	}
}
