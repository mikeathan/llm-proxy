// client_lifecycle.go handles the background connection management for a single MCP Client.
// It manages the connection loop, "Quiet-Pulse" backoff, re-connection logic, and initialization handshakes.
package mcp

import (
	"context"
	"fmt"
	"llm-proxy/internal/network"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// Start initiates the connection manager in a background goroutine.
// It returns immediately and handles connection/reconnection asynchronously.
func (c *Client) Start(ctx context.Context) {
	go c.manageConnection(ctx)
}

// manageConnection handles the connection lifecycle with "Quiet-Pulse" backoff.
func (c *Client) manageConnection(ctx context.Context) {
	defer close(c.done)

	const (
		minDelay     = 5 * time.Second
		maxDelay     = 5 * time.Minute
		idleInterval = 10 * time.Second
	)

	delay := minDelay
	failureCount := 0
	pingFailureCount := 0

	// Initial connection attempt
	if err := c.connect(ctx); err != nil {
		c.logger.Warn("Failed to establish initial MCP connection", "server", c.Name, "error", err)
		failureCount++
	} else {
		// Reset on success
		delay = minDelay
		failureCount = 0
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			c.mu.RLock()
			initialized := c.initialized
			c.mu.RUnlock()

			if initialized {
				// IDLE STATE: Client is healthy, check connection with Ping
				// Create a short timeout for the ping
				pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				err := c.client.Ping(pingCtx)
				cancel()

				if err != nil {
					pingFailureCount++
					if pingFailureCount <= 3 {
						c.logger.Warn("MCP connection unhealthy (ping failed)", "server", c.Name, "error", err, "attempt", pingFailureCount)
					} else {
						c.logger.Debug("MCP connection unhealthy (ping failed), suppressing logs", "server", c.Name, "error", err, "attempt", pingFailureCount)
					}

					// Force disconnect to trigger reconnection logic
					c.mu.Lock()
					c.initialized = false
					// Close client to ensure clean state
					if c.client != nil {
						_ = c.client.Close()
						c.client = nil
					}
					c.mu.Unlock()

					// Fall through to RECONNECT STATE immediately
				} else {
					// Healthy
					pingFailureCount = 0
					timer.Reset(idleInterval)
					continue
				}
			}

			// RECONNECT STATE: Client is down, try to connect
			err := c.connect(ctx)
			if err == nil {
				// SUCCESS: Reset backoff and failure count
				delay = minDelay
				failureCount = 0
				timer.Reset(idleInterval) // Go to idle check
				continue
			}

			// FAILURE: Handle backoff and logging
			failureCount++

			// Log based on failure count ("Quiet-Pulse")
			if failureCount <= 3 {
				c.logger.Warn("Failed to reconnect to MCP server", "server", c.Name, "error", err, "attempt", failureCount)
			} else if failureCount == 4 {
				c.logger.Info("MCP unreachable. Muting logs and retrying every 5 minutes in background.", "server", c.Name)
			}
			// For failureCount > 4, we suppress logs (Quiet mode)

			// Update delay with exponential backoff, capped at maxDelay
			if delay < maxDelay {
				delay *= 2
				if delay > maxDelay {
					delay = maxDelay
				}
			}

			// If we hit the cap, we stick to maxDelay
			timer.Reset(delay)
		}
	}
}

// connect attempts to establish the MCP connection.
func (c *Client) connect(ctx context.Context) error {
	c.logger.Info("Connecting to MCP server", "server", c.Name, "url", c.URL)

	origin := network.ResolveOrigin(c.BindAddr)
	mcpClient, err := client.NewSSEMCPClient(
		c.URL,
		client.WithHeaders(map[string]string{
			"Origin": origin,
		}),
	)
	if err != nil {
		return fmt.Errorf("failed to create SSE client: %w", err)
	}

	// Register connection lost handler
	mcpClient.OnConnectionLost(func(err error) {
		c.logger.Warn("MCP connection lost", "server", c.Name, "error", err)
		c.mu.Lock()
		c.initialized = false
		c.client = nil
		c.mu.Unlock()
	})

	// Start the transport
	if err := mcpClient.Start(ctx); err != nil {
		return fmt.Errorf("failed to start MCP client: %w", err)
	}
	c.logger.Info("MCP transport started", "server", c.Name)

	// Perform initialization handshake
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "llm-proxy",
		Version: "1.0.0",
	}
	initReq.Params.Capabilities = mcp.ClientCapabilities{}

	c.logger.Info("Sending MCP initialize request", "server", c.Name)
	result, err := mcpClient.Initialize(ctx, initReq)
	if err != nil {
		// Close client on failure to allow clean retry
		c.logger.Error("MCP initialize failed", "server", c.Name, "error", err)
		_ = mcpClient.Close()
		return fmt.Errorf("failed to initialize MCP session: %w", err)
	}

	c.logger.Info("MCP session initialized",
		"server", c.Name,
		"remote_server", result.ServerInfo.Name,
		"version", result.ServerInfo.Version,
		"protocol", result.ProtocolVersion,
	)

	// Atomic commit of the initialized client
	c.mu.Lock()
	c.client = mcpClient
	c.initialized = true
	c.lastSuccess = time.Now()
	// Copy subscriptions to local slice to avoid holding lock during network calls
	subs := make([]string, 0, len(c.subscriptions))
	for uri := range c.subscriptions {
		subs = append(subs, uri)
	}
	
	c.mu.Unlock()

	// Re-subscribe to resources
	for _, uri := range subs {
		if err := c.subscribeInternal(ctx, mcpClient, uri); err != nil {
			c.logger.Error("Failed to re-subscribe to resource", "server", c.Name, "uri", uri, "error", err)
		} else {
			c.logger.Info("Re-subscribed to resource", "server", c.Name, "uri", uri)
		}
	}

	// Verify tool availability
	verifyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tools, err := c.ListTools(verifyCtx)
	if err != nil {
		c.logger.Warn("Verification: Failed to list tools after connect", "server", c.Name, "error", err)
	} else {
		c.logger.Info("Verification: MCP connection ready", "server", c.Name, "tool_count", len(tools))
	}

	return nil
}
