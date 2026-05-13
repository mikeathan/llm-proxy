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
	UseNativeTools() bool
}

// MultiToolProvider aggregates tools from multiple providers.
type MultiToolProvider struct {
	Providers      []ToolProvider
	useNativeTools bool
}

func NewMultiToolProvider(useNativeTools bool, providers ...ToolProvider) *MultiToolProvider {
	return &MultiToolProvider{Providers: providers, useNativeTools: useNativeTools}
}

func (p *MultiToolProvider) ListTools(ctx context.Context) ([]proxy.Tool, error) {
	var all []proxy.Tool
	for _, provider := range p.Providers {
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
	for _, provider := range p.Providers {
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

func (p *MultiToolProvider) UseNativeTools() bool {
	return p.useNativeTools
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
	// Try primary first
	res, err := e.primary.ExecuteTool(ctx, call)
	if err == nil {
		return res, nil
	}

	// Only fallback to secondary if the tool was NOT found in primary.
	// If primary FOUND the tool but it failed (e.g. guardrail), return that error immediately.
	if err != ErrToolNotInternal {
		return nil, err
	}

	return e.secondary.ExecuteTool(ctx, call)
}

func decodeArgs(raw string, target any) error {
	if raw == "" {
		return nil
	}
	return json.Unmarshal([]byte(raw), target)
}
