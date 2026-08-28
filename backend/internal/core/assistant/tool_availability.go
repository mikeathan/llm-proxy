// tool_availability.go — the single narrow waist where the tool schema an
// agent can see is resolved from guardrail policy + caller constraints. Every
// strategy and channel consumes deps.Provider.ListTools, so resolving here
// guarantees nothing that guardrail policy statically disables is ever exposed.
package assistant

import (
	"llm-proxy/internal/core/assistant/guardrails"
)

// resolveToolProvider applies the guardrail-derived static tool exclusions
// (DisabledToolNames) plus the caller's allowed/excluded tool sets to the base
// provider. Intersection semantics: allow ∩ exclude ∩ guardrail-disabled. When
// nothing needs filtering the base provider is returned unwrapped so the common
// case adds no indirection.
func resolveToolProvider(base ToolProvider, gr *guardrails.GuardrailEngine, workspaceID string, allowed, excluded []string) ToolProvider {
	exclude := make(map[string]bool, len(excluded))
	for _, n := range excluded {
		exclude[n] = true
	}
	if gr != nil {
		for _, n := range gr.DisabledToolNames(workspaceID) {
			exclude[n] = true
		}
	}
	if len(allowed) == 0 && len(exclude) == 0 {
		return base
	}
	allow := make(map[string]bool, len(allowed))
	for _, n := range allowed {
		allow[n] = true
	}
	return &filteredToolProvider{inner: base, allow: allow, exclude: exclude}
}
