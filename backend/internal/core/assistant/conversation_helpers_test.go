package assistant

import (
	"fmt"
	"testing"

	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
)

func TestLoadAgentsFile(t *testing.T) {
	tmp := t.TempDir()
	mgr := persistence.NewWorkspaceManager(storage.NewPathResolver(tmp, tmp, tmp))
	const wsID = "ws1"

	t.Run("returns default when no file exists", func(t *testing.T) {
		agentsFileCache.Clear()
		if got := LoadAgentsFile(mgr, wsID); got != prompts.DefaultAgentsMD {
			t.Errorf("expected DefaultAgentsMD fallback, got %q", got)
		}
	})

	t.Run("returns AGENTS.md content when present", func(t *testing.T) {
		agentsFileCache.Clear()
		custom := "# Custom\nAlways respond in French."
		if err := mgr.WriteTaskFile(wsID, models.RulesFilename, custom); err != nil {
			t.Fatalf("write task file: %v", err)
		}
		if got := LoadAgentsFile(mgr, wsID); got != custom {
			t.Errorf("expected custom AGENTS.md content, got %q", got)
		}
	})

	t.Run("returns default when workspaceID empty", func(t *testing.T) {
		agentsFileCache.Clear()
		if got := LoadAgentsFile(mgr, ""); got != prompts.DefaultAgentsMD {
			t.Errorf("expected default for empty workspaceID, got %q", got)
		}
	})

	t.Run("returns default when manager nil", func(t *testing.T) {
		agentsFileCache.Clear()
		if got := LoadAgentsFile(nil, wsID); got != prompts.DefaultAgentsMD {
			t.Errorf("expected default for nil manager, got %q", got)
		}
	})
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
		events := []AgentEvent{
			{Type: EventToolCall, Payload: proxy.ToolCall{
				ID: "tc1", Function: proxy.FunctionCall{Name: "list_files", Arguments: "{}"},
			}},
			{Type: EventToolResult, Payload: map[string]any{
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
		events := []AgentEvent{
			{Type: EventToolCall, Payload: proxy.ToolCall{ID: "tc1", Function: proxy.FunctionCall{Name: "a", Arguments: "{}"}}},
			{Type: EventToolResult, Payload: map[string]any{"id": "tc1", "result": "r1"}},
			{Type: EventToolCall, Payload: proxy.ToolCall{ID: "tc2", Function: proxy.FunctionCall{Name: "b", Arguments: "{}"}}},
			{Type: EventToolResult, Payload: map[string]any{"id": "tc2", "result": "r2"}},
			{Type: EventMessage, Payload: proxy.Message{Role: proxy.AssistantRole, Content: "done"}},
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
		events := []AgentEvent{
			{Type: EventToolStream, Payload: "Now let me explore the structure..."},
			{Type: EventToolCall, Payload: proxy.ToolCall{ID: "tc1", Function: proxy.FunctionCall{Name: "list_files", Arguments: "{}"}}},
			{Type: EventToolResult, Payload: map[string]any{"id": "tc1", "result": "file1.txt"}},
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
		events := []AgentEvent{
			{Type: EventToolStream, Payload: "I need to run two tools in parallel."},
			{Type: EventToolCall, Payload: proxy.ToolCall{ID: "tc1", Function: proxy.FunctionCall{Name: "read_file", Arguments: "{}"}}},
			{Type: EventToolCall, Payload: proxy.ToolCall{ID: "tc2", Function: proxy.FunctionCall{Name: "list", Arguments: "{}"}}},
			{Type: EventToolResult, Payload: map[string]any{"id": "tc1", "result": "content"}},
			{Type: EventToolResult, Payload: map[string]any{"id": "tc2", "result": "list"}},
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
		events := []AgentEvent{
			{Type: EventToolStream, Payload: "Reasoning text."},
			{Type: EventToolCall, Payload: proxy.ToolCall{ID: "tc1", Function: proxy.FunctionCall{Name: "tool_a", Arguments: "{}"}}},
			{Type: EventToolResult, Payload: map[string]any{"id": "tc1", "result": "res"}},
			// This EventMessage has ToolCalls — it duplicates the assistant
			// message already built by flushPendingGroup from EventToolCall.
			{Type: EventMessage, Payload: proxy.Message{
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
		events := []AgentEvent{
			{Type: EventToolStream, Payload: "Streaming text."},
			{Type: EventToolCall, Payload: proxy.ToolCall{ID: "tc1", Function: proxy.FunctionCall{Name: "a", Arguments: "{}"}}},
			{Type: EventToolResult, Payload: map[string]any{"id": "tc1", "result": "r"}},
			{Type: EventMessage, Payload: proxy.Message{Role: proxy.AssistantRole, Content: "Final answer."}},
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
		events := []AgentEvent{
			{Type: EventReasoning, Payload: "I need to think about this step by step..."},
			{Type: EventToolStream, Payload: "Let me check the files first."},
			{Type: EventToolCall, Payload: proxy.ToolCall{ID: "tc1", Function: proxy.FunctionCall{Name: "list_files", Arguments: "{}"}}},
			{Type: EventToolResult, Payload: map[string]any{"id": "tc1", "result": "file1.txt"}},
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
		events := []AgentEvent{
			{Type: EventToolStream, Payload: "Initial visible text."},
			{Type: EventReasoning, Payload: "Initial thinking."},
			{Type: EventToolCall, Payload: proxy.ToolCall{ID: "tc1", Function: proxy.FunctionCall{Name: "a", Arguments: "{}"}}},
			{Type: EventToolStream, Payload: "Later visible text."},
			{Type: EventReasoning, Payload: "Later thinking."},
			{Type: EventToolCall, Payload: proxy.ToolCall{ID: "tc2", Function: proxy.FunctionCall{Name: "b", Arguments: "{}"}}},
			{Type: EventToolResult, Payload: map[string]any{"id": "tc1", "result": "r1"}},
			{Type: EventToolResult, Payload: map[string]any{"id": "tc2", "result": "r2"}},
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
		events := []AgentEvent{
			{Type: EventToolStream, Payload: "First reasoning."},
			{Type: EventToolCall, Payload: proxy.ToolCall{ID: "tc1", Function: proxy.FunctionCall{Name: "a", Arguments: "{}"}}},
			{Type: EventToolStream, Payload: "First reasoning.\nAlso more text after."},
			{Type: EventToolCall, Payload: proxy.ToolCall{ID: "tc2", Function: proxy.FunctionCall{Name: "b", Arguments: "{}"}}},
			{Type: EventToolResult, Payload: map[string]any{"id": "tc1", "result": "r1"}},
			{Type: EventToolResult, Payload: map[string]any{"id": "tc2", "result": "r2"}},
		}
		h := buildPartialHistory(base, events)
		if h[1].Content != "First reasoning." {
			t.Errorf("expected snapshot at first call (%q), got %q", "First reasoning.", h[1].Content)
		}
	})

	t.Run("unknown event type skipped", func(t *testing.T) {
		events := []AgentEvent{
			{Type: "bogus", Payload: "whatever"},
			{Type: EventMessage, Payload: proxy.Message{Role: proxy.AssistantRole, Content: "ok"}},
		}
		h := buildPartialHistory(base, events)
		if len(h) != 2 {
			t.Fatalf("expected 2 messages (base + message), got %d", len(h))
		}
	})

	t.Run("nil payload skipped", func(t *testing.T) {
		events := []AgentEvent{
			{Type: EventToolCall, Payload: nil},
			{Type: EventToolResult, Payload: nil},
			{Type: EventMessage, Payload: nil},
		}
		h := buildPartialHistory(base, events)
		if len(h) != 1 {
			t.Fatalf("expected only base message, got %d", len(h))
		}
	})

	t.Run("multi-turn with both fields preserved through cycles", func(t *testing.T) {
		events := []AgentEvent{
			// Turn 1: reasoning + visible + tool cycle
			{Type: EventReasoning, Payload: "First: I should list files."},
			{Type: EventToolStream, Payload: "Let me check the root directory."},
			{Type: EventToolCall, Payload: proxy.ToolCall{ID: "tc1", Function: proxy.FunctionCall{Name: "list", Arguments: "{}"}}},
			{Type: EventToolResult, Payload: map[string]any{"id": "tc1", "result": "file1.txt\nfile2.txt"}},
			// Turn 2: reasoning + visible (no tool calls) → EventMessage without tool calls
			{Type: EventReasoning, Payload: "Second: I have the listing, now report."},
			{Type: EventToolStream, Payload: "Here is the file report."},
			{Type: EventMessage, Payload: proxy.Message{Role: proxy.AssistantRole, Content: "Here is the file report."}},
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

func TestAgentsFileCache_BoundedEviction(t *testing.T) {
	agentsFileCache.Clear()

	for i := 0; i < agentsFileCacheMaxEntries+1; i++ {
		key := agentsCacheKey + fmt.Sprintf("ws-%d", i)
		agentsFileCache.Put(key, fmt.Sprintf("content-%d", i))
	}

	if n := agentsFileCache.Len(); n > agentsFileCacheMaxEntries {
		t.Fatalf("cache exceeded max entries: got %d, want ≤%d", n, agentsFileCacheMaxEntries)
	}

	if agentsFileCache.Contains(agentsCacheKey + "ws-0") {
		t.Error("oldest entry ws-0 should have been evicted when cache was at capacity")
	}
	if !agentsFileCache.Contains(agentsCacheKey + fmt.Sprintf("ws-%d", agentsFileCacheMaxEntries)) {
		t.Error("most recent entry should still be present after eviction")
	}
}

// newNeutralizeEngine builds an engine whose internet_search availability is
// controlled by searchConfigured — the same predicate the schema-hide gate uses.
func newNeutralizeEngine(searchConfigured bool) *guardrails.GuardrailEngine {
	gr := guardrails.NewGuardrailEngine(func() models.AgentGuardrailsConfig {
		return models.AgentGuardrailsConfig{Search: models.SearchGuardrailsConfig{Enabled: true}}
	}, storage.NewPathResolver("", "", ""), nil, nil)
	gr.SetSearchAvailable(func() bool { return searchConfigured })
	return gr
}

// unavailableHistory is the shape that breaks a retry: an assistant tool call
// followed by the terminal result that says "Do NOT retry it".
func unavailableHistory(tool string) []proxy.Message {
	return []proxy.Message{
		{Role: proxy.UserRole, Content: "search the internet for ai news"},
		{Role: proxy.AssistantRole, ToolCalls: []proxy.ToolCall{
			{ID: "call_1", Type: "function", Function: proxy.FunctionCall{Name: tool}},
		}},
		{Role: proxy.ToolRole, ToolCallID: "call_1", Content: fmt.Sprintf(
			prompts.ToolUnavailablePrompt, tool, "tavily API error (status 401)")},
	}
}

// The reported bug: after the credential is fixed, the replayed "Do NOT retry"
// notice must stop telling the model the tool is broken.
func TestNeutralizeStaleToolFailures_RewritesWhenToolAvailableAgain(t *testing.T) {
	history := unavailableHistory(models.ToolInternetSearch)

	n := NeutralizeStaleToolFailures(history, newNeutralizeEngine(true), "ws-1")

	if n != 1 {
		t.Fatalf("neutralized = %d, want 1", n)
	}
	got := history[2].Content
	if got == fmt.Sprintf(prompts.ToolUnavailablePrompt, models.ToolInternetSearch, "tavily API error (status 401)") {
		t.Fatal("stale unavailable notice was not rewritten")
	}
	if want := fmt.Sprintf(prompts.ToolUnavailableRetryPrompt, models.ToolInternetSearch, models.ToolInternetSearch); got != want {
		t.Errorf("content = %q, want %q", got, want)
	}
}

// A tool that is still unconfigured must keep its notice: the history is
// accurate, and the model should not be told to retry a broken tool.
func TestNeutralizeStaleToolFailures_LeavesStillUnavailableTool(t *testing.T) {
	history := unavailableHistory(models.ToolInternetSearch)
	original := history[2].Content

	n := NeutralizeStaleToolFailures(history, newNeutralizeEngine(false), "ws-1")

	if n != 0 {
		t.Fatalf("neutralized = %d, want 0 for a still-unavailable tool", n)
	}
	if history[2].Content != original {
		t.Errorf("content changed for an unavailable tool: %q", history[2].Content)
	}
}

// Only terminal "tool unavailable" notices are candidates. Successful results,
// transient errors and guardrail denials must survive untouched.
func TestNeutralizeStaleToolFailures_LeavesOtherResultsAlone(t *testing.T) {
	history := []proxy.Message{
		{Role: proxy.UserRole, Content: "do work"},
		{Role: proxy.AssistantRole, ToolCalls: []proxy.ToolCall{
			{ID: "call_ok", Type: "function", Function: proxy.FunctionCall{Name: models.ToolInternetSearch}},
			{ID: "call_err", Type: "function", Function: proxy.FunctionCall{Name: models.ToolInternetSearch}},
			{ID: "call_denied", Type: "function", Function: proxy.FunctionCall{Name: models.ToolInternetSearch}},
		}},
		{Role: proxy.ToolRole, ToolCallID: "call_ok", Content: `{"results":[{"title":"ok"}]}`},
		{Role: proxy.ToolRole, ToolCallID: "call_err", Content: "server returned unexpected status: 503"},
		{Role: proxy.ToolRole, ToolCallID: "call_denied", Content: "guardrail denied this tool call"},
	}

	n := NeutralizeStaleToolFailures(history, newNeutralizeEngine(true), "ws-1")

	if n != 0 {
		t.Fatalf("neutralized = %d, want 0", n)
	}
	if history[2].Content != `{"results":[{"title":"ok"}]}` {
		t.Error("successful result was rewritten")
	}
	if history[3].Content != "server returned unexpected status: 503" {
		t.Error("transient error was rewritten")
	}
	if history[4].Content != "guardrail denied this tool call" {
		t.Error("guardrail denial was rewritten")
	}
}

func TestNeutralizeStaleToolFailures_NilEngineOrEmptyHistoryIsNoOp(t *testing.T) {
	history := unavailableHistory(models.ToolInternetSearch)
	if n := NeutralizeStaleToolFailures(history, nil, "ws-1"); n != 0 {
		t.Errorf("nil engine: neutralized = %d, want 0", n)
	}
	if n := NeutralizeStaleToolFailures(nil, newNeutralizeEngine(true), "ws-1"); n != 0 {
		t.Errorf("empty history: neutralized = %d, want 0", n)
	}
}
