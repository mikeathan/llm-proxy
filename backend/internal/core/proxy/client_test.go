package proxy_test

import (
	"context"
	"encoding/json"
	"io"
	"llm-proxy/internal/core/proxy"
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

			var req proxy.ChatRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode request: %v", err)
			}

			resp := proxy.ChatResponse{
				Choices: []proxy.Choice{
					{
						Message: proxy.Message{
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

	client := proxy.NewLLMClient("http://example.test/", "test-model", httpClient, nil)
	out, err := client.Chat(context.Background(), proxy.ChatRequest{
		Model: "test",
		Messages: []proxy.Message{
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

	client := proxy.NewLLMClient("http://example.test", "test-model", httpClient, nil)
	_, err := client.Chat(context.Background(), proxy.ChatRequest{Model: "test"})
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

	client := proxy.NewLLMClient("http://example.test", "test-model", httpClient, nil)
	_, err := client.Chat(context.Background(), proxy.ChatRequest{Model: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
}
func TestClientStreamTimeout(t *testing.T) {
	// Reduce timeout for testing
	oldTimeout := proxy.StreamChunkTimeout
	proxy.StreamChunkTimeout = 50 * time.Millisecond
	defer func() { proxy.StreamChunkTimeout = oldTimeout }()

	httpClient := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			pr, pw := io.Pipe()
			//pw.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"))
			// We won't write anything more to pw, causing a hang
			go func() {
				// Wait longer than the timeout
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

	client := proxy.NewLLMClient("http://example.test", "test-model", httpClient, nil)
	ch, err := client.Stream(context.Background(), proxy.ChatRequest{Model: "test"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Wait for the stream to finish (it should timeout)
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
