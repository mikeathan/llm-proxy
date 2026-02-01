package mcp

import (
	"context"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// OnPromptUpdate registers a callback for system prompt resource updates.
// Note: This is an instance method on Client.
func (c *Client) OnPromptUpdate(handler func(content string)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onPromptUpdate = handler
}

// handleNotification processes incoming MCP notifications.
func (c *Client) handleNotification(ctx context.Context, notification mcp.JSONRPCNotification) {
	// Debug log if needed, but keep it quiet by default unless verbose
	// c.logger.Debug("Received MCP notification", "method", notification.Method)

	switch notification.Method {
	case "notifications/resources/updated":
		c.handleResourceUpdated(ctx, notification)
	default:
		c.logger.Debug("Unhandled notification", "server", c.Name, "method", notification.Method)
	}
}

// handleResourceUpdated processes resource update notifications.
func (c *Client) handleResourceUpdated(ctx context.Context, notification mcp.JSONRPCNotification) {
	// Extract the URI from the notification params
	params := notification.Params.AdditionalFields
	uri, ok := params["uri"].(string)
	if !ok {
		c.logger.Warn("Resource update notification missing URI", "server", c.Name)
		return
	}

	c.logger.Info("Resource updated", "server", c.Name, "uri", uri)

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
					c.logger.Error("Initial prompt sync failed after reconnect", "server", c.Name, "error", err)
					return
				}
				promptHandler(content)
				c.logger.Info("System-prompt sync complete after notification", "server", c.Name)
			}()
		}
	}
}
