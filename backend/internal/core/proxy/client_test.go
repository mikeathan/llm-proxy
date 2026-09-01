package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"llm-proxy/internal/core/llm/providers"
	"llm-proxy/internal/platform/network"
	"llm-proxy/models"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newTestResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

// timeoutNetError is a net.Error that reports a timeout, used to exercise the
// transport-error classifier's timeout bucket.
type timeoutNetError struct{}

func (timeoutNetError) Error() string   { return "i/o timeout" }
func (timeoutNetError) Timeout() bool   { return true }
func (timeoutNetError) Temporary() bool { return false }

func TestClientChatSuccess(t *testing.T) {
	var gotPath string
	var gotMethod string
	var gotContentType string

	httpClient := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			gotPath = r.URL.Path
			gotMethod = r.Method
			gotContentType = r.Header.Get("Content-Type")

			var req ChatRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}

			resp := ChatResponse{
				Choices: []Choice{
					{
						Message: Message{
							Role:    "assistant",
							Content: "hello",
						},
					},
				},
			}
			data, err := json.Marshal(resp)
			if err != nil {
				t.Fatalf("encode response: %v", err)
			}
			return newTestResponse(http.StatusOK, string(data)), nil
		}),
	}

	client := NewLLMClient("http://example.test/", "test-model", httpClient, nil)
	out, err := client.Chat(context.Background(), ChatRequest{
		Model: "test",
		Messages: []Message{
			{Role: "user", Content: "ping"},
		},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if out == nil || len(out.Choices) != 1 || out.Choices[0].Message.Content != "hello" {
		t.Fatalf("unexpected response: %#v", out)
	}
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("unexpected method: %s", gotMethod)
	}
	if gotContentType != "application/json" {
		t.Fatalf("unexpected content-type: %s", gotContentType)
	}
}

func TestClientChatHTTPErrorIncludesBody(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			return newTestResponse(http.StatusBadRequest, "bad news"), nil
		}),
	}

	client := NewLLMClient("http://example.test", "test-model", httpClient, nil)
	_, err := client.Chat(context.Background(), ChatRequest{Model: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "bad news") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIsRetryableHTTPStatus(t *testing.T) {
	retryable := []int{429, 502, 503, 504, 529}
	for _, code := range retryable {
		if !IsRetryableHTTPStatus(code) {
			t.Errorf("status %d should be retryable", code)
		}
	}
	nonRetryable := []int{400, 401, 403, 404, 418, 500, 501}
	for _, code := range nonRetryable {
		if IsRetryableHTTPStatus(code) {
			t.Errorf("status %d should NOT be retryable", code)
		}
	}
}

func TestLLMClient_Chat_RetriesOn503ThenSucceeds(t *testing.T) {
	var calls int
	httpClient := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			if calls < httpRetryMaxAttempts {
				return newTestResponse(http.StatusServiceUnavailable, `{"error":{"message":"ResourceExhausted"}}`), nil
			}
			return newTestResponse(http.StatusOK, `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`), nil
		}),
	}

	client := NewLLMClient("http://example.test", "test-model", httpClient, nil)
	out, err := client.Chat(context.Background(), ChatRequest{Model: "test"})
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if out.Choices[0].Message.Content != "ok" {
		t.Fatalf("unexpected content: %#v", out)
	}
	if calls != httpRetryMaxAttempts {
		t.Errorf("expected %d calls (retries until success), got %d", httpRetryMaxAttempts, calls)
	}
}

func TestLLMClient_Chat_NoRetryOn400(t *testing.T) {
	var calls int
	httpClient := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			return newTestResponse(http.StatusBadRequest, "bad request"), nil
		}),
	}

	client := NewLLMClient("http://example.test", "test-model", httpClient, nil)
	_, err := client.Chat(context.Background(), ChatRequest{Model: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Errorf("non-retryable status must not retry, got %d calls", calls)
	}
	if !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "bad request") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLLMClient_Chat_NoRetryOn500(t *testing.T) {
	var calls int
	httpClient := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			return newTestResponse(http.StatusInternalServerError, `{"error":{"message":"Failed to parse tool call"}}`), nil
		}),
	}

	client := NewLLMClient("http://example.test", "test-model", httpClient, nil)
	_, err := client.Chat(context.Background(), ChatRequest{Model: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Errorf("500 must not be retried (deterministic provider error), got %d calls", calls)
	}
}

