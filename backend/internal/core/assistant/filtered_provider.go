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
