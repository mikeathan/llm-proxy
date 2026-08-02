package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"runtime"
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/core/llm/providers"
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


