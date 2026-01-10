package assistant_test

import (
	"context"
	"errors"
	"testing"

	"llm-proxy/internal/assistant"
	"llm-proxy/internal/assistant/devices"
	"llm-proxy/internal/mocks"
	"llm-proxy/internal/nodeherder"
	"llm-proxy/internal/proxy"
)

func TestAssistant_ExecuteTool_QueryMetrics_Success(t *testing.T) {
	mockNode := mocks.NewMockNodeHerder(nil)

	expected := &nodeherder.MetricsQueryResponse{
		Expose: "temperature",
		From:   1735689600000,
		To:     1735776000000,
		Values: []nodeherder.MetricsQueryDeviceResponse{
			{
				DeviceId:  "dev1",
				Value:     21.5,
				Timestamp: 1735689600000,
			},
		},
	}

	mockNode.SetMetricsResult(expected)
	mockNode.SetDeviceContextResult(&nodeherder.LLMDeviceContext{
		Devices: []nodeherder.LLMDevice{
			{
				ID:   "dev1",
				Name: "Living Room Sensor",
				Exposes: []nodeherder.LLMExpose{
					{Name: "temperature"},
				},
			},
		},
	})

	logger := &mocks.MockLogger{}

	a := assistant.NewEngine(mockNode, logger)

	call := proxy.ToolCall{
		ID: "conv1",
		Function: proxy.FunctionCall{
			Name: "query_metrics",
			Arguments: `{
				"target_name": "living room",
				"expose": "temperature (°C)",
				"time": {
					"from": 1735689600000,
					"to":   1735776000000
				},
				"aggregation": "avg"
			}`,
		},
	}

	out, err := a.ExecuteTool(context.Background(), call)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if out.Response == nil {
		t.Fatal("expected response payload")
	}
	if out.Response.Expose != expected.Expose ||
		out.Response.From != expected.From ||
		out.Response.To != expected.To {
		t.Fatalf("unexpected output: %+v", out.Response)
	}

	if len(out.Response.Values) != len(expected.Values) {
		t.Fatalf("unexpected values: %+v", out.Response.Values)
	}

	if out.Aggregation != nodeherder.AggregationType("avg") {
		t.Fatalf("unexpected aggregation: %s", out.Aggregation)
	}

	if mockNode.CallCount() != 2 {
		t.Fatalf("expected 2 backend calls, got %d", mockNode.CallCount())
	}

	if len(logger.Errors()) != 0 {
		t.Fatalf("unexpected logged errors: %v", logger.Errors())
	}
}

func TestAssistant_ExecuteTool_QueryMetrics_InvalidJSON(t *testing.T) {
	mockNode := mocks.NewMockNodeHerder(nil)
	logger := &mocks.MockLogger{}

	a := assistant.NewEngine(mockNode, logger)

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
	mockNode.SetDeviceContextResult(&nodeherder.LLMDeviceContext{
		Devices: []nodeherder.LLMDevice{
			{
				ID:   "dev1",
				Name: "Kitchen Sensor",
				Exposes: []nodeherder.LLMExpose{
					{Name: "temperature"},
				},
			},
		},
	})

	a := assistant.NewEngine(mockNode, logger)

	call := proxy.ToolCall{
		Function: proxy.FunctionCall{
			Name: "query_metrics",
			Arguments: `{
				"target_name": "kitchen",
				"expose": "temperature",
				"time": {
					"from": 1735689600000,
    				"to": 1735776000000
				}
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

	a := assistant.NewEngine(mockNode, logger)

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

func TestAssistant_ExecuteTool_AmbiguousDevice(t *testing.T) {
	mockNode := mocks.NewMockNodeHerder(nil)
	mockNode.SetDeviceContextResult(&nodeherder.LLMDeviceContext{
		Devices: []nodeherder.LLMDevice{
			{
				ID:   "dev1",
				Name: "Attic Sensor",
				Exposes: []nodeherder.LLMExpose{
					{Name: "temperature"},
				},
			},
			{
				ID:   "dev2",
				Name: "Attic Temperature Sensor",
				Exposes: []nodeherder.LLMExpose{
					{Name: "temperature"},
				},
			},
		},
	})

	logger := &mocks.MockLogger{}
	a := assistant.NewEngine(mockNode, logger)

	call := proxy.ToolCall{
		Function: proxy.FunctionCall{
			Name: "query_metrics",
			Arguments: `{
				"target_name": "attic",
				"expose": "temperature"
			}`,
		},
	}

	_, err := a.ExecuteTool(context.Background(), call)
	if err == nil {
		t.Fatal("expected error")
	}

	var amb *devices.AmbiguousDeviceError
	if !errors.As(err, &amb) {
		t.Fatalf("expected AmbiguousDeviceError, got %v", err)
	}
}

func TestAssistant_ExecuteToolWithDevice_UsesExplicitID(t *testing.T) {
	mockNode := mocks.NewMockNodeHerder(nil)
	mockNode.SetMetricsResult(&nodeherder.MetricsQueryResponse{
		Expose: "temperature",
		Values: []nodeherder.MetricsQueryDeviceResponse{{DeviceId: "dev9", Value: 22}},
	})
	logger := &mocks.MockLogger{}

	a := assistant.NewEngine(mockNode, logger)
	call := proxy.ToolCall{
		Function: proxy.FunctionCall{
			Name: "query_metrics",
			Arguments: `{
				"target_name": "anything",
				"expose": "temperature"
			}`,
		},
	}

	_, err := a.ExecuteToolWithDevice(context.Background(), call, "dev9")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mockNode.CallCount() != 1 {
		t.Fatalf("expected 1 backend call, got %d", mockNode.CallCount())
	}
}
