package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"llm-proxy/internal/core/llm/providers"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/network"
	"llm-proxy/utils"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"
)

type Client interface {
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	Stream(ctx context.Context, req ChatRequest) (<-chan *ChatResponse, error)
	// ReasoningField reports the JSON wire field this upstream expects for the
	// reasoning budget. It is derived from the actual destination contract
	// (llama.cpp servers use "thinking_budget_tokens"; OpenAI-compatible
	// gateways use "reasoning_budget"), NOT the config provider slug — the same
	// slug can front either backend.
	ReasoningField() string
}

// Reasoning field wire names.
const (
	ReasoningFieldBudget      = "reasoning_budget"
	ReasoningFieldThinkTokens = "thinking_budget_tokens"
)

// DefaultReasoningField is the wire field used for OpenAI-compatible gateways.
const DefaultReasoningField = ReasoningFieldBudget

type LLMClient struct {
	httpClient         *http.Client
	chatCompletionsURL string
	headers            http.Header
	model              string
	reasoningField     string
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

	// SharedTransport is the pooled transport shared by all outbound HTTP
	// clients.  Single source of truth lives in platform/network; this alias
	// keeps existing callers compiling.
	SharedTransport = network.SharedTransport
)

// NewLLMClient builds a client for an upstream that speaks the OpenAI-compatible
// chat protocol. The reasoning budget is sent as "reasoning_budget" by default.
func NewLLMClient(baseURL string, model string, httpClient *http.Client, headers http.Header) Client {
	return newLLMClient(baseURL, model, httpClient, headers, DefaultReasoningField)
}

// NewLLMClientForLocal builds a client for a local llama.cpp server, which
// expects the reasoning budget under "thinking_budget_tokens".
func NewLLMClientForLocal(baseURL string, model string, httpClient *http.Client, headers http.Header) Client {
	return newLLMClient(baseURL, model, httpClient, headers, ReasoningFieldThinkTokens)
}

func newLLMClient(baseURL string, model string, httpClient *http.Client, headers http.Header, reasoningField string) Client {
	if httpClient == nil {
		httpClient = &http.Client{
			Transport: SharedTransport,
		}
	}
	chatURL := utils.SanitiseUrl(baseURL)
	if !strings.HasSuffix(chatURL, "/v1/chat/completions") && !strings.HasSuffix(chatURL, "/chat/completions") {
		chatURL += "/v1/chat/completions"
	}
	return &LLMClient{chatCompletionsURL: chatURL, model: model, httpClient: httpClient, headers: headers, reasoningField: reasoningField}
}

// ReasoningField returns the wire field name this upstream expects for the
// reasoning budget.
func (c *LLMClient) ReasoningField() string {
	return c.reasoningField
}

// SetReasoningBudget applies budget to req under the field name reported by
// field, clearing the other field so only one is ever sent on the wire.
func SetReasoningBudget(req *ChatRequest, field string, budget int) {
	switch field {
	case ReasoningFieldThinkTokens:
		req.ThinkingBudgetTokens = budget
		req.ReasoningBudget = 0
	default:
		req.ReasoningBudget = budget
		req.ThinkingBudgetTokens = 0
	}
}

// ClearReasoningParams strips ALL reasoning-enable params from req. Used as a
// last-resort fallback when a provider rejects an otherwise-valid reasoning
// parameter (isUnsupportedParameterError). After this the request carries no
// reasoning hints and the provider's defaults apply.
func ClearReasoningParams(req *ChatRequest) {
	req.ReasoningBudget = 0
	req.ThinkingBudgetTokens = 0
	req.ReasoningEffort = ""
	req.Reasoning = nil
	req.ChatTemplateKwargs = nil
}

// Upstream transient-failure retry policy. These are transport-layer retries
// for capacity/availability errors, distinct from the agent-level semantic
// fallbacks (prefill rejection, tool-support, unsupported-parameter) in
// stream.go. Deliberately minimal and hard-coded; expose via config only if
// runtime tuning is later required.
const (
	httpRetryMaxAttempts = 3
	httpRetryBaseBackoff = 800 * time.Millisecond
)

// LLMHTTPError is a typed error for non-2xx upstream responses. Its Error()
// preserves the legacy "LLM chat error %d: %s" / "LLM stream error %d: %s"
// strings so existing callers/tests matching those literals keep working.
type LLMHTTPError struct {
	Kind       string // "chat" or "stream" — selects the legacy Error() format
	StatusCode int
	Body       string
}

func (e *LLMHTTPError) Error() string {
	if e.Kind == "stream" {
		return fmt.Sprintf("LLM stream error %d: %s", e.StatusCode, e.Body)
	}
	return fmt.Sprintf("LLM chat error %d: %s", e.StatusCode, e.Body)
}

