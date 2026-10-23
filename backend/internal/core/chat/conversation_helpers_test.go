package chat

import (
	"strings"
	"testing"
	"time"

	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/assistant/profile"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
)

// The system message is rebuilt every turn from the current profile and notes; the same inputs give the same text.
func TestConversationSystemMessage(t *testing.T) {
	tmp := t.TempDir()
	mgr := persistence.NewWorkspaceManager(storage.NewPathResolver(tmp, tmp, tmp))
	if err := mgr.WriteTaskFile("ws-system-message", models.RulesFilename, "Reports go in reports/."); err != nil {
		t.Fatal(err)
	}
	session := &models.AssistantSession{ID: "conv_1", WorkspaceID: "ws-system-message", ContextVersion: "v1"} // an older session stores no zone: the caller passes the effective one
	general := profile.Profile{ID: "general", Instructions: "GENERAL BEHAVIOUR"}
	runner := profile.Profile{ID: "task-runner", Instructions: "RUNNER BEHAVIOUR"}

	msg := conversationSystemMessage(mgr, session, "UTC", general, true)
	for _, want := range []string{"GENERAL BEHAVIOUR", "RULES:", "Reports go in reports/.", "Conversation ID: conv_1", "Timezone: UTC"} {
		if !strings.Contains(msg, want) {
			t.Errorf("system message lacks %q:\n%s", want, msg)
		}
	}
	if again := conversationSystemMessage(mgr, session, "UTC", general, true); again != msg {
		t.Error("the same inputs must give the same system message (the model server's cache depends on it)")
	}
	if switched := conversationSystemMessage(mgr, session, "UTC", runner, true); !strings.Contains(switched, "RUNNER BEHAVIOUR") || strings.Contains(switched, "GENERAL BEHAVIOUR") {
		t.Errorf("a profile switch changes the behaviour block:\n%s", switched)
	}
}

func TestWithSystemMessage(t *testing.T) {
	user := proxy.Message{Role: proxy.UserRole, Content: "hi"}
	replaced := withSystemMessage([]proxy.Message{{Role: proxy.SystemRole, Content: "old"}, user}, "new")
	if len(replaced) != 2 || replaced[0].Content != "new" || replaced[1].Content != user.Content {
		t.Errorf("an existing system head is replaced: %+v", replaced)
	}
	added := withSystemMessage([]proxy.Message{user}, "new")
	if len(added) != 2 || added[0].Role != proxy.SystemRole || added[0].Content != "new" {
		t.Errorf("a missing system head is added: %+v", added)
	}
	original := []proxy.Message{{Role: proxy.SystemRole, Content: "old"}, user}
	withSystemMessage(original, "new")
	if original[0].Content != "old" {
		t.Error("the input history must not be modified")
	}
}