func TestLLMClient_Chat_RetriesOn529(t *testing.T) {
	var calls int
	httpClient := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			if calls < httpRetryMaxAttempts {
				return newTestResponse(529, `{"message":"Service temporarily overloaded","type":"Overloaded","code":529}`), nil
			}
			return newTestResponse(http.StatusOK, `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`), nil
		}),
	}

	client := NewLLMClient("http://example.test", "test-model", httpClient, nil)
	out, err := client.Chat(context.Background(), ChatRequest{Model: "test"})
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if out.Choices[0].Message.Content != "ok" {
		t.Fatalf("unexpected content: %#v", out)
	}
	if calls != httpRetryMaxAttempts {
		t.Errorf("expected %d calls (retries until success), got %d", httpRetryMaxAttempts, calls)
	}
}

func TestLLMClient_Chat_RetriesOn500InferenceConnection(t *testing.T) {
	var calls int
	httpClient := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			if calls < httpRetryMaxAttempts {
				// NVIDIA NIM transient inference-connection failure.
				return newTestResponse(http.StatusInternalServerError, `{"type":"urn:inference-connection:problem-details:internal-server-error","title":"Internal Server Error","status":500,"detail":"Inference connection error while making inference request"}`), nil
			}
			return newTestResponse(http.StatusOK, `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`), nil
		}),
	}

	client := NewLLMClient("http://example.test", "test-model", httpClient, nil)
	out, err := client.Chat(context.Background(), ChatRequest{Model: "test"})
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if out.Choices[0].Message.Content != "ok" {
		t.Fatalf("unexpected content: %#v", out)
	}
	if calls != httpRetryMaxAttempts {
		t.Errorf("expected %d calls (retries until success), got %d", httpRetryMaxAttempts, calls)
	}
}

func TestLLMClient_Chat_RetryExhaustedReturnsError(t *testing.T) {
	var calls int
	httpClient := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			return newTestResponse(http.StatusServiceUnavailable, "busy"), nil
		}),
	}

	client := NewLLMClient("http://example.test", "test-model", httpClient, nil)
	_, err := client.Chat(context.Background(), ChatRequest{Model: "test"})
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	var httpErr *LLMHTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != 503 {
		t.Errorf("expected *LLMHTTPError 503, got %#v", err)
	}
	if calls != httpRetryMaxAttempts {
		t.Errorf("expected %d attempts, got %d", httpRetryMaxAttempts, calls)
	}
}

func TestLLMClient_Chat_RetryCancelledByContext(t *testing.T) {
	var calls int
	httpClient := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			return newTestResponse(http.StatusServiceUnavailable, "busy"), nil
		}),
	}

	client := NewLLMClient("http://example.test", "test-model", httpClient, nil)
	// Timeout (200ms) is well under the first retry backoff (800ms), so the
	// backoff wait must abort on ctx cancellation after the first attempt.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err := client.Chat(ctx, ChatRequest{Model: "test"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
	if calls != 1 {
		t.Errorf("expected exactly 1 attempt before ctx cancel, got %d", calls)
	}
}

func TestLLMClient_Chat_RetryObserver_NotFiredOnSuccess(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			return newTestResponse(http.StatusOK, `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`), nil
		}),
	}
	client := NewLLMClient("http://example.test", "test-model", httpClient, nil)

	fired := 0
	ctx := WithRetryObserver(context.Background(), func(RetryInfo) { fired++ })
	if _, err := client.Chat(ctx, ChatRequest{Model: "test"}); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if fired != 0 {
		t.Errorf("observer must not fire on success, fired %d times", fired)
	}
}

func TestLLMClient_Chat_RetryObserver_NotFiredOnNonRetryable(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			return newTestResponse(http.StatusBadRequest, "bad request"), nil
		}),
	}
	client := NewLLMClient("http://example.test", "test-model", httpClient, nil)

	fired := 0
	ctx := WithRetryObserver(context.Background(), func(RetryInfo) { fired++ })
	if _, err := client.Chat(ctx, ChatRequest{Model: "test"}); err == nil {
		t.Fatal("expected error")
	}
	if fired != 0 {
		t.Errorf("observer must not fire on non-retryable status, fired %d times", fired)
	}
}

