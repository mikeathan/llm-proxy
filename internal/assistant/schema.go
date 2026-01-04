package assistant

import "llm-proxy/internal/proxy"

func MetricsToolSchema() proxy.Tool {
	return proxy.Tool{
		Type: "function",
		Function: proxy.FunctionSchema{
			Name:        "query_metrics",
			Description: "Query historical device metrics. Supports time ranges or aggregate queries (last, min, max, avg, count). Returns timestamps and values.",
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
					"time": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"from": map[string]any{
								"type":        "integer",
								"description": "Start time as unix milliseconds",
							},
							"to": map[string]any{
								"type":        "integer",
								"description": "End time as unix milliseconds",
							},
							"lookback": map[string]any{
								"type":        "string",
								"description": "Relative time range (e.g. 24h, 7d)",
							},
						},
					},
					"aggregation": map[string]any{
						"type": "string",
						"enum": []string{"last", "min", "max", "avg", "count"},
					},
				},
				"required": []string{"device_id", "expose"},
			},
		},
	}
}
