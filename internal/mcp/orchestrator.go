// orchestrator.go manages the set of active MCP Clients based on configuration.
// It acts as the Orchestrator, syncing the active clients with the config
// and providing aggregated methods to query all servers at once.
package mcp

import (
	"context"
	"fmt"

	"llm-proxy/internal/logging"
	"llm-proxy/models"

	"github.com/mark3labs/mcp-go/mcp"
)

// NewOrchestrator creates a new MCP Orchestrator.
func NewOrchestrator(logger logging.Logger) *Orchestrator {
	return &Orchestrator{
		logger:  logger,
		clients: make(map[string]*Client),
	}
}

// OnPromptUpdate registers a global callback for system prompt updates across all clients.
func (m *Orchestrator) OnPromptUpdate(handler func(content string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onPromptUpdate = handler

	// Propagate to existing clients?
	for _, c := range m.clients {
		c.OnPromptUpdate(handler)
	}
}

// Reload updates the pool of MCP clients based on the new configuration.
// It starts new clients, re-configures existing ones if needed, and stops removed ones.
func (m *Orchestrator) Reload(ctx context.Context, serverConfigs []models.MCPServerConfig, bindAddr string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	activeNames := make(map[string]bool)

	for _, cfg := range serverConfigs {
		if !cfg.Enabled {
			continue
		}
		activeNames[cfg.Name] = true

		existing, exists := m.clients[cfg.Name]
		if exists {
			// Check if URL changed
			if existing.URL != cfg.URL {
				m.logger.Info("MCP Server URL changed, restarting client", "name", cfg.Name, "old_url", existing.URL, "new_url", cfg.URL)
				existing.Stop()
				m.startClient(ctx, cfg, bindAddr)
			}
			// URL is same, do nothing
		} else {
			m.startClient(ctx, cfg, bindAddr)
		}
	}

	// Remove clients that are no longer in the config or disabled
	for name, client := range m.clients {
		if !activeNames[name] {
			m.logger.Info("MCP Server removed or disabled, stopping client", "name", name)
			client.Stop()
			delete(m.clients, name)
		}
	}
}

// startClient initializes and starts a new Client.
// Caller must hold lock.
func (m *Orchestrator) startClient(parentCtx context.Context, cfg models.MCPServerConfig, bindAddr string) {
	client := NewClient(cfg.Name, cfg.URL, bindAddr, m.logger)

	// Create a context for this client that can be cancelled
	ctx, cancel := context.WithCancel(parentCtx)
	client.cancelFunc = cancel

	// Propagate handlers
	if m.onPromptUpdate != nil {
		client.OnPromptUpdate(m.onPromptUpdate)
	}

	m.clients[cfg.Name] = client

	// Start background connection management
	client.Start(ctx)
}

// Close stops all clients.
func (m *Orchestrator) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, client := range m.clients {
		client.Stop()
	}
	m.clients = make(map[string]*Client)
}

// Aggregated Methods

// ListTools returns a combined list of tools from all available clients.
func (m *Orchestrator) ListTools(ctx context.Context) ([]mcp.Tool, error) {
	m.mu.RLock()
	clients := make([]*Client, 0, len(m.clients))
	for _, c := range m.clients {
		clients = append(clients, c)
	}
	m.mu.RUnlock()

	var allTools []mcp.Tool

	for _, c := range clients {
		if !c.IsInitialized() {
			continue
		}
		tools, err := c.ListTools(ctx)
		if err != nil {
			m.logger.Warn("Failed to list tools from server", "server", c.Name, "error", err)
			continue
		}
		allTools = append(allTools, tools...)
	}

	return allTools, nil
}

// CallTool attempts to call a tool on the appropriate server.
func (m *Orchestrator) CallTool(ctx context.Context, name string, args map[string]any) (*mcp.CallToolResult, error) {
	m.mu.RLock()
	clients := make([]*Client, 0, len(m.clients))
	for _, c := range m.clients {
		clients = append(clients, c)
	}
	m.mu.RUnlock()

	for _, c := range clients {
		if !c.IsInitialized() {
			continue
		}
		// Try to call on each Client.
		// Note: Ideally we check ListTools first.
		tools, err := c.ListTools(ctx)
		if err != nil {
			continue
		}
		for _, t := range tools {
			if t.Name == name {
				return c.CallTool(ctx, name, args)
			}
		}
	}

	return nil, fmt.Errorf("tool %s not found on any active MCP server", name)
}

// ReadResource attempts to read a resource from any server that has it.
// This is needed for the adapter.
func (m *Orchestrator) ReadResource(ctx context.Context, uri string) (string, error) {
	m.mu.RLock()
	clients := make([]*Client, 0, len(m.clients))
	for _, c := range m.clients {
		clients = append(clients, c)
	}
	m.mu.RUnlock()

	type result struct {
		content string
		err     error
	}

	// Buffered channel to avoid blocked goroutines
	ch := make(chan result, len(clients))

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	activeCount := 0
	for _, c := range clients {
		if !c.IsInitialized() {
			continue
		}
		activeCount++
		go func(c *Client) {
			content, err := c.ReadResource(ctx, uri)
			ch <- result{content: content, err: err}
		}(c)
	}

	if activeCount == 0 {
		return "", fmt.Errorf("resource %s not found (no active servers)", uri)
	}

	var lastErr error
	for i := 0; i < activeCount; i++ {
		res := <-ch
		if res.err == nil {
			return res.content, nil
		}
		// Ignore context canceled errors if they are due to our own cancel
		if ctx.Err() == nil {
			lastErr = res.err
		}
	}

	return "", fmt.Errorf("failed to read resource %s from any server: last error: %w", uri, lastErr)
}

// Subscribe registers a subscription for a resource on all applicable clients.
func (m *Orchestrator) Subscribe(ctx context.Context, uri string) {
	m.mu.RLock()
	clients := make([]*Client, 0, len(m.clients))
	for _, c := range m.clients {
		clients = append(clients, c)
	}
	m.mu.RUnlock()

	for _, c := range clients {
		if err := c.Subscribe(ctx, uri); err != nil {
			m.logger.Warn("Failed to subscribe to resource", "server", c.Name, "uri", uri, "error", err)
		}
	}
}
