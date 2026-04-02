package proxy_test

import (
	"testing"

	"llm-proxy/internal/core/proxy"
)

func TestParseContentToolCalls_StandardToolCall(t *testing.T) {
	content := `<function-name>query_device</function-name>
<args-json-object>
{
  "target_name": "Living Room Light",
  "metrics": ["state"],
  "time_scope": "last_hour",
  "aggregation": "last"
}
</args-json-object>`

	calls, ok := proxy.ParseContentToolCalls(content)
	if !ok {
		t.Fatal("expected ok=true, got false")
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
	// Arguments should be valid, non-empty JSON
	if tc.Function.Arguments == "" || tc.Function.Arguments == "{}" {
		t.Errorf("expected non-empty arguments, got %q", tc.Function.Arguments)
	}
}

func TestParseContentToolCalls_NoToolCall(t *testing.T) {
	content := "The temperature in the living room is 22°C."
	calls, ok := proxy.ParseContentToolCalls(content)
	if ok {
		t.Error("expected ok=false for plain text, got true")
	}
	if len(calls) != 0 {
		t.Errorf("expected 0 calls, got %d", len(calls))
	}
}

func TestParseContentToolCalls_EmptyContent(t *testing.T) {
	calls, ok := proxy.ParseContentToolCalls("")
	if ok {
		t.Error("expected ok=false for empty content")
	}
	if len(calls) != 0 {
		t.Errorf("expected 0 calls, got %d", len(calls))
	}
}

func TestParseContentToolCalls_MultipleToolCalls(t *testing.T) {
	content := `<function-name>get_temperature</function-name>
<args-json-object>{"room": "attic"}</args-json-object>
<function-name>get_temperature</function-name>
<args-json-object>{"room": "basement"}</args-json-object>`

	calls, ok := proxy.ParseContentToolCalls(content)
	if !ok {
		t.Fatal("expected ok=true")
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
	// IDs must be unique
	if calls[0].ID == calls[1].ID {
		t.Errorf("expected unique IDs, both are %q", calls[0].ID)
	}
}

func TestParseContentToolCalls_MissingArgs(t *testing.T) {
	// Function name present but no args block — should still parse with default
	content := `<function-name>list_devices</function-name>`

	calls, ok := proxy.ParseContentToolCalls(content)
	if !ok {
		t.Fatal("expected ok=true when function-name is present")
	}
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Function.Name != "list_devices" {
		t.Errorf("expected 'list_devices', got %q", calls[0].Function.Name)
	}
	// Should fall back to empty args object
	if calls[0].Function.Arguments != "{}" {
		t.Errorf("expected '{}' as fallback args, got %q", calls[0].Function.Arguments)
	}
}

func TestParseContentToolCalls_FunctionNameTrimmed(t *testing.T) {
	content := "<function-name>  declare_intent  </function-name>\n<args-json-object>{}</args-json-object>"
	calls, ok := proxy.ParseContentToolCalls(content)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if calls[0].Function.Name != "declare_intent" {
		t.Errorf("expected trimmed name 'declare_intent', got %q", calls[0].Function.Name)
	}
}

// --- Tests for the <tools> JSON format ---

func TestParseContentToolCalls_ToolsTagFormat(t *testing.T) {
	content := "<tools>\n{\"name\": \"query_device\", \"arguments\": {\"target_name\": \"Living Room Light\", \"metrics\": [\"state\"], \"time_scope\": \"today\", \"aggregation\": \"last\"}}\n</tools>"

	calls, ok := proxy.ParseContentToolCalls(content)
	if !ok {
		t.Fatal("expected ok=true for <tools> format")
	}
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	tc := calls[0]
	if tc.Function.Name != "query_device" {
		t.Errorf("expected name 'query_device', got %q", tc.Function.Name)
	}
	if tc.Type != "function" {
		t.Errorf("expected type 'function', got %q", tc.Type)
	}
	if tc.Function.Arguments == "" || tc.Function.Arguments == "{}" {
		t.Errorf("expected non-empty arguments, got %q", tc.Function.Arguments)
	}
}

func TestParseContentToolCalls_ToolsTagMultiple(t *testing.T) {
	content := `<tools>
{"name": "get_state", "arguments": {"room": "kitchen"}}
</tools>
<tools>
{"name": "get_state", "arguments": {"room": "hallway"}}
</tools>`

	calls, ok := proxy.ParseContentToolCalls(content)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(calls))
	}
	if calls[0].ID == calls[1].ID {
		t.Error("expected unique IDs for each call")
	}
}

func TestParseContentToolCalls_ToolsTagMalformedJSON(t *testing.T) {
	// Malformed JSON inside <tools> should be silently skipped
	content := "<tools>\nnot-json\n</tools>"
	calls, ok := proxy.ParseContentToolCalls(content)
	if ok {
		t.Error("expected ok=false for malformed JSON in <tools>")
	}
	if len(calls) != 0 {
		t.Errorf("expected 0 calls, got %d", len(calls))
	}
}

func TestParseContentToolCalls_ToolsTagMissingName(t *testing.T) {
	// Valid JSON but missing "name" key - should be rejected
	content := `<tools>{"arguments": {"foo": "bar"}}</tools>`
	calls, ok := proxy.ParseContentToolCalls(content)
	if ok {
		t.Error("expected ok=false when name is missing")
	}
	if len(calls) != 0 {
		t.Errorf("expected 0 calls, got %d", len(calls))
	}
}

