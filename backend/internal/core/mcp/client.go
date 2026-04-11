// client.go handles the operational methods of a single MCP Client,
// such as calling tools, listing resources, and reading resources.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"

	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/network"
)

// getKeepAliveInterval reads the TCP KeepAlive interval from environment variable
// MCP_KEEPALIVE_INTERVAL. If missing or invalid, defaults to 15 seconds.
// This interval is optimized for LAN/Wi-Fi environments to prevent NAT state-table
// expiry and Wi-Fi chip sleep cycles that can silently drop idle TCP connections.
func getKeepAliveInterval() time.Duration {
	if val := os.Getenv("MCP_KEEPALIVE_INTERVAL"); val != "" {
		if seconds, err := strconv.Atoi(val); err == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
	}
	return 15 * time.Second
}

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
	pulseLog := logging.NewPulseLogger(c.logger, c.Name)

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
				pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
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
func (c *Client) connect(ctx context.Context, logger *logging.PulseLogger) error {

	origin := network.ResolveOrigin(c.BindAddr)

	// Create a custom HTTP client with aggressive TCP keep-alives to prevent
	// silent connection drops by network infrastructure (NATs, Firewalls, Wi-Fi sleep)
	dialer := &net.Dialer{
		Timeout: 30 * time.Second,
		// KeepAlive is the OS-level TCP heartbeat.
		// 15s is often safer than 10s to avoid aggressive triggers on busy networks.
		// Optimized for LAN/Wi-Fi to prevent NAT state-table expiry and Wi-Fi chip sleep cycles.
		KeepAlive: getKeepAliveInterval(),
	}

	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		DialContext:         dialer.DialContext,
		ForceAttemptHTTP2:   true,
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 2, // Limits reuse to prevent "sticky" dead connections
		// Set IdleConnTimeout to 0 or a very high value.
		// We want the TCP layer, not the HTTP layer, to manage "idleness".
		IdleConnTimeout:       0,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	httpClient := &http.Client{
		Transport: transport,
		// CRITICAL: Do NOT set Timeout here, it will kill the SSE stream.
	}

	mcpClient, err := client.NewSSEMCPClient(
		c.URL,
		client.WithHeaders(map[string]string{
			"Origin": origin,
		}),
		client.WithHTTPClient(httpClient),
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
		subCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := c.subscribeInternal(subCtx, mcpClient, uri)
		cancel()

		if err != nil {
			logger.Error("Failed to re-subscribe to resource", "server", c.Name, "uri", uri, "error", err)
		} else {
			logger.Info("Re-subscribed to resource", "server", c.Name, "uri", uri)
		}
	}

	return nil
}

func (c *Client) IsInitialized() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.initialized
}

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

func (c *Client) OnPromptUpdate(handler func(content string)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onPromptUpdate = handler
}

func (c *Client) handleNotification(ctx context.Context, notification mcp.JSONRPCNotification) {

	switch notification.Method {
	case "notifications/resources/updated":
		c.handleResourceUpdated(ctx, notification)
	default:
		c.logger.Debug("Unhandled notification", "server", c.Name, "method", notification.Method)
	}
}

func (c *Client) handleResourceUpdated(ctx context.Context, notification mcp.JSONRPCNotification) {
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
