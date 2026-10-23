package assistant

import (
	"fmt"
	"testing"

	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
)

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