func TestLLMClient_Chat_RetryObserver_FiresOnStatusRetry(t *testing.T) {
	var calls int
	httpClient := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				return newTestResponse(http.StatusServiceUnavailable, "busy"), nil
			}
			return newTestResponse(http.StatusOK, `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`), nil
		}),
	}
	client := NewLLMClient("http://example.test", "test-model", httpClient, nil)

	var infos []RetryInfo
	ctx := WithRetryObserver(context.Background(), func(info RetryInfo) { infos = append(infos, info) })
	if _, err := client.Chat(ctx, ChatRequest{Model: "test"}); err != nil {
		t.Fatalf("expected success after retry, got %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("expected 1 retry notification, got %d", len(infos))
	}
	got := infos[0]
	if got.Reason != RetryReasonStatus {
		t.Errorf("expected Reason=status, got %q", got.Reason)
	}
	if got.Attempt != 1 {
		t.Errorf("expected Attempt=1, got %d", got.Attempt)
	}
	if got.MaxAttempts != httpRetryMaxAttempts {
		t.Errorf("expected MaxAttempts=%d, got %d", httpRetryMaxAttempts, got.MaxAttempts)
	}
	if got.Status != http.StatusServiceUnavailable {
		t.Errorf("expected Status=503, got %d", got.Status)
	}
}

func TestLLMClient_Chat_RetryObserver_FiresOnTransportRetry(t *testing.T) {
	var calls int
	httpClient := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				return nil, errors.New("unexpected EOF")
			}
			return newTestResponse(http.StatusOK, `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`), nil
		}),
	}
	client := NewLLMClient("http://example.test", "test-model", httpClient, nil)

	var infos []RetryInfo
	ctx := WithRetryObserver(context.Background(), func(info RetryInfo) { infos = append(infos, info) })
	if _, err := client.Chat(ctx, ChatRequest{Model: "test"}); err != nil {
		t.Fatalf("expected success after retry, got %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("expected 1 retry notification, got %d", len(infos))
	}
	got := infos[0]
	if got.Reason != RetryReasonTransport {
		t.Errorf("expected Reason=transport, got %q", got.Reason)
	}
	if got.Attempt != 1 {
		t.Errorf("expected Attempt=1, got %d", got.Attempt)
	}
	if !strings.Contains(got.Error, "unexpected EOF") {
		t.Errorf("expected transport error text in Error, got %q", got.Error)
	}
	if got.ErrClass != "connection-closed" {
		t.Errorf("expected ErrClass=connection-closed, got %q", got.ErrClass)
	}
	if got.Status != 0 {
		t.Errorf("expected Status=0 for transport retry, got %d", got.Status)
	}
}

// TestClassifyTransportError locks the stable transport-error buckets so the
// generic "unexpected EOF" surfacing cannot regress into an undiagnosable error.
func TestClassifyTransportError(t *testing.T) {
	resetOpErr := &net.OpError{Op: "read", Err: syscall.ECONNRESET}

	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, ""},
		{"raw unexpected EOF", io.ErrUnexpectedEOF, "connection-closed"},
		{"error text unexpected EOF", errors.New("unexpected EOF"), "connection-closed"},
		{"url wrapped EOF", &url.Error{Op: "Post", URL: "https://x", Err: io.ErrUnexpectedEOF}, "connection-closed"},
		{"connection reset", syscall.ECONNRESET, "connection-reset"},
		{"net op connection reset", resetOpErr, "connection-reset"},
		{"net timeout", timeoutNetError{}, "timeout"},
		{"url wrapped timeout", &url.Error{Op: "Post", URL: "https://x", Err: timeoutNetError{}}, "timeout"},
		{"http2 stream", errors.New("http2: stream closed"), "http2"},
		{"http2 with EOF text", errors.New("http2: stream closed: unexpected EOF"), "http2"},
		{"tls handshake", errors.New("tls: handshake failure"), "tls"},
		{"tls with EOF text", errors.New("tls: use of closed connection: EOF"), "tls"},
		{"unknown", errors.New("boom"), "network"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyTransportError(tt.err); got != tt.want {
				t.Errorf("classifyTransportError(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}

// TestLLMClient_Chat_TransportErrorClassifiedOnExhaustion verifies that when
// transport retries are exhausted the surfaced error carries the classified
// category AND the original transport text (via %w) so the caller sees both
// the diagnostic bucket and the raw error.
func TestLLMClient_Chat_TransportErrorClassifiedOnExhaustion(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			return nil, errors.New("unexpected EOF")
		}),
	}
	client := NewLLMClient("http://example.test", "test-model", httpClient, nil)
	_, err := client.Chat(context.Background(), ChatRequest{Model: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"connection-closed", "after 3 attempts", "unexpected EOF"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("expected %q in error, got %q", want, err.Error())
		}
	}
}

