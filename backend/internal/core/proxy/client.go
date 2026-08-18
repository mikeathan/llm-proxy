package proxy

import (
	"bufio"
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
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
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

// Path fragments used to join a provider base URL into a chat endpoint.
// Provider manifests commonly ship base URLs that already carry a trailing
// /v1 (e.g. NVIDIA's default_base_url), so the join must not double it.
const (
	chatCompletionsPath = "/chat/completions"
	v1Prefix            = "/v1"
)

type LLMClient struct {
	httpClient         *http.Client
	chatCompletionsURL string
	headers            http.Header
	model              string
	reasoningField     string
}

// retryObserverKey is the context key for a per-request retry observer. A
// single LLMClient is shared across concurrent agents (see RuntimeClientProvider),
// so the observer must travel on the request context rather than on the client.
type retryObserverKeyType struct{}

var retryObserverKey retryObserverKeyType

// RetryReason classifies why an upstream attempt is being retried.
type RetryReason string

const (
	// RetryReasonTransport is a connection-level failure (e.g. unexpected EOF).
	RetryReasonTransport RetryReason = "transport"
	// RetryReasonStatus is a retryable HTTP status (429/502/503/504/529).
	RetryReasonStatus RetryReason = "status"
)

// RetryInfo describes a single retry that is about to happen. It is
// observational only — it carries no decision power over the retry/backoff.
type RetryInfo struct {
	Reason      RetryReason
	Attempt     int // 1-based attempt being retried (2nd overall attempt => 2)
	MaxAttempts int
	Error       string // transport error text (Reason == RetryReasonTransport)
	// ErrClass is a short stable bucket for the transport failure (e.g.
	// "connection-closed", "timeout", "tls") so the UI can explain WHY the
	// upstream is unreachable instead of showing a generic transport string.
	ErrClass  string // only set when Reason == RetryReasonTransport
	Status    int    // upstream HTTP status (Reason == RetryReasonStatus)
	ElapsedMs int64  // time since this LLM call started
}

// RetryObserver is invoked before each retry so callers can surface transient
// upstream failures to the UI. It must never block the retry loop.
type RetryObserver func(RetryInfo)

// WithRetryObserver attaches an observer to ctx, mirroring the UsageTracker
// context pattern. It is idempotent: an existing observer is preserved.
func WithRetryObserver(ctx context.Context, obs RetryObserver) context.Context {
	if RetryObserverFrom(ctx) != nil {
		return ctx
	}
	return context.WithValue(ctx, retryObserverKey, obs)
}

// RetryObserverFrom returns the observer attached to ctx, or nil.
func RetryObserverFrom(ctx context.Context) RetryObserver {
	if obs, ok := ctx.Value(retryObserverKey).(RetryObserver); ok {
		return obs
	}
	return nil
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
// Cloud clients use the dedicated HTTP/1.1-only transport with a 45s
// response-header timeout so a saturated gateway (e.g. NVIDIA's free tier, which
// holds requests ~60s then drops the connection) is classified as a clean
// client-side "timeout" instead of a bare "unexpected EOF".
func NewLLMClient(baseURL string, model string, httpClient *http.Client, headers http.Header) Client {
	return newLLMClient(baseURL, model, httpClient, headers, DefaultReasoningField, network.CloudLLMChatTransport)
}

// NewLLMClientForLocal builds a client for a local llama.cpp server, which
// expects the reasoning budget under "thinking_budget_tokens". Local servers
// get the 10-minute response-header timeout to accommodate long prefill on
// reasoning models.
func NewLLMClientForLocal(baseURL string, model string, httpClient *http.Client, headers http.Header) Client {
	return newLLMClient(baseURL, model, httpClient, headers, ReasoningFieldThinkTokens, network.LLMChatTransport)
}

func newLLMClient(baseURL string, model string, httpClient *http.Client, headers http.Header, reasoningField string, defaultTransport *http.Transport) Client {
	if httpClient == nil {
		httpClient = &http.Client{
			Transport: defaultTransport,
		}
	}
	chatURL := utils.SanitiseUrl(baseURL)
	switch {
	case strings.HasSuffix(chatURL, chatCompletionsPath):
		// already a full chat endpoint
	case strings.HasSuffix(chatURL, v1Prefix):
		// provider manifests commonly include a trailing /v1 (e.g. NVIDIA);
		// appending the full path would double it into /v1/v1/chat/completions.
		chatURL += chatCompletionsPath
	default:
		chatURL += v1Prefix + chatCompletionsPath
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
		529:                           // NVIDIA NIM "Service temporarily overloaded"
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

// classifyTransportError buckets a connection-level upstream failure into a
// short, stable category so operators get an actionable signal instead of a
// generic transport string. Providers such as NVIDIA close the connection (EOF
// / RST) for models a key is not entitled to, which a bare "unexpected EOF"
// cannot distinguish from a TLS failure or a dropped proxy tunnel.
func classifyTransportError(err error) string {
	if err == nil {
		return ""
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		err = urlErr.Err
	}
	if err == nil {
		return "network"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout"
	}
	switch {
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return "connection-closed"
	case errors.Is(err, syscall.ECONNRESET):
		return "connection-reset"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "connection-refused"
	case errors.Is(err, syscall.ECONNABORTED):
		return "connection-aborted"
	case strings.Contains(err.Error(), "http2"):
		return "http2"
	case strings.Contains(err.Error(), "tls"):
		return "tls"
	case strings.Contains(err.Error(), "EOF"):
		return "connection-closed"
	default:
		return "network"
	}
}

// transportErrorHint maps a classified transport bucket to a short,
// human-readable explanation so the surfaced error says WHY the upstream is
// unreachable instead of only showing the raw transport text. Unknown classes
// get generic guidance.
func transportErrorHint(class string) string {
	switch class {
	case "connection-closed":
		return "the provider closed the connection before responding — usually a provider queue timeout, model overload, or the API key/model not being entitled. Retry or check provider status."
	case "timeout":
		return "the provider did not respond within the client timeout — usually provider overload. Retry later or switch to a less-loaded model."
	case "connection-reset":
		return "the provider reset the connection — usually a load-balancer or gateway drop. Retry."
	case "connection-refused":
		return "the provider refused the connection — the endpoint may be down or unreachable. Check the base URL and network."
	case "connection-aborted":
		return "the connection was aborted — usually a gateway or proxy teardown. Retry."
	case "tls":
		return "TLS negotiation failed — check the provider certificate and endpoint."
	case "http2":
		return "HTTP/2 framing failed — the provider's HTTP/2 path is broken; the client forces HTTP/1.1."
	default:
		return "check provider status, the endpoint URL, and network connectivity."
	}
}

// TransportError is returned when upstream transport retries are exhausted. It
// carries the classified category, attempt count, elapsed time, and the
// original error (via Unwrap) so callers can surface WHY the upstream is
// unreachable — a bare "unexpected EOF" hides whether the gateway dropped the
// connection, timed out, reset, or failed TLS. Error() appends a short
// human-readable hint for the bucket.
type TransportError struct {
	Class    string
	Attempts int
	Elapsed  time.Duration
	URL      string
	Err      error
}

func (e *TransportError) Error() string {
	return fmt.Sprintf("upstream %s failure after %d attempts (elapsed %s) %s: %v — %s",
		e.Class, e.Attempts, e.Elapsed.Round(time.Second), e.URL, e.Err, transportErrorHint(e.Class))
}

func (e *TransportError) Unwrap() error { return e.Err }

// doRequest issues the HTTP POST with bounded retry + backoff for transient
// transport errors and retryable 5xx responses. It returns the (not-yet-closed)
// response on the first 200; the caller owns the body. Each attempt recreates
// the request body, and the loop bails immediately if ctx is cancelled.
func (c *LLMClient) doRequest(ctx context.Context, kind, url string, headers http.Header, body []byte) (*http.Response, error) {
	var lastErr error
	start := time.Now()
	observer := RetryObserverFrom(ctx)
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
			class := classifyTransportError(err)
			logging.Warn("LLM upstream transport error, retrying",
				"attempt", attempt+1, "max", httpRetryMaxAttempts,
				"model", c.model, "url", url, "kind", kind,
				"error_class", class, "error", err)
			if observer != nil && attempt < httpRetryMaxAttempts-1 {
				observer(RetryInfo{
					Reason:      RetryReasonTransport,
					Attempt:     attempt + 1,
					MaxAttempts: httpRetryMaxAttempts,
					Error:       err.Error(),
					ErrClass:    class,
					ElapsedMs:   time.Since(start).Milliseconds(),
				})
			}
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
			logging.Warn("LLM upstream non-retryable error",
				"model", c.model, "url", url, "kind", kind,
				"status", resp.StatusCode, "error", httpErr.Error())
			return nil, httpErr
		}
		lastErr = httpErr
		logging.Warn("LLM upstream retryable status, retrying",
			"attempt", attempt+1, "max", httpRetryMaxAttempts,
			"model", c.model, "url", url, "kind", kind, "status", resp.StatusCode)
		if observer != nil && attempt < httpRetryMaxAttempts-1 {
			observer(RetryInfo{
				Reason:      RetryReasonStatus,
				Attempt:     attempt + 1,
				MaxAttempts: httpRetryMaxAttempts,
				Status:      resp.StatusCode,
				ElapsedMs:   time.Since(start).Milliseconds(),
			})
		}
	}

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	var httpErr *LLMHTTPError
	if lastErr != nil && !errors.As(lastErr, &httpErr) {
		// Transport retries exhausted: surface the classified category alongside
		// the original error so the raw text (e.g. "unexpected EOF") no longer
		// hides what actually failed (timeout / closed connection / reset / TLS).
		// The typed TransportError carries elapsed time and a human hint too.
		return nil, &TransportError{
			Class:    classifyTransportError(lastErr),
			Attempts: httpRetryMaxAttempts,
			Elapsed:  time.Since(start),
			URL:      url,
			Err:      lastErr,
		}
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

		reader := bufio.NewReader(resp.Body)
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

func readSSELine(r *bufio.Reader) (string, error) {
	line, err := r.ReadBytes('\n')
	// Strip the trailing line terminator so callers see exactly the line
	// content, preserving the pre-refactor semantics (no trailing \n/\r).
	line = bytes.TrimRight(line, "\r\n")
	if err != nil {
		return string(line), err
	}
	return string(line), nil
}
