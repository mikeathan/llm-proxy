package assistant

import (
	"context"
	"fmt"
	"llm-proxy/internal/core/nodeherder"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/core/tools"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
)

// LocalToolRegistry manages Go-based tools implemented in this repository.
type LocalToolRegistry struct {
	toolDefinitions []proxy.Tool
	handlers        map[string]ToolHandler
	terminal        *tools.TerminalTools
	communication   *tools.CommunicationTools
	search          *tools.InternetTools
	fs              *tools.FileSystemTools
}

func NewLocalToolRegistry(
	terminal *tools.TerminalTools,
	comm *tools.CommunicationTools,
	search *tools.InternetTools,
	fsTools *tools.FileSystemTools,
) *LocalToolRegistry {
	r := &LocalToolRegistry{
		handlers:      make(map[string]ToolHandler),
		terminal:      terminal,
		communication: comm,
		search:        search,
		fs:            fsTools,
	}
	r.registerAll()
	return r
}

// ToolHandler is a function that executes a local tool.
type ToolHandler func(ctx context.Context, rawArgs string) (any, error)

// InitializeAgentStack is a facade that wires up all tool providers and engines.
func InitializeAgentStack(
	appCtx interface {
		Config() *models.Config
	},
	mcp nodeherder.MCPService,
	logger logging.Logger,
) (ToolProvider, Engine, *GuardrailEngine) {
	cfg := appCtx.Config()

	// 1. Load defaults from manifests
	defaultGuardrails := tools.GetDefaultGuardrails()

	// 2. Initialize local machine capabilities
	terminal := tools.NewTerminalTools(func() models.TerminalGuardrailsConfig {
		tc := appCtx.Config().Guardrails.Terminal
		if !tc.Enabled && len(tc.AllowedCommands) == 0 {
			return defaultGuardrails.Terminal
		}
		return tc
	})

	// 3. Initialize Guardrail Engine with defaults from manifests
	// config.json now only needs to contain overrides (if any)
	guardrails := NewGuardrailEngine(func() models.AgentGuardrailsConfig {
		current := appCtx.Config().Guardrails
		// Merge logic: if a category is disabled in config, we keep it as is.
		// If it's empty, we might use the defaults.
		// For now, let's keep it simple: use defaults if config is empty.
		if !current.Terminal.Enabled && !current.FileSystem.Enabled && !current.Search.Enabled {
			return defaultGuardrails
		}
		return current
	})

	// 3. Initialize Communications
	commCfg := cfg.Communication
	comm := tools.NewCommunicationTools()
	if commCfg.Telegram.Enabled {
		comm.AddNotifier(&tools.TelegramNotifier{
			Token:  commCfg.Telegram.Token,
			ChatID: commCfg.Telegram.ChatID,
		})
	}

	// 4. Initialize Search
	searchCfg := cfg.Search
	var search *tools.InternetTools
	if searchCfg.TavilyKey != "" {
		search = tools.NewInternetTools(&tools.TavilyProvider{
			APIKey: searchCfg.TavilyKey,
		})
	}

	// 5. Initialize FileSystem
	fsTools := tools.NewFileSystemTools(func() models.FileSystemGuardrailsConfig {
		fc := appCtx.Config().Guardrails.FileSystem
		if !fc.Enabled && len(fc.AllowedPaths) == 0 {
			return defaultGuardrails.FileSystem
		}
		return fc
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

// ExecuteTool satisfies the Engine interface for local tools.
func (r *LocalToolRegistry) ExecuteTool(ctx context.Context, call proxy.ToolCall) (any, error) {
	handler, ok := r.handlers[call.Function.Name]
	if !ok {
		return nil, fmt.Errorf("tool %s not found in local registry", call.Function.Name)
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
	name := "execute_terminal_command"
	
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
		return r.terminal.ExecuteCommand(ctx, args.Command)
	}
}

func (r *LocalToolRegistry) registerCommunicationTools() {
	name := "notify_user"
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
		if r.communication == nil {
			return nil, fmt.Errorf("communication tools not configured")
		}
		return "Notification sent successfully", r.communication.NotifyAll(ctx, args.Message)
	}
}

func (r *LocalToolRegistry) registerSearchTools() {
	name := "internet_search"
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
		if r.search == nil {
			return nil, fmt.Errorf("search tools not configured")
		}
		return r.search.Search(ctx, args.Query)
	}
}

func (r *LocalToolRegistry) registerFileSystemTools() {
	// 1. List Directory
	r.toolDefinitions = append(r.toolDefinitions, proxy.Tool{
		Type: "function",
		Function: proxy.FunctionSchema{
			Name:        "list_directory",
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

	r.handlers["list_directory"] = func(ctx context.Context, rawArgs string) (any, error) {
		var args struct{ Path string `json:"path"` }
		if err := decodeArgs(rawArgs, &args); err != nil {
			return nil, err
		}
		return r.fs.ListDirectory(args.Path)
	}

	// 2. Read File
	r.toolDefinitions = append(r.toolDefinitions, proxy.Tool{
		Type: "function",
		Function: proxy.FunctionSchema{
			Name:        "read_file",
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

	r.handlers["read_file"] = func(ctx context.Context, rawArgs string) (any, error) {
		var args struct{ Path string `json:"path"` }
		if err := decodeArgs(rawArgs, &args); err != nil {
			return nil, err
		}
		return r.fs.ReadFile(args.Path)
	}

	// 3. Write File
	r.toolDefinitions = append(r.toolDefinitions, proxy.Tool{
		Type: "function",
		Function: proxy.FunctionSchema{
			Name:        "write_file",
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

	r.handlers["write_file"] = func(ctx context.Context, rawArgs string) (any, error) {
		var args struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := decodeArgs(rawArgs, &args); err != nil {
			return nil, err
		}
		return "File written successfully", r.fs.WriteFile(args.Path, args.Content)
	}
}