// TestNewLLMClient_URLNormalization guards against the doubled /v1 regression
// that masked real upstream errors behind a confusing transport failure.
func TestNewLLMClient_URLNormalization(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		want    string
	}{
		{"bare host", "https://api.example.com", "https://api.example.com/v1/chat/completions"},
		{"trailing slash", "https://api.example.com/", "https://api.example.com/v1/chat/completions"},
		{"already has /v1", "https://api.example.com/v1", "https://api.example.com/v1/chat/completions"},
		{"trailing /v1 slash", "https://api.example.com/v1/", "https://api.example.com/v1/chat/completions"},
		{"full chat endpoint", "https://api.example.com/v1/chat/completions", "https://api.example.com/v1/chat/completions"},
		{"chat endpoint without v1", "https://api.example.com/chat/completions", "https://api.example.com/chat/completions"},
		{"nvidia manifest default", "https://integrate.api.nvidia.com/v1", "https://integrate.api.nvidia.com/v1/chat/completions"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewLLMClient(tt.baseURL, "test-model", nil, nil)
			got := client.(*LLMClient).chatCompletionsURL
			if got != tt.want {
				t.Errorf("chatCompletionsURL = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLLMClient_Chat_RetryObserver_NotFiredOnFinalAttempt(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			return newTestResponse(http.StatusServiceUnavailable, "busy"), nil
		}),
	}
	client := NewLLMClient("http://example.test", "test-model", httpClient, nil)

	var infos []RetryInfo
	ctx := WithRetryObserver(context.Background(), func(info RetryInfo) { infos = append(infos, info) })
	if _, err := client.Chat(ctx, ChatRequest{Model: "test"}); err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	// With 3 max attempts and every attempt failing, retries happen for attempts
	// 1 and 2 only; the final (3rd) attempt failure does not trigger a retry.
	if len(infos) != httpRetryMaxAttempts-1 {
		t.Errorf("expected %d retry notifications (all but final attempt), got %d", httpRetryMaxAttempts-1, len(infos))
	}
}

func TestLLMClient_Stream_RetriesOn503ThenSucceeds(t *testing.T) {
	var calls int
	body := "data: {\"choices\":[{\"message\":{\"role\":\"assistant\",\"content\":\"hi\"}}]}\ndata: [DONE]\n"
	httpClient := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			if calls < httpRetryMaxAttempts {
				return newTestResponse(http.StatusServiceUnavailable, "busy"), nil
			}
			return newTestResponse(http.StatusOK, body), nil
		}),
	}

	client := NewLLMClient("http://example.test", "test-model", httpClient, nil)
	ch, err := client.Stream(context.Background(), ChatRequest{Model: "test"})
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	var got *ChatResponse
	for resp := range ch {
		got = resp
	}
	if got == nil || got.Choices[0].Message.Content != "hi" {
		t.Fatalf("unexpected stream result: %#v", got)
	}
	if calls != httpRetryMaxAttempts {
		t.Errorf("expected %d calls, got %d", httpRetryMaxAttempts, calls)
	}
}

func TestClientChatInvalidJSONResponse(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			return newTestResponse(http.StatusOK, "{not-json"), nil
		}),
	}

	client := NewLLMClient("http://example.test", "test-model", httpClient, nil)
	_, err := client.Chat(context.Background(), ChatRequest{Model: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestClientStreamTimeout(t *testing.T) {
	oldTimeout := StreamChunkTimeout
	StreamChunkTimeout = 50 * time.Millisecond
	defer func() { StreamChunkTimeout = oldTimeout }()

	httpClient := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			pr, pw := io.Pipe()
			go func() {
				time.Sleep(200 * time.Millisecond)
				pw.Close()
			}()

			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       pr,
				Header:     make(http.Header),
			}, nil
		}),
	}

	client := NewLLMClient("http://example.test", "test-model", httpClient, nil)
	ch, err := client.Stream(context.Background(), ChatRequest{Model: "test"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	start := time.Now()
	count := 0
	for range ch {
		count++
	}
	duration := time.Since(start)

	if duration < 50*time.Millisecond {
		t.Errorf("stream finished too early: %v", duration)
	}
	if duration > 150*time.Millisecond {
		t.Errorf("stream took too long to timeout: %v", duration)
	}
}

func TestClientStream_LargeAndCRLF(t *testing.T) {
	// Large multi-line payload (forces buffered multi-byte reads) plus a
	// CRLF-terminated line, asserting every chunk arrives intact and in order.
	// Guards the SSE reader against data loss/truncation across the refactor.
	large := strings.Repeat("x", 100000)
	body := "data: {\"choices\":[{\"delta\":{\"content\":\"a\"}}]}\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"" + large + "\"}}]}\r\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"b\"}}]}\n" +
		"data: [DONE]\n"

	httpClient := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			return newTestResponse(http.StatusOK, body), nil
		}),
	}

	client := NewLLMClient("http://example.test", "test-model", httpClient, nil)
	ch, err := client.Stream(context.Background(), ChatRequest{Model: "test"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	var contents []string
	for resp := range ch {
		if len(resp.Choices) > 0 {
			contents = append(contents, resp.Choices[0].Delta.Content)
		}
	}

	want := []string{"a", large, "b"}
	if len(contents) != len(want) {
		t.Fatalf("expected %d chunks, got %d: %q", len(want), len(contents), contents)
	}
	for i := range want {
		if contents[i] != want[i] {
			t.Errorf("chunk %d: expected len=%d got len=%d (truncated/corrupt)",
				i, len(want[i]), len(contents[i]))
		}
	}
}

func TestClientChat_ResponseFormatInBody(t *testing.T) {
	var capturedBody []byte
	httpClient := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			capturedBody, _ = io.ReadAll(r.Body)
			return newTestResponse(http.StatusOK, `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`), nil
		}),
	}

	client := NewLLMClient("http://example.test", "test-model", httpClient, nil)
	_, err := client.Chat(context.Background(), ChatRequest{
		Model: "test",
		Messages: []Message{
			{Role: "user", Content: "hi"},
		},
		ResponseFormat: &ResponseFormat{
			Type: "json_object",
		},
	})
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}

	var bodyMap map[string]interface{}
	if err := json.Unmarshal(capturedBody, &bodyMap); err != nil {
		t.Fatalf("unmarshal captured body: %v", err)
	}
	rf, ok := bodyMap["response_format"]
	if !ok {
		t.Fatal("expected response_format in request body")
	}
	rfMap, ok := rf.(map[string]interface{})
	if !ok {
		t.Fatalf("expected response_format to be an object, got %T", rf)
	}
	if rfMap["type"] != "json_object" {
		t.Errorf("expected type 'json_object', got '%v'", rfMap["type"])
	}
}

