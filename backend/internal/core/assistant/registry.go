// registry.go — LocalToolRegistry, all tool registration, and
// InitializeAgentStack (the top-level wiring function).  The init helper
// functions (initTerminalTools, initCommunicationTools, etc.) are extracted
// here to keep InitializeAgentStack as a short orchestration function.
package assistant

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"llm-proxy/internal/core"
	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/nodeherder"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/core/tools"
	"llm-proxy/internal/core/tools/searchproviders"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/memory"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/safe"
	"llm-proxy/internal/platform/sandbox"
	"llm-proxy/internal/platform/sizewatch"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/internal/platform/units"
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
	searchTools *tools.InternetTools,
	fsTools *tools.FileSystemTools,
	network *tools.NetworkTools,
	memoryTools *tools.MemoryToolProvider,
) *LocalToolRegistry {
	r := &LocalToolRegistry{
		handlers:      make(map[string]ToolHandler),
		Terminal:      terminal,
		Communication: comm,
		Search:        searchTools,
		FileSystem:    fsTools,
		Network:       network,
		Memory:        memoryTools,
	}
	r.registerAll()
	return r
}

// ToolHandler is a function that executes a local tool.
type ToolHandler func(ctx context.Context, rawArgs string) (any, error)

// AgentStackDeps bundles the services InitializeAgentStack is wired from at
// bootstrap. Grouping keeps the stack constructor at ≤3 parameters (repo rule:
// 0–3 ideal, 4 ceiling, anything beyond 4 must be a struct) — the predecessor
// took 8 positional arguments that were order-sensitive at the call site.
type AgentStackDeps struct {
	Persistence  *persistence.WorkspaceManager
	Logger       logging.Logger
	ShellManager shell.ShellProvider
	Observer     tools.StreamObserver
	EgressProxy  *url.URL
	EgressEnv    func() []string
}

// configSecretsReader is the live config + secret read surface the tool
// initializers need — narrow on purpose (interface segregation) rather than
// dragging the whole application context in.
type configSecretsReader interface {
	Secrets() models.SecretsStore
	GetRegistry() models.RegistryData
}

// AgentStackContext is the application-context surface InitializeAgentStack
// wires the stack from. It is declared here, in the consuming package, so
// *app.AppContext satisfies it implicitly without assistant importing app.
type AgentStackContext interface {
	configSecretsReader
	Resolver() storage.Resolver
	GetGuardrails() models.AgentGuardrailsConfig
	MemoryStore() *memory.Store
	HostSettings() models.HostSettings
	SetSandboxProvider(sandbox.Provider)
}

// stackCtx is the shared wiring for the tool-provider constructors, built once
// in InitializeAgentStack so each init* helper takes exactly one argument.
type stackCtx struct {
	deps          AgentStackDeps
	resolver      storage.Resolver
	readConfig    readConfigFunc
	defaults      models.AgentGuardrailsConfig
	hostNetworkOn func() bool
	sandbox       sandbox.Provider
	storageOver   func(workspaceID, workspaceRoot string) bool
}

func initTerminalTools(s stackCtx) *tools.TerminalTools {
	return tools.NewTerminalTools(tools.TerminalToolsDeps{
		ConfigProvider: func(ctx context.Context) models.TerminalGuardrailsConfig {
			return getEffectiveConfig(ctx, s.readConfig, s.defaults, func(c *models.AgentGuardrailsConfig) models.TerminalGuardrailsConfig {
				return c.Terminal
			})
		},
		PathResolver: func(workspaceID string) string {
			if workspaceID == "" || s.deps.Persistence == nil {
				return ""
			}
			return s.resolver.WorkspaceDir(workspaceID)
		},
		ShellPool:   s.deps.ShellManager,
		Observer:    s.deps.Observer,
		NetworkOn:   s.hostNetworkOn,
		EgressEnv:   s.deps.EgressEnv,
		Sandbox:     s.sandbox,
		StorageOver: s.storageOver,
	})
}
func initCommunicationTools(appCtx configSecretsReader, network *tools.NetworkTools) *tools.CommunicationTools {
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
	safe.Go("webhook re-registration", func() {
		regCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := wa.RegisterWebhook(regCtx, cfg.WebhookURL, cfg.Settings["webhook_token"]); err != nil {
			logging.Warn("failed to re-register webhook on startup", "connector", name, "error", err)
		} else {
			logging.Info("re-registered webhook on startup", "connector", name, "url", cfg.WebhookURL)
		}
	})
}

