package mcp

import (
	"context"
	"sync"
	"time"

	"llm-proxy/internal/logging"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// Client represents a connection to a single MCP server.
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

	onPromptUpdate func(content string)

	cancelFunc context.CancelFunc // Used to stop the background manager
	done       chan struct{}      // Closed when manager loop exits
}

// Orchestrator (formerly ClientPool) orchestrates multiple MCP Clients.
type Orchestrator struct {
	logger         logging.Logger
	mu             sync.RWMutex
	clients        map[string]*Client // Keyed by server name
	onPromptUpdate func(content string)
}

// ClientInterface allows mocking the client or using the real one.
type ClientInterface interface {
	ListResources(ctx context.Context) ([]mcp.Resource, error)
	ReadResource(ctx context.Context, uri string) (string, error)
	CallTool(ctx context.Context, name string, args map[string]any) (*mcp.CallToolResult, error)
	ListTools(ctx context.Context) ([]mcp.Tool, error)
}
