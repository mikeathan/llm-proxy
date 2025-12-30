package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"llm-proxy/internal/nodeherder"
	"llm-proxy/internal/proxy"
)

type QueryMetricsArgs struct {
	DeviceID   string `json:"device_id"`
	Metric     string `json:"metric"`
	From       int64  `json:"from,omitempty"`
	To         int64  `json:"to,omitempty"`
	Aggregate  string `json:"aggregate,omitempty"`
	Resolution string `json:"resolution,omitempty"`
}

type Assistant struct {
	nodeherder nodeherder.NodeHerderService
}

func (a *Assistant) ExecuteTool(ctx context.Context, call proxy.ToolCall) (string, error) {
	function := call.Function
	switch function.Name {

	case "query_metrics":
		var args QueryMetricsArgs
		json.Unmarshal([]byte(function.Arguments), &args)

		req := &nodeherder.QueryRequest{
			DeviceID:   args.DeviceID,
			Metric:     args.Metric,
			From:       args.From,
			To:         args.To,
			Aggregate:  args.Aggregate,
			Resolution: args.Resolution,
		}

		res, err := a.nodeherder.QueryMetrics(ctx, req)
		if err != nil {
			return "", err
		}

		bytes, err := json.Marshal(res)
		if err != nil {
			return "", err
		}
		to fix the response here!
		return string(bytes), nil

	default:
		return "", fmt.Errorf("unknown tool: %s", function.Name)
	}
}
