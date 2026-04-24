package assistant

import (
	"context"
	"fmt"
	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/assistant/prompts"
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
	Network         *tools.NetworkTools
}

func NewLocalToolRegistry(
	terminal *tools.TerminalTools,
	comm *tools.CommunicationTools,
	search *tools.InternetTools,
	fsTools *tools.FileSystemTools,
	network *tools.NetworkTools,
) *LocalToolRegistry {
	r := &LocalToolRegistry{
		handlers:      make(map[string]ToolHandler),
		Terminal:      terminal,
		Communication: comm,
		Search:        search,
		FileSystem:    fsTools,
		Network:       network,
	}
	r.registerAll()
	return r
}

// ToolHandler is a function that executes a local tool.
type ToolHandler func(ctx context.Context, rawArgs string) (any, error)

func InitializeAgentStack(
	appCtx interface {
		GetSystem() models.SystemConfig
		GetRegistry() models.RegistryData
		Resolver() storage.Resolver
		Secrets() models.SecretsStore
		GetGuardrails() models.AgentGuardrailsConfig
	},
	persistence *persistence.WorkspaceManager,
	mcp nodeherder.MCPService,
	logger logging.Logger,
	poolManager tools.SandboxProvider,
	observer tools.StreamObserver,
) (ToolProvider, Engine, *guardrails.GuardrailEngine) {
	resolver := appCtx.Resolver()
	// 1. Load defaults from settings/manifests
	defaultGuardrails := appCtx.GetGuardrails()
	terminal := tools.NewTerminalTools(func(ctx context.Context) models.TerminalGuardrailsConfig {
		cfg := defaultGuardrails.Terminal
		wsID := models.GetWorkspaceID(ctx)
		if wsID != "" && persistence != nil {
			if wsCfg, err := persistence.ReadConfig(wsID); err == nil && wsCfg.Guardrails != nil {
				cfg = wsCfg.Guardrails.Terminal
			}
		}
		return cfg
	}, func(workspaceID string) string {
		if workspaceID == "" || persistence == nil {
			return ""
		}
		return resolver.WorkspaceDir(workspaceID)
	})
	if poolManager != nil {
		terminal.SetSandboxProvider(poolManager, observer)
	}

	// 3. Initialize Guardrail Engine with granular merging
	// We want to use config.json values if they exist, otherwise fallback to defaults.
	grEngine := guardrails.NewGuardrailEngine(func() models.AgentGuardrailsConfig {
		return defaultGuardrails
	}, resolver, persistence)

	// 3. Initialize Communications
	reg := appCtx.GetRegistry()
	commCfg := reg.Communication
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

	// 6. Initialize Network
	network := tools.NewNetworkTools(func(ctx context.Context) models.NetworkGuardrailsConfig {
		cfg := defaultGuardrails.Network
		wsID := models.GetWorkspaceID(ctx)
		if wsID != "" && persistence != nil {
			if wsCfg, err := persistence.ReadConfig(wsID); err == nil && wsCfg.Guardrails != nil {
				cfg = wsCfg.Guardrails.Network
			}
		}
		return cfg
	})

	localRegistry := NewLocalToolRegistry(terminal, comm, search, fsTools, network)

	// 6. Aggregate Tools: Local Registry + Remote MCP
	provider := NewMultiToolProvider(localRegistry, mcp)
	mcpEngine := NewEngine(mcp, logger)
	engine := NewCompositeEngine(localRegistry, mcpEngine)

	return provider, engine, grEngine
}

// ListTools satisfies the ToolProvider interface.
func (r *LocalToolRegistry) ListTools(ctx context.Context) ([]proxy.Tool, error) {
	return r.toolDefinitions, nil
}

// GetSystemPrompt satisfies the ToolProvider interface.
func (r *LocalToolRegistry) GetSystemPrompt() (string, error) {
	return prompts.LocalAssistantPrompt, nil
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

func (r *LocalToolRegistry) addTool(toolKey string, toolName string) {
	params, desc, err := tools.LoadManifestAsTool("", toolKey, toolName)
	if err != nil {
		fmt.Printf("Warning: failed to load manifest for %s: %v\n", toolKey, err)
		return
	}

	r.toolDefinitions = append(r.toolDefinitions, proxy.Tool{
		Type: "function",
		Function: proxy.FunctionSchema{
			Name:        toolName,
			Description: desc,
			Parameters:  params,
		},
	})
}

func (r *LocalToolRegistry) registerAll() {
	r.registerTerminalTools()
	r.registerCommunicationTools()
	r.registerSearchTools()
	r.registerFileSystemTools()
	r.registerNetworkTools()
}

func (r *LocalToolRegistry) registerTerminalTools() {
	name := models.ToolTerminalExecute
	r.addTool("terminal", name)

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
	r.addTool("communication", name)

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
	r.addTool("search", name)

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
	r.addTool("filesystem", models.ToolDirectoryList)
	r.addTool("filesystem", models.ToolFileRead)
	r.addTool("filesystem", models.ToolFileWrite)

	r.handlers[models.ToolDirectoryList] = func(ctx context.Context, rawArgs string) (any, error) {
		var args struct {
			Path string `json:"path"`
		}
		if err := decodeArgs(rawArgs, &args); err != nil {
			return nil, err
		}
		return r.FileSystem.ListDirectory(ctx, args.Path)
	}

	r.handlers[models.ToolFileRead] = func(ctx context.Context, rawArgs string) (any, error) {
		var args struct {
			Path string `json:"path"`
		}
		if err := decodeArgs(rawArgs, &args); err != nil {
			return nil, err
		}
		return r.FileSystem.ReadFile(ctx, args.Path)
	}

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

func (r *LocalToolRegistry) registerNetworkTools() {
	r.addTool("network", models.ToolNetworkFetch)
	r.addTool("network", models.ToolNetworkScan)
	r.addTool("network", models.ToolNetworkInfo)

	r.handlers[models.ToolNetworkFetch] = func(ctx context.Context, rawArgs string) (any, error) {
		var args struct {
			URL string `json:"url"`
		}
		if err := decodeArgs(rawArgs, &args); err != nil {
			return nil, err
		}
		if r.Network == nil {
			return nil, fmt.Errorf("network tools not configured")
		}
		return r.Network.FetchURL(ctx, args.URL)
	}

	r.handlers[models.ToolNetworkScan] = func(ctx context.Context, rawArgs string) (any, error) {
		var args tools.ScanArgs
		if err := decodeArgs(rawArgs, &args); err != nil {
			return nil, err
		}
		if r.Network == nil {
			return nil, fmt.Errorf("network tools not configured")
		}
		return r.Network.ScanLocalNetwork(ctx, args)
	}

	r.handlers[models.ToolNetworkInfo] = func(ctx context.Context, rawArgs string) (any, error) {
		if r.Network == nil {
			return nil, fmt.Errorf("network tools not configured")
		}
		return r.Network.GetNetworkInfo(ctx)
	}
}