func TestBuildPartialHistory(t *testing.T) {
	base := []proxy.Message{
		{Role: proxy.UserRole, Content: "hello"},
	}

	t.Run("empty events returns base unchanged", func(t *testing.T) {
		h := buildPartialHistory(base, nil)
		if len(h) != 1 || h[0].Content != "hello" {
			t.Fatalf("expected base unchanged, got %v", h)
		}
	})

	t.Run("single tool cycle", func(t *testing.T) {
		events := []assistant.AgentEvent{
			{Type: assistant.EventToolCall, Payload: proxy.ToolCall{
				ID: "tc1", Function: proxy.FunctionCall{Name: "list_files", Arguments: "{}"},
			}},
			{Type: assistant.EventToolResult, Payload: map[string]any{
				"id": "tc1", "name": "list_files", "result": "file1.txt",
			}},
		}
		h := buildPartialHistory(base, events)
		if len(h) != 3 {
			t.Fatalf("expected 3 messages, got %d", len(h))
		}
		if h[1].Role != proxy.AssistantRole || len(h[1].ToolCalls) != 1 {
			t.Error("expected assistant with tool call")
		}
		if h[2].Role != proxy.ToolRole || h[2].Content != "file1.txt" {
			t.Errorf("expected tool result, got role=%s content=%q", h[2].Role, h[2].Content)
		}
	})

	t.Run("multiple tool cycles and text-only EventMessage appended", func(t *testing.T) {
		events := []assistant.AgentEvent{
			{Type: assistant.EventToolCall, Payload: proxy.ToolCall{ID: "tc1", Function: proxy.FunctionCall{Name: "a", Arguments: "{}"}}},
			{Type: assistant.EventToolResult, Payload: map[string]any{"id": "tc1", "result": "r1"}},
			{Type: assistant.EventToolCall, Payload: proxy.ToolCall{ID: "tc2", Function: proxy.FunctionCall{Name: "b", Arguments: "{}"}}},
			{Type: assistant.EventToolResult, Payload: map[string]any{"id": "tc2", "result": "r2"}},
			{Type: assistant.EventMessage, Payload: proxy.Message{Role: proxy.AssistantRole, Content: "done"}},
		}
		h := buildPartialHistory(base, events)
		if len(h) != 6 {
			t.Fatalf("expected 6 messages, got %d", len(h))
		}
		if h[5].Content != "done" {
			t.Errorf("expected final message, got %q", h[5].Content)
		}
	})

	t.Run("streaming content becomes assistant Content", func(t *testing.T) {
		events := []assistant.AgentEvent{
			{Type: assistant.EventToolStream, Payload: "Now let me explore the structure..."},
			{Type: assistant.EventToolCall, Payload: proxy.ToolCall{ID: "tc1", Function: proxy.FunctionCall{Name: "list_files", Arguments: "{}"}}},
			{Type: assistant.EventToolResult, Payload: map[string]any{"id": "tc1", "result": "file1.txt"}},
		}
		h := buildPartialHistory(base, events)
		if len(h) != 3 {
			t.Fatalf("expected 3 messages, got %d", len(h))
		}
		if h[1].Content != "Now let me explore the structure..." {
			t.Errorf("expected streaming content in assistant message, got %q", h[1].Content)
		}
	})

	t.Run("parallel tool calls grouped into one message", func(t *testing.T) {
		events := []assistant.AgentEvent{
			{Type: assistant.EventToolStream, Payload: "I need to run two tools in parallel."},
			{Type: assistant.EventToolCall, Payload: proxy.ToolCall{ID: "tc1", Function: proxy.FunctionCall{Name: "read_file", Arguments: "{}"}}},
			{Type: assistant.EventToolCall, Payload: proxy.ToolCall{ID: "tc2", Function: proxy.FunctionCall{Name: "list", Arguments: "{}"}}},
			{Type: assistant.EventToolResult, Payload: map[string]any{"id": "tc1", "result": "content"}},
			{Type: assistant.EventToolResult, Payload: map[string]any{"id": "tc2", "result": "list"}},
		}
		h := buildPartialHistory(base, events)
		if len(h) != 4 {
			t.Fatalf("expected 4 messages, got %d", len(h))
		}
		if len(h[1].ToolCalls) != 2 {
			t.Errorf("expected 2 tool calls grouped, got %d", len(h[1].ToolCalls))
		}
		if h[1].Content != "I need to run two tools in parallel." {
			t.Errorf("expected streaming content, got %q", h[1].Content)
		}
		if h[1].ToolCalls[0].Function.Name != "read_file" {
			t.Error("expected read_file as first tool call")
		}
		if h[1].ToolCalls[1].Function.Name != "list" {
			t.Error("expected list as second tool call")
		}
	})

	t.Run("EventMessage with tool calls is skipped", func(t *testing.T) {
		events := []assistant.AgentEvent{
			{Type: assistant.EventToolStream, Payload: "Reasoning text."},
			{Type: assistant.EventToolCall, Payload: proxy.ToolCall{ID: "tc1", Function: proxy.FunctionCall{Name: "tool_a", Arguments: "{}"}}},
			{Type: assistant.EventToolResult, Payload: map[string]any{"id": "tc1", "result": "res"}},
			// This EventMessage has ToolCalls — it duplicates the assistant
			// message already built by flushPendingGroup from EventToolCall.
			{Type: assistant.EventMessage, Payload: proxy.Message{
				Role: proxy.AssistantRole, Content: "Reasoning text.",
				ToolCalls: []proxy.ToolCall{{ID: "tc1", Function: proxy.FunctionCall{Name: "tool_a", Arguments: "{}"}}},
			}},
		}
		h := buildPartialHistory(base, events)
		// base + 1 assistant + 1 tool = 3 — duplicate assistant skipped
		if len(h) != 3 {
			t.Fatalf("expected 3 messages (duplicate skipped), got %d", len(h))
		}
	})

	t.Run("EventMessage without tool calls is appended", func(t *testing.T) {
		events := []assistant.AgentEvent{
			{Type: assistant.EventToolStream, Payload: "Streaming text."},
			{Type: assistant.EventToolCall, Payload: proxy.ToolCall{ID: "tc1", Function: proxy.FunctionCall{Name: "a", Arguments: "{}"}}},
			{Type: assistant.EventToolResult, Payload: map[string]any{"id": "tc1", "result": "r"}},
			{Type: assistant.EventMessage, Payload: proxy.Message{Role: proxy.AssistantRole, Content: "Final answer."}},
		}
		h := buildPartialHistory(base, events)
		// base + 1 assistant + 1 tool + 1 message = 4
		if len(h) != 4 {
			t.Fatalf("expected 4 messages, got %d", len(h))
		}
		if h[3].Content != "Final answer." {
			t.Errorf("expected text-only EventMessage appended, got %q", h[3].Content)
		}
	})

	t.Run("ReasoningContent preserved separately from Content", func(t *testing.T) {
		events := []assistant.AgentEvent{
			{Type: assistant.EventReasoning, Payload: "I need to think about this step by step..."},
			{Type: assistant.EventToolStream, Payload: "Let me check the files first."},
			{Type: assistant.EventToolCall, Payload: proxy.ToolCall{ID: "tc1", Function: proxy.FunctionCall{Name: "list_files", Arguments: "{}"}}},
			{Type: assistant.EventToolResult, Payload: map[string]any{"id": "tc1", "result": "file1.txt"}},
		}
		h := buildPartialHistory(base, events)
		if len(h) != 3 {
			t.Fatalf("expected 3 messages, got %d", len(h))
		}
		if h[1].Content != "Let me check the files first." {
			t.Errorf("expected Content from EventToolStream, got %q", h[1].Content)
		}
		if h[1].ReasoningContent != "I need to think about this step by step..." {
			t.Errorf("expected ReasoningContent from EventReasoning, got %q", h[1].ReasoningContent)
		}
	})

	t.Run("both Content and ReasoningContent snap at first tool call", func(t *testing.T) {
		events := []assistant.AgentEvent{
			{Type: assistant.EventToolStream, Payload: "Initial visible text."},
			{Type: assistant.EventReasoning, Payload: "Initial thinking."},
			{Type: assistant.EventToolCall, Payload: proxy.ToolCall{ID: "tc1", Function: proxy.FunctionCall{Name: "a", Arguments: "{}"}}},
			{Type: assistant.EventToolStream, Payload: "Later visible text."},
			{Type: assistant.EventReasoning, Payload: "Later thinking."},
			{Type: assistant.EventToolCall, Payload: proxy.ToolCall{ID: "tc2", Function: proxy.FunctionCall{Name: "b", Arguments: "{}"}}},
			{Type: assistant.EventToolResult, Payload: map[string]any{"id": "tc1", "result": "r1"}},
			{Type: assistant.EventToolResult, Payload: map[string]any{"id": "tc2", "result": "r2"}},
		}
		h := buildPartialHistory(base, events)
		// Turn: Content="Initial visible text.", ReasoningContent="Initial thinking."
		if h[1].Content != "Initial visible text." {
			t.Errorf("expected Content snap at first call, got %q", h[1].Content)
		}
		if h[1].ReasoningContent != "Initial thinking." {
			t.Errorf("expected ReasoningContent snap at first call, got %q", h[1].ReasoningContent)
		}
	})

	t.Run("turnContent is snapshot at first tool call", func(t *testing.T) {
		events := []assistant.AgentEvent{
			{Type: assistant.EventToolStream, Payload: "First reasoning."},
			{Type: assistant.EventToolCall, Payload: proxy.ToolCall{ID: "tc1", Function: proxy.FunctionCall{Name: "a", Arguments: "{}"}}},
			{Type: assistant.EventToolStream, Payload: "First reasoning.\nAlso more text after."},
			{Type: assistant.EventToolCall, Payload: proxy.ToolCall{ID: "tc2", Function: proxy.FunctionCall{Name: "b", Arguments: "{}"}}},
			{Type: assistant.EventToolResult, Payload: map[string]any{"id": "tc1", "result": "r1"}},
			{Type: assistant.EventToolResult, Payload: map[string]any{"id": "tc2", "result": "r2"}},
		}
		h := buildPartialHistory(base, events)
		if h[1].Content != "First reasoning." {
			t.Errorf("expected snapshot at first call (%q), got %q", "First reasoning.", h[1].Content)
		}
	})

	t.Run("unknown event type skipped", func(t *testing.T) {
		events := []assistant.AgentEvent{
			{Type: "bogus", Payload: "whatever"},
			{Type: assistant.EventMessage, Payload: proxy.Message{Role: proxy.AssistantRole, Content: "ok"}},
		}
		h := buildPartialHistory(base, events)
		if len(h) != 2 {
			t.Fatalf("expected 2 messages (base + message), got %d", len(h))
		}
	})

	t.Run("nil payload skipped", func(t *testing.T) {
		events := []assistant.AgentEvent{
			{Type: assistant.EventToolCall, Payload: nil},
			{Type: assistant.EventToolResult, Payload: nil},
			{Type: assistant.EventMessage, Payload: nil},
		}
		h := buildPartialHistory(base, events)
		if len(h) != 1 {
			t.Fatalf("expected only base message, got %d", len(h))
		}
	})

	t.Run("multi-turn with both fields preserved through cycles", func(t *testing.T) {
		events := []assistant.AgentEvent{
			// Turn 1: reasoning + visible + tool cycle
			{Type: assistant.EventReasoning, Payload: "First: I should list files."},
			{Type: assistant.EventToolStream, Payload: "Let me check the root directory."},
			{Type: assistant.EventToolCall, Payload: proxy.ToolCall{ID: "tc1", Function: proxy.FunctionCall{Name: "list", Arguments: "{}"}}},
			{Type: assistant.EventToolResult, Payload: map[string]any{"id": "tc1", "result": "file1.txt\nfile2.txt"}},
			// Turn 2: reasoning + visible (no tool calls) → EventMessage without tool calls
			{Type: assistant.EventReasoning, Payload: "Second: I have the listing, now report."},
			{Type: assistant.EventToolStream, Payload: "Here is the file report."},
			{Type: assistant.EventMessage, Payload: proxy.Message{Role: proxy.AssistantRole, Content: "Here is the file report."}},
		}
		h := buildPartialHistory(base, events)
		// base + turn1_assistant + turn1_tool + turn2_message = 4
		if len(h) != 4 {
			t.Fatalf("expected 4 messages, got %d", len(h))
		}
		if h[1].Content != "Let me check the root directory." {
			t.Errorf("turn1 content: expected %q, got %q", "Let me check the root directory.", h[1].Content)
		}
		if h[1].ReasoningContent != "First: I should list files." {
			t.Errorf("turn1 reasoning: expected %q, got %q", "First: I should list files.", h[1].ReasoningContent)
		}
		if h[3].Content != "Here is the file report." {
			t.Errorf("turn2 message: expected %q, got %q", "Here is the file report.", h[3].Content)
		}
	})
}

