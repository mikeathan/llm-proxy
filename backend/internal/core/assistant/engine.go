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

	var args map[string]any
	if call.Function.Arguments != "" {
		if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
			// LLM sometimes sends a string instead of an object, try to handle or fail
			return nil, fmt.Errorf("failed to parse tool arguments: %w", err)
		}
	} else {
		args = make(map[string]any)
	}

	return a.mcp.CallTool(ctx, call.Function.Name, args)
}
