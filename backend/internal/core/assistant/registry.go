// registry.go — LocalToolRegistry, all tool registration, and
// InitializeAgentStack (the top-level wiring function).  The init helper
// functions (initTerminalTools, initCommunicationTools, etc.) are extracted
// here to keep InitializeAgentStack as a short orchestration function.
package assistant

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"llm-proxy/internal/core"
	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/nodeherder"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/core/tools"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/memory"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/internal/shell"
	"llm-proxy/models"
)

// readConfigFunc loads a workspace config, potentially from a cache.
type readConfigFunc func(workspaceID string) (*models.WorkspaceConfig, error)

// workspaceConfigCache bounds ReadConfig results per workspace, invalidated by
// file mtime and bounded by size + TTL to prevent unbounded growth (PL-3).
const (
	workspaceConfigCacheMaxEntries = 100
	workspaceConfigCacheTTL        = 5 * time.Minute
)

// workspaceConfigEntry is the cached value: the config plus the mtime it was
// read at, so the validity predicate can detect out-of-band file edits.
type workspaceConfigEntry struct {
	config  *models.WorkspaceConfig
	modTime time.Time
}

// newCachedConfigReader builds a readConfigFunc that caches per-workspace
// config reads, invalidated by mtime and bounded by TTL + size (PL-3).
func newCachedConfigReader(persistence *persistence.WorkspaceManager, resolver storage.Resolver) readConfigFunc {
	mtimeValid := func(wsID string, e *workspaceConfigEntry) bool {
		info, err := os.Stat(resolver.Config(wsID))
		return err == nil && info.ModTime().Equal(e.modTime)
	}
	cache := core.NewTTLCache[string, *workspaceConfigEntry](workspaceConfigCacheMaxEntries, workspaceConfigCacheTTL, mtimeValid)
	return func(wsID string) (*models.WorkspaceConfig, error) {
		e, err := cache.Get(wsID, func() (*workspaceConfigEntry, error) {
			cfg, err := persistence.ReadConfig(wsID)
			if err != nil {
				return nil, err
			}
			// ReadConfig and this Stat resolve to the same path, so a stat
			// failure here is anomalous. Serve the freshly read config but
			// decline caching (ErrNoCache) so a broken stat can never occupy
			// a bounded cache slot with an entry that can never be a hit.
			info, serr := os.Stat(resolver.Config(wsID))
			if serr != nil {
				return &workspaceConfigEntry{config: cfg}, core.ErrNoCache
			}
			return &workspaceConfigEntry{config: cfg, modTime: info.ModTime()}, nil
		})
		if err != nil {
			return nil, err
		}
		return e.config, nil
	}
}

// getEffectiveConfig retrieves the active configuration for a tool, prioritizing workspace-level overrides merged with defaults.
func getEffectiveConfig[T any](
	ctx context.Context,
	readConfig readConfigFunc,
	defaults models.AgentGuardrailsConfig,
	getSpecific func(*models.AgentGuardrailsConfig) T,
) T {
	wsID := models.GetWorkspaceID(ctx)
	if wsID != "" {
		if wsCfg, err := readConfig(wsID); err == nil && wsCfg.Guardrails != nil {
			// Merge workspace overrides into a copy of system defaults
			merged := defaults
			merged.MergeWith(wsCfg.Guardrails)
			return getSpecific(&merged)
		}
	}
	return getSpecific(&defaults)
}

// registerTool simplifies the addition of a local tool and its handler to the registry.
func registerTool[T any](r *LocalToolRegistry, category, toolName string, fn func(context.Context, T) (any, error)) {
	r.addTool(category, toolName)
	r.handlers[toolName] = func(ctx context.Context, rawArgs string) (any, error) {
		var args T
		if err := decodeArgs(rawArgs, &args); err != nil {
			return nil, err
		}
		return fn(ctx, args)
	}
}

// LocalToolRegistry manages Go-based tools implemented in this repository.
type LocalToolRegistry struct {
	toolDefinitions []proxy.Tool
	handlers        map[string]ToolHandler
	Terminal        *tools.TerminalTools
	Communication   *tools.CommunicationTools
	Search          *tools.InternetTools
	FileSystem      *tools.FileSystemTools
	Network         *tools.NetworkTools
	Memory          *tools.MemoryToolProvider
}

