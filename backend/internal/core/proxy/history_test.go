package proxy

import (
	"testing"
)

func TestNormalizeHistory_Empty(t *testing.T) {
	got := NormalizeHistory(nil, true)
	if got != nil {
		t.Fatal("expected nil for empty input")
	}
}

func TestNormalizeHistory_NativeTools_PreservesToolCalls(t *testing.T) {
	msg := Message{
		Role:    AssistantRole,
		Content: "",
		ToolCalls: []ToolCall{
			{ID: "call_1", Type: "function", Function: FunctionCall{Name: "list_directory", Arguments: `{"path":"."}`}},
		},
	}
	got := NormalizeHistory([]Message{msg}, true)
	if len(got) != 1 {
		t.Fatalf("expected 1 message, got %d", len(got))
	}
	if len(got[0].ToolCalls) != 1 {
		t.Fatal("native mode should preserve tool calls")
	}
	if got[0].ToolCalls[0].Function.Name != "list_directory" {
		t.Fatalf("expected list_directory, got %s", got[0].ToolCalls[0].Function.Name)
	}
}

func TestNormalizeHistory_XMLMode_StripsToolCalls(t *testing.T) {
	msg := Message{
		Role:    AssistantRole,
		Content: "Here are the files:",
		ToolCalls: []ToolCall{
			{ID: "call_1", Type: "function", Function: FunctionCall{Name: "list_directory", Arguments: `{"path":"."}`}},
		},
	}
	got := NormalizeHistory([]Message{msg}, false)
	if len(got) != 1 {
		t.Fatalf("expected 1 message, got %d", len(got))
	}
	if len(got[0].ToolCalls) != 0 {
		t.Fatal("XML mode should strip tool calls")
	}
	if got[0].Content != "Here are the files:" {
		t.Fatalf("content should be preserved, got %q", got[0].Content)
	}
}

func TestNormalizeHistory_XMLMode_EmptyContentWithToolCall(t *testing.T) {
	msg := Message{
		Role:    AssistantRole,
		Content: "",
		ToolCalls: []ToolCall{
			{ID: "call_1", Type: "function", Function: FunctionCall{Name: "list_directory", Arguments: `{"path":"."}`}},
		},
	}
	got := NormalizeHistory([]Message{msg}, false)
	if len(got) != 1 {
		t.Fatalf("expected 1 message, got %d", len(got))
	}
	expected := "<tool_call>\n{\"tool\": \"list_directory\", \"args\": {\"path\":\".\"}}\n</tool_call>"
	if got[0].Content != expected {
		t.Fatalf("expected %q, got %q", expected, got[0].Content)
	}
	if len(got[0].ToolCalls) != 0 {
		t.Fatal("XML mode should strip tool calls")
	}
}

func TestNormalizeHistory_XMLMode_MultipleToolCalls(t *testing.T) {
	msg := Message{
		Role:    AssistantRole,
		Content: "",
		ToolCalls: []ToolCall{
			{ID: "call_1", Type: "function", Function: FunctionCall{Name: "list_directory"}},
			{ID: "call_2", Type: "function", Function: FunctionCall{Name: "read_file"}},
		},
	}
	got := NormalizeHistory([]Message{msg}, false)
	if len(got) != 1 {
		t.Fatalf("expected 1 message, got %d", len(got))
	}
	expected := "<tool_call>\n{\"tool\": \"list_directory\", \"args\": {}}\n</tool_call>\n<tool_call>\n{\"tool\": \"read_file\", \"args\": {}}\n</tool_call>"
	if got[0].Content != expected {
		t.Fatalf("expected %q, got %q", expected, got[0].Content)
	}
}

func TestNormalizeHistory_XMLMode_ToolRoleConversion(t *testing.T) {
	msgs := []Message{
		{Role: AssistantRole, Content: "Called tool: list_directory"},
		{Role: ToolRole, Content: "file1.txt\nfile2.txt", ToolCallID: "call_1"},
	}
	got := NormalizeHistory(msgs, false)
	if len(got) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(got))
	}
	if got[1].Role != UserRole {
		t.Fatal("tool role should be converted to user role in XML mode")
	}
	expected := "Tool result [call_1]: file1.txt\nfile2.txt"
	if got[1].Content != expected {
		t.Fatalf("expected %q, got %q", expected, got[1].Content)
	}
}

func TestNormalizeHistory_XMLMode_ToolRoleWithoutID(t *testing.T) {
	msgs := []Message{
		{Role: AssistantRole, Content: "Called tool: scan"},
		{Role: ToolRole, Content: "no output"},
	}
	got := NormalizeHistory(msgs, false)
	if len(got) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(got))
	}
	if got[1].Role != UserRole {
		t.Fatal("tool role should be converted to user role")
	}
	expected := "Observation: no output"
	if got[1].Content != expected {
		t.Fatalf("expected %q, got %q", expected, got[1].Content)
	}
}

func TestNormalizeHistory_XMLMode_EmptyAssistantNoToolCall(t *testing.T) {
	msg := Message{
		Role:    AssistantRole,
		Content: "",
	}
	got := NormalizeHistory([]Message{msg}, false)
	if len(got) != 1 {
		t.Fatalf("expected 1 message, got %d", len(got))
	}
	// Genuinely empty assistant messages are left empty (no fabricated
	// "Thinking..." placeholder) — the UI renders a neutral state instead.
	if got[0].Content != "" {
		t.Fatalf("empty assistant content should remain empty, got %q", got[0].Content)
	}
}

