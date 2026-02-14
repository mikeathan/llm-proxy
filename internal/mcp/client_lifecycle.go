// client_lifecycle.go handles the background connection management for a single MCP Client.
// It manages the connection loop, "Quiet-Pulse" backoff, re-connection logic, and initialization handshakes.
package mcp

import (
	"context"
	"fmt"
	"llm-proxy/internal/logger"
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
	// Wrap the client logger with PulseLogger for automatic suppression
	pulseLog := logger.NewPulseLogger(c.logger, c.Name)

	// Initial connection attempt
	pulseLog.Info("Connecting to MCP server", "server", c.Name, "url", c.URL)

	if err := c.connect(ctx, pulseLog); err != nil {
		pulseLog.Warn("Failed to establish initial MCP connection", "server", c.Name, "error", err)
	} else {
		// Reset logic handled by success in connect
		delay = minDelay
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
					pulseLog.Alive() // Ensure clean state before counting failure (defensive)
					// We reuse PulseLogger for pings. If ping fails, it's a failure.
					pulseLog.Warn("MCP connection unhealthy (ping failed)", "server", c.Name, "error", err)

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
					pulseLog.Alive()
					timer.Reset(idleInterval)
					continue
				}
			}

			// RECONNECT STATE: Client is down, try to connect
			pulseLog.Info("Connecting to MCP server", "server", c.Name, "url", c.URL)
			err := c.connect(ctx, pulseLog)
			if err == nil {
				// SUCCESS: Reset backoff and failure count
				delay = minDelay
				// Success logged in connect
				timer.Reset(idleInterval) // Go to idle check
				continue
			}

			// FAILURE: Handle backoff and logging
			pulseLog.Warn("Failed to reconnect to MCP server", "server", c.Name, "error", err)

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
func (c *Client) connect(ctx context.Context, logger *logger.PulseLogger) error {

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
		logger.Warn("MCP connection lost", "server", c.Name, "error", err)
		c.mu.Lock()

		c.initialized = false
		c.client = nil
		c.mu.Unlock()
	})

	// Start the transport
	if err := mcpClient.Start(ctx); err != nil {
		return fmt.Errorf("failed to start MCP client: %w", err)
	}
	logger.Info("MCP transport started", "server", c.Name)

	// Perform initialization handshake
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "llm-proxy",
		Version: "1.0.0",
	}
	initReq.Params.Capabilities = mcp.ClientCapabilities{}

	logger.Info("Sending MCP initialize request", "server", c.Name)
	result, err := mcpClient.Initialize(ctx, initReq)
	if err != nil {
		// Close client on failure to allow clean retry
		logger.Error("MCP initialize failed", "server", c.Name, "error", err)
		_ = mcpClient.Close()
		return fmt.Errorf("failed to initialize MCP session: %w", err)
	}

	logger.Success("MCP session initialized",
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
			logger.Error("Failed to re-subscribe to resource", "server", c.Name, "uri", uri, "error", err)
		} else {
			logger.Info("Re-subscribed to resource", "server", c.Name, "uri", uri)
		}
	}

	return nil
}