func NewLocalToolRegistry(
	terminal *tools.TerminalTools,
	comm *tools.CommunicationTools,
	search *tools.InternetTools,
	fsTools *tools.FileSystemTools,
	network *tools.NetworkTools,
	memoryTools *tools.MemoryToolProvider,
) *LocalToolRegistry {
	r := &LocalToolRegistry{
		handlers:      make(map[string]ToolHandler),
		Terminal:      terminal,
		Communication: comm,
		Search:        search,
		FileSystem:    fsTools,
		Network:       network,
		Memory:        memoryTools,
	}
	r.registerAll()
	return r
}

// ToolHandler is a function that executes a local tool.
type ToolHandler func(ctx context.Context, rawArgs string) (any, error)

func initTerminalTools(
	resolver storage.Resolver,
	persistence *persistence.WorkspaceManager,
	readConfig readConfigFunc,
	defaultGuardrails models.AgentGuardrailsConfig,
	shellManager shell.ShellProvider,
	observer tools.StreamObserver,
) *tools.TerminalTools {
	terminal := tools.NewTerminalTools(func(ctx context.Context) models.TerminalGuardrailsConfig {
		return getEffectiveConfig(ctx, readConfig, defaultGuardrails, func(c *models.AgentGuardrailsConfig) models.TerminalGuardrailsConfig {
			return c.Terminal
		})
	}, func(workspaceID string) string {
		if workspaceID == "" || persistence == nil {
			return ""
		}
		return resolver.WorkspaceDir(workspaceID)
	})
	if shellManager != nil {
		terminal.SetShellProvider(shellManager, observer)
	}
	return terminal
}

func initCommunicationTools(appCtx interface {
	GetRegistry() models.RegistryData
	Secrets() models.SecretsStore
}, network *tools.NetworkTools) *tools.CommunicationTools {
	reg := appCtx.GetRegistry()
	comm := tools.NewCommunicationTools()
	for name, cfg := range reg.Communication.Connectors {
		if !cfg.Enabled {
			continue
		}
		conn, ok := buildConnector(name, cfg, appCtx.Secrets(), network)
		if !ok {
			continue
		}
		comm.AddConnector(name, cfg.Type, conn)
		scheduleWebhookReregistration(name, cfg, conn)
	}
	return comm
}

// buildConnector resolves and instantiates the registered factory for a
// connector config entry. ok=false means the type is unknown or its required
// credentials are missing, and the connector is skipped.
func buildConnector(name string, cfg models.ConnectorConfig, secrets models.SecretsStore, network *tools.NetworkTools) (tools.Connector, bool) {
	factory, ok := tools.GetConnectorFactory(cfg.Type)
	if !ok {
		logging.Warn("unknown communication connector type", "name", name, "type", cfg.Type)
		return nil, false
	}
	return factory(name, cfg, secrets, network)
}

// scheduleWebhookReregistration best-effort re-applies a connector's stored
// webhook URL on startup. Connectors that don't implement tools.WebhookAware are
// skipped. Failures are logged but never block startup.
func scheduleWebhookReregistration(name string, cfg models.ConnectorConfig, conn tools.Connector) {
	if cfg.WebhookURL == "" {
		return
	}
	wa, ok := conn.(tools.WebhookAware)
	if !ok {
		return
	}
	go func() {
		regCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := wa.RegisterWebhook(regCtx, cfg.WebhookURL, cfg.Settings["webhook_token"]); err != nil {
			logging.Warn("failed to re-register webhook on startup", "connector", name, "error", err)
		} else {
			logging.Info("re-registered webhook on startup", "connector", name, "url", cfg.WebhookURL)
		}
	}()
}

func initNetworkTools(
	persistence *persistence.WorkspaceManager,
	readConfig readConfigFunc,
	defaultGuardrails models.AgentGuardrailsConfig,
	logger logging.Logger,
) *tools.NetworkTools {
	return tools.NewNetworkTools(func(ctx context.Context) models.NetworkGuardrailsConfig {
		return getEffectiveConfig(ctx, readConfig, defaultGuardrails, func(c *models.AgentGuardrailsConfig) models.NetworkGuardrailsConfig {
			return c.Network
		})
	}, logger)
}

func initSearchTools(appCtx interface {
	Secrets() models.SecretsStore
}, network *tools.NetworkTools) *tools.InternetTools {
	tavilyKey := appCtx.Secrets().GetSecret("search", "tavily")
	if tavilyKey == "" {
		return nil
	}
	return tools.NewInternetTools(&tools.TavilyProvider{
		APIKey: tavilyKey,
		Client: network.HTTPClient(),
	})
}

