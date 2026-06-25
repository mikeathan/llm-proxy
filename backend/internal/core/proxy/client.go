package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

const (
	// DefaultResponseHeaderTimeout is the time allowed for the server to send response headers.
	// This is set to a high value to support reasoning models that think for a long time.
	DefaultResponseHeaderTimeout = 10 * time.Minute
	// DefaultIdleConnTimeout is the maximum amount of time an idle (keep-alive) connection will remain idle before closing itself.
	DefaultIdleConnTimeout = 90 * time.Second
	// DefaultStreamChunkTimeout is the time allowed between individual streaming chunks.
	// Increased to 5 minutes to accommodate large-context prefill times on local models.
	DefaultStreamChunkTimeout = 5 * time.Minute
)

var (
	// StreamChunkTimeout allows overriding the chunk timeout for testing.
	StreamChunkTimeout = DefaultStreamChunkTimeout

	// SharedTransport is a pooled transport shared by all LLMClient instances to ensure
	// connection reuse and prevent socket exhaustion
	SharedTransport = &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ResponseHeaderTimeout: DefaultResponseHeaderTimeout,
		IdleConnTimeout:       DefaultIdleConnTimeout,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
	}
)

func NewLLMClient(baseURL string, model string, httpClient *http.Client, headers http.Header) Client {
	if httpClient == nil {
		httpClient = &http.Client{
			Transport: SharedTransport,
		}
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

	// When the context is cancelled, force-close the response body so
	// that blocked reads (ReadAll, Decode) exit immediately instead of
	// waiting for the server to finish or TCP teardown.
	go func() {
		<-ctx.Done()
		resp.Body.Close()
	}()

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

		// When the context is cancelled, force-close the response body so
		// the read loop exits immediately instead of waiting for TCP teardown.
		go func() {
			<-ctx.Done()
			resp.Body.Close()
		}()

		reader := io.Reader(resp.Body)
		for {
			// Set a per-chunk timeout. If the server doesn't send a line within
			// StreamChunkTimeout, we close the stream.
			timer := time.AfterFunc(StreamChunkTimeout, func() {
				resp.Body.Close()
			})

			line, err := readSSELine(reader)
			timer.Stop()

			if err != nil {
				if err != io.EOF {
					if errors.Is(err, context.Canceled) {
						logging.Debug("LLM stream closed by context cancel")
					} else {
						logging.Error("LLM stream read error or timeout", "error", err)
					}
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
