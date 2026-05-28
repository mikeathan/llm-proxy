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