func initMemoryTools(store *memory.Store) *tools.MemoryToolProvider {
	if store == nil {
		return nil
	}
	return tools.NewMemoryToolProvider(store)
}

func initFileSystemTools(
	resolver storage.Resolver,
	persistence *persistence.WorkspaceManager,
	readConfig readConfigFunc,
	defaultGuardrails models.AgentGuardrailsConfig,
) *tools.FileSystemTools {
	return tools.NewFileSystemTools(func(ctx context.Context) models.FileSystemGuardrailsConfig {
		cfg := getEffectiveConfig(ctx, readConfig, defaultGuardrails, func(c *models.AgentGuardrailsConfig) models.FileSystemGuardrailsConfig {
			return c.FileSystem
		})
		allowed := make([]string, 0, len(cfg.AllowedPaths)+1)
		if wsID := models.GetWorkspaceID(ctx); wsID != "" {
			wsPath := resolver.WorkspaceDir(wsID)
			allowed = append(allowed, wsPath)
		}
		allowed = append(allowed, cfg.AllowedPaths...)
		cfg.AllowedPaths = allowed
		return cfg
	})
}

func InitializeAgentStack(
	appCtx interface {
		GetSystem() models.SystemConfig
		GetRegistry() models.RegistryData
		Resolver() storage.Resolver
		Secrets() models.SecretsStore
		GetGuardrails() models.AgentGuardrailsConfig
		MemoryStore() *memory.Store
	},
	persistence *persistence.WorkspaceManager,
	mcp nodeherder.MCPService,
	logger logging.Logger,
	shellManager shell.ShellProvider,
	observer tools.StreamObserver,
) (ToolProvider, Engine, *guardrails.GuardrailEngine) {
	resolver := appCtx.Resolver()
	defaultGuardrails := appCtx.GetGuardrails()

	readConfig := newCachedConfigReader(persistence, resolver)
	terminal := initTerminalTools(resolver, persistence, readConfig, defaultGuardrails, shellManager, observer)
	grEngine := guardrails.NewGuardrailEngine(func() models.AgentGuardrailsConfig {
		return defaultGuardrails
	}, resolver, persistence, func(workspaceID string) (*models.WorkspaceConfig, error) { return readConfig(workspaceID) })
	network := initNetworkTools(persistence, readConfig, defaultGuardrails, logger)
	comm := initCommunicationTools(appCtx, network)
	search := initSearchTools(appCtx, network)
	fsTools := initFileSystemTools(resolver, persistence, readConfig, defaultGuardrails)
	memTools := initMemoryTools(appCtx.MemoryStore())

	localRegistry := NewLocalToolRegistry(terminal, comm, search, fsTools, network, memTools)
	provider := NewMultiToolProvider(false, localRegistry, mcp)
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
	prompt := prompts.LocalAssistantPrompt

	if len(r.toolDefinitions) > 0 {
		prompt += "\n\nAVAILABLE TOOLS:\n"
		prompt += r.FormatToolsForPrompt()
	}

	return prompt, nil
}

// UseNativeTools indicates if tools should be passed via the API.
// Local models default to text-only to avoid confusing non-function-calling
// models with API-level tool definitions they cannot process.  Models that
// support native function calling can opt in via ModelConfig.ToolCallFormat.
// UseNativeTools returns false because local models default to XML text mode.
// API-level tool schemas confuse non-function-calling local servers; models
// that support native tools opt in via ModelConfig.ToolCallFormat = "native".
func (r *LocalToolRegistry) UseNativeTools() bool {
	return false
}

