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

	a.logger.Debug("raw tool args",
		"target_name", args.TargetName,
		"expose", args.Expose,
		"aggregation", args.Aggregation,
		"time", args.Time,
	)

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
	a.logger.Debug("device context loaded",
		"device_count", len(deviceCtx.Devices),
	)

	a.logger.Debug("resolving device",
		"target", args.TargetName,
		"expose", args.Expose,
	)

	device, err := ResolveDevice(deviceCtx, args.TargetName, args.Expose)
	if err != nil {
		a.logger.Error("device resolution failed",
			"target", args.TargetName,
			"expose", args.Expose,
			"error", err,
		)
		return nil, err
	}

	a.logger.Info("device resolved",
		"name", device.Name,
		"id", device.ID,
	)

	timeQuery, err := buildTimeQuery(args.Time)
	if err != nil {
		return nil, err
	}
	a.logger.Debug("normalized time query",
		"input", args.Time,
		"result", timeQuery,
	)

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

	if t.From < 0 {
		t.From = 0
	}
	if t.To < 0 {
		t.To = 0
	}

	switch t.Lookback {
	case "", "10s", "30s", "1m", "5m", "10m", "30m",
		"1h", "6h", "12h", "24h", "1d", "7d", "30d":
		// allowed
	default:
		// reject or normalize nonsense like "last"
		t.Lookback = ""
	}

	if t.Lookback == "" && t.From == 0 && t.To == 0 {
		t.Lookback = "24h"
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

		// TODO: use Clock interface for testability

		to := time.Now().UTC()
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
	s = strings.TrimSpace(strings.ToLower(s))

	switch s {
	case "day", "1 day":
		return 24 * time.Hour, nil
	case "week", "1 week":
		return 7 * 24 * time.Hour, nil
	case "hour", "1 hour":
		return time.Hour, nil
	case "minute", "1 minute":
		return time.Minute, nil
	}

	if strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, err
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}

	return time.ParseDuration(s)
}
