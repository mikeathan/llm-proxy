// Package mcp provides an MCP client for connecting to the NodeHerder MCP server.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

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

	mu            sync.RWMutex
	initialized   bool
	subscriptions map[string]struct{}

	onPromptUpdate func(content string)

	retryInterval time.Duration
}

// NewMCPClient creates a new MCP client configured to connect to the given SSE URL.
// The sseURL should point to the MCP events endpoint (e.g., http://localhost:4110/api/mcp/events).
func NewMCPClient(sseURL string, logger logging.Logger) *MCPClient {
	return &MCPClient{
		sseURL:        sseURL,
		logger:        logger,
		retryInterval: 5 * time.Second,
		subscriptions: make(map[string]struct{}),
	}
}

// Start initiates the connection manager in a background goroutine.
// It returns immediately and handles connection/reconnection asynchronously.
func (c *MCPClient) Start(ctx context.Context) {
	go c.manageConnection(ctx)
}

// manageConnection handles the connection lifecycle, including retries.
func (c *MCPClient) manageConnection(ctx context.Context) {
	// Initial connection attempt
	if err := c.connect(ctx); err != nil {
		c.logger.Warn("Failed to establish initial MCP connection", "error", err)
	}

	ticker := time.NewTicker(c.retryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.mu.RLock()
			initialized := c.initialized
			c.mu.RUnlock()

			if !initialized {
				if err := c.connect(ctx); err != nil {
					c.logger.Warn("Failed to reconnect to MCP server", "error", err)
				}
			}
		}
	}
}

// connect attempts to establish the MCP connection.
func (c *MCPClient) connect(ctx context.Context) error {
	c.logger.Info("Connecting to MCP server", "url", c.sseURL)

	mcpClient, err := client.NewSSEMCPClient(c.sseURL)
	if err != nil {
		return fmt.Errorf("failed to create SSE client: %w", err)
	}

	// Register notification handler
	mcpClient.OnNotification(func(notification mcp.JSONRPCNotification) {
		c.handleNotification(ctx, notification)
	})

	// Register connection lost handler
	mcpClient.OnConnectionLost(func(err error) {
		c.logger.Warn("MCP connection lost", "error", err)
		c.mu.Lock()
		c.initialized = false
		c.client = nil
		c.mu.Unlock()
	})

	// Start the transport
	if err := mcpClient.Start(ctx); err != nil {
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

	result, err := mcpClient.Initialize(ctx, initReq)
	if err != nil {
		// Close client on failure to allow clean retry
		_ = mcpClient.Close()
		return fmt.Errorf("failed to initialize MCP session: %w", err)
	}

	c.logger.Info("MCP session initialized",
		"server", result.ServerInfo.Name,
		"version", result.ServerInfo.Version,
		"protocol", result.ProtocolVersion,
	)

	// Atomic commit of the initialized client
	c.mu.Lock()
	c.client = mcpClient
	c.initialized = true
	// Copy subscriptions to local slice to avoid holding lock during network calls
	subs := make([]string, 0, len(c.subscriptions))
	for uri := range c.subscriptions {
		subs = append(subs, uri)
	}
	c.mu.Unlock()

	// Re-subscribe to resources
	for _, uri := range subs {
		if err := c.subscribeInternal(ctx, mcpClient, uri); err != nil {
			c.logger.Error("Failed to re-subscribe to resource", "uri", uri, "error", err)
		} else {
			c.logger.Info("Re-subscribed to resource", "uri", uri)
		}
	}

	return nil
}

// Close closes the MCP client connection.
func (c *MCPClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client != nil {
		// The mcp-go client doesn't have a Close method exposed directly that we can call safely if we want to reuse?
		// Actually it does, we just invoke it on the current instance.
		// However, manageConnection uses ctx.Done() to stop.
		// We explicitly mark as uninitialized.
		c.initialized = false
		// We can try to close it if the SDK supports it nicely
		// _ = c.client.Close()
		// But for now, just clearing state is enough as context cancel will clean up underlying transport.
		c.client = nil
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
	c.mu.RLock()
	client := c.client
	initialized := c.initialized
	c.mu.RUnlock()

	if !initialized || client == nil {
		return nil, fmt.Errorf("MCP client not initialized")
	}

	result, err := client.ListResources(ctx, mcp.ListResourcesRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list resources: %w", err)
	}

	return result.Resources, nil
}

// ReadResource reads the content of a resource by URI.
func (c *MCPClient) ReadResource(ctx context.Context, uri string) (string, error) {
	c.mu.RLock()
	client := c.client
	initialized := c.initialized
	c.mu.RUnlock()

	if !initialized || client == nil {
		return "", fmt.Errorf("MCP client not initialized")
	}

	req := mcp.ReadResourceRequest{}
	req.Params.URI = uri

	result, err := client.ReadResource(ctx, req)
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
	c.mu.Lock()
	c.subscriptions[uri] = struct{}{}
	client := c.client
	initialized := c.initialized
	c.mu.Unlock()

	if !initialized || client == nil {
		// We recorded the subscription, so it will be picked up on next connect
		c.logger.Info("Subscription queued (client not ready)", "uri", uri)
		return nil
	}

	if err := c.subscribeInternal(ctx, client, uri); err != nil {
		return err
	}

	c.logger.Info("Subscribed to resource", "uri", uri)
	return nil
}

func (c *MCPClient) subscribeInternal(ctx context.Context, client *client.Client, uri string) error {
	req := mcp.SubscribeRequest{}
	req.Params.URI = uri

	if err := client.Subscribe(ctx, req); err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", uri, err)
	}
	return nil
}

// ListTools retrieves the list of available tools from the MCP server.
func (c *MCPClient) ListTools(ctx context.Context) ([]mcp.Tool, error) {
	c.mu.RLock()
	client := c.client
	initialized := c.initialized
	c.mu.RUnlock()

	if !initialized || client == nil {
		return nil, fmt.Errorf("MCP client not initialized")
	}

	result, err := client.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list tools: %w", err)
	}

	return result.Tools, nil
}

