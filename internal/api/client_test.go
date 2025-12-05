package api_test

import (
	"encoding/json"
	"io"
	"llm-proxy/internal/api"
	"llm-proxy/models"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLLMProxyClient_Query_Success(t *testing.T) {
	// Mock response matching your updated struct
	mockResp := models.CompletionResponse{
		ID:      "123",
		Object:  "chat.completion",
		Created: 11111111,
		Choices: []struct {
			Index        int            `json:"index"`
			Message      models.Message `json:"message"`
			FinishReason string         `json:"finish_reason"`
		}{
			{
				Index: 0,
				Message: models.Message{
					Role:    "assistant",
					Content: "hello world",
				},
				FinishReason: "stop",
			},
		},
	}

	// Create mock server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		// Required method
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}

		// Required path
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		// Required header
		if r.Header.Get("X-Model-Name") != "test-model" {
			t.Fatalf("missing or incorrect X-Model-Name header")
		}

		// Validate JSON body contains messages
		body, _ := io.ReadAll(r.Body)
		var payload map[string]interface{}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("invalid JSON in request: %v", err)
		}
		if _, ok := payload["messages"]; !ok {
			t.Fatalf("messages field missing in request body")
		}

		// Respond with mock JSON
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResp)
	}))
	defer ts.Close()

	client := api.NewLLMProxyClient(ts.URL)

	// Call Query
	resp, err := client.Query("test-model", []models.Message{
		{Role: "user", Content: "hello"},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Assertions
	if resp.ID != mockResp.ID {
		t.Fatalf("expected ID %s, got %s", mockResp.ID, resp.ID)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(resp.Choices))
	}
	if resp.Choices[0].Message.Content != "hello world" {
		t.Fatalf("unexpected assistant message: %s", resp.Choices[0].Message.Content)
	}
}

func TestLLMProxyClient_Query_ErrorStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer ts.Close()

	client := api.NewLLMProxyClient(ts.URL)

	_, err := client.Query("model", []models.Message{
		{Role: "user", Content: "test"},
	})

	if err == nil {
		t.Fatalf("expected error but got nil")
	}

	if err.Error() == "" || err.Error() == "proxy request error:" {
		t.Fatalf("missing error message content: %v", err)
	}
}
