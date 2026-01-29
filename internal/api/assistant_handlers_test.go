package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"llm-proxy/internal/assistant"
	"llm-proxy/internal/logging"
	"llm-proxy/internal/mocks"
	"llm-proxy/internal/proxy"
)

// noopLogger for testing
type noopLogger struct{}

func (l *noopLogger) Debug(msg string, args ...any)   {}
func (l *noopLogger) Info(msg string, args ...any)    {}
func (l *noopLogger) Warn(msg string, args ...any)    {}
func (l *noopLogger) Error(msg string, args ...any)   {}
func (l *noopLogger) With(args ...any) logging.Logger { return l }
func (l *noopLogger) SetLevel(logging.Level)          {}
func (l *noopLogger) Level() logging.Level            { return logging.LevelInfo }

func TestHandleAssistant_AgnosticFlow(t *testing.T) {
	// Setup
	logger := &noopLogger{}
	mockMCP := mocks.NewMockNodeHerder(nil)
	mockClient := &mocks.MockLLMClientProvider{
		Client: &mocks.MockLLMClient{},
	}
	mockLimiter := &mocks.MockRateLimiter{}

	// Setup MCP resources
	mockMCP.SetSystemPrompt("System Prompt")

	// Setup MCP Tools
	mockMCP.SetToolsResult([]proxy.Tool{
		{
			Type: "function",
			Function: proxy.FunctionSchema{
				Name: "query_device",
			},
		},
	})

	// Setup Engine
	engine := assistant.NewEngine(mockMCP, logger)

	// service
	service := mocks.NewMockAssistantService(
		mockClient,
		mockLimiter,
		engine,
		mockMCP,
	)

	handler := NewAssistantMessageHandler(service)

	// Mock LLM Response to call tool
	clientMock := mockClient.Client.(*mocks.MockLLMClient)

	// 1. First LLM response: Call Tool
	toolArgs := map[string]any{"target_name": "lamp"}
	argsJSON, _ := json.Marshal(toolArgs)

	clientMock.Responses = []proxy.ChatResponse{
		{
			Choices: []proxy.Choice{{
				Message: proxy.Message{
					Role: proxy.AssistantRole,
					ToolCalls: []proxy.ToolCall{{
						ID:   "call_1",
						Type: "function",
						Function: proxy.FunctionCall{
							Name:      "query_device",
							Arguments: string(argsJSON),
						},
					}},
				},
			}},
		},
		{
			Choices: []proxy.Choice{{
				Message: proxy.Message{
					Role:    proxy.AssistantRole,
					Content: "The lamp is on.",
				},
			}},
		},
	}

	// Setup Tool Execution Result
	mockMCP.SetCallToolResult(map[string]any{"state": "on"}, nil)

	// Request
	reqBody := `{"conversation_id": "conv1", "message": "check lamp"}`
	req := httptest.NewRequest("POST", "/api/conversation/message", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Execute
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body: %s", w.Code, w.Body.String())
	}

	// Check response body
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if reply, ok := resp["reply"].(string); !ok || reply != "The lamp is on." {
		t.Errorf("expected reply 'The lamp is on.', got %v", resp)
	}

	// Verify calls
	// 1. GetSystemPrompt
	// 2. ListTools (called by callModel)
	// 3. CallTool (called by processToolCall -> engine)

	// We can check call count roughly or add specific spy methods if needed.
	// MockNodeHerder CallCount aggregates all calls.
	if mockMCP.CallCount() < 3 {
		t.Errorf("expected at least 3 MCP calls, got %d", mockMCP.CallCount())
	}
}
