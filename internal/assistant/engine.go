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
	DeviceID    string `json:"device_id"`
	Expose      string `json:"expose"`
	From        int64  `json:"from,omitempty"`
	To          int64  `json:"to,omitempty"`
	Aggregation string `json:"aggregation,omitempty"`
	Resolution  string `json:"resolution,omitempty"`
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

	if q.Aggregation == "" && (q.From == 0 || q.To == 0) {
		return fmt.Errorf("either aggregate or from+to must be provided")
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

	now := time.Now().UnixMilli()
	from := args.From
	to := args.To

	if args.Aggregation == "" {
		if from == 0 && to == 0 {
			// default window: last 24 hours
			to = now
			from = now - 24*60*60*1000
		}
	} else {
		// aggregation query: ignore time range entirely
		from = 0
		to = 0
	}

	return &nodeherder.MetricsQueryRequest{
		DeviceIDs:  []string{args.DeviceID},
		Expose:     args.Expose,
		From:       from,
		To:         to,
		Aggregate:  args.Aggregation,
		Resolution: args.Resolution,
	}, nil
}
