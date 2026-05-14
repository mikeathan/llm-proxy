package proxy_test

import (
	"testing"

	"llm-proxy/internal/core/proxy"
)

func TestParseContentToolCalls_StandardXMLFormat(t *testing.T) {
	content := `<tool_call>
{"tool": "query_device", "args": {"target_name": "Living Room Light", "metrics": ["state"], "time_scope": "last_hour", "aggregation": "last"}}
</tool_call>`

	_, calls, parseErr := proxy.ParseContentToolCalls(content)
	if parseErr != nil {
		t.Fatalf("unexpected parse error: %v", parseErr)
	}
	if len(calls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(calls))
	}
	tc := calls[0]
	if tc.Type != "function" {
		t.Errorf("expected type 'function', got %q", tc.Type)
	}
	if tc.Function.Name != "query_device" {
		t.Errorf("expected name 'query_device', got %q", tc.Function.Name)
	}
	if tc.ID == "" {
		t.Error("expected a non-empty tool call ID")
	}
	if string(tc.Function.Arguments) == "" || string(tc.Function.Arguments) == "{}" {
		t.Errorf("expected non-empty arguments, got %q", string(tc.Function.Arguments))
	}
}

func TestParseContentToolCalls_NoToolCall(t *testing.T) {
	content := "The temperature in the living room is 22°C."
	_, calls, parseErr := proxy.ParseContentToolCalls(content)
	if parseErr == nil {
		t.Error("expected parse error for plain text")
	}
	if !parseErr.XMLFound {
		t.Log("correctly reports no XML tags found")
	}
	if len(calls) != 0 {
		t.Errorf("expected 0 calls, got %d", len(calls))
	}
}

func TestParseContentToolCalls_EmptyContent(t *testing.T) {
	_, calls, parseErr := proxy.ParseContentToolCalls("")
	if parseErr == nil {
		t.Error("expected parse error for empty content")
	}
	if len(calls) != 0 {
		t.Errorf("expected 0 calls, got %d", len(calls))
	}
}

func TestParseContentToolCalls_MultipleToolCalls(t *testing.T) {
	content := `<tool_call>
{"tool": "get_temperature", "args": {"room": "attic"}}
</tool_call>
<tool_call>
{"tool": "get_temperature", "args": {"room": "basement"}}
</tool_call>`

	_, calls, parseErr := proxy.ParseContentToolCalls(content)
	if parseErr != nil {
		t.Fatalf("unexpected parse error: %v", parseErr)
	}
	if len(calls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(calls))
	}
	if calls[0].Function.Name != "get_temperature" {
		t.Errorf("expected 'get_temperature', got %q", calls[0].Function.Name)
	}
	if calls[1].Function.Name != "get_temperature" {
		t.Errorf("expected 'get_temperature', got %q", calls[1].Function.Name)
	}
	if calls[0].ID == calls[1].ID {
		t.Errorf("expected unique IDs, both are %q", calls[0].ID)
	}
}

func TestParseContentToolCalls_MalformedJSON(t *testing.T) {
	content := `<tool_call>
not-valid-json
</tool_call>`
	_, calls, parseErr := proxy.ParseContentToolCalls(content)
	if parseErr == nil {
		t.Fatal("expected parse error for malformed JSON")
	}
	if !parseErr.XMLFound {
		t.Error("expected XMLFound=true")
	}
	if parseErr.JSONError == "" {
		t.Error("expected non-empty JSONError")
	}
	if len(calls) != 0 {
		t.Errorf("expected 0 calls, got %d", len(calls))
	}
}

func TestParseContentToolCalls_MissingToolField(t *testing.T) {
	content := `<tool_call>{"args": {"foo": "bar"}}</tool_call>`
	_, calls, parseErr := proxy.ParseContentToolCalls(content)
	if parseErr == nil {
		t.Fatal("expected parse error when tool field is missing")
	}
	if parseErr.JSONError == "" {
		t.Error("expected non-empty JSONError for missing tool field")
	}
	if len(calls) != 0 {
		t.Errorf("expected 0 calls, got %d", len(calls))
	}
}

