package assistant

import (
	"context"
	"errors"
	"testing"
	"time"

	"llm-proxy/internal/assistant/devices"
	"llm-proxy/internal/assistant/tools"
	"llm-proxy/internal/logging"
	"llm-proxy/internal/nodeherder"
	"llm-proxy/internal/proxy"
)

type testNodeHerder struct {
	deviceCtx     *nodeherder.LLMDeviceContext
	deviceErr     error
	metricsResult *nodeherder.MetricsQueryResponse
	metricsErr    error
	err           error
	callCount     int
}

func (t *testNodeHerder) GetDeviceContext() (*nodeherder.LLMDeviceContext, error) {
	t.callCount++
	if t.deviceErr != nil {
		return t.deviceCtx, t.deviceErr
	}
	return t.deviceCtx, t.err
}

func (t *testNodeHerder) QueryMetrics(ctx context.Context, req *nodeherder.MetricsQueryRequest) (*nodeherder.MetricsQueryResponse, error) {
	t.callCount++
	if t.metricsErr != nil {
		return t.metricsResult, t.metricsErr
	}
	return t.metricsResult, t.err
}

func (t *testNodeHerder) SetDeviceContextResult(ctx *nodeherder.LLMDeviceContext) {
	t.deviceCtx = ctx
}

func (t *testNodeHerder) SetMetricsResult(res *nodeherder.MetricsQueryResponse) {
	t.metricsResult = res
}

func (t *testNodeHerder) SetMetricsError(err error) {
	t.metricsErr = err
}

func (t *testNodeHerder) CallCount() int {
	return t.callCount
}

type noopLogger struct{}

func (noopLogger) Debug(string, ...any) {}
func (noopLogger) Info(string, ...any)  {}
func (noopLogger) Warn(string, ...any)  {}
func (noopLogger) Error(string, ...any) {}
func (noopLogger) With(...any) logging.Logger {
	return noopLogger{}
}
func (noopLogger) SetLevel(logging.Level) {}
func (noopLogger) Level() logging.Level   { return logging.LevelDebug }

type fixedClock struct {
	now time.Time
}

func (f fixedClock) Now() time.Time {
	return f.now
}

func TestAssistant_ExecuteTool_QueryMetrics_Success(t *testing.T) {
	mockNode := &testNodeHerder{}

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

	a := NewEngine(mockNode, noopLogger{})

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

}

func TestAssistant_ExecuteTool_QueryMetrics_InvalidJSON(t *testing.T) {
	mockNode := &testNodeHerder{}
	a := NewEngine(mockNode, noopLogger{})

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
}

func TestAssistant_ExecuteTool_QueryMetrics_BackendError(t *testing.T) {
	mockNode := &testNodeHerder{err: errors.New("backend failed")}
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

	a := NewEngine(mockNode, noopLogger{})

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
}

