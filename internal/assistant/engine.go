package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"llm-proxy/internal/logging"
	"llm-proxy/internal/nodeherder"
	"llm-proxy/internal/proxy"
	"strconv"
	"strings"
	"time"
)

type QueryMetricsArgs struct {
	TargetName  string         `json:"target_name"` // semantic device name (e.g. "garden")
	Expose      string         `json:"expose"`
	Time        *TimeQueryArgs `json:"time,omitempty"`
	Aggregation string         `json:"aggregation,omitempty"`
	Resolution  string         `json:"resolution,omitempty"`
}

type TimeQueryArgs struct {
	From     int64  `json:"from,omitempty"`
	To       int64  `json:"to,omitempty"`
	Lookback string `json:"lookback,omitempty"`
}

type ToolResult struct {
	Response    *nodeherder.MetricsQueryResponse
	Aggregation nodeherder.AggregationType
}

type Engine interface {
	ExecuteTool(ctx context.Context, call proxy.ToolCall) (*ToolResult, error)
}

type assistantEngine struct {
	nodeherder nodeherder.NodeHerderService
	logger     logging.Logger
}

func NewEngine(nodeherder nodeherder.NodeHerderService, logger logging.Logger) Engine {
	return &assistantEngine{
		nodeherder: nodeherder,
		logger:     logger,
	}
}

func (a *assistantEngine) ExecuteTool(ctx context.Context, call proxy.ToolCall) (*ToolResult, error) {

	a.logger.Info("tool call", "name", call.Function.Name, "conversation", call.ID)

	if call.Function.Name != "query_metrics" {
		return nil, fmt.Errorf("unknown tool: %s", call.Function.Name)
	}

	var args QueryMetricsArgs
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
		return nil, err
	}

	if err := args.Validate(); err != nil {
		return nil, err
	}

	// Enforce deterministic defaults
	if args.Aggregation == "" {
		args.Aggregation = string(nodeherder.AggLast)
	}

	// Load device context
	deviceCtx, err := a.nodeherder.GetDeviceContext()
	if err != nil {
		return nil, err
	}

	// Resolve the correct device deterministically
	device, err := ResolveDevice(deviceCtx, args.TargetName, args.Expose)
	if err != nil {
		return nil, err
	}

	timeQuery, err := buildTimeQuery(args.Time)
	if err != nil {
		return nil, err
	}

	req := &nodeherder.MetricsQueryRequest{
		DeviceIDs:   []string{device.ID},
		Expose:      args.Expose,
		Time:        timeQuery,
		Aggregation: args.Aggregation,
		Resolution:  args.Resolution,
	}

	a.logger.Info("normalized tool request",
		"device", device.Name,
		"device_id", device.ID,
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

func (q QueryMetricsArgs) Validate() error {
	if q.TargetName == "" || q.Expose == "" {
		return fmt.Errorf("target_name and expose are required")
	}
	return nil
}

func buildTimeQuery(t *TimeQueryArgs) (*nodeherder.TimeQuery, error) {
	if t == nil {
		return nil, nil
	}

	q := &nodeherder.TimeQuery{}

	if t.To != 0 && t.To == t.From {
		t.To = 0
	}

	if t.Lookback != "" {
		dur, err := parseLookback(t.Lookback)
		if err != nil {
			return nil, err
		}

		to := time.Now().UTC()
		if t.To != 0 {
			to = time.UnixMilli(t.To).UTC()
		}
		from := to.Add(-dur)

		q.From = from
		q.To = to
		return q, nil
	}

	if t.From != 0 {
		q.From = time.UnixMilli(t.From).UTC()
	}
	if t.To != 0 {
		q.To = time.UnixMilli(t.To).UTC()
	}

	return q, nil
}

func parseLookback(s string) (time.Duration, error) {
	if strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, err
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}
