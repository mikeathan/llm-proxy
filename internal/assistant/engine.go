package assistant

import (
	"context"
	"fmt"
	"llm-proxy/internal/assistant/devices"
	"llm-proxy/internal/assistant/tools"
	"llm-proxy/internal/logging"
	"llm-proxy/internal/nodeherder"
	"llm-proxy/internal/proxy"
	"llm-proxy/utils"
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
	clock      utils.Clock
	normalize  tools.NormalizeConfig
}

func NewEngine(nodeherder nodeherder.NodeHerderService, logger logging.Logger) Engine {
	return &assistantEngine{
		nodeherder: nodeherder,
		logger:     logger,
		clock:      &utils.RealClock{},
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

func (a *assistantEngine) executeMetrics(ctx context.Context, args tools.NormalizedMetricsArgs, deviceID, deviceName string) (*ToolResult, error) {
	// Handle binary event filtering when a positive outcome is specified
	if args.PositiveOutcome != nil {
		req := &nodeherder.MetricsQueryRequest{
			DeviceIDs:        []string{deviceID},
			Expose:           args.Expose,
			Time:             args.Time,
			Aggregation:      nodeherder.LastEvent,
			AggregationValue: *args.PositiveOutcome,
		}

		a.logger.Info("normalized tool request for binary event (last_event)",
			"device", deviceName,
			"device_id", deviceID,
			"expose", req.Expose,
			"aggregation", req.Aggregation,
			"aggregation_value", req.AggregationValue,
			"time", req.Time,
		)

		res, err := a.nodeherder.QueryMetrics(ctx, req)
		if err != nil {
			return nil, err
		}

		return &ToolResult{
			Response:    res,
			Aggregation: nodeherder.AggLast, // Treat as 'last' for normalization to match previous behavior
		}, nil
	}

	if args.Aggregation == "event" || args.Aggregation == string(nodeherder.LastEvent) {
		if args.EventValue == nil {
			return nil, fmt.Errorf("event_value is required for event aggregation")
		}

		req := &nodeherder.MetricsQueryRequest{
			DeviceIDs:        []string{deviceID},
			Expose:           args.Expose,
			Time:             args.Time,
			Aggregation:      nodeherder.LastEvent,
			AggregationValue: *args.EventValue,
		}

		a.logger.Info("normalized tool request for event (last_event)",
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
			Aggregation: nodeherder.LastEvent,
		}, nil
	}

	req := &nodeherder.MetricsQueryRequest{
		DeviceIDs:   []string{deviceID},
		Expose:      args.Expose,
		Time:        args.Time,
		Aggregation: nodeherder.AggregationType(args.Aggregation),
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

	// Adaptive expansion for last queries only
	if req.Aggregation == nodeherder.AggLast &&
		len(res.Values) == 0 &&
		args.Time != nil {

		res, err = a.expandLookbackAndRetry(ctx, req)
		if err != nil {
			return nil, err
		}
	}

	return &ToolResult{
		Response:    res,
		Aggregation: req.Aggregation,
	}, nil
}

func (a *assistantEngine) expandLookbackAndRetry(ctx context.Context, req *nodeherder.MetricsQueryRequest) (*nodeherder.MetricsQueryResponse, error) {

	a.logger.Info("no data in recent window, expanding lookback")

	req.Time = tools.BuildMaxLookbackTime(a.clock, a.normalize)

	a.logger.Info("retrying with expanded window",
		"from", req.Time.From,
		"to", req.Time.To,
	)

	return a.nodeherder.QueryMetrics(ctx, req)
}
