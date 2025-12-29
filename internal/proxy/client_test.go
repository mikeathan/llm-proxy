package proxy_test

import (
	"context"
	"encoding/json"
	"llm-proxy/internal/proxy"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientChatSuccess(t *testing.T) {
	var gotPath string
	var gotMethod string
	var gotContentType string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	client := proxy.NewLLMClient(server.URL+"/", server.Client())
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad news", http.StatusBadRequest)
	}))
	t.Cleanup(server.Close)

	client := proxy.NewLLMClient(server.URL, server.Client())
	_, err := client.Chat(context.Background(), proxy.ChatRequest{Model: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "bad news") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClientChatInvalidJSONResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{not-json"))
	}))
	t.Cleanup(server.Close)

	client := proxy.NewLLMClient(server.URL, server.Client())
	_, err := client.Chat(context.Background(), proxy.ChatRequest{Model: "test"})
	if err == nil {
		t.Fatal("expected error")
	}
}
