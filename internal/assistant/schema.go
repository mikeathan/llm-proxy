package assistant

import "llm-proxy/internal/proxy"

type MetricsQuery struct {
	DeviceID    string `json:"device_id"`
	Expose      string `json:"expose"`
	From        string `json:"from"`
	To          string `json:"to"`
	Aggregation string `json:"aggregation,omitempty"`
}

func MetricsToolSchema() proxy.Tool {
	return proxy.Tool{
		Type: "function",
		Function: proxy.FunctionSchema{
			Name:        "query_metrics",
			Description: "Query time-series metrics from the provided device context",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"device_id": map[string]any{
						"type":        "string",
						"description": "ID of the device",
					},
					"expose": map[string]any{
						"type":        "string",
						"description": "Expose name of the device",
					},
					"from": map[string]any{
						"type":        "string",
						"description": "Start time (ISO8601 or relative: e.g. '24h')",
					},
					"to": map[string]any{
						"type":        "string",
						"description": "End time (ISO8601 or 'now')",
					},
					"aggregation": map[string]any{
						"type": "string",
						"enum": []string{"last", "min", "max", "avg", "count"},
					},
				},
				"required": []string{"device_id", "expose", "from", "to"},
			},
		},
	}
}