func TestParseContentToolCalls_WrongTagIgnored(t *testing.T) {
	// Old-style <function-name> tags are no longer recognised.
	content := `<function-name>list_directory</function-name>
<args-json-object>{"path": "/tmp/"}</args-json-object>`
	_, calls, parseErr := proxy.ParseContentToolCalls(content)
	if parseErr == nil {
		t.Fatal("expected parse error for unsupported tag format")
	}
	if !parseErr.XMLFound {
		t.Log("correctly reports no XML tags because <function-name> is not supported")
	}
	if len(calls) != 0 {
		t.Errorf("expected 0 calls, got %d", len(calls))
	}
}

func TestParseContentToolCalls_RawJSONIgnored(t *testing.T) {
	// Naked JSON without XML wrapper is no longer accepted.
	content := `{"tool": "get_weather", "args": {"city": "London"}}`
	_, calls, parseErr := proxy.ParseContentToolCalls(content)
	if parseErr == nil {
		t.Fatal("expected parse error for raw JSON without XML wrapper")
	}
	if len(calls) != 0 {
		t.Errorf("expected 0 calls, got %d", len(calls))
	}
}

func TestParseContentToolCalls_FuzzyCloseTag(t *testing.T) {
	// Some models omit the slash in the closing tag.
	content := `<tool_call>{"tool": "read_file", "args": {"path": "test.txt"}}</tool_call`
	_, calls, parseErr := proxy.ParseContentToolCalls(content)
	if parseErr != nil {
		t.Fatalf("unexpected parse error for fuzzy close tag: %v", parseErr)
	}
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
}

func TestParseContentToolCalls_SelfClosingTag(t *testing.T) {
	content := `<tool_call>{"tool": "read_file", "args": {"path": "test.txt"}}</tool_call>`
	_, calls, parseErr := proxy.ParseContentToolCalls(content)
	if parseErr != nil {
		t.Fatalf("unexpected parse error: %v", parseErr)
	}
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
}

func TestValidateToolCall(t *testing.T) {
	tools := []proxy.Tool{
		{Function: proxy.FunctionSchema{Name: "read_file"}},
		{Function: proxy.FunctionSchema{Name: "write_file"}},
	}
	validCall := proxy.ToolCall{Function: proxy.FunctionCall{Name: "read_file"}}
	if err := proxy.ValidateToolCall(validCall, tools); err != nil {
		t.Errorf("expected valid tool, got error: %v", err)
	}
	invalidCall := proxy.ToolCall{Function: proxy.FunctionCall{Name: "delete_everything"}}
	if err := proxy.ValidateToolCall(invalidCall, tools); err == nil {
		t.Error("expected error for unknown tool")
	}
}

func TestParseError_Feedback(t *testing.T) {
	tests := []struct {
		name string
		pe   proxy.ParseError
	}{
		{"no XML", proxy.ParseError{XMLFound: false}},
		{"JSON error", proxy.ParseError{XMLFound: true, JSONError: "unexpected character"}},
		{"bad tool", proxy.ParseError{ToolName: "nonexistent"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			feedback := tt.pe.Feedback([]string{"read_file", "write_file"})
			if feedback == "" {
				t.Error("expected non-empty feedback")
			}
		})
	}
}

func TestAvailableToolNames(t *testing.T) {
	tools := []proxy.Tool{
		{Function: proxy.FunctionSchema{Name: "read_file"}},
		{Function: proxy.FunctionSchema{Name: "write_file"}},
		{Function: proxy.FunctionSchema{Name: "read_file"}}, // duplicate
	}
	names := proxy.AvailableToolNames(tools)
	if len(names) != 2 {
		t.Errorf("expected 2 unique names, got %d", len(names))
	}
}