func TestAssistant_ExecuteTool_UnknownTool(t *testing.T) {
	mockNode := &testNodeHerder{}
	a := NewEngine(mockNode, noopLogger{})

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
	mockNode := &testNodeHerder{}
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

	a := NewEngine(mockNode, noopLogger{})

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
	mockNode := &testNodeHerder{}
	mockNode.SetMetricsResult(&nodeherder.MetricsQueryResponse{
		Expose: "temperature",
		Values: []nodeherder.MetricsQueryDeviceResponse{{DeviceId: "dev9", Value: 22}},
	})
	a := NewEngine(mockNode, noopLogger{})
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

type recordingNodeHerder struct {
	deviceCtx *nodeherder.LLMDeviceContext
	responses []*nodeherder.MetricsQueryResponse
	calls     []*nodeherder.MetricsQueryRequest
}

func (r *recordingNodeHerder) GetDeviceContext() (*nodeherder.LLMDeviceContext, error) {
	return r.deviceCtx, nil
}

func (r *recordingNodeHerder) QueryMetrics(ctx context.Context, req *nodeherder.MetricsQueryRequest) (*nodeherder.MetricsQueryResponse, error) {
	r.calls = append(r.calls, cloneMetricsRequest(req))
	idx := len(r.calls) - 1
	if idx < len(r.responses) {
		return r.responses[idx], nil
	}
	return &nodeherder.MetricsQueryResponse{}, nil
}

func cloneMetricsRequest(req *nodeherder.MetricsQueryRequest) *nodeherder.MetricsQueryRequest {
	if req == nil {
		return nil
	}
	clone := *req
	clone.DeviceIDs = append([]string(nil), req.DeviceIDs...)
	if req.Time != nil {
		tCopy := *req.Time
		clone.Time = &tCopy
	}
	return &clone
}

func TestAssistant_ExecuteTool_ExpandsLookbackOnEmptyLast(t *testing.T) {
	now := time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC)
	cfg := tools.NormalizeConfig{
		DefaultLookback:     time.Hour,
		MaxLookback:         24 * time.Hour,
		MaxExplicitLookback: 7 * 24 * time.Hour,
		MaxFutureSkew:       time.Minute,
	}

	node := &recordingNodeHerder{
		deviceCtx: &nodeherder.LLMDeviceContext{
			Devices: []nodeherder.LLMDevice{
				{
					ID:   "dev1",
					Name: "Kitchen Sensor",
					Exposes: []nodeherder.LLMExpose{
						{Name: "temperature"},
					},
				},
			},
		},
		responses: []*nodeherder.MetricsQueryResponse{
			{Expose: "temperature", Values: nil},
			{
				Expose: "temperature",
				Values: []nodeherder.MetricsQueryDeviceResponse{
					{DeviceId: "dev1", Value: 22.5},
				},
			},
		},
	}

	engine := &assistantEngine{
		nodeherder: node,
		logger:     noopLogger{},
		clock:      fixedClock{now: now},
		normalize:  cfg,
	}

	call := proxy.ToolCall{
		Function: proxy.FunctionCall{
			Name: "query_metrics",
			Arguments: `{
				"target_name": "kitchen",
				"expose": "temperature",
				"aggregation": "last",
				"time": {
					"from": 1735689600000,
					"to": 1735776000000
				}
			}`,
		},
	}

	out, err := engine.ExecuteTool(context.Background(), call)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Response == nil || out.Response.Expose != "temperature" {
		t.Fatalf("unexpected response: %#v", out.Response)
	}

	if len(node.calls) != 2 {
		t.Fatalf("expected 2 backend calls, got %d", len(node.calls))
	}

	expected := tools.BuildMaxLookbackTime(engine.clock, cfg)
	expanded := node.calls[1].Time
	if expanded == nil {
		t.Fatal("expected expanded time window")
	}
	if !expanded.From.Equal(expected.From) || !expanded.To.Equal(expected.To) {
		t.Fatalf("unexpected expanded window: %v - %v", expanded.From, expanded.To)
	}
}

func TestAssistant_ExecuteTool_NoExpandForNonLastAggregation(t *testing.T) {
	node := &recordingNodeHerder{
		deviceCtx: &nodeherder.LLMDeviceContext{
			Devices: []nodeherder.LLMDevice{
				{
					ID:   "dev1",
					Name: "Office Sensor",
					Exposes: []nodeherder.LLMExpose{
						{Name: "temperature"},
					},
				},
			},
		},
		responses: []*nodeherder.MetricsQueryResponse{
			{Expose: "temperature", Values: nil},
		},
	}

	engine := &assistantEngine{
		nodeherder: node,
		logger:     noopLogger{},
		clock:      fixedClock{now: time.Date(2025, 1, 2, 12, 0, 0, 0, time.UTC)},
		normalize:  tools.DefaultNormalizeConfig(),
	}

	call := proxy.ToolCall{
		Function: proxy.FunctionCall{
			Name: "query_metrics",
			Arguments: `{
				"target_name": "office",
				"expose": "temperature",
				"aggregation": "avg"
			}`,
		},
	}

	_, err := engine.ExecuteTool(context.Background(), call)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(node.calls) != 1 {
		t.Fatalf("expected 1 backend call, got %d", len(node.calls))
	}
}
