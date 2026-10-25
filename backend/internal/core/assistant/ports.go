// ports.go — the two interfaces the agent consumes for tools. The implementations (the local
// registry, the MCP engine and their composites) live in core/toolregistry.
package assistant

import (
	"context"

	"llm-proxy/internal/core/proxy"
)

// ToolProvider defines how tools and prompts are discovered.
type ToolProvider interface {
	ListTools(ctx context.Context) ([]proxy.Tool, error)
	UseNativeTools() bool
}

// Engine defines how tool execution is dispatched.
type Engine interface {
	ExecuteTool(ctx context.Context, call proxy.ToolCall) (any, error)
}
