package proxy_test

import (
	"encoding/json"
	"llm-proxy/internal/core/proxy"
	"strings"
	"testing"
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
	// Some models truncate the closing tag (omit the trailing '>').
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

func TestParseNativeToolCalls_QwenStyle(t *testing.T) {
	content := `<function=execute_terminal_command><parameter=command>npm init -y</parameter></function>`
	cleaned, calls, err := proxy.ParseNativeToolCalls(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Function.Name != "execute_terminal_command" {
		t.Errorf("expected 'execute_terminal_command', got %q", calls[0].Function.Name)
	}
	if cleaned == content {
		t.Error("content should have been cleaned of native tags")
	}
}

func TestParseNativeToolCalls_AttributeStyle(t *testing.T) {
	content := `<tool name="read_file"><parameter name="path">test.txt</parameter></tool>`
	_, calls, err := proxy.ParseNativeToolCalls(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Function.Name != "read_file" {
		t.Errorf("expected 'read_file', got %q", calls[0].Function.Name)
	}
	args := calls[0].Function.Arguments
	if !strings.Contains(args, `"path"`) || !strings.Contains(args, `test.txt`) {
		t.Errorf("expected args to contain path=test.txt, got %s", args)
	}
}

func TestParseNativeToolCalls_NoMatches(t *testing.T) {
	content := "just some plain text thinking about tools..."
	_, calls, err := proxy.ParseNativeToolCalls(content)
	if err == nil {
		t.Fatal("expected error for no matches")
	}
	if len(calls) != 0 {
		t.Errorf("expected 0 calls, got %d", len(calls))
	}
}

func TestParseNativeToolCalls_MultipleBlocks(t *testing.T) {
	content := `<function=read_file><parameter=path>a.txt</parameter></function>` +
		`<function=write_file><parameter=path>b.txt</parameter><parameter=content>hello</parameter></function>`
	_, calls, err := proxy.ParseNativeToolCalls(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}
	if calls[0].Function.Name != "read_file" {
		t.Errorf("expected 'read_file', got %q", calls[0].Function.Name)
	}
	if calls[1].Function.Name != "write_file" {
		t.Errorf("expected 'write_file', got %q", calls[1].Function.Name)
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

func TestParseContentToolCalls_KeylessDialect_KeyValueRest(t *testing.T) {
	// Observed dialect: {"list_directory", "path": "."} (bare string first,
	// args as bare key:value members).
	content := `<tool_call>{"list_directory", "path": "."}</tool_call>`
	cleaned, calls, parseErr := proxy.ParseContentToolCalls(content)
	if parseErr != nil {
		t.Fatalf("expected keyless dialect to be salvaged, got error: %v", parseErr)
	}
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Function.Name != "list_directory" {
		t.Errorf("expected tool list_directory, got %q", calls[0].Function.Name)
	}
	if calls[0].Function.Arguments != `{"path": "."}` {
		t.Errorf("expected args {\"path\": \".\"}, got %s", calls[0].Function.Arguments)
	}
	if cleaned != "" {
		t.Errorf("expected content cleaned of the tool call, got %q", cleaned)
	}
}

func TestParseContentToolCalls_KeylessDialect_ObjectRest(t *testing.T) {
	// Variant where the remainder is already an object:
	// {"list_directory", {"path": "."}}
	content := `<tool_call>{"list_directory", {"path": "."}}</tool_call>`
	_, calls, parseErr := proxy.ParseContentToolCalls(content)
	if parseErr != nil {
		t.Fatalf("expected keyless dialect to be salvaged, got error: %v", parseErr)
	}
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Function.Name != "list_directory" {
		t.Errorf("expected tool list_directory, got %q", calls[0].Function.Name)
	}
	var got map[string]string
	if err := json.Unmarshal([]byte(calls[0].Function.Arguments), &got); err != nil {
		t.Fatalf("args not valid JSON: %v (%s)", err, calls[0].Function.Arguments)
	}
	if got["path"] != "." {
		t.Errorf("expected args path=., got %v", got)
	}
}

func TestParseContentToolCalls_KeylessDialect_MultipleArgs(t *testing.T) {
	content := `<tool_call>{"write_file", "path": "a.txt", "content": "hi"}</tool_call>`
	_, calls, parseErr := proxy.ParseContentToolCalls(content)
	if parseErr != nil {
		t.Fatalf("expected salvage, got error: %v", parseErr)
	}
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Function.Name != "write_file" {
		t.Errorf("expected tool write_file, got %q", calls[0].Function.Name)
	}
	if calls[0].Function.Arguments != `{"path": "a.txt", "content": "hi"}` {
		t.Errorf("unexpected args: %s", calls[0].Function.Arguments)
	}
}

func TestParseContentToolCalls_KeylessDialect_GarbageRestRejected(t *testing.T) {
	// The remainder must itself be valid JSON; anything else still fails
	// strict parsing and must NOT be fabricated into a call.
	content := `<tool_call>{"list_directory", ,,,}</tool_call>`
	_, calls, parseErr := proxy.ParseContentToolCalls(content)
	if parseErr == nil {
		t.Fatal("expected parse error when the keyless remainder is not JSON")
	}
	if len(calls) != 0 {
		t.Errorf("expected 0 calls, got %d", len(calls))
	}
}

func TestParseContentToolCalls_KeylessDialect_NotTriggeredByValidJSON(t *testing.T) {
	// {"tool": "...", ...} starts with a string followed by a colon, so the
	// keyless regex must not match — valid calls go through the strict path.
	content := `<tool_call>{"tool": "list_directory", "args": {"path": "."}}</tool_call>`
	_, calls, parseErr := proxy.ParseContentToolCalls(content)
	if parseErr != nil {
		t.Fatalf("unexpected error: %v", parseErr)
	}
	if len(calls) != 1 || calls[0].Function.Name != "list_directory" {
		t.Fatalf("expected list_directory call, got %+v", calls)
	}
}

func TestParseContentToolCalls_DoubledOpeningTag(t *testing.T) {
	// Ornith-1.5-35B emits <tool_call><tool_call>{...}: the XML extraction
	// starts at the first tag, so the body begins with a stray tag. The inner
	// JSON is valid — stripping the leading tags must let it parse.
	content := "<tool_call>\n<tool_call>\n{\n  \"tool\": \"list_directory\",\n  \"args\": {\n    \"path\": \".\"\n  }\n}\n</tool_call>"
	_, calls, parseErr := proxy.ParseContentToolCalls(content)
	if parseErr != nil {
		t.Fatalf("expected doubled opening tag to be tolerated, got error: %v", parseErr)
	}
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Function.Name != "list_directory" {
		t.Errorf("expected tool list_directory, got %q", calls[0].Function.Name)
	}
	var got map[string]string
	if err := json.Unmarshal([]byte(calls[0].Function.Arguments), &got); err != nil {
		t.Fatalf("args not valid JSON: %v (%s)", err, calls[0].Function.Arguments)
	}
	if got["path"] != "." {
		t.Errorf("expected args path=., got %v", got)
	}
}

// TestParseNativeToolCalls_InvokeAttributes verifies the <invoke name="…">
// attributes dialect — emitted by Ornith-1.5 (2026-08-31 smoke-test run) — is
// parsed with its <parameter name="…"> values, even when wrapped in stray
// <tool_call> markers.
func TestParseNativeToolCalls_InvokeAttributes(t *testing.T) {
	content := `<tool_call>
<invoke name="execute_terminal_command">
<parameter name="command">cd dev-test && npx tsc index.ts</parameter>
</invoke>
</tool_call>
<tool_call>
<tool_call>
<invoke name="read_file">
<parameter name="path">dev-test/index.ts</parameter>
</invoke>
</tool_call>`
	_, calls, parseErr := proxy.ParseNativeToolCalls(content)
	if parseErr != nil || len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d (err %v)", len(calls), parseErr)
	}
	if calls[0].Function.Name != "execute_terminal_command" {
		t.Errorf("call 0 name = %q", calls[0].Function.Name)
	}
	if !strings.Contains(calls[0].Function.Arguments, "npx tsc index.ts") {
		t.Errorf("call 0 args = %q", calls[0].Function.Arguments)
	}
	if calls[1].Function.Name != "read_file" {
		t.Errorf("call 1 name = %q", calls[1].Function.Name)
	}
	if !strings.Contains(calls[1].Function.Arguments, "dev-test/index.ts") {
		t.Errorf("call 1 args = %q", calls[1].Function.Arguments)
	}
}
