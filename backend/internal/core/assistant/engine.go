package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/core/nodeherder"
	"llm-proxy/internal/core/proxy"
)

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

	// Generic tool execution
	var args map[string]any
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
		// If arguments are empty or invalid JSON, try to handle gracefully or error.
		// LLM sometimes sends empty string for no args.
		if call.Function.Arguments == "" {
			args = make(map[string]any)
		} else {
			return nil, fmt.Errorf("failed to parse arguments: %w", err)
		}
	}

	return a.mcp.CallTool(ctx, call.Function.Name, args)
}
