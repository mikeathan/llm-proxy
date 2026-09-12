// tool_availability.go — the single narrow waist where the tool schema an
// agent can see is resolved from guardrail policy + caller constraints. Every
// strategy and channel consumes deps.Provider.ListTools, so resolving here
// guarantees nothing that guardrail policy statically disables is ever exposed.
// filteredToolProvider (formerly filtered_provider.go) was merged into this file.
package assistant

import (
	"context"

	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/models"
)

// resolveToolProvider applies the guardrail-derived static tool exclusions
// (DisabledToolNames, workspace-tier) plus the caller's allowed/excluded tool
// sets. Intersection: allow ∩ exclude ∩ guardrail-disabled. Used when the run
// has no explicit network scope (chat, default automations).
func resolveToolProvider(base ToolProvider, gr *guardrails.GuardrailEngine, workspaceID string, allowed, excluded []string) ToolProvider {
	if gr != nil {
		excluded = append(excluded, gr.DisabledToolNames(workspaceID)...)
	}
	return applyToolFilter(base, allowed, excluded)
}

// resolveToolProviderForScope resolves schema availability for a run carrying an
// explicitly resolved network scope (automation grant, plan §4.4). Inherit/unknown
// delegates to the workspace-tier resolution.
func resolveToolProviderForScope(base ToolProvider, gr *guardrails.GuardrailEngine, workspaceID string, scope models.NetworkScope, allowed, excluded []string) ToolProvider {
	if scope == models.NetworkScopeInherit || !scope.Valid() || gr == nil {
		return resolveToolProvider(base, gr, workspaceID, allowed, excluded)
	}
	excluded = append(excluded, gr.DisabledToolNamesForScope(workspaceID, scope)...)
	return applyToolFilter(base, allowed, excluded)
}

// applyToolFilter wraps base in a filteredToolProvider when the allowed or
// excluded sets are non-empty; otherwise it returns base unwrapped so the
// common case adds no indirection.
func applyToolFilter(base ToolProvider, allowed, excluded []string) ToolProvider {
	if len(allowed) == 0 && len(excluded) == 0 {
		return base
	}
	allow := make(map[string]bool, len(allowed))
	for _, n := range allowed {
		allow[n] = true
	}
	excl := make(map[string]bool, len(excluded))
	for _, n := range excluded {
		excl[n] = true
	}
	return &filteredToolProvider{inner: base, allow: allow, exclude: excl}
}

// filteredToolProvider restricts the inner provider's tool list to the caller's
// allowed set minus the excluded set. When both sets are empty it passes the
// inner list through untouched.
type filteredToolProvider struct {
	inner   ToolProvider
	exclude map[string]bool
	allow   map[string]bool // when non-empty, only tools in this set pass through
}

func (f *filteredToolProvider) ListTools(ctx context.Context) ([]proxy.Tool, error) {
	tools, err := f.inner.ListTools(ctx)
	if err != nil {
		return nil, err
	}
	if len(f.allow) == 0 && len(f.exclude) == 0 {
		return tools, nil
	}
	filtered := make([]proxy.Tool, 0, len(tools))
	for _, t := range tools {
		if len(f.allow) > 0 && !f.allow[t.Function.Name] {
			continue
		}
		if f.exclude[t.Function.Name] {
			continue
		}
		filtered = append(filtered, t)
	}
	return filtered, nil
}

func (f *filteredToolProvider) GetSystemPrompt() (string, error) {
	return f.inner.GetSystemPrompt()
}

func (f *filteredToolProvider) UseNativeTools() bool {
	return f.inner.UseNativeTools()
}
