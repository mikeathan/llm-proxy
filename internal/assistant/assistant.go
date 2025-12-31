package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"llm-proxy/internal/logging"
	"llm-proxy/internal/nodeherder"
	"llm-proxy/internal/proxy"
)

type QueryMetricsArgs struct {
	DeviceID   string `json:"device_id"`
	Metric     string `json:"metric"`
	From       any    `json:"from,omitempty"`
	To         any    `json:"to,omitempty"`
	Aggregate  string `json:"aggregate,omitempty"`
	Resolution string `json:"resolution,omitempty"`
}

type Assistant struct {
	nodeherder nodeherder.NodeHerderService
	logger     logging.Logger
}

func NewAssistant(nodeherder nodeherder.NodeHerderService, logger logging.Logger) *Assistant {
	return &Assistant{
		nodeherder: nodeherder,
		logger:     logger,
	}
}
func (a *Assistant) ExecuteTool(ctx context.Context, call proxy.ToolCall) (string, error) {

	a.logger.Info("tool call", "name", call.Function.Name, "conversation", call.ID)

	function := call.Function
	switch function.Name {

	case "query_metrics":
		req, err := buildMetricsQueryRequest(call.Function.Arguments)
		if err != nil {
			a.logger.Error("tool parse failed", "name", call.Function.Name, "error", err)
			return "", err
		}

		res, err := a.nodeherder.QueryMetrics(ctx, req)
		if err != nil {
			a.logger.Error("tool execution failed", "name", call.Function.Name, "error", err)

			return "", err
		}

		bytes, err := json.Marshal(res)
		if err != nil {
			a.logger.Error("tool marshal failed", "name", call.Function.Name, "error", err)
			return "", err
		}

		a.logger.Info("tool completed", "name", call.Function.Name)
		return string(bytes), nil

	default:
		return "", fmt.Errorf("unknown tool: %s", function.Name)
	}
}

func buildMetricsQueryRequest(argJSON string) (*nodeherder.MetricsQueryRequest, error) {
	var args QueryMetricsArgs
	if err := json.Unmarshal([]byte(argJSON), &args); err != nil {
		return nil, err
	}

	return &nodeherder.MetricsQueryRequest{
		DeviceID:   args.DeviceID,
		Metric:     args.Metric,
		From:       args.From,
		To:         args.To,
		Aggregate:  args.Aggregate,
		Resolution: args.Resolution,
	}, nil
}
