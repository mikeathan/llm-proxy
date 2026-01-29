// Package mcp provides an MCP client for connecting to the NodeHerder MCP server.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	"llm-proxy/internal/logging"
)

// MCPClient wraps the MCP SDK client and provides a simplified interface
// for interacting with the NodeHerder MCP server.
type MCPClient struct {
	client *client.Client
	logger logging.Logger
	sseURL string

	mu          sync.RWMutex
	initialized bool

	onDevicesUpdate func(content string)
	onPromptUpdate  func(content string)
}

// NewMCPClient creates a new MCP client configured to connect to the given SSE URL.
// The sseURL should point to the MCP events endpoint (e.g., http://localhost:4110/api/mcp/events).
func NewMCPClient(sseURL string, logger logging.Logger) *MCPClient {
	return &MCPClient{
		sseURL: sseURL,
		logger: logger,
	}
}

// Start connects to the MCP server via SSE and performs the initialization handshake.
func (c *MCPClient) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.initialized {
		return nil
	}

	c.logger.Info("Connecting to MCP server", "url", c.sseURL)

	mcpClient, err := client.NewSSEMCPClient(c.sseURL)
	if err != nil {
		return fmt.Errorf("failed to create SSE client: %w", err)
	}

	c.client = mcpClient

	// Register notification handler before starting
	c.client.OnNotification(c.handleNotification)

	// Register connection lost handler for reconnection
	c.client.OnConnectionLost(func(err error) {
		c.logger.Warn("MCP connection lost", "error", err)
		c.mu.Lock()
		c.initialized = false
		c.mu.Unlock()
		// TODO: Implement reconnection logic
	})

	// Start the transport
	if err := c.client.Start(ctx); err != nil {
		return fmt.Errorf("failed to start MCP client: %w", err)
	}

	// Perform initialization handshake
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "llm-proxy",
		Version: "1.0.0",
	}
	initReq.Params.Capabilities = mcp.ClientCapabilities{}

	result, err := c.client.Initialize(ctx, initReq)
	if err != nil {
		return fmt.Errorf("failed to initialize MCP session: %w", err)
	}

	c.logger.Info("MCP session initialized",
		"server", result.ServerInfo.Name,
		"version", result.ServerInfo.Version,
		"protocol", result.ProtocolVersion,
	)

	c.initialized = true
	return nil
}

// Close closes the MCP client connection.
func (c *MCPClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client != nil {
		// The mcp-go client doesn't have a Close method exposed directly
		// The connection will be cleaned up when the context is cancelled
		c.initialized = false
	}
	return nil
}

// IsInitialized returns whether the client has completed initialization.
func (c *MCPClient) IsInitialized() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.initialized
}

// ListResources retrieves the list of available resources from the MCP server.
func (c *MCPClient) ListResources(ctx context.Context) ([]mcp.Resource, error) {
	if !c.IsInitialized() {
		return nil, fmt.Errorf("MCP client not initialized")
	}

	result, err := c.client.ListResources(ctx, mcp.ListResourcesRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list resources: %w", err)
	}

	return result.Resources, nil
}

// ReadResource reads the content of a resource by URI.
func (c *MCPClient) ReadResource(ctx context.Context, uri string) (string, error) {
	if !c.IsInitialized() {
		return "", fmt.Errorf("MCP client not initialized")
	}

	req := mcp.ReadResourceRequest{}
	req.Params.URI = uri

	result, err := c.client.ReadResource(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to read resource %s: %w", uri, err)
	}

	if len(result.Contents) == 0 {
		return "", fmt.Errorf("no content returned for resource %s", uri)
	}

	// Extract text content from the first result
	content := result.Contents[0]
	if textContent, ok := content.(mcp.TextResourceContents); ok {
		return textContent.Text, nil
	}

	// Try to marshal as JSON if it's not a simple text content
	jsonBytes, err := json.Marshal(content)
	if err != nil {
		return "", fmt.Errorf("failed to parse resource content: %w", err)
	}
	return string(jsonBytes), nil
}

// Subscribe subscribes to updates for a resource URI.
func (c *MCPClient) Subscribe(ctx context.Context, uri string) error {
	if !c.IsInitialized() {
		return fmt.Errorf("MCP client not initialized")
	}

	req := mcp.SubscribeRequest{}
	req.Params.URI = uri

	if err := c.client.Subscribe(ctx, req); err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", uri, err)
	}

	c.logger.Info("Subscribed to resource", "uri", uri)
	return nil
}

// ListTools retrieves the list of available tools from the MCP server.
func (c *MCPClient) ListTools(ctx context.Context) ([]mcp.Tool, error) {
	if !c.IsInitialized() {
		return nil, fmt.Errorf("MCP client not initialized")
	}

	result, err := c.client.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list tools: %w", err)
	}

	return result.Tools, nil
}

// CallTool invokes a tool on the MCP server.
func (c *MCPClient) CallTool(ctx context.Context, name string, args map[string]any) (*mcp.CallToolResult, error) {
	if !c.IsInitialized() {
		return nil, fmt.Errorf("MCP client not initialized")
	}

	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args

	result, err := c.client.CallTool(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to call tool %s: %w", name, err)
	}

	return result, nil
}

// OnDevicesUpdate registers a callback for device resource updates.
func (c *MCPClient) OnDevicesUpdate(handler func(content string)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onDevicesUpdate = handler
}

// OnPromptUpdate registers a callback for system prompt resource updates.
func (c *MCPClient) OnPromptUpdate(handler func(content string)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onPromptUpdate = handler
}

// handleNotification processes incoming MCP notifications.
func (c *MCPClient) handleNotification(notification mcp.JSONRPCNotification) {
	c.logger.Debug("Received MCP notification", "method", notification.Method)

	switch notification.Method {
	case "notifications/resources/updated":
		c.handleResourceUpdated(notification)
	default:
		c.logger.Debug("Unhandled notification", "method", notification.Method)
	}
}

// handleResourceUpdated processes resource update notifications.
func (c *MCPClient) handleResourceUpdated(notification mcp.JSONRPCNotification) {
	// Extract the URI from the notification params
	params := notification.Params.AdditionalFields
	uri, ok := params["uri"].(string)
	if !ok {
		c.logger.Warn("Resource update notification missing URI")
		return
	}

	c.logger.Info("Resource updated", "uri", uri)

	// Trigger the appropriate callback
	c.mu.RLock()
	devicesHandler := c.onDevicesUpdate
	promptHandler := c.onPromptUpdate
	c.mu.RUnlock()

	switch uri {
	case "nodeherder://devices":
		if devicesHandler != nil {
			// Re-fetch the resource content
			ctx := context.Background()
			content, err := c.ReadResource(ctx, uri)
			if err != nil {
				c.logger.Error("Failed to re-fetch devices resource", "error", err)
				return
			}
			devicesHandler(content)
		}
	case "nodeherder://system-prompt":
		if promptHandler != nil {
			ctx := context.Background()
			content, err := c.ReadResource(ctx, uri)
			if err != nil {
				c.logger.Error("Failed to re-fetch system-prompt resource", "error", err)
				return
			}
			promptHandler(content)
		}
	}
}