func TestClientChat_ResponseFormatNotSet(t *testing.T) {
	var capturedBody []byte
	httpClient := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			capturedBody, _ = io.ReadAll(r.Body)
			return newTestResponse(http.StatusOK, `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`), nil
		}),
	}

	client := NewLLMClient("http://example.test", "test-model", httpClient, nil)
	_, err := client.Chat(context.Background(), ChatRequest{
		Model: "test",
		Messages: []Message{
			{Role: "user", Content: "hi"},
		},
	})
	if err != nil {
		t.Fatalf("Chat failed: %v", err)
	}

	var bodyMap map[string]interface{}
	if err := json.Unmarshal(capturedBody, &bodyMap); err != nil {
		t.Fatalf("unmarshal captured body: %v", err)
	}
	if _, ok := bodyMap["response_format"]; ok {
		t.Error("expected no response_format in request body when not set")
	}
}

func TestSetReasoningBudget(t *testing.T) {
	cases := []struct {
		name    string
		field   string
		budget  int
		wantRB  int
		wantTBT int
	}{
		{"think tokens non-zero", ReasoningFieldThinkTokens, 1500, 0, 1500},
		{"budget non-zero", ReasoningFieldBudget, 2048, 2048, 0},
		// Unknown field name falls through to reasoning_budget (safe default).
		{"unknown field defaults to budget", "some_future_field", 512, 512, 0},
		// Zeroing must clear both sides so nothing leaks onto the wire.
		{"think tokens zero", ReasoningFieldThinkTokens, 0, 0, 0},
		{"budget zero", ReasoningFieldBudget, 0, 0, 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := ChatRequest{}
			SetReasoningBudget(&req, c.field, c.budget)
			if req.ReasoningBudget != c.wantRB {
				t.Errorf("ReasoningBudget = %d, want %d", req.ReasoningBudget, c.wantRB)
			}
			if req.ThinkingBudgetTokens != c.wantTBT {
				t.Errorf("ThinkingBudgetTokens = %d, want %d", req.ThinkingBudgetTokens, c.wantTBT)
			}
		})
	}
}

// TestSetReasoningBudget_Exclusive ensures the non-selected field is always
// cleared, even when it previously held a value (guards against leaking the
// wrong field across retries).
func TestSetReasoningBudget_Exclusive(t *testing.T) {
	req := ChatRequest{ThinkingBudgetTokens: 999}
	SetReasoningBudget(&req, ReasoningFieldBudget, 100)
	if req.ThinkingBudgetTokens != 0 {
		t.Errorf("expected ThinkingBudgetTokens cleared, got %d", req.ThinkingBudgetTokens)
	}
	if req.ReasoningBudget != 100 {
		t.Errorf("expected ReasoningBudget = 100, got %d", req.ReasoningBudget)
	}

	req = ChatRequest{ReasoningBudget: 999}
	SetReasoningBudget(&req, ReasoningFieldThinkTokens, 200)
	if req.ReasoningBudget != 0 {
		t.Errorf("expected ReasoningBudget cleared, got %d", req.ReasoningBudget)
	}
	if req.ThinkingBudgetTokens != 200 {
		t.Errorf("expected ThinkingBudgetTokens = 200, got %d", req.ThinkingBudgetTokens)
	}
}