func initNetworkTools(s stackCtx) *tools.NetworkTools {
	netTools := tools.NewNetworkTools(func(ctx context.Context) models.NetworkGuardrailsConfig {
		cfg := getEffectiveConfig(ctx, s.readConfig, s.defaults, func(c *models.AgentGuardrailsConfig) models.NetworkGuardrailsConfig {
			return c.Network
		})
		// An explicitly stamped run scope (automation grant) overrides the
		// workspace L1 flags for this run's tool calls. The scope→config
		// mapping is centralized in models.WithRunScope (same source the
		// guardrail engine uses), so the runtime tools and the validator can
		// never disagree about what a scope means. Absent scope (chat) keeps
		// the merged workspace policy untouched.
		if scope, ok := models.RunNetworkScopeFrom(ctx); ok {
			if scoped, changed := cfg.WithRunScope(scope); changed {
				cfg = scoped
			}
		}
		return cfg
	}, s.deps.Logger)
	if s.deps.EgressProxy != nil {
		netTools.SetProxy(s.deps.EgressProxy)
	}
	return netTools
}

// searchSelection is one live read of the internet-search configuration: the
// selected provider, its registered spec, the stored key, and the result cap.
// ok is false when no usable provider is configured (unknown/unregistered
// provider, or a key-requiring provider with no stored key).
type searchSelection struct {
	provider models.SearchProvider
	spec     tools.SearchProviderSpec
	key      string
	max      int
	ok       bool
}

// searchTarget reads the live registry config and secret once, so the tool
// resolver and the guardrail availability predicate can never drift. A provider
// is available iff its spec is registered and (it does not require a key OR the
// stored `search:<provider>` secret is non-empty). An unset provider falls back
// to the default. No I/O and no context — safe from the guardrail gate.
func searchTarget(registry func() models.RegistryData, secrets models.SecretsStore) searchSelection {
	cfg := models.DefaultSearchConfig()
	if registry != nil {
		cfg = registry().Search
	}
	provider := cfg.Provider
	if provider == "" {
		provider = models.DefaultSearchConfig().Provider
	}
	spec, registered := searchproviders.GetSearchProvider(provider)
	if !registered {
		return searchSelection{}
	}
	key := ""
	if spec.RequiresKey {
		if secrets == nil {
			return searchSelection{}
		}
		key = secrets.GetSecret(models.CategorySearch, string(provider))
		if key == "" {
			return searchSelection{}
		}
	}
	return searchSelection{provider: provider, spec: spec, key: key, max: cfg.MaxResults, ok: true}
}

// initSearchTools builds the internet_search tool with a lazy resolver plus the
// availability predicate the guardrail engine uses to schema-hide the tool when
// no provider is configured. Both read the same searchTarget, so the tool and
// the gate never disagree. The tool is nil-safe, so a residual call after the
// gate reaches it and returns ErrSearchNotConfigured.
func initSearchTools(appCtx configSecretsReader, network *tools.NetworkTools) (*tools.InternetTools, func() bool) {
	target := func() searchSelection {
		return searchTarget(appCtx.GetRegistry, appCtx.Secrets())
	}

	internet := tools.NewInternetTools(func(ctx context.Context) (tools.SearchProvider, error) {
		sel := target()
		if !sel.ok {
			return nil, tools.ErrSearchNotConfigured
		}
		provider, err := sel.spec.New(tools.SearchProviderConfig{
			APIKey:     sel.key,
			Client:     network.HTTPClient(),
			MaxResults: sel.max,
		})
		if err != nil {
			logging.Warn("internet search provider unavailable", "provider", string(sel.provider), "error", err)
			return nil, fmt.Errorf("internet search provider %q: %w", sel.provider, err)
		}
		return provider, nil
	})

	return internet, func() bool { return target().ok }
}

func initMemoryTools(store *memory.Store) *tools.MemoryToolProvider {
	if store == nil {
		return nil
	}
	return tools.NewMemoryToolProvider(store)
}

