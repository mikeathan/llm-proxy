package assistant

import (
	"context"
	"fmt"
	"llm-proxy/internal/core/nodeherder"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/core/tools"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
)

// LocalToolRegistry manages Go-based tools implemented in this repository.
type LocalToolRegistry struct {
	toolDefinitions []proxy.Tool
	handlers        map[string]ToolHandler
	Terminal        *tools.TerminalTools
	Communication   *tools.CommunicationTools
	Search          *tools.InternetTools
	FileSystem      *tools.FileSystemTools
}

func NewLocalToolRegistry(
	terminal *tools.TerminalTools,
	comm *tools.CommunicationTools,
	search *tools.InternetTools,
	fsTools *tools.FileSystemTools,
) *LocalToolRegistry {
	r := &LocalToolRegistry{
		handlers:      make(map[string]ToolHandler),
		Terminal:      terminal,
		Communication: comm,
		Search:        search,
		FileSystem:    fsTools,
	}
	r.registerAll()
	return r
}

// ToolHandler is a function that executes a local tool.
type ToolHandler func(ctx context.Context, rawArgs string) (any, error)

// InitializeAgentStack is a facade that wires up all tool providers and engines.
func InitializeAgentStack(
	appCtx interface {
		GetSystem() models.SystemConfig
		RootDir() string
		WorkspacesDir() string
		Secrets() models.SecretsStore
	},
	persistence *persistence.WorkspaceManager,
	mcp nodeherder.MCPService,
	logger logging.Logger,
) (ToolProvider, Engine, *GuardrailEngine) {
	sys := appCtx.GetSystem()
	rootDir := appCtx.RootDir()

	// 1. Load defaults from manifests (prefer disk if in dev)
	defaultGuardrails := tools.GetDefaultGuardrails(rootDir)

	// 2. Initialize local machine capabilities
	terminal := tools.NewTerminalTools(func(ctx context.Context) models.TerminalGuardrailsConfig {
		cfg := defaultGuardrails.Terminal
		wsID := models.GetWorkspaceID(ctx)
		if wsID != "" && persistence != nil {
			if wsCfg, err := persistence.ReadConfig(wsID); err == nil && wsCfg.Guardrails != nil {
				cfg = wsCfg.Guardrails.Terminal
			}
		}
		return cfg
	})

	// 3. Initialize Guardrail Engine with granular merging
	// We want to use config.json values if they exist, otherwise fallback to defaults.
	resolver := storage.NewPathResolver(appCtx.WorkspacesDir())
	guardrails := NewGuardrailEngine(func() models.AgentGuardrailsConfig {
		return defaultGuardrails
	}, resolver, persistence)

	// 3. Initialize Communications
	commCfg := sys.Communication
	comm := tools.NewCommunicationTools()
	telegramToken := appCtx.Secrets().GetSecret("communication", "telegram")
	if commCfg.Telegram.Enabled && telegramToken != "" {
		comm.AddNotifier(&tools.TelegramNotifier{
			Token:  telegramToken,
			ChatID: commCfg.Telegram.ChatID,
		})
	}

	// 4. Initialize Search
	var search *tools.InternetTools
	tavilyKey := appCtx.Secrets().GetSecret("search", "tavily")
	if tavilyKey != "" {
		search = tools.NewInternetTools(&tools.TavilyProvider{
			APIKey: tavilyKey,
		})
	}

	// 5. Initialize FileSystem
	fsTools := tools.NewFileSystemTools(func(ctx context.Context) models.FileSystemGuardrailsConfig {
		cfg := defaultGuardrails.FileSystem
		wsID := models.GetWorkspaceID(ctx)
		if wsID != "" {
			// 1. Apply workspace-specific overrides if they exist
			if persistence != nil {
				if wsCfg, err := persistence.ReadConfig(wsID); err == nil && wsCfg.Guardrails != nil {
					cfg = wsCfg.Guardrails.FileSystem
				}
			}
			// 2. STAMP the physical jailing path. This ensures the tool itself
			// considers its own workspace as an authorized path.
			cfg.AllowedPaths = append(cfg.AllowedPaths, resolver.WorkspaceDir(wsID))
		}
		return cfg
	})

	localRegistry := NewLocalToolRegistry(terminal, comm, search, fsTools)

	// 6. Aggregate Tools: Local Registry + Remote MCP
	provider := NewMultiToolProvider(localRegistry, mcp)
	mcpEngine := NewEngine(mcp, logger)
	engine := NewCompositeEngine(localRegistry, mcpEngine)

	return provider, engine, guardrails
}

// ListTools satisfies the ToolProvider interface.
func (r *LocalToolRegistry) ListTools(ctx context.Context) ([]proxy.Tool, error) {
	return r.toolDefinitions, nil
}

// GetSystemPrompt satisfies the ToolProvider interface.
func (r *LocalToolRegistry) GetSystemPrompt() (string, error) {
	return "You are a helpful assistant with access to local system tools and remote MCP services.", nil
}

var ErrToolNotInternal = fmt.Errorf("tool not found in local registry")

