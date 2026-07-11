package assistant

import (
	"context"

	"llm-proxy/internal/core/proxy"
)

type filteredToolProvider struct {
	inner   ToolProvider
	exclude map[string]bool
}

func NewFilteredToolProvider(inner ToolProvider, exclude []string) ToolProvider {
	m := make(map[string]bool, len(exclude))
	for _, name := range exclude {
		m[name] = true
	}
	return &filteredToolProvider{inner: inner, exclude: m}
}

func (f *filteredToolProvider) ListTools(ctx context.Context) ([]proxy.Tool, error) {
	tools, err := f.inner.ListTools(ctx)
	if err != nil {
		return nil, err
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
