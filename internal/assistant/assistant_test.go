package assistant_test

import (
	"context"
	"errors"
	"llm-proxy/internal/assistant"
	"llm-proxy/internal/mocks"
	"llm-proxy/internal/nodeherder"
	"llm-proxy/internal/proxy"

	"strings"
	"testing"
)

func TestAssistant_ExecuteTool_QueryMetrics_Success(t *testing.T) {
	mockNode := mocks.NewMockNodeHerder(nil)

	expected := &nodeherder.MetricsQueryResponse{
		Expose: "temperature",
	}

	mockNode.SetMetricsResult(expected)

	logger := &mocks.MockLogger{}

	a := assistant.NewAssistant(mockNode, logger)

	call := proxy.ToolCall{
		ID: "conv1",
		Function: proxy.FunctionCall{
			Name: "query_metrics",
			Arguments: `{
				"device_id": "dev1",
				"expose": "temperature",
				"from": "2025-01-01T00:00:00Z",
				"to": "now",
				"aggregation": "avg"
			}`,
		},
	}

	out, err := a.ExecuteTool(context.Background(), call)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(out, `"expose":"temperature"`) {
		t.Fatalf("unexpected output: %s", out)
	}

	if mockNode.CallCount() != 1 {
		t.Fatalf("expected 1 backend call, got %d", mockNode.CallCount())
	}

	if len(logger.Errors()) != 0 {
		t.Fatalf("unexpected logged errors: %v", logger.Errors())
	}
}

func TestAssistant_ExecuteTool_QueryMetrics_InvalidJSON(t *testing.T) {
	mockNode := mocks.NewMockNodeHerder(nil)
	logger := &mocks.MockLogger{}

	a := assistant.NewAssistant(mockNode, logger)

	call := proxy.ToolCall{
		Function: proxy.FunctionCall{
			Name:      "query_metrics",
			Arguments: `{invalid`,
		},
	}

	_, err := a.ExecuteTool(context.Background(), call)
	if err == nil {
		t.Fatal("expected error")
	}

	if len(logger.Errors()) == 0 {
		t.Fatalf("expected error to be logged")
	}
}

func TestAssistant_ExecuteTool_QueryMetrics_BackendError(t *testing.T) {
	mockNode := mocks.NewMockNodeHerder(errors.New("backend failed"))
	logger := &mocks.MockLogger{}

	a := assistant.NewAssistant(mockNode, logger)

	call := proxy.ToolCall{
		Function: proxy.FunctionCall{
			Name: "query_metrics",
			Arguments: `{
				"device_id": "dev1",
				"expose": "temperature",
				"from": "2025-01-01T00:00:00Z",
				"to": "now"
			}`,
		},
	}

	_, err := a.ExecuteTool(context.Background(), call)
	if err == nil {
		t.Fatal("expected error")
	}

	if mockNode.CallCount() != 1 {
		t.Fatalf("expected 1 backend call")
	}

	if len(logger.Errors()) == 0 {
		t.Fatalf("expected error to be logged")
	}
}

func TestAssistant_ExecuteTool_UnknownTool(t *testing.T) {
	mockNode := mocks.NewMockNodeHerder(nil)
	logger := &mocks.MockLogger{}

	a := assistant.NewAssistant(mockNode, logger)

	call := proxy.ToolCall{
		Function: proxy.FunctionCall{
			Name: "no_such_tool",
		},
	}

	_, err := a.ExecuteTool(context.Background(), call)
	if err == nil {
		t.Fatal("expected error")
	}
}