// CallTool invokes a tool on the MCP server.
func (c *MCPClient) CallTool(ctx context.Context, name string, args map[string]any) (*mcp.CallToolResult, error) {
	c.mu.RLock()
	client := c.client
	initialized := c.initialized
	c.mu.RUnlock()

	if !initialized || client == nil {
		return nil, fmt.Errorf("MCP client not initialized")
	}

	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args

	result, err := client.CallTool(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to call tool %s: %w", name, err)
	}

	return result, nil
}

// OnPromptUpdate registers a callback for system prompt resource updates.
func (c *MCPClient) OnPromptUpdate(handler func(content string)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onPromptUpdate = handler
}

// handleNotification processes incoming MCP notifications.
func (c *MCPClient) handleNotification(ctx context.Context, notification mcp.JSONRPCNotification) {
	c.logger.Debug("Received MCP notification", "method", notification.Method)

	switch notification.Method {
	case "notifications/resources/updated":
		c.handleResourceUpdated(ctx, notification)
	default:
		c.logger.Debug("Unhandled notification", "method", notification.Method)
	}
}

// handleResourceUpdated processes resource update notifications.
func (c *MCPClient) handleResourceUpdated(ctx context.Context, notification mcp.JSONRPCNotification) {
	// Extract the URI from the notification params
	params := notification.Params.AdditionalFields
	uri, ok := params["uri"].(string)
	if !ok {
		c.logger.Warn("Resource update notification missing URI")
		return
	}

	c.logger.Info("Resource updated", "uri", uri)

	switch uri {
	case "nodeherder://system-prompt":
		c.mu.RLock()
		promptHandler := c.onPromptUpdate
		c.mu.RUnlock()

		if promptHandler != nil {
			// Fetch and update using the lifecycle context
			go func() {
				// Use a timeout for the fetch operation to avoid hanging
				fetchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				defer cancel()

				content, err := c.ReadResource(fetchCtx, uri)
				if err != nil {
					c.logger.Error("Initial prompt sync failed after reconnect", "error", err)
					return
				}
				promptHandler(content)
				c.logger.Info("Initial system-prompt sync complete after reconnection")
			}()
		}
	}
}
