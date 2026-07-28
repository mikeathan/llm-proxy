package proxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
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

func TestIsLocalModelURL(t *testing.T) {
	cases := []struct {
		name      string
		baseURL   string
		modelHost string
		want      bool
	}{
		// Exact host match (no scheme/port on either side).
		{"bare host exact", "http://127.0.0.1", "127.0.0.1", true},
		{"bare host mismatch", "http://127.0.0.1", "10.0.0.5", false},
		{"localhost exact", "http://localhost", "localhost", true},

		// Host with port in the URL, host-only modelHost.
		{"url has port host only", "http://127.0.0.1:8080/v1/chat/completions", "127.0.0.1", true},
		{"url has port host only https", "https://127.0.0.1:9000", "127.0.0.1", true},
		{"url different port", "http://127.0.0.1:9999", "127.0.0.1", true},

		// Host:port in modelHost matches URL host:port.
		{"hostport exact", "http://127.0.0.1:8080", "127.0.0.1:8080", true},
		{"hostport mismatch port", "http://127.0.0.1:8080", "127.0.0.1:8081", false},

		// Path suffix on the URL still resolves to the local host.
		{"path suffix", "http://127.0.0.1:8080/v1/chat/completions", "127.0.0.1", true},
		{"path suffix explicit", "http://localhost:1234/chat/completions", "localhost:1234", true},

		// Cloud / external URLs must NOT be treated as local.
		{"openai cloud", "https://api.openai.com/v1/chat/completions", "127.0.0.1", false},
		{"nvidia cloud", "https://integrate.api.nvidia.com/v1/chat/completions", "127.0.0.1", false},
		{"openrouter cloud", "https://openrouter.ai/api/v1/chat/completions", "127.0.0.1", false},
		{"lan but not modelhost", "http://192.168.1.50:8080", "127.0.0.1", false},

		// Empty modelHost => never local (cannot decide).
		{"empty modelhost", "http://127.0.0.1:8080", "", false},

		// Trailing slash normalization.
		{"trailing slash host", "http://127.0.0.1/", "127.0.0.1", true},
		{"trailing slash modelhost", "http://127.0.0.1:8080", "127.0.0.1/", true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsLocalModelURL(c.baseURL, c.modelHost); got != c.want {
				t.Errorf("IsLocalModelURL(%q, %q) = %v, want %v", c.baseURL, c.modelHost, got, c.want)
			}
		})
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


