package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"llm-proxy/internal/core/nodeherder"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"

	mcp_sdk "github.com/mark3labs/mcp-go/mcp"
)

type MCPNodeHerder struct {
	orchestrator *Orchestrator
	mirror       *ResourceMirror
	logger       logging.Logger
}

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

func (n *MCPNodeHerder) GetSystemPrompt() (string, error) {
	prompt := n.mirror.GetSystemPrompt()
	if !n.orchestrator.HasActiveClients() || prompt == "" {
		// Try to fetch explicitly if missing or no clients active (force refresh if reconnecting)
		content, err := n.orchestrator.ReadResource(context.Background(), "nodeherder://system-prompt")
		if err != nil {
			n.logger.Warn("System prompt not available, utilizing fallback", "error", err)
			return "You are a helpful assistant. Note: IoT device context service is currently disconnected.", nil
		}

		n.mirror.SetSystemPrompt(content)
		return content, nil
	}
	return prompt, nil
}

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
		schemaRequiredFirst(schemaMap)

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

func (n *MCPNodeHerder) CallTool(ctx context.Context, name string, args map[string]any) (any, error) {
	result, err := n.orchestrator.CallTool(ctx, name, args)
	if err != nil {
		return nil, fmt.Errorf("tool call failed: %w", err)
	}
	return toolResultToAny(result)
}

// toolResultToAny turns an MCP tool result into what the assistant handler expects: a tool error becomes an error that
// carries the server's message; the first content block is the primary response, returned as a structured object when
// its text is JSON, as text otherwise, and as it is when it is not text (an image, say).
func toolResultToAny(result *mcp_sdk.CallToolResult) (any, error) {
	if result.IsError {
		errMsg := "unknown tool error"
		if len(result.Content) > 0 {
			if textContent, ok := result.Content[0].(mcp_sdk.TextContent); ok {
				errMsg = textContent.Text
			}
		}
		return errMsg, fmt.Errorf("tool execution error: %s", errMsg)
	}
	if len(result.Content) == 0 {
		return nil, fmt.Errorf("empty tool result")
	}
	textContent, ok := result.Content[0].(mcp_sdk.TextContent)
	if !ok {
		return result.Content[0], nil
	}
	var structured any
	if err := json.Unmarshal([]byte(textContent.Text), &structured); err == nil {
		return structured, nil
	}
	return textContent.Text, nil
}

func (n *MCPNodeHerder) UseNativeTools() bool {
	return true
}

// schemaRequiredFirst puts a tool schema's required arguments first in its properties (see models.OrderedByRequired),
// because the schema reaches us already decoded into a map and a map has no order of its own. A schema without a
// properties object, or whose required list is not a list of names, is left as it is.
func schemaRequiredFirst(schema map[string]any) {
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return
	}
	var required []string
	if list, ok := schema["required"].([]any); ok {
		for _, item := range list {
			if name, ok := item.(string); ok {
				required = append(required, name)
			}
		}
	}
	schema["properties"] = models.OrderedByRequired(props, required)
}