// IsRetryableHTTPStatus reports whether an upstream status code is worth
// retrying (transient capacity/availability). 500 is intentionally excluded by
// status alone: providers such as NVIDIA NIM return deterministic 500s for
// malformed tool-call arguments, which must not be retried. Transient 500s are
// detected by body content via IsRetryableResponse.
func IsRetryableHTTPStatus(code int) bool {
	switch code {
	case http.StatusTooManyRequests, // 429
		http.StatusBadGateway,         // 502
		http.StatusServiceUnavailable, // 503
		http.StatusGatewayTimeout,     // 504
		529: // NVIDIA NIM "Service temporarily overloaded"
		return true
	default:
		return false
	}
}

// inferenceConnectionMarker is the NVIDIA NIM problem-detail type used for a
// transient upstream inference-connection failure. A 500 carrying it is
// retryable; a 500 carrying a deterministic error (e.g. malformed tool-call
// arguments) is not.
const inferenceConnectionMarker = "inference-connection"

// IsRetryableResponse reports whether a non-2xx response should be retried,
// combining the status-code classifier with body inspection for transient 500s.
func IsRetryableResponse(status int, body string) bool {
	if IsRetryableHTTPStatus(status) {
		return true
	}
	if status == http.StatusInternalServerError && strings.Contains(strings.ToLower(body), inferenceConnectionMarker) {
		return true
	}
	return false
}

// reqMaxTokens extracts the requested max_tokens from a serialized request body
// so a typed output-cap error can report what was asked.  Returns 0 when the
// field is absent or unparseable.
func reqMaxTokens(body []byte) int {
	var probe struct {
		MaxTokens int `json:"max_tokens"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return 0
	}
	return probe.MaxTokens
}

// backoffForRetry returns the wait before retry attempt N (N >= 1) using
// exponential growth (800ms, 1.6s, 3.2s) with ±20% jitter to avoid thundering
// herds.
func backoffForRetry(attempt int) time.Duration {
	base := httpRetryBaseBackoff * time.Duration(1<<uint(attempt-1))
	jitter := time.Duration(rand.Int64N(int64(base / 5)))
	return base - base/10 + jitter
}

// doRequest issues the HTTP POST with bounded retry + backoff for transient
// transport errors and retryable 5xx responses. It returns the (not-yet-closed)
// response on the first 200; the caller owns the body. Each attempt recreates
// the request body, and the loop bails immediately if ctx is cancelled.
func (c *LLMClient) doRequest(ctx context.Context, kind, url string, headers http.Header, body []byte) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt < httpRetryMaxAttempts; attempt++ {
		if attempt > 0 {
			timer := time.NewTimer(backoffForRetry(attempt))
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		for k, vv := range headers {
			for _, v := range vv {
				req.Header.Add(k, v)
			}
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = err
			logging.Warn("LLM upstream transport error, retrying",
				"attempt", attempt+1, "max", httpRetryMaxAttempts, "error", err)
			continue
		}

		if resp.StatusCode == http.StatusOK {
			return resp, nil
		}

		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		httpErr := &LLMHTTPError{Kind: kind, StatusCode: resp.StatusCode, Body: string(b)}
		// An output-cap 400 is a deterministic capability failure: convert it
		// to the typed OutputCapError immediately (never retried, never parsed
		// outside the provider edge) so the caller can clamp and retry once.
		if resp.StatusCode == http.StatusBadRequest {
			if oe := providers.ParseOutputCapError(httpErr.Body); oe != nil {
				oe.Requested = reqMaxTokens(body)
				return nil, oe
			}
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if !IsRetryableResponse(resp.StatusCode, httpErr.Body) {
			return nil, httpErr
		}
		lastErr = httpErr
		logging.Warn("LLM upstream retryable status, retrying",
			"attempt", attempt+1, "max", httpRetryMaxAttempts, "status", resp.StatusCode)
	}

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return nil, lastErr
}

func (c *LLMClient) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if req.Model == "" {
		req.Model = c.model
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("LLM chat serialisation error: %s", err.Error())
	}

	resp, err := c.doRequest(ctx, "chat", c.chatCompletionsURL, c.headers, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// When the context is cancelled, force-close the response body so
	// that blocked reads (ReadAll, Decode) exit immediately instead of
	// waiting for the server to finish or TCP teardown. The goroutine also
	// exits when the body read completes normally, so it does not leak.
	bodyDone := make(chan struct{})
	defer close(bodyDone)
	go func() {
		select {
		case <-ctx.Done():
			resp.Body.Close()
		case <-bodyDone:
		}
	}()

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

	resp, err := c.doRequest(ctx, "stream", c.chatCompletionsURL, c.headers, body)
	if err != nil {
		return nil, err
	}

	ch := make(chan *ChatResponse, 100)
	go func() {
		defer resp.Body.Close()
		defer close(ch)

		// When the context is cancelled, force-close the response body so
		// the read loop exits immediately instead of waiting for TCP teardown.
		// The goroutine also exits when the read loop finishes normally, so
		// it does not leak on a completed stream.
		bodyDone := make(chan struct{})
		defer close(bodyDone)
		go func() {
			select {
			case <-ctx.Done():
				resp.Body.Close()
			case <-bodyDone:
			}
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
