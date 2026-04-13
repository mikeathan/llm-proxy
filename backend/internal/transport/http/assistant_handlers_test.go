package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/testing/mocks"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/persistence"
	"os"
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

	// Setup Persistence
	tmpWorkspaces := t.TempDir()
	service.PersistenceMgr = persistence.NewWorkspaceManager(tmpWorkspaces)
	defer os.RemoveAll(tmpWorkspaces)

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
	reqBody := `{"conversation_id": "conv1", "workspace_id": "test-ws", "message": "check lamp"}`
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
	// Check that the second call to Chat included the tool result
	if len(clientMock.Requests) < 2 {
		t.Fatalf("expected 2 calls to Chat, got %d", len(clientMock.Requests))
	}

	secondCallReq := clientMock.Requests[1] // The call AFTER the tool execution
	msgs := secondCallReq.Messages

	// Expected history: System, User, Assistant (Tool Call), Tool (Result)
	if len(msgs) != 4 {
		t.Errorf("expected 4 messages in history for second call, got %d", len(msgs))
		for i, m := range msgs {
			t.Logf("Message %d: Role=%s Content=%s ToolCalls=%d", i, m.Role, m.Content, len(m.ToolCalls))
		}
	} else {
		// Verify the sequence
		if msgs[2].Role != proxy.AssistantRole {
			t.Errorf("expected message 2 to be Assistant, got %s", msgs[2].Role)
		}
		if len(msgs[2].ToolCalls) == 0 {
			t.Errorf("expected message 2 to have tool calls")
		}

		if msgs[3].Role != proxy.ToolRole {
			t.Errorf("expected message 3 to be Tool, got %s", msgs[3].Role)
		}
		if msgs[3].ToolCallID != "call_1" {
			t.Errorf("expected message 3 ToolCallID to be 'call_1', got %s", msgs[3].ToolCallID)
		}

		// Verify content is the JSON result
		expectedContent := `{"state":"on"}`
		if msgs[3].Content != expectedContent { // approximate check, might depend on map ordering
			// Decode to map to compare if order varies
			var contentMap map[string]any
			if err := json.Unmarshal([]byte(msgs[3].Content), &contentMap); err != nil {
				t.Errorf("failed to unmarshal tool content: %v", err)
			}
			if s, ok := contentMap["state"].(string); !ok || s != "on" {
				t.Errorf("expected tool content state 'on', got %v", contentMap)
			}
		}
	}
}