func TestNormalizeHistory_XMLMode_FullNativeRoundTrip(t *testing.T) {
	history := []Message{
		{Role: SystemRole, Content: "You are a helpful assistant."},
		{Role: UserRole, Content: "List files and write a report."},
		{Role: AssistantRole, Content: "",
			ToolCalls: []ToolCall{
				{ID: "tc1", Type: "function", Function: FunctionCall{Name: "list_directory", Arguments: `{"path":"."}`}},
			},
		},
		{Role: ToolRole, Content: "file1.txt\nfile2.txt", ToolCallID: "tc1"},
		{Role: AssistantRole, Content: "",
			ToolCalls: []ToolCall{
				{ID: "tc2", Type: "function", Function: FunctionCall{Name: "write_file", Arguments: `{"path":"report.md","content":"done"}`}},
			},
		},
		{Role: ToolRole, Content: "File written", ToolCallID: "tc2"},
		{Role: AssistantRole, Content: "Task complete."},
	}
	got := NormalizeHistory(history, false)
	if len(got) != 7 {
		t.Fatalf("expected 7 messages, got %d", len(got))
	}
	if got[0].Content != "You are a helpful assistant." {
		t.Fatalf("system prompt should be preserved, got %q", got[0].Content)
	}
	if got[1].Content != "List files and write a report." {
		t.Fatalf("user message should be preserved, got %q", got[1].Content)
	}
	if got[2].Content != "<tool_call>\n{\"tool\": \"list_directory\", \"args\": {\"path\":\".\"}}\n</tool_call>" {
		t.Fatalf("expected XML tool call, got %q", got[2].Content)
	}
	if len(got[2].ToolCalls) != 0 {
		t.Fatal("tool calls should be stripped in XML mode")
	}
	if got[3].Role != UserRole {
		t.Fatal("tool role should be converted to user role")
	}
	if got[4].Content != "<tool_call>\n{\"tool\": \"write_file\", \"args\": {\"path\":\"report.md\",\"content\":\"done\"}}\n</tool_call>" {
		t.Fatalf("expected XML tool call, got %q", got[4].Content)
	}
	if got[5].Role != UserRole {
		t.Fatal("tool role should be converted to user role")
	}
	if got[6].Content != "Task complete." {
		t.Fatalf("final assistant message should be preserved, got %q", got[6].Content)
	}
}

func TestNormalizeHistory_NativeMode_PreservesToolResults(t *testing.T) {
	history := []Message{
		{Role: AssistantRole, Content: "",
			ToolCalls: []ToolCall{
				{ID: "tc1", Type: "function", Function: FunctionCall{Name: "list_directory"}},
			},
		},
		{Role: ToolRole, Content: "file1.txt", ToolCallID: "tc1"},
	}
	got := NormalizeHistory(history, true)
	if len(got) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(got))
	}
	if got[0].Role != AssistantRole {
		t.Fatal("assistant role should be preserved in native mode")
	}
	if len(got[0].ToolCalls) != 1 {
		t.Fatal("tool calls should be preserved in native mode")
	}
	if got[1].Role != ToolRole {
		t.Fatal("tool role should be preserved in native mode")
	}
	if got[1].Content != "file1.txt" {
		t.Fatalf("tool result content should be preserved, got %q", got[1].Content)
	}
}

func TestNormalizeHistory_XMLMode_MultiTurnUserNotMerged(t *testing.T) {
	// Simulates a multi-turn conversation: user asks task → agent completes →
	// user asks a new question. Tool results (converted to UserRole) must NOT
	// be merged with the second user message, so the model sees a clean "tell me a joke".
	history := []Message{
		{Role: SystemRole, Content: "You are a helpful assistant."},
		{Role: UserRole, Content: "List files in the workspace."},
		{Role: AssistantRole, Content: "",
			ToolCalls: []ToolCall{
				{ID: "tc1", Type: "function", Function: FunctionCall{Name: "list_directory", Arguments: `{"path":"."}`}},
			},
		},
		{Role: ToolRole, Content: "file1.txt", ToolCallID: "tc1"},
		{Role: AssistantRole, Content: "Here is the listing."},
		{Role: UserRole, Content: "tell me a joke"},
	}
	got := NormalizeHistory(history, false)
	if len(got) != 6 {
		t.Fatalf("expected 6 messages (no merging of user messages), got %d", len(got))
	}
	// First user message
	if got[1].Role != UserRole || got[1].Content != "List files in the workspace." {
		t.Fatalf("first user message should be preserved, got %q", got[1].Content)
	}
	// Tool result converted to user role (index 3)
	if got[3].Role != UserRole {
		t.Fatal("tool role should be converted to user role")
	}
	if !startsWith(got[3].Content, "Tool result [tc1]") {
		t.Fatalf("tool result prefix expected, got %q", got[3].Content)
	}
	// Second user message MUST be a separate message, not merged with the tool result
	if got[5].Role != UserRole || got[5].Content != "tell me a joke" {
		t.Fatalf("second user message should be separate and clean, got role=%v content=%q", got[5].Role, got[5].Content)
	}
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func TestNormalizeHistory_XMLMode_ContentAndToolCallPreservesContent(t *testing.T) {
	msg := Message{
		Role:    AssistantRole,
		Content: "Let me check the directory.",
		ToolCalls: []ToolCall{
			{ID: "tc1", Type: "function", Function: FunctionCall{Name: "list_directory"}},
		},
	}
	got := NormalizeHistory([]Message{msg}, false)
	if len(got) != 1 {
		t.Fatalf("expected 1 message, got %d", len(got))
	}
	if got[0].Content != "Let me check the directory." {
		t.Fatalf("non-empty content should be preserved, got %q", got[0].Content)
	}
	if len(got[0].ToolCalls) != 0 {
		t.Fatal("tool calls should still be stripped")
	}
}
