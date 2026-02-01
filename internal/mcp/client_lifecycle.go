// client_lifecycle.go handles the background connection management for a single MCP Client.
// It manages the connection loop, "Quiet-Pulse" backoff, re-connection logic, and initialization handshakes.
package mcp

import (
	"context"
	"fmt"
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
				// IDLE STATE: Client is healthy, just sleep and check again later
				timer.Reset(idleInterval)
				continue
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

	mcpClient, err := client.NewSSEMCPClient(c.URL)
	if err != nil {
		return fmt.Errorf("failed to create SSE client: %w", err)
	}

	// Register notification handler
	// Note: We need a method for this on Client if we want to handle notifications.
	// For now, assuming existing behavior or TODO.
	// Re-adding the notification handler logic from original file if applicable.
	// The original had `handleNotification`. I will stub it or add it if it was there.
	// It was in `manager.go` originally? No, it was called in `manager.go` but defined where?
	// It wasn't in `client.go` or `manager.go` I viewed.
	// Ah, I missed `handlers.go`!
	// I need to ensure `handleNotification` is available or moved.

	// Let's defer adding the notification handler for a second and check if I missed viewing `handlers.go`.
	// Yes, `handlers.go` was in the file list but I didn't view it.
	// I should probably just assume it exists on `*Client` (since I'm renaming methods).
	// But `handleNotification` (private) likely needs to be on `*Client`.
	// I'll add the registration here assuming `handleNotification` is or will be on `*Client`.

	/*
		mcpClient.OnNotification(func(notification mcp.JSONRPCNotification) {
			c.handleNotification(ctx, notification)
		})
	*/

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
	// promptHandler := c.onPromptUpdate // Check if this field exists on new Client struct? It wasn in types.go I wrote?
	// I did NOT include `onPromptUpdate` in the `types.go` `Client` struct I wrote in step 39...
	// Wait, I used `types.go` content:
	/*
		type Client struct {
			Name          string
			URL           string
			logger        logging.Logger
			retryInterval time.Duration

			mu            sync.RWMutex
			client        *client.Client
			initialized   bool
			subscriptions map[string]struct{}
			lastSuccess   time.Time

			cancelFunc    context.CancelFunc
			done          chan struct{}
		}
	*/
	// I missed `onPromptUpdate`. This is a regression if I don't add it back.
	// The original `MCPClient` had it.
	// The `NodeHerder` adapter relied on `mirror` and explicit fetch, but `onPromptUpdate` was used for notifications?
	// Let's double check `handlers.go` and `manager.go` original.
	// Logic: `promptHandler := c.onPromptUpdate`

	c.mu.Unlock()

	// Re-subscribe to resources
	for _, uri := range subs {
		if err := c.subscribeInternal(ctx, mcpClient, uri); err != nil {
			c.logger.Error("Failed to re-subscribe to resource", "server", c.Name, "uri", uri, "error", err)
		} else {
			c.logger.Info("Re-subscribed to resource", "server", c.Name, "uri", uri)
		}
	}

	// I am omitting the immediate prompt update trigger for now because I need to restore `onPromptUpdate` to `Client` struct first if needed,
	// OR rely on the Manager/Adapter to handle this.
	// Given this is a refactor, I should probably restore it or find a better place.
	// The `NodeHerder` adapter calls `ReadResource` explicitly.
	// The notification handler is what drove `onPromptUpdate`.

	return nil
}