func initFileSystemTools(s stackCtx) *tools.FileSystemTools {
	return tools.NewFileSystemTools(func(ctx context.Context) models.FileSystemGuardrailsConfig {
		cfg := getEffectiveConfig(ctx, s.readConfig, s.defaults, func(c *models.AgentGuardrailsConfig) models.FileSystemGuardrailsConfig {
			return c.FileSystem
		})
		allowed := make([]string, 0, len(cfg.AllowedPaths)+1)
		if wsID := models.GetWorkspaceID(ctx); wsID != "" {
			wsPath := s.resolver.WorkspaceDir(wsID)
			allowed = append(allowed, wsPath)
		}
		allowed = append(allowed, cfg.AllowedPaths...)
		cfg.AllowedPaths = allowed
		return cfg
	})
}
func InitializeAgentStack(
	appCtx AgentStackContext,
	mcp nodeherder.MCPService,
	deps AgentStackDeps,
) (ToolProvider, Engine, *guardrails.GuardrailEngine) {
	resolver := appCtx.Resolver()
	defaultGuardrails := appCtx.GetGuardrails()

	readConfig := newCachedConfigReader(deps.Persistence, resolver)
	// Host-level network state keys the shell pool (D8). Resolved live so a
	// sandboxing.network toggle splits/merges shells on the next command; an
	// automation run scope overrides it at the run level (Phase 0, executor
	// ResolveRunScope → ctx stamp → executeShell).
	hostNetworkOn := func() bool { return appCtx.HostSettings().Sandboxing.NetworkAllowed() }
	// OS confinement provider for agent children, built here at the single
	// composition root and shared by every spawn site (pooled shell +
	// executeLocal). The provider is snapshotted from the requested config at
	// boot — toggling sandboxing.filesystem applies on restart.
	sandboxProvider := sandbox.New(sandbox.Config{
		Filesystem:   appCtx.HostSettings().Sandboxing.FilesystemEnabled(),
		MaxStorageGB: appCtx.HostSettings().Sandboxing.MaxStorageGB,
	})
	// Downgrade-never-bypass: make what is actually enforced visible at startup
	// and on the host-settings surface (SPEC-006 §II.7.4).
	appCtx.SetSandboxProvider(sandboxProvider)
	logging.Info("sandbox effective state", "state", sandboxProvider.Effective().String())
	// Workspace disk accounting (max_storage_gb, plan Phase 3): best-effort,
	// TTL-cached size checks that block shell spawns past the boundary. Honest
	// labeling: accounting, not a kernel-enforced quota (TOCTOU window exists).
	// max_storage_gb is read live so a settings edit applies on the next spawn.
	sizeCache := sizewatch.NewCache(0) // default 60s TTL
	storageOver := func(workspaceID, workspaceRoot string) bool {
		limitGB := appCtx.HostSettings().Sandboxing.MaxStorageGB
		if limitGB <= 0 || workspaceRoot == "" {
			return false
		}
		return sizeCache.Bytes(workspaceRoot, time.Now()) > units.GiB(limitGB)
	}
	stack := stackCtx{
		deps: deps, resolver: resolver, readConfig: readConfig, defaults: defaultGuardrails,
		hostNetworkOn: hostNetworkOn, sandbox: sandboxProvider, storageOver: storageOver,
	}
	terminal := initTerminalTools(stack)
	network := initNetworkTools(stack)
	searchTools, searchAvailable := initSearchTools(appCtx, network)
	grEngine := guardrails.NewGuardrailEngine(func() models.AgentGuardrailsConfig {
		return defaultGuardrails
	}, stack.resolver, stack.deps.Persistence, func(workspaceID string) (*models.WorkspaceConfig, error) { return stack.readConfig(workspaceID) })
	// Host-level network hard gate (plan D1/R4): resolved live from host settings so
	// toggling sandboxing.network takes effect on the next validation/schema pass.
	// NetworkAllowed() treats an absent (legacy) key as allowed — only an explicit
	// false trips the gate.
	grEngine.SetHostNetworkAllowed(func() bool {
		return appCtx.HostSettings().Sandboxing.NetworkAllowed()
	})
	// internet_search is schema-hidden while no provider is configured (live
	// predicate over registry + secrets — takes effect on the next run without a
	// restart).
	grEngine.SetSearchAvailable(searchAvailable)
	comm := initCommunicationTools(appCtx, network)
	fsTools := initFileSystemTools(stack)
	memTools := initMemoryTools(appCtx.MemoryStore())

	localRegistry := NewLocalToolRegistry(terminal, comm, searchTools, fsTools, network, memTools)
	provider := NewMultiToolProvider(false, localRegistry, mcp)
	mcpEngine := NewEngine(mcp, deps.Logger)
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
		// InternetTools is nil-safe (nil receiver or resolver ⇒
		// ErrSearchNotConfigured), so no nil check is needed here.
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
