package assistant

import (
	"context"
	"fmt"
	"llm-proxy/internal/assistant/devices"
	"llm-proxy/internal/assistant/tools"
	"llm-proxy/internal/logging"
	"llm-proxy/internal/nodeherder"
	"llm-proxy/internal/proxy"
	"strings"
)

type ToolResult struct {
	Response    *nodeherder.MetricsQueryResponse
	Aggregation nodeherder.AggregationType
}

type Engine interface {
	ExecuteTool(ctx context.Context, call proxy.ToolCall) (*ToolResult, error)
	ExecuteToolWithDevice(ctx context.Context, call proxy.ToolCall, deviceID string) (*ToolResult, error)
}

type assistantEngine struct {
	nodeherder nodeherder.NodeHerderService
	logger     logging.Logger
	clock      tools.Clock
	normalize  tools.NormalizeConfig
}

func NewEngine(nodeherder nodeherder.NodeHerderService, logger logging.Logger) Engine {
	return &assistantEngine{
		nodeherder: nodeherder,
		logger:     logger,
		clock:      tools.RealClock{},
		normalize:  tools.DefaultNormalizeConfig(),
	}
}

func (a *assistantEngine) ExecuteTool(ctx context.Context, call proxy.ToolCall) (*ToolResult, error) {
	// ExecuteTool parses and normalizes LLM tool args, resolves a device, and executes
	// the metrics query with a sanitized request.
	a.logger.Info("tool call", "name", call.Function.Name, "conversation", call.ID)

	if call.Function.Name != "query_metrics" {
		return nil, fmt.Errorf("unknown tool: %s", call.Function.Name)
	}

	args, err := tools.ParseMetricsArgs(call.Function.Arguments)
	if err != nil {
		return nil, err
	}

	a.logger.Debug("raw tool args",
		"target_name", args.TargetName,
		"expose", args.Expose,
		"aggregation", args.Aggregation,
		"time", args.Time,
	)

	normalized, err := tools.NormalizeMetricsArgs(args, a.normalize, a.clock)
	if err != nil {
		return nil, err
	}

	deviceCtx, err := a.nodeherder.GetDeviceContext()
	if err != nil {
		return nil, err
	}
	a.logger.Debug("device context loaded", "device_count", len(deviceCtx.Devices))

	a.logger.Debug("resolving device",
		"target", normalized.TargetName,
		"expose", normalized.Expose,
	)

	device, err := devices.ResolveDevice(deviceCtx, normalized.TargetName, normalized.Expose)
	if err != nil {
		return nil, err
	}

	return a.executeMetrics(ctx, normalized, device.ID, device.Name)
}

func (a *assistantEngine) ExecuteToolWithDevice(ctx context.Context, call proxy.ToolCall, deviceID string) (*ToolResult, error) {
	// ExecuteToolWithDevice bypasses resolution after clarification and reuses the
	// normalized tool args for the resolved device id.
	if call.Function.Name != "query_metrics" {
		return nil, fmt.Errorf("unknown tool: %s", call.Function.Name)
	}

	if strings.TrimSpace(deviceID) == "" {
		return nil, fmt.Errorf("device_id is required")
	}

	args, err := tools.ParseMetricsArgs(call.Function.Arguments)
	if err != nil {
		return nil, err
	}

	normalized, err := tools.NormalizeMetricsArgs(args, a.normalize, a.clock)
	if err != nil {
		return nil, err
	}

	return a.executeMetrics(ctx, normalized, deviceID, "")
}

func (a *assistantEngine) executeMetrics(ctx context.Context, normalized tools.NormalizedMetricsArgs, deviceID, deviceName string) (*ToolResult, error) {
	req := &nodeherder.MetricsQueryRequest{
		DeviceIDs:   []string{deviceID},
		Expose:      normalized.Expose,
		Time:        normalized.Time,
		Aggregation: normalized.Aggregation,
		Resolution:  normalized.Resolution,
	}

	a.logger.Info("normalized tool request",
		"device", deviceName,
		"device_id", deviceID,
		"expose", req.Expose,
		"aggregation", req.Aggregation,
		"time", req.Time,
	)

	res, err := a.nodeherder.QueryMetrics(ctx, req)
	if err != nil {
		return nil, err
	}

	return &ToolResult{
		Response:    res,
		Aggregation: nodeherder.AggregationType(req.Aggregation),
	}, nil
}