func TestWithLocalTime_AnnotatesOnlyTheLastUserMessageOnACopy(t *testing.T) {
	history := []proxy.Message{
		{Role: proxy.SystemRole, Content: "sys"},
		{Role: proxy.UserRole, Content: "first"},
		{Role: proxy.AssistantRole, Content: "ok"},
		{Role: proxy.UserRole, Content: "remind me tomorrow"},
	}
	// Two days after the conversation began: the note carries today's date, not the start date.
	now := time.Date(2026, 10, 14, 19, 29, 0, 0, time.UTC)
	got := withLocalTime(history, now, "Europe/London")
	if !strings.Contains(got[3].Content, "Wed 14 Oct 2026 20:29") || !strings.HasPrefix(got[3].Content, "remind me tomorrow") {
		t.Errorf("last user message = %q", got[3].Content)
	}
	if got[1].Content != "first" || got[0].Content != "sys" {
		t.Errorf("earlier messages must be untouched: %+v", got)
	}
	if history[3].Content != "remind me tomorrow" {
		t.Errorf("the stored history was modified: %q", history[3].Content)
	}
}

func TestWithLocalTime_LeavesAHistoryWithoutAUserTailAlone(t *testing.T) {
	history := []proxy.Message{{Role: proxy.AssistantRole, Content: "hi"}}
	if got := withLocalTime(history, time.Now(), "UTC"); got[0].Content != "hi" {
		t.Errorf("got %+v", got)
	}
	if got := withLocalTime(nil, time.Now(), "UTC"); got != nil {
		t.Errorf("got %+v", got)
	}
}

