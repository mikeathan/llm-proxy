package nodeherder

import (
	"context"
	"llm-proxy/internal/core/proxy"
)

// MCPService defines the interface for an agnostic MCP client.
type MCPService interface {
	GetSystemPrompt() (string, error)
	ListTools(ctx context.Context) ([]proxy.Tool, error)
	CallTool(ctx context.Context, name string, args map[string]any) (any, error)
}
