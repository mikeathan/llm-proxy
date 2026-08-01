package assistant

import (
	"context"

	"llm-proxy/internal/core/proxy"
)

type filteredToolProvider struct {
	inner   ToolProvider
	exclude map[string]bool
	allow   map[string]bool // when non-empty, only tools in this set pass through
}

func NewFilteredToolProvider(inner ToolProvider, exclude []string) ToolProvider {
	m := make(map[string]bool, len(exclude))
	for _, name := range exclude {
		m[name] = true
	}
	return &filteredToolProvider{inner: inner, exclude: m}
}

// NewAllowedToolsProvider returns a ToolProvider that only exposes tools in
// the allowed list. When allowed is empty or nil, all tools pass through
// (backward compatible). This is designed for unattended automations where
// the tool set must be restricted at the provider level.
func NewAllowedToolsProvider(inner ToolProvider, allowed []string) ToolProvider {
	m := make(map[string]bool, len(allowed))
	for _, name := range allowed {
		m[name] = true
	}
	return &filteredToolProvider{inner: inner, allow: m}
}

func (f *filteredToolProvider) ListTools(ctx context.Context) ([]proxy.Tool, error) {
	tools, err := f.inner.ListTools(ctx)
	if err != nil {
		return nil, err
	}
	if len(f.allow) > 0 {
		filtered := make([]proxy.Tool, 0, len(tools))
		for _, t := range tools {
			if f.allow[t.Function.Name] {
				filtered = append(filtered, t)
			}
		}
		return filtered, nil
	}
	if len(f.exclude) == 0 {
		return tools, nil
	}
	filtered := make([]proxy.Tool, 0, len(tools))
	for _, t := range tools {
		if !f.exclude[t.Function.Name] {
			filtered = append(filtered, t)
		}
	}
	return filtered, nil
}

func (f *filteredToolProvider) GetSystemPrompt() (string, error) {
	return f.inner.GetSystemPrompt()
}

func (f *filteredToolProvider) UseNativeTools() bool {
	return f.inner.UseNativeTools()
}
