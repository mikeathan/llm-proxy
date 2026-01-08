package assistant

import "llm-proxy/internal/proxy"

func MetricsToolSchema() proxy.Tool {
	return proxy.Tool{
		Type: "function",
		Function: proxy.FunctionSchema{
			Name:        "query_metrics",
			Description: "Query historical metrics for a specific device",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"target_name": map[string]any{
						"type":        "string",
						"description": "Natural language device name, e.g. 'garden', 'attic', 'living room'",
					},
					"expose": map[string]any{
						"type":        "string",
						"description": "Metric key ONLY from device.exposes.name. NEVER include units, symbols, or descriptions. Example: 'temperature', not 'temperature (°C)'",
					},
					"time": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"from":     map[string]any{"type": "integer"},
							"to":       map[string]any{"type": "integer"},
							"lookback": map[string]any{"type": "string"},
						},
					},
					"aggregation": map[string]any{
						"type":        "string",
						"description": "Aggregation: last, min, max, avg, count",
					},
					"resolution": map[string]any{"type": "string"},
				},
				"required": []string{"target_name", "expose"},
			},
		},
	}
}
