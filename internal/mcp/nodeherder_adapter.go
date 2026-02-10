package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"llm-proxy/internal/logging"
	"llm-proxy/internal/nodeherder"
	"llm-proxy/internal/proxy"

	mcp_sdk "github.com/mark3labs/mcp-go/mcp"
)

// MCPNodeHerder implements nodeherder.NodeHerderService using MCP Orchestrator and resource mirror.
type MCPNodeHerder struct {
	orchestrator *Orchestrator
	mirror       *ResourceMirror
	logger       logging.Logger
}

// NewMCPNodeHerder creates a new MCPNodeHerder.
func NewMCPNodeHerder(orchestrator *Orchestrator, mirror *ResourceMirror, logger logging.Logger) nodeherder.MCPService {
	// Register the prompt update handler on the pool
	orchestrator.OnPromptUpdate(func(content string) {
		logger.Info("Received system-prompt update via notification")
		mirror.SetSystemPrompt(content)
	})

	return &MCPNodeHerder{
		orchestrator: orchestrator,
		mirror:       mirror,
		logger:       logger,
	}
}

// GetSystemPrompt returns the current system prompt from the mirror.
func (n *MCPNodeHerder) GetSystemPrompt() (string, error) {
	prompt := n.mirror.GetSystemPrompt()
	if prompt == "" {
		// Try to fetch explicitly if missing
		content, err := n.orchestrator.ReadResource(context.Background(), "nodeherder://system-prompt")
		if err != nil {
			n.logger.Warn("System prompt not available, utilizing fallback", "error", err)
			return "You are a helpful assistant. The home automation system (NodeHerder) is currently disconnected. You cannot access device states or control devices. Please inform the user of this connection status if they request device interactions.", nil
		}

		n.mirror.SetSystemPrompt(content)
		return content, nil
	}
	return prompt, nil
}

// ListTools returns the list of tools available on the MCP server.
func (n *MCPNodeHerder) ListTools(ctx context.Context) ([]proxy.Tool, error) {
	mcpTools, err := n.orchestrator.ListTools(ctx)
	if err != nil {
		return nil, err
	}

	var tools []proxy.Tool
	for _, t := range mcpTools {
		// Convert InputSchema struct to map
		var schemaMap map[string]any
		b, _ := json.Marshal(t.InputSchema)
		_ = json.Unmarshal(b, &schemaMap)

		tools = append(tools, proxy.Tool{
			Type: "function",
			Function: proxy.FunctionSchema{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  schemaMap,
			},
		})
	}
	return tools, nil
}

// CallTool executes a tool on the MCP server.
func (n *MCPNodeHerder) CallTool(ctx context.Context, name string, args map[string]any) (any, error) {
	result, err := n.orchestrator.CallTool(ctx, name, args)
	if err != nil {
		return nil, fmt.Errorf("tool call failed: %w", err)
	}

	if result.IsError {
		errMsg := "unknown tool error"
		if len(result.Content) > 0 {
			if textContent, ok := result.Content[0].(mcp_sdk.TextContent); ok {
				errMsg = textContent.Text
			}
		}
		return nil, fmt.Errorf("tool execution error: %s", errMsg)
	}

	if len(result.Content) == 0 {
		return nil, fmt.Errorf("empty tool result")
	}

	// Extract content. MCP supports list of content (text/image/embedded).
	// For LLM Proxy, we assume the first content block is the primary JSON response or Text.
	// We handle TextContent primarily.
	content := result.Content[0]
	var textStr string
	if textContent, ok := content.(mcp_sdk.TextContent); ok {
		textStr = textContent.Text
	} else {
		// Just marshal whatever it is (e.g. image?)
		return content, nil
	}

	// Try to unmarshal as JSON to return a structured object if possible,
	// because assistant handler expects 'any' which it can marshal back to JSON or inspect.
	var structured any
	if err := json.Unmarshal([]byte(textStr), &structured); err == nil {
		return structured, nil
	}

	// Return raw text if not JSON
	return textStr, nil
}