// ExecuteTool satisfies the Engine interface for local tools.
func (r *LocalToolRegistry) ExecuteTool(ctx context.Context, call proxy.ToolCall) (any, error) {
	handler, ok := r.handlers[call.Function.Name]
	if !ok {
		return nil, ErrToolNotInternal
	}
	return handler(ctx, call.Function.Arguments)
}

func (r *LocalToolRegistry) registerAll() {
	r.registerTerminalTools()
	r.registerCommunicationTools()
	r.registerSearchTools()
	r.registerFileSystemTools()
}

func (r *LocalToolRegistry) registerTerminalTools() {
	name := models.ToolTerminalExecute

	// 1. Define Schema
	r.toolDefinitions = append(r.toolDefinitions, proxy.Tool{
		Type: "function",
		Function: proxy.FunctionSchema{
			Name:        name,
			Description: "Execute a shell command on the host terminal and return the output.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{
						"type":        "string",
						"description": "The full shell command to execute (e.g., 'nmap -v 192.168.1.1').",
					},
				},
				"required": []string{"command"},
			},
		},
	})

	// 2. Register Handler
	r.handlers[name] = func(ctx context.Context, rawArgs string) (any, error) {
		var args struct {
			Command string `json:"command"`
		}
		if err := decodeArgs(rawArgs, &args); err != nil {
			return nil, err
		}
		return r.Terminal.ExecuteCommand(ctx, args.Command)
	}
}

func (r *LocalToolRegistry) registerCommunicationTools() {
	name := models.ToolNotifyUser
	r.toolDefinitions = append(r.toolDefinitions, proxy.Tool{
		Type: "function",
		Function: proxy.FunctionSchema{
			Name:        name,
			Description: "Send a message or notification to the user via configured platforms (e.g., Telegram).",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"message": map[string]any{
						"type":        "string",
						"description": "The message to send.",
					},
				},
				"required": []string{"message"},
			},
		},
	})

	r.handlers[name] = func(ctx context.Context, rawArgs string) (any, error) {
		var args struct {
			Message string `json:"message"`
		}
		if err := decodeArgs(rawArgs, &args); err != nil {
			return nil, err
		}
		if r.Communication == nil {
			return nil, fmt.Errorf("communication tools not configured")
		}
		return "Notification sent successfully", r.Communication.NotifyAll(ctx, args.Message)
	}
}

func (r *LocalToolRegistry) registerSearchTools() {
	name := models.ToolInternetSearch
	r.toolDefinitions = append(r.toolDefinitions, proxy.Tool{
		Type: "function",
		Function: proxy.FunctionSchema{
			Name:        name,
			Description: "Search the internet for real-time information, news, or technical details.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "The search query.",
					},
				},
				"required": []string{"query"},
			},
		},
	})

	r.handlers[name] = func(ctx context.Context, rawArgs string) (any, error) {
		var args struct {
			Query string `json:"query"`
		}
		if err := decodeArgs(rawArgs, &args); err != nil {
			return nil, err
		}
		if r.Search == nil {
			return nil, fmt.Errorf("search tools not configured")
		}
		return r.Search.Search(ctx, args.Query)
	}
}

func (r *LocalToolRegistry) registerFileSystemTools() {
	// 1. List Directory
	r.toolDefinitions = append(r.toolDefinitions, proxy.Tool{
		Type: "function",
		Function: proxy.FunctionSchema{
			Name:        models.ToolDirectoryList,
			Description: "List files and subdirectories in a specific path.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string", "description": "The directory path."},
				},
				"required": []string{"path"},
			},
		},
	})

	r.handlers[models.ToolDirectoryList] = func(ctx context.Context, rawArgs string) (any, error) {
		var args struct {
			Path string `json:"path"`
		}
		if err := decodeArgs(rawArgs, &args); err != nil {
			return nil, err
		}
		return r.FileSystem.ListDirectory(ctx, args.Path)
	}

	// 2. Read File
	r.toolDefinitions = append(r.toolDefinitions, proxy.Tool{
		Type: "function",
		Function: proxy.FunctionSchema{
			Name:        models.ToolFileRead,
			Description: "Read the entire content of a file.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string", "description": "The file path."},
				},
				"required": []string{"path"},
			},
		},
	})

	r.handlers[models.ToolFileRead] = func(ctx context.Context, rawArgs string) (any, error) {
		var args struct {
			Path string `json:"path"`
		}
		if err := decodeArgs(rawArgs, &args); err != nil {
			return nil, err
		}
		return r.FileSystem.ReadFile(ctx, args.Path)
	}

	// 3. Write File
	r.toolDefinitions = append(r.toolDefinitions, proxy.Tool{
		Type: "function",
		Function: proxy.FunctionSchema{
			Name:        models.ToolFileWrite,
			Description: "Create or update a file with specific content.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]any{"type": "string", "description": "The file path."},
					"content": map[string]any{"type": "string", "description": "The content to write."},
				},
				"required": []string{"path", "content"},
			},
		},
	})

	r.handlers[models.ToolFileWrite] = func(ctx context.Context, rawArgs string) (any, error) {
		var args struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := decodeArgs(rawArgs, &args); err != nil {
			return nil, err
		}
		return "File written successfully", r.FileSystem.WriteFile(ctx, args.Path, args.Content)
	}
}
