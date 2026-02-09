// client.go handles the operational methods of a single MCP Client,
// such as calling tools, listing resources, and reading resources.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	"llm-proxy/internal/logging"
)

// NewClient creates a new MCP Client for a specific server.
func NewClient(name, sseURL, bindAddr string, logger logging.Logger) *Client {
	return &Client{
		Name:          name,
		URL:           sseURL,
		BindAddr:      bindAddr,
		logger:        logger,
		retryInterval: 5 * time.Second,
		subscriptions: make(map[string]struct{}),
		done:          make(chan struct{}),
	}
}

// Close closes the MCP client connection and stops the manager.
func (c *Client) Stop() {
	// Cancel the context to stop the manager loop
	if c.cancelFunc != nil {
		c.cancelFunc()
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Close the actual client
	if c.client != nil {
		_ = c.client.Close()
		c.client = nil
	}
	c.initialized = false
}

// IsInitialized returns whether the client has completed initialization.
func (c *Client) IsInitialized() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.initialized
}

// ListResources retrieves the list of available resources from the MCP server.
func (c *Client) ListResources(ctx context.Context) ([]mcp.Resource, error) {
	c.mu.RLock()
	client := c.client
	initialized := c.initialized
	c.mu.RUnlock()

	if !initialized || client == nil {
		return nil, fmt.Errorf("MCP client %s not initialized", c.Name)
	}

	result, err := client.ListResources(ctx, mcp.ListResourcesRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list resources from %s: %w", c.Name, err)
	}

	return result.Resources, nil
}

// ReadResource reads the content of a resource by URI.
func (c *Client) ReadResource(ctx context.Context, uri string) (string, error) {
	c.mu.RLock()
	client := c.client
	initialized := c.initialized
	c.mu.RUnlock()

	if !initialized || client == nil {
		return "", fmt.Errorf("MCP client %s not initialized", c.Name)
	}

	req := mcp.ReadResourceRequest{}
	req.Params.URI = uri

	result, err := client.ReadResource(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to read resource %s from %s: %w", uri, c.Name, err)
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
func (c *Client) Subscribe(ctx context.Context, uri string) error {
	c.mu.Lock()
	c.subscriptions[uri] = struct{}{}
	client := c.client
	initialized := c.initialized
	c.mu.Unlock()

	if !initialized || client == nil {
		// We recorded the subscription, so it will be picked up on next connect
		c.logger.Info("Subscription queued (client not ready)", "server", c.Name, "uri", uri)
		return nil
	}

	if err := c.subscribeInternal(ctx, client, uri); err != nil {
		return err
	}

	c.logger.Info("Subscribed to resource", "server", c.Name, "uri", uri)
	return nil
}

func (c *Client) subscribeInternal(ctx context.Context, client *client.Client, uri string) error {
	req := mcp.SubscribeRequest{}
	req.Params.URI = uri

	if err := client.Subscribe(ctx, req); err != nil {
		return fmt.Errorf("failed to subscribe to %s: %w", uri, err)
	}
	return nil
}

// ListTools retrieves the list of available tools from the MCP server.
func (c *Client) ListTools(ctx context.Context) ([]mcp.Tool, error) {
	c.mu.RLock()
	client := c.client
	initialized := c.initialized
	c.mu.RUnlock()

	if !initialized || client == nil {
		return nil, fmt.Errorf("MCP client %s not initialized", c.Name)
	}

	result, err := client.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list tools from %s: %w", c.Name, err)
	}

	c.logger.Debug("ListTools called", "server", c.Name, "count", len(result.Tools))
	return result.Tools, nil
}

// CallTool invokes a tool on the MCP server.
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (*mcp.CallToolResult, error) {
	c.mu.RLock()
	client := c.client
	initialized := c.initialized
	c.mu.RUnlock()

	if !initialized || client == nil {
		return nil, fmt.Errorf("MCP client %s not initialized", c.Name)
	}

	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args

	result, err := client.CallTool(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to call tool %s on %s: %w", name, c.Name, err)
	}

	return result, nil
}
