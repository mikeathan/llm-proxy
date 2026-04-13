package assistant

import (
	"context"
	"encoding/json"
	"llm-proxy/internal/core/proxy"
	"strings"
)

// ToolProvider defines how tools and prompts are discovered.
type ToolProvider interface {
	ListTools(ctx context.Context) ([]proxy.Tool, error)
	GetSystemPrompt() (string, error)
}

// MultiToolProvider aggregates tools from multiple providers.
type MultiToolProvider struct {
	providers []ToolProvider
}

func NewMultiToolProvider(providers ...ToolProvider) *MultiToolProvider {
	return &MultiToolProvider{providers: providers}
}

func (p *MultiToolProvider) ListTools(ctx context.Context) ([]proxy.Tool, error) {
	var all []proxy.Tool
	for _, provider := range p.providers {
		tools, err := provider.ListTools(ctx)
		if err != nil {
			continue // Skip failing providers
		}
		all = append(all, tools...)
	}
	return all, nil
}

func (p *MultiToolProvider) GetSystemPrompt() (string, error) {
	var fullPrompt []string
	for _, provider := range p.providers {
		pStr, err := provider.GetSystemPrompt()
		if err != nil || pStr == "" {
			continue
		}
		fullPrompt = append(fullPrompt, pStr)
	}

	if len(fullPrompt) == 0 {
		return "", nil
	}

	// Join multiple prompts if multiple providers provide them
	return strings.Join(fullPrompt, "\n\n---\n\n"), nil
}

// CompositeEngine delegates tool execution to either a primary (builtin) or secondary (mcp) engine.
type CompositeEngine struct {
	primary   Engine
	secondary Engine
}

func NewCompositeEngine(primary, secondary Engine) *CompositeEngine {
	return &CompositeEngine{primary: primary, secondary: secondary}
}

func (e *CompositeEngine) ExecuteTool(ctx context.Context, call proxy.ToolCall) (any, error) {
	// Try primary first, then fallback to secondary
	res, err := e.primary.ExecuteTool(ctx, call)
	if err == nil {
		return res, nil
	}

	return e.secondary.ExecuteTool(ctx, call)
}

func decodeArgs(raw string, target any) error {
	if raw == "" {
		return nil
	}
	return json.Unmarshal([]byte(raw), target)
}
