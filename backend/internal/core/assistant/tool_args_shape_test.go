package assistant

import (
	"strings"
	"testing"

	"llm-proxy/internal/core/proxy"
)

func TestArgumentShape(t *testing.T) {
	delegate := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"tasks": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"agent":   map[string]any{"type": "string"},
						"task":    map[string]any{"type": "string"},
						"context": map[string]any{"type": "string"},
					},
					"required": []any{"agent", "task"},
				},
			},
		},
		"required": []any{"tasks"},
	}
	flat := map[string]any{
		"type":       "object",
		"properties": map[string]any{"path": map[string]any{"type": "string"}, "limit": map[string]any{"type": "integer"}},
		"required":   []string{"path"},
	}
	tests := []struct {
		name   string
		params map[string]any
		want   string
	}{
		{"nested list of objects", delegate, `{tasks: [{agent, task, context?}]}`},
		{"required first, optional marked", flat, `{path, limit?}`},
		{"no properties", map[string]any{"type": "object"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := argumentShape(tt.params); got != tt.want {
				t.Errorf("argumentShape() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateToolArgs_ErrorShowsTheExpectedShape(t *testing.T) {
	tools := []proxy.Tool{{Type: "function", Function: proxy.FunctionSchema{Name: "delegate_task", Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{"tasks": map[string]any{"type": "array", "items": map[string]any{
			"type":       "object",
			"properties": map[string]any{"agent": map[string]any{"type": "string"}, "task": map[string]any{"type": "string"}},
			"required":   []any{"agent", "task"},
		}}},
		"required": []any{"tasks"},
	}}}}
	tc := proxy.ToolCall{Function: proxy.FunctionCall{Name: "delegate_task", Arguments: `{"agent":"web-researcher","task":"x"}`}}
	err := validateToolArgs(tc, tools)
	if err == nil {
		t.Fatal("expected an error for a call without tasks")
	}
	for _, want := range []string{"missing required parameter 'tasks'", `{tasks: [{agent, task}]}`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
}