func TestLLMClient_ReasoningField(t *testing.T) {
	local := NewLLMClientForLocal("http://127.0.0.1:8080", "m", nil, nil)
	if got := local.ReasoningField(); got != ReasoningFieldThinkTokens {
		t.Errorf("local client ReasoningField = %q, want %q", got, ReasoningFieldThinkTokens)
	}

	cloud := NewLLMClient("https://api.openai.com", "m", nil, nil)
	if got := cloud.ReasoningField(); got != ReasoningFieldBudget {
		t.Errorf("cloud client ReasoningField = %q, want %q", got, ReasoningFieldBudget)
	}
}

// TestLLMClient_OutputCap400_TypedError verifies §2.6/§5: a 400 carrying an
// output-cap phrasing is converted to the typed OutputCapError (never retried,
// never a raw string), while an unrelated 400 stays an LLMHTTPError.
func TestLLMClient_OutputCap400_TypedError(t *testing.T) {
	calls := 0
	httpClient := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			return newTestResponse(http.StatusBadRequest, `{"error":{"message":"max_tokens is greater than the maximum allowed (4096)"}}`), nil
		}),
	}
	client := NewLLMClient("http://example.test", "test-model", httpClient, nil)
	_, err := client.Chat(context.Background(), ChatRequest{Model: "test", MaxTokens: 8192})
	if err == nil {
		t.Fatal("expected error")
	}
	var oe *providers.OutputCapError
	if !errors.As(err, &oe) {
		t.Fatalf("expected *OutputCapError, got %T: %v", err, err)
	}
	if oe.Requested != 8192 {
		t.Errorf("expected Requested 8192, got %d", oe.Requested)
	}
	if oe.Available != 4096 {
		t.Errorf("expected Available 4096, got %d", oe.Available)
	}
	if !errors.Is(err, providers.ErrOutputCapExceeded) {
		t.Fatalf("expected errors.Is ErrOutputCapExceeded, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("output-cap 400 must not be retried, got %d calls", calls)
	}

	// Unrelated 400 stays a plain LLMHTTPError.
	calls = 0
	httpClient2 := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			return newTestResponse(http.StatusBadRequest, `{"error":"bad request"}`), nil
		}),
	}
	client2 := NewLLMClient("http://example.test", "test-model", httpClient2, nil)
	_, err2 := client2.Chat(context.Background(), ChatRequest{Model: "test"})
	var httpErr *LLMHTTPError
	if !errors.As(err2, &httpErr) {
		t.Fatalf("expected *LLMHTTPError for unrelated 400, got %T: %v", err2, err2)
	}
	if calls != 1 {
		t.Fatalf("unrelated 400 must not be retried either, got %d calls", calls)
	}
}

// do not leak the ctx-done force-close goroutine when the response completes
// normally (the context is never cancelled). Baseline goroutine count is taken
// before the call and compared after, with a small retry window to let any
// short-lived goroutines exit.
func TestLLMClient_NoGoroutineLeakOnNormalCompletion(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			return newTestResponse(http.StatusOK, `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`), nil
		}),
	}

	waitFor := func(target func() int, want int) {
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if target() <= want {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	}

	// Warm up once so any lazy connection setup settles before the baseline.
	client := NewLLMClient("http://example.test", "test-model", httpClient, nil)
	if _, err := client.Chat(context.Background(), ChatRequest{Model: "test"}); err != nil {
		t.Fatalf("warmup Chat: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	baseline := runtime.NumGoroutine()

	for i := 0; i < 3; i++ {
		if _, err := client.Chat(context.Background(), ChatRequest{Model: "test"}); err != nil {
			t.Fatalf("Chat: %v", err)
		}
	}

	waitFor(runtime.NumGoroutine, baseline+1)
	if got := runtime.NumGoroutine(); got > baseline+1 {
		t.Errorf("goroutines leaked after Chat completions: before=%d after=%d", baseline, got)
	}

	// Stream: drain to completion, then check no growth.
	streamHTTP := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			return newTestResponse(http.StatusOK, "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\ndata: [DONE]\n"), nil
		}),
	}
	streamClient := NewLLMClient("http://example.test", "test-model", streamHTTP, nil)
	for i := 0; i < 3; i++ {
		ch, err := streamClient.Stream(context.Background(), ChatRequest{Model: "test"})
		if err != nil {
			t.Fatalf("Stream: %v", err)
		}
		for range ch {
		}
	}

	waitFor(runtime.NumGoroutine, baseline+1)
	if got := runtime.NumGoroutine(); got > baseline+1 {
		t.Errorf("goroutines leaked after Stream completions: before=%d after=%d", baseline, got)
	}
}

