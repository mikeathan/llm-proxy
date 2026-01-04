package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"llm-proxy/internal/logging"
	"llm-proxy/internal/nodeherder"
	"llm-proxy/internal/proxy"
	"time"
)

type QueryMetricsArgs struct {
	DeviceID    string         `json:"device_id"`
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

type Engine interface {
	ExecuteTool(ctx context.Context, call proxy.ToolCall) (*nodeherder.MetricsQueryResponse, error)
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
func (a *assistantEngine) ExecuteTool(ctx context.Context, call proxy.ToolCall) (*nodeherder.MetricsQueryResponse, error) {

	a.logger.Info("tool call", "name", call.Function.Name, "conversation", call.ID)

	function := call.Function
	switch function.Name {

	case "query_metrics":
		a.logger.Info("tool args", "name", call.Function.Name, "args", call.Function.Arguments)
		req, err := buildMetricsQueryRequest(call.Function.Arguments)
		if err != nil {
			a.logger.Error("tool parse failed", "name", call.Function.Name, "error", err)
			return nil, err
		}

		res, err := a.nodeherder.QueryMetrics(ctx, req)
		if err != nil {
			a.logger.Error("tool execution failed", "name", call.Function.Name, "error", err)

			return nil, err
		}

		a.logger.Info("tool completed", "name", call.Function.Name)
		return res, nil

	default:
		return nil, fmt.Errorf("unknown tool: %s", function.Name)
	}
}

func (q QueryMetricsArgs) Validate() error {
	if q.DeviceID == "" || q.Expose == "" {
		return fmt.Errorf("device_id and expose are required")
	}

	if q.Aggregation == "" {
		if q.Time == nil {
			return fmt.Errorf("either aggregate or time must be provided")
		}
		if q.Time.Lookback == "" && (q.Time.From == 0 || q.Time.To == 0) {
			return fmt.Errorf("time.from and time.to must be provided when lookback is empty")
		}
	}

	return nil
}

func buildMetricsQueryRequest(argJSON string) (*nodeherder.MetricsQueryRequest, error) {
	var args QueryMetricsArgs
	if err := json.Unmarshal([]byte(argJSON), &args); err != nil {
		return nil, err
	}

	if err := args.Validate(); err != nil {
		return nil, err
	}

	var timeQuery *nodeherder.TimeQuery
	if args.Time != nil {
		var from, to time.Time

		if args.Time.From != 0 {
			from = time.UnixMilli(args.Time.From)
		}
		if args.Time.To != 0 {
			to = time.UnixMilli(args.Time.To)
		}

		timeQuery = &nodeherder.TimeQuery{
			From:     from,
			To:       to,
			Lookback: args.Time.Lookback,
		}
	}

	return &nodeherder.MetricsQueryRequest{
		DeviceIDs:  []string{args.DeviceID},
		Expose:     args.Expose,
		Time:       timeQuery,
		Aggregate:  args.Aggregation,
		Resolution: args.Resolution,
	}, nil
}
