package assistant_test

import (
	"context"
	"encoding/json"
	"testing"

	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/testing/mocks"
	"llm-proxy/internal/core/proxy"
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

// Simple test for generic engine
func TestExecuteTool(t *testing.T) {
	mockMCP := mocks.NewMockNodeHerder(nil)
	logger := &noopLogger{}

	engine := assistant.NewEngine(mockMCP, logger)

	expectedArgs := map[string]any{
		"target_name": "lamp 1",
		"expose":      "status",
	}
	argsBytes, _ := json.Marshal(expectedArgs)

	mockMCP.SetCallToolResult(map[string]any{"status": "ok"}, nil)

	call := proxy.ToolCall{
		ID:   "call_1",
		Type: "function",
		Function: proxy.FunctionCall{
			Name:      "query_device",
			Arguments: string(argsBytes),
		},
	}

	result, err := engine.ExecuteTool(context.Background(), call)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify result
	resMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", result)
	}
	if resMap["status"] != "ok" {
		t.Errorf("expected status ok, got %v", resMap["status"])
	}

	// Verify mock call count (CallTool)
	if mockMCP.CallCount() != 1 {
		t.Errorf("expected 1 call, got %d", mockMCP.CallCount())
	}
}