// TestNewLLMClient_DefaultTransportSelection verifies cloud and local clients
// pick the matching HTTP/1.1-only pooled transport when no custom httpClient is
// supplied: cloud gets the 45s response-header timeout (NVIDIA free tier holds
// saturated requests ~60s then drops the connection), local keeps the long
// timeout for slow prefill.
func TestNewLLMClient_DefaultTransportSelection(t *testing.T) {
	cloud := NewLLMClient("https://api.example.com", "m", nil, nil).(*LLMClient)
	if cloud.httpClient == nil {
		t.Fatal("expected cloud client to build a default httpClient")
	}
	ctr, ok := cloud.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("cloud transport type = %T, want *http.Transport", cloud.httpClient.Transport)
	}
	if ctr != network.CloudLLMChatTransport {
		t.Error("cloud client must use network.CloudLLMChatTransport")
	}

	local := NewLLMClientForLocal("http://127.0.0.1:8080", "m", nil, nil).(*LLMClient)
	if local.httpClient == nil {
		t.Fatal("expected local client to build a default httpClient")
	}
	ltr, ok := local.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("local transport type = %T, want *http.Transport", local.httpClient.Transport)
	}
	if ltr != network.LLMChatTransport {
		t.Error("local client must use network.LLMChatTransport")
	}
}

