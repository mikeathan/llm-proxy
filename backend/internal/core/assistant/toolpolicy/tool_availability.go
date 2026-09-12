// Package toolpolicy resolves the tool schema an agent can see from guardrail
// policy plus caller constraints, and (soon) the per-tool failure policy.
//
// It is the single narrow waist: every strategy and channel consumes
// deps.Provider.ListTools, so resolving here guarantees nothing that guardrail
// policy statically disables is ever exposed.
package toolpolicy

import (
	"context"

	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/models"
)

// Provider is the narrow tool-provider surface this package filters. Declared
// locally (consumer-defined) so the policy package never imports the assistant
// root — which would be an import cycle. Values of assistant.ToolProvider
// satisfy it structurally.
type Provider interface {
	ListTools(ctx context.Context) ([]proxy.Tool, error)
	GetSystemPrompt() (string, error)
	UseNativeTools() bool
}

// Resolve applies the guardrail-derived static tool exclusions
// (DisabledToolNames, workspace-tier) plus the caller's allowed/excluded tool
// sets. Intersection: allow ∩ exclude ∩ guardrail-disabled. Used when the run
// has no explicit network scope (chat, default automations).
func Resolve(base Provider, gr *guardrails.GuardrailEngine, workspaceID string, allowed, excluded []string) Provider {
	if gr != nil {
		excluded = append(excluded, gr.DisabledToolNames(workspaceID)...)
	}
	return applyFilter(base, allowed, excluded)
}

// ResolveForScope resolves schema availability for a run carrying an explicitly
// resolved network scope (automation grant, plan §4.4). Inherit/unknown
// delegates to the workspace-tier resolution.
func ResolveForScope(base Provider, gr *guardrails.GuardrailEngine, workspaceID string, scope models.NetworkScope, allowed, excluded []string) Provider {
	if scope == models.NetworkScopeInherit || !scope.Valid() || gr == nil {
		return Resolve(base, gr, workspaceID, allowed, excluded)
	}
	excluded = append(excluded, gr.DisabledToolNamesForScope(workspaceID, scope)...)
	return applyFilter(base, allowed, excluded)
}

// applyFilter wraps base in a filteredProvider when the allowed or excluded
// sets are non-empty; otherwise it returns base unwrapped so the common case
// adds no indirection.
func applyFilter(base Provider, allowed, excluded []string) Provider {
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
	return &filteredProvider{inner: base, allow: allow, exclude: excl}
}

// filteredProvider restricts the inner provider's tool list to the caller's
// allowed set minus the excluded set. When both sets are empty it passes the
// inner list through untouched.
type filteredProvider struct {
	inner   Provider
	exclude map[string]bool
	allow   map[string]bool // when non-empty, only tools in this set pass through
}

func (f *filteredProvider) ListTools(ctx context.Context) ([]proxy.Tool, error) {
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

func (f *filteredProvider) GetSystemPrompt() (string, error) {
	return f.inner.GetSystemPrompt()
}

func (f *filteredProvider) UseNativeTools() bool {
	return f.inner.UseNativeTools()
}
