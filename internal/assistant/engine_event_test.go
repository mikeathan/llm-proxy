package assistant

import (
	"context"
	"llm-proxy/internal/assistant/tools"
	"llm-proxy/internal/nodeherder"
	"llm-proxy/internal/proxy"
	"testing"
	"time"
)

func TestAssistant_ExecuteTool_EventAggregation(t *testing.T) {
	node := &recordingNodeHerder{
		deviceCtx: &nodeherder.LLMDeviceContext{
			Devices: []nodeherder.LLMDevice{
				{
					ID:   "dev1",
					Name: "Door Sensor",
					Exposes: []nodeherder.LLMExpose{
						{Name: "contact", Type: "binary"},
					},
				},
			},
		},
		responses: []*nodeherder.MetricsQueryResponse{
			{
				Expose: "contact",
				Values: []nodeherder.MetricsQueryDeviceResponse{
					{DeviceId: "dev1", Value: true, Timestamp: 1000},
				},
			},
		},
	}

	engine := &assistantEngine{
		nodeherder: node,
		logger:     noopLogger{},
		clock:      fixedClock{now: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)},
		normalize:  tools.DefaultNormalizeConfig(),
	}

	// Case 1: aggregation="event" with event_value
	call := proxy.ToolCall{
		Function: proxy.FunctionCall{
			Name: "query_metrics",
			Arguments: `{
				"target_name": "door",
				"expose": "contact",
				"aggregation": "event",
				"event_value": true
			}`,
		},
	}

	out, err := engine.ExecuteTool(context.Background(), call)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(node.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(node.calls))
	}

	req := node.calls[0]
	if req.Aggregation != nodeherder.LastEvent {
		t.Errorf("expected aggregation 'last_event', got '%s'", req.Aggregation)
	}

	if val, ok := req.AggregationValue.(bool); !ok || !val {
		t.Errorf("expected aggregation value true, got %v", req.AggregationValue)
	}

	if out.Aggregation != nodeherder.LastEvent {
		t.Errorf("expected output aggregation 'last_event', got '%s'", out.Aggregation)
	}
}

func TestAssistant_ExecuteTool_PositiveOutcome(t *testing.T) {
	node := &recordingNodeHerder{
		deviceCtx: &nodeherder.LLMDeviceContext{
			Devices: []nodeherder.LLMDevice{
				{
					ID:   "dev1",
					Name: "Motion Sensor",
					Exposes: []nodeherder.LLMExpose{
						{Name: "occupancy", Type: "binary"},
					},
				},
			},
		},
		responses: []*nodeherder.MetricsQueryResponse{
			{
				Expose: "occupancy",
				Values: []nodeherder.MetricsQueryDeviceResponse{
					{DeviceId: "dev1", Value: true, Timestamp: 2000},
				},
			},
		},
	}

	engine := &assistantEngine{
		nodeherder: node,
		logger:     noopLogger{},
		clock:      fixedClock{now: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)},
		normalize:  tools.DefaultNormalizeConfig(),
	}

	// Case 2: positive_outcome=true
	call := proxy.ToolCall{
		Function: proxy.FunctionCall{
			Name: "query_metrics",
			Arguments: `{
				"target_name": "motion",
				"expose": "occupancy",
				"time": {"lookback": "1h"},
				"positive_outcome": true
			}`,
		},
	}

	out, err := engine.ExecuteTool(context.Background(), call)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(node.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(node.calls))
	}

	req := node.calls[0]
	if req.Aggregation != nodeherder.LastEvent {
		t.Errorf("expected aggregation 'last_event', got '%s'", req.Aggregation)
	}

	// positive_outcome uses LastEvent on server but maps to AggLast on return (as per code)
	if out.Aggregation != nodeherder.AggLast {
		t.Errorf("expected output aggregation 'last', got '%s'", out.Aggregation)
	}

	if val, ok := req.AggregationValue.(bool); !ok || !val {
		t.Errorf("expected aggregation value true, got %v", req.AggregationValue)
	}
}
