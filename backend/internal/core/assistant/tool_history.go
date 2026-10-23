package assistant

import (
	"fmt"
	"strings"

	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/proxy"
)

// NeutralizeStaleToolFailures rewrites replayed terminal "tool unavailable"
// results once the tool is available again, and reports how many it changed.
//
// A missing/rejected credential is a run-scoped condition, but its tool result
// is persisted and replayed into every later run in the conversation. That
// result carries the literal instruction "Do NOT retry it", so after an operator
// reconfigures the credential the model still refuses to call a tool that now
// works — it reports the capability as broken instead. Rewriting those stale
// results at replay time removes the obsolete instruction without touching
// still-broken tools or the run that originally hit the failure.
func NeutralizeStaleToolFailures(history []proxy.Message, gr *guardrails.GuardrailEngine, workspaceID string) int {
	if gr == nil || len(history) == 0 {
		return 0
	}
	available := currentToolAvailability(gr, workspaceID, history)
	if len(available) == 0 {
		return 0
	}

	neutralized := 0
	for i := range history {
		if history[i].Role != proxy.ToolRole {
			continue
		}
		tool, ok := staleToolUnavailable(history[i].Content)
		if !ok || !available[tool] {
			continue
		}
		history[i].Content = fmt.Sprintf(prompts.ToolUnavailableRetryPrompt, tool, tool)
		neutralized++
	}
	return neutralized
}

// currentToolAvailability maps each tool named in a replayed tool call to
// whether it is runnable now. It reuses the guardrail engine's denied-tool set —
// the same rule that decides schema visibility — so availability can never drift
// from what the model is actually offered.
func currentToolAvailability(gr *guardrails.GuardrailEngine, workspaceID string, history []proxy.Message) map[string]bool {
	denied := make(map[string]bool)
	for _, name := range gr.DisabledToolNames(workspaceID) {
		denied[name] = true
	}
	available := make(map[string]bool)
	for _, m := range history {
		for _, tc := range m.ToolCalls {
			available[tc.Function.Name] = !denied[tc.Function.Name]
		}
	}
	return available
}

// staleToolUnavailable reports whether a replayed tool result is the terminal
// "tool unavailable" notice, and which tool it names.
func staleToolUnavailable(content string) (string, bool) {
	if !strings.HasPrefix(content, prompts.ToolUnavailablePrefix) {
		return "", false
	}
	rest := content[len(prompts.ToolUnavailablePrefix):]
	tool, _, found := strings.Cut(rest, " — ")
	if !found || tool == "" {
		return "", false
	}
	return tool, true
}