// TestLLMClient_TransportExhaustionReturnsTypedError verifies that when
// transport retries are exhausted the surfaced error is the typed
// *TransportError carrying the classified bucket, attempt count, elapsed time,
// URL, and the wrapped original error — so the UI/logs can explain WHY (e.g.
// NVIDIA dropping the connection) instead of a bare "unexpected EOF".
func TestLLMClient_TransportExhaustionReturnsTypedError(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			return nil, errors.New("unexpected EOF")
		}),
	}
	client := NewLLMClient("http://example.test", "test-model", httpClient, nil)
	start := time.Now()
	_, err := client.Chat(context.Background(), ChatRequest{Model: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
	var terr *TransportError
	if !errors.As(err, &terr) {
		t.Fatalf("expected *TransportError, got %T: %v", err, err)
	}
	if terr.Class != "connection-closed" {
		t.Errorf("Class = %q, want connection-closed", terr.Class)
	}
	if terr.Attempts != httpRetryMaxAttempts {
		t.Errorf("Attempts = %d, want %d", terr.Attempts, httpRetryMaxAttempts)
	}
	if terr.Elapsed < time.Duration(0) || time.Since(start)-terr.Elapsed > time.Second {
		t.Errorf("Elapsed = %v, want ≈ time since call start", terr.Elapsed)
	}
	if terr.URL != "http://example.test/v1/chat/completions" {
		t.Errorf("URL = %q, want chat endpoint", terr.URL)
	}
	if !errors.Is(err, io.ErrUnexpectedEOF) && !strings.Contains(err.Error(), "unexpected EOF") {
		t.Errorf("expected wrapped original error text, got %q", err.Error())
	}
	// Human hint must be present so the surfaced error explains the failure.
	if !strings.Contains(err.Error(), "closed the connection before responding") {
		t.Errorf("expected human hint in error, got %q", err.Error())
	}
}

// TestLLMClient_CloudHeaderTimeoutClassifiesAsTimeout verifies the cloud
// transport's shorter ResponseHeaderTimeout turns a provider that accepts the
// connection but never responds into a clean "timeout" classification instead
// of an opaque transport failure. The test server accepts then stalls headers.
func TestLLMClient_CloudHeaderTimeoutClassifiesAsTimeout(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Accept the request and hold the connection open without writing
		// response headers, so the client's ResponseHeaderTimeout fires.
		<-release
	}))
	// Close order matters: release the handler BEFORE srv.Close() so it does
	// not wait on a handler that is blocked on the channel.
	defer func() { close(release); srv.Close() }()

	client := NewLLMClient(srv.URL, "test-model", nil, nil).(*LLMClient)
	// Override the transport's header timeout to keep the test fast while
	// still exercising the real timeout classification path. The client
	// captured the transport pointer at construction, so the mutation is
	// visible to its requests.
	orig := network.CloudLLMChatTransport.ResponseHeaderTimeout
	network.CloudLLMChatTransport.ResponseHeaderTimeout = 100 * time.Millisecond
	defer func() { network.CloudLLMChatTransport.ResponseHeaderTimeout = orig }()

	_, err := client.Chat(context.Background(), ChatRequest{Model: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
	var terr *TransportError
	if !errors.As(err, &terr) {
		t.Fatalf("expected *TransportError, got %T: %v", err, err)
	}
	if terr.Class != "timeout" {
		t.Errorf("Class = %q, want timeout (got %q)", terr.Class, err.Error())
	}
	if !strings.Contains(err.Error(), "did not respond within the client timeout") {
		t.Errorf("expected timeout hint in error, got %q", err.Error())
	}
}

// TestIsModelStartingResponse verifies the upstream "model is still loading"
// classifier (HTTP 202/503 with a {"status":"starting"} body).
func TestIsModelStartingResponse(t *testing.T) {
	cases := []struct {
		status int
		body   string
		want   bool
	}{
		{http.StatusAccepted, `{"status":"starting"}`, true},
		{http.StatusServiceUnavailable, `{"status":"starting"}`, true},
		{http.StatusAccepted, `{"status":"Starting"}`, true},
		// Raw llama.cpp native loading response.
		{http.StatusServiceUnavailable, `{"error":{"message":"Loading model","type":"unavailable_error","code":503}}`, true},
		{http.StatusServiceUnavailable, `{"error":{"message":"loading model","type":"unavailable_error","code":503}}`, true},
		{http.StatusAccepted, `{"status":"ready"}`, false},
		{http.StatusAccepted, `not json`, false},
		{http.StatusOK, `{"status":"starting"}`, false},
		{http.StatusInternalServerError, `{"status":"starting"}`, false},
		{http.StatusServiceUnavailable, `{"error":{"message":"model overloaded","type":"unavailable_error","code":503}}`, false},
	}
	for _, tc := range cases {
		if got := isModelStartingResponse(tc.status, tc.body); got != tc.want {
			t.Errorf("isModelStartingResponse(%d, %q) = %v, want %v", tc.status, tc.body, got, tc.want)
		}
	}
}

// TestLLMClient_PollsModelStarting verifies an upstream "model is starting"
// response — in either wire form (this proxy's 202 {"status":"starting"} or
// raw llama.cpp's 503 "Loading model") — is polled until the model is ready
// instead of failing the run.
func TestLLMClient_PollsModelStarting(t *testing.T) {
	restore := models.ModelStartPollInterval
	models.ModelStartPollInterval = 5 * time.Millisecond
	defer func() { models.ModelStartPollInterval = restore }()

	startingBodies := []string{
		`{"status":"starting"}`,
		`{"error":{"message":"Loading model","type":"unavailable_error","code":503}}`,
	}
	for _, startingBody := range startingBodies {
		status := http.StatusAccepted
		if startingBody != `{"status":"starting"}` {
			status = http.StatusServiceUnavailable
		}
		t.Run(startingBody, func(t *testing.T) {
			calls := 0
			rt := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				if calls < 3 {
					return newTestResponse(status, startingBody), nil
				}
				return newTestResponse(http.StatusOK, `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`), nil
			})

			client := NewLLMClient("http://example.com", "m", &http.Client{Transport: rt}, nil)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			resp, err := client.Chat(ctx, ChatRequest{Model: "m", Messages: []Message{{Role: "user", Content: "hi"}}})
			if err != nil {
				t.Fatalf("expected success after model start, got %v", err)
			}
			if resp == nil || len(resp.Choices) == 0 {
				t.Fatalf("expected a response, got %+v", resp)
			}
			if calls != 3 {
				t.Errorf("expected 3 upstream calls (2 starting + 1 ready), got %d", calls)
			}
		})
	}
}

// TestLLMClient_ModelStartingBoundedByContext verifies the poll gives up with
// the context error when the model never becomes ready.
func TestLLMClient_ModelStartingBoundedByContext(t *testing.T) {
	restore := models.ModelStartPollInterval
	models.ModelStartPollInterval = 5 * time.Millisecond
	defer func() { models.ModelStartPollInterval = restore }()

	rt := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return newTestResponse(http.StatusAccepted, `{"status":"starting"}`), nil
	})

	client := NewLLMClient("http://example.com", "m", &http.Client{Transport: rt}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	_, err := client.Chat(ctx, ChatRequest{Model: "m", Messages: []Message{{Role: "user", Content: "hi"}}})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline exceeded, got %v", err)
	}
}
