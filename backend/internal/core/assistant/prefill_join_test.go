package assistant

import (
	"testing"

	"llm-proxy/internal/core/assistant/prompts"
)

// The prefill opens the model's reply with the start of a tool call. Models that continue it are
// joined to it; models that restate the whole call (Nemotron) must not get it doubled.
func TestJoinPrefill(t *testing.T) {
	prefill := prompts.AutomationPrefline
	tests := []struct {
		name, content, want string
	}{
		{"continues the prefill", `list_directory","args":{"path":"."}}` + "\n</tool_call>", prefill + `list_directory","args":{"path":"."}}` + "\n</tool_call>"},
		{"restates the whole call", "<tool_call>\n{\"tool\":\"list_directory\",\"args\":{}}\n</tool_call>", "<tool_call>\n{\"tool\":\"list_directory\",\"args\":{}}\n</tool_call>"},
		{"restates the whole call after whitespace", "\n  <tool_call>\n{\"tool\":\"a\",\"args\":{}}", "<tool_call>\n{\"tool\":\"a\",\"args\":{}}"},
		{"restates the JSON without the tag", `{"tool":"a","args":{}}`, "<tool_call>\n" + `{"tool":"a","args":{}}`},
		{"no content yet", "", prefill},
	}
	for _, tt := range tests {
		if got := joinPrefill(prefill, tt.content); got != tt.want {
			t.Errorf("%s: joinPrefill = %q, want %q", tt.name, got, tt.want)
		}
	}
}
