// tool_provider.go — ToolProvider and Engine interfaces (with
// MultiToolProvider, CompositeEngine, assistantEngine implementations),
// and the NewEngine constructor.  engine.go was merged into this file.
package assistant

import (
	"context"
	"fmt"
	"llm-proxy/internal/core/nodeherder"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
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
			continue
		}
		all = append(all, tools...)
	}
	return deduplicateTools(all), nil
}

// deduplicateTools removes tools with duplicate names, keeping the first
// occurrence.  This prevents conflicts when MCP servers re-list tools that
// are already registered locally.
func deduplicateTools(tools []proxy.Tool) []proxy.Tool {
	seen := make(map[string]bool, len(tools))
	result := make([]proxy.Tool, 0, len(tools))
	for _, t := range tools {
		if !seen[t.Function.Name] {
			seen[t.Function.Name] = true
			result = append(result, t)
		}
	}
	return result
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
		return res, err
	}

	return e.secondary.ExecuteTool(ctx, call)
}

func decodeArgs(raw string, target any) error {
	return proxy.DecodeToolArgs(raw, target)
}

// Engine defines how tool execution is dispatched.
type Engine interface {
	ExecuteTool(ctx context.Context, call proxy.ToolCall) (any, error)
}

type assistantEngine struct {
	mcp    nodeherder.MCPService
	logger logging.Logger
}

func NewEngine(mcp nodeherder.MCPService, logger logging.Logger) Engine {
	return &assistantEngine{
		mcp:    mcp,
		logger: logger,
	}
}

func (a *assistantEngine) ExecuteTool(ctx context.Context, call proxy.ToolCall) (any, error) {
	a.logger.Info("tool call", "name", call.Function.Name, "conversation", call.ID)

	var args map[string]any
	if err := decodeArgs(call.Function.Arguments, &args); err != nil {
		return nil, fmt.Errorf("failed to parse tool arguments: %w", err)
	}
	if args == nil {
		args = make(map[string]any)
	}

	return a.mcp.CallTool(ctx, call.Function.Name, args)
}