// TestTruncateHistory_PreservesFirstUserMessage verifies Fix B: truncation for
// oversized history must never drop the original user task, or the persisted
// session renders blank on reopen.
func TestTruncateHistory_PreservesFirstUserMessage(t *testing.T) {
	const budget = 12 * 1024
	big := strings.Repeat("x", budget) // exceeds budget alone
	history := []proxy.Message{
		{Role: proxy.SystemRole, Content: "system prompt"},
		{Role: proxy.UserRole, Content: "list all files and report"},
		{Role: proxy.AssistantRole, Content: big},
		{Role: proxy.ToolRole, Content: "result"},
	}
	out := TruncateHistory(history, budget)

	foundUser := false
	for _, m := range out {
		if m.Role == proxy.UserRole && m.Content == "list all files and report" {
			foundUser = true
		}
	}
	if !foundUser {
		t.Fatalf("first user message dropped by truncation; out=%d msgs", len(out))
	}
}

// TestTruncateHistory_PersistedCeilingPreservesToolCalls verifies the Bug 2 fix:
// the persisted-session ceiling is MaxPersistedHistoryChars (256KB), not the
// old 12KB cap (since removed). A multi-tool history that exceeds the OLD 12KB
// bound but stays under the new ceiling must be persisted in full — every tool
// call/result retained — so a reload shows the complete reasoning trail instead
// of dropping earlier tool calls.
func TestTruncateHistory_PersistedCeilingPreservesToolCalls(t *testing.T) {
	// Total content well above the old 12KB bound but under the 256KB ceiling.
	reasoning := strings.Repeat("r", 8*1024)
	history := []proxy.Message{
		{Role: proxy.SystemRole, Content: "system prompt"},
		{Role: proxy.UserRole, Content: "list all files and report"},
	}
	toolCount := 6
	for i := 0; i < toolCount; i++ {
		history = append(history,
			proxy.Message{
				Role:             proxy.AssistantRole,
				Content:          "",
				ReasoningContent: reasoning,
				ToolCalls: []proxy.ToolCall{{
					ID:       "tc-1",
					Type:     "function",
					Function: proxy.FunctionCall{Name: "list_directory", Arguments: `{"path":"/tmp"}`},
				}},
			},
			proxy.Message{Role: proxy.ToolRole, Content: "result", ToolCallID: "tc-1"},
		)
	}

	out := TruncateHistory(history, MaxPersistedHistoryChars)

	if len(out) != len(history) {
		t.Fatalf("persisted history truncated: got %d msgs, want %d (full) — tool calls lost", len(out), len(history))
	}
	toolMsgs := 0
	for _, m := range out {
		if m.Role == proxy.ToolRole {
			toolMsgs++
		}
	}
	if toolMsgs != toolCount {
		t.Errorf("expected %d tool messages preserved, got %d", toolCount, toolMsgs)
	}
}