// FormatToolsForPrompt converts the tool definitions into a readable format.
func (r *LocalToolRegistry) FormatToolsForPrompt() string {
	var sb strings.Builder
	for _, t := range r.toolDefinitions {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", t.Function.Name, t.Function.Description))
		if params, ok := t.Function.Parameters.(map[string]any); ok {
			if props, ok := params["properties"].(map[string]any); ok {
				sb.WriteString("  Parameters:\n")
				for pName, pDetails := range props {
					if details, ok := pDetails.(map[string]any); ok {
						pType := details["type"]
						pDesc := details["description"]
						sb.WriteString(fmt.Sprintf("    * %s (%v): %v\n", pName, pType, pDesc))
					}
				}
			}
		}
		sb.WriteString("\n")
	}
	return sb.String()
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
	params, desc, err := tools.LoadManifestAsTool(toolKey, toolName)
	if err != nil {
		logging.Warn("failed to load tool manifest", "key", toolKey, "error", err)
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
	r.registerMemoryTools()
	r.registerSystemTools()
}

func (r *LocalToolRegistry) registerTerminalTools() {
	registerTool(r, "terminal", models.ToolTerminalExecute, func(ctx context.Context, args struct {
		Command string `json:"command"`
		Cwd     string `json:"cwd"`
	}) (any, error) {
		return r.Terminal.ExecuteCommand(ctx, args.Command, args.Cwd)
	})
}

func (r *LocalToolRegistry) registerCommunicationTools() {
	registerTool(r, "communication", models.ToolNotifyUser, func(ctx context.Context, args struct {
		Message   string `json:"message"`
		Connector string `json:"connector"` // optional — empty sends to all connectors
	}) (any, error) {
		if r.Communication == nil {
			return nil, fmt.Errorf("communication tools not configured")
		}
		if err := r.Communication.NotifyAll(ctx, args.Message, args.Connector); err != nil {
			return nil, err
		}
		return "Notification sent successfully", nil
	})
}

func (r *LocalToolRegistry) registerSearchTools() {
	registerTool(r, "search", models.ToolInternetSearch, func(ctx context.Context, args struct {
		Query string `json:"query"`
	}) (any, error) {
		if r.Search == nil {
			return nil, fmt.Errorf("search tools not configured")
		}
		return r.Search.Search(ctx, args.Query)
	})
}

func (r *LocalToolRegistry) registerFileSystemTools() {
	registerTool(r, "filesystem", models.ToolDirectoryList, func(ctx context.Context, args struct {
		Path string `json:"path"`
	}) (any, error) {
		return r.FileSystem.ListDirectory(ctx, args.Path)
	})

	registerTool(r, "filesystem", models.ToolFileRead, func(ctx context.Context, args struct {
		Path string `json:"path"`
	}) (any, error) {
		return r.FileSystem.ReadFile(ctx, args.Path)
	})

	registerTool(r, "filesystem", models.ToolFileWrite, func(ctx context.Context, args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}) (any, error) {
		if err := r.FileSystem.WriteFile(ctx, args.Path, args.Content); err != nil {
			return "", err
		}
		return "File written successfully", nil
	})

	registerTool(r, "filesystem", models.ToolFileAppend, func(ctx context.Context, args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}) (any, error) {
		if err := r.FileSystem.AppendFile(ctx, args.Path, args.Content); err != nil {
			return "", err
		}
		return "Content appended successfully", nil
	})

	registerTool(r, "filesystem", models.ToolFileEditBlock, func(ctx context.Context, args struct {
		Path     string `json:"path"`
		OldBlock string `json:"old_block"`
		NewBlock string `json:"new_block"`
	}) (any, error) {
		return r.FileSystem.EditFileBlock(ctx, args.Path, args.OldBlock, args.NewBlock)
	})
}

func (r *LocalToolRegistry) registerNetworkTools() {
	registerTool(r, "network", models.ToolNetworkFetch, func(ctx context.Context, args struct {
		URL string `json:"url"`
	}) (any, error) {
		if r.Network == nil {
			return nil, fmt.Errorf("network tools not configured")
		}
		return r.Network.FetchURL(ctx, args.URL)
	})

	registerTool(r, "network", models.ToolNetworkScan, func(ctx context.Context, args tools.ScanArgs) (any, error) {
		if r.Network == nil {
			return nil, fmt.Errorf("network tools not configured")
		}
		return r.Network.ScanLocalNetwork(ctx, args)
	})

	registerTool(r, "network", models.ToolNetworkInfo, func(ctx context.Context, args struct{}) (any, error) {
		if r.Network == nil {
			return nil, fmt.Errorf("network tools not configured")
		}
		return r.Network.GetNetworkInfo(ctx)
	})
}
func (r *LocalToolRegistry) registerMemoryTools() {
	if r.Memory == nil {
		return
	}
	registerTool(r, models.CategoryMemory, models.ToolMemorySearch, r.Memory.Search)
	registerTool(r, models.CategoryMemory, models.ToolMemoryUpdate, r.Memory.Update)
}

func (r *LocalToolRegistry) registerSystemTools() {
	registerTool(r, models.CategorySystem, models.ToolSystemError, func(ctx context.Context, args struct {
		Error string `json:"error"`
	}) (any, error) {
		// This tool allows the system to send error feedback back to the agent
		// as a tool result when it makes a mistake in its tool-calling format.
		return fmt.Sprintf("SYSTEM ERROR: %s", args.Error), nil
	})

}
