package app

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"path/filepath"

	"llm-proxy/internal/buildinfo"
	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/automation"
	"llm-proxy/internal/core/llm"
	"llm-proxy/internal/core/mcp"
	"llm-proxy/internal/core/nodeherder"
	"llm-proxy/internal/core/orchestrator"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/core/proxy/recorder"
	"llm-proxy/internal/core/tools"
	"llm-proxy/internal/recordings"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/memory"
	"llm-proxy/internal/platform/metrics"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/ratelimiter"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/internal/shell"
	api "llm-proxy/internal/transport/http"
	"llm-proxy/models"
	"llm-proxy/utils"
)

type Core struct {
	AppCtx  *AppContext
	Runtime llm.RuntimeManager
}

type Infra struct {
	Logger     logging.Logger
	Clock      utils.Clock
	NodeHerder nodeherder.MCPService
}

type Container struct {
	Core        Core
	Infra       Infra
	Dispatcher  *automation.Dispatcher
	RecordDir   string // absolute path to runs directory, empty when recording is disabled
	RunLoggingEnabled bool  // per-run output enabled
}

// Building automation task executor
func (c *Container) BuildTaskExecutor(svc api.AssistantService) automation.TaskExecutor {
	return automation.NewLLMTaskExecutor(svc)
}

func (c *Container) BuildAppServices() *AppServices {
	var recordingStore *recordings.RecordingStore
	if c.RecordDir != "" {
		var rsErr error
		recordingStore, rsErr = recordings.NewRecordingStore(c.RecordDir)
		if rsErr != nil {
			logging.Warn("Failed to init recording store", "dir", c.RecordDir, "error", rsErr)
		}
	}

	s := &AppServices{
		Runtime:        c.Core.Runtime,
		AppCtx:         c.Core.AppCtx,
		nodeHerder:     c.Infra.NodeHerder,
		logger:         c.Infra.Logger,
		Clock:          c.Infra.Clock,
		persistence:    persistence.NewWorkspaceManager(storage.NewPathResolver(c.Core.AppCtx.RootDir(), c.Core.AppCtx.WorkspacesDir(), c.Core.AppCtx.MetadataDir())),
		limiter:        ratelimiter.NewLimiter(c.Infra.Clock),
		RecordingStore: recordingStore,
		runLoggingEnabled:    c.RunLoggingEnabled,
	}

	factory := func(baseURL string, model string, headers http.Header) proxy.Client {
		client := proxy.NewLLMClient(baseURL, model, nil, headers)
		// Always wrap in RecordingClient so that run-specific recording.jsonl is supported,
		// but only set recordDir if recording is globally enabled.
		client = recorder.New(client, c.RecordDir, model)
		if c.RecordDir != "" {
			logging.Debug("recording LLM responses", "model", model, "dir", c.RecordDir)
		}

		return client
	}

	s.clientProvider = proxy.NewRuntimeClientProvider(s, c.Core.Runtime, factory)
	s.dispatcher = c.Dispatcher
	s.guardrailDecisionStore = assistant.NewGuardrailDecisionStore()

	// Initialize Shell/Terminal Subsystem
	shellManager, streamObserver := c.initShellOrchestrator(s)

	// Initialize unified tool providers and engines (Local Registry + Remote MCP)
	s.toolProvider, s.engine, s.guardrailEngine = assistant.InitializeAgentStack(
		s.AppCtx,
		s.persistence,
		s.nodeHerder,
		s.logger,
		shellManager,
		streamObserver,
	)

	return s
}

// initShellOrchestrator spins up the background persistent shell manager
// and configures the streaming T-Junction metrics observer for the frontend.
func (c *Container) initShellOrchestrator(s *AppServices) (shell.ShellProvider, tools.StreamObserver) {
	var shellManager shell.ShellProvider

	settings := s.AppCtx.HostSettings()
	// We still check if the feature is enabled in host settings for backward compatibility
	// though it is now a native host shell rather than a WASM sandbox.
	if !settings.Sandboxing.Enabled {
		log.Fatal("[SECURITY] Terminal execution is required for agentic execution. Set terminal.enabled = true in host settings.")
	}

	if sm, err := shell.NewHostShellManager(); err == nil {
		shellManager = sm
		c.Infra.Logger.Debug("Host Shell Manager initialized successfully")
	} else {
		log.Fatalf("[SECURITY] Failed to start Host Shell Manager: %v", err)
	}

	streamObserver := func(streamType string, chunk []byte) {
		s.Events().Publish("global", assistant.AgentEvent{
			Type: assistant.EventToolStream, // Emits to the frontend console via EventBus
			Payload: map[string]any{
				"stream": streamType,
				"output": string(chunk),
			},
		})
	}

	if sm, ok := shellManager.(metrics.TerminalSource); ok {
		s.AppCtx.SetTerminalSource(sm)
	}
	s.AppCtx.SetShellProvider(shellManager)
	return shellManager, streamObserver
}

type AppServices struct {
	Runtime              llm.RuntimeManager
	AppCtx               *AppContext
	nodeHerder           nodeherder.MCPService
	toolProvider         assistant.ToolProvider
	clientProvider       proxy.LLMClientProvider
	engine               assistant.Engine
	guardrailEngine      *guardrails.GuardrailEngine
	persistence          *persistence.WorkspaceManager
	logger               logging.Logger
	Clock                utils.Clock
	dispatcher           *automation.Dispatcher
	limiter              ratelimiter.Limiter
	guardrailDecisionStore *assistant.GuardrailDecisionStore
	RecordingStore       *recordings.RecordingStore
	runLoggingEnabled          bool
}

func (s AppServices) Shutdown() {
	if s.Runtime != nil {
		logging.Info("Shutting down LLM runtime...")
		s.Runtime.Shutdown()
	}
	if s.AppCtx != nil {
		s.AppCtx.Shutdown()
	}
}

func (s AppServices) GetClientForModel(ctx context.Context, modelName string) (proxy.Client, error) {
	return s.clientProvider.GetClientForModel(ctx, modelName)
}

func (s AppServices) NodeHerder() nodeherder.MCPService {
	return s.nodeHerder
}

func (s AppServices) ToolProvider() assistant.ToolProvider {
	return s.toolProvider
}

func (s AppServices) ClientProvider() proxy.LLMClientProvider {
	return s.clientProvider
}

func (s AppServices) Logger() logging.Logger {
	return s.logger
}

func (s AppServices) Limiter() ratelimiter.Limiter {
	return s.limiter
}

func (s AppServices) SelectModels() (string, string) {
	return s.AppCtx.SelectModels()
}

func (s AppServices) Engine() assistant.Engine {
	return s.engine
}

func (s AppServices) GuardrailEngine() *guardrails.GuardrailEngine {
	return s.guardrailEngine
}

func (s AppServices) ModelConfig(modelName string) (models.ModelConfig, bool) {
	if s.Runtime == nil {
		return models.ModelConfig{}, false
	}
	for _, m := range s.Runtime.ListModels() {
		if m.Name == modelName {
			return m, true
		}
	}
	return models.ModelConfig{}, false
}

func (s AppServices) Orchestrator() *orchestrator.Orchestrator {
	if s.AppCtx != nil {
		return s.AppCtx.Orchestrator()
	}
	return nil
}

func (s AppServices) Persistence() *persistence.WorkspaceManager {
	return s.persistence
}

func (s AppServices) ProcessLogger(workspaceID string) logging.Logger {
	return s.AppCtx.ProcessLogger(workspaceID)
}

func (s AppServices) RootDir() string {
	return s.AppCtx.RootDir()
}

func (s *AppServices) Events() *automation.EventBus {
	if s.dispatcher == nil {
		return nil
	}
	return s.dispatcher.Events()
}

func (s *AppServices) SetDispatcher(d *automation.Dispatcher) {
	s.dispatcher = d
}

func (s AppServices) GuardrailDecisionStore() *assistant.GuardrailDecisionStore {
	return s.guardrailDecisionStore
}

func (s AppServices) MemoryStore() *memory.Store {
	return s.AppCtx.MemoryStore()
}

func (s AppServices) RecordDir() string {
	if s.RecordingStore != nil {
		return s.RecordingStore.RecordDir()
	}
	return ""
}

func (s AppServices) RunLoggingEnabled() bool {
	return s.AppCtx.RunLoggingEnabled()
}

func (s AppServices) GetPlaybackClient(ctx context.Context, ref string) (proxy.Client, error) {
	if s.RecordingStore == nil {
		return nil, fmt.Errorf("recording store not available (start server with --record-dir)")
	}
	meta, ok := s.RecordingStore.Get(ref)
	if !ok {
		return nil, fmt.Errorf("recording %q not found", ref)
	}
	pc, err := recordings.NewPlaybackClient(meta.FilePath)
	if err != nil {
		return nil, fmt.Errorf("load recording %s: %w", meta.FilePath, err)
	}
	return NewPlaybackBridge(pc), nil
}

func bootstrap(dataMgr *storage.DataManager, logger logging.Logger, recordEnabled bool, enableRuns bool) *Container {
	if logger == nil {
		log.Fatal("Logger is required")
	}
	clock := utils.NewRealClock()

	// 1. Load System Config for MCP/Runtime defaults
	logging.Debug("Loading system configuration...")
	sys := dataMgr.System().Get()

	// 1.1 Apply persisted log level
	if sys.Server.LogLevel != "" {
		logger.SetLevel(logging.Level(sys.Server.LogLevel))
	}

	// 1.5 Initialize Network for Infrastructure (MCP, Cloud LLMs)
	networkTools := tools.NewNetworkTools(func(ctx context.Context) models.NetworkGuardrailsConfig {
		// Use global guardrails from data manager
		return dataMgr.Settings().Get().Guardrails.Network
	}, logger)

	// Configure MCP Service (Bridge logic: we still pass sys config parts)
	logging.Debug("Configuring MCP services...")
	nodeHerder, err := configureMCP(dataMgr, logger, networkTools.DialContext())
	if err != nil {
		logging.Error("Failed to configure MCP service", "error", err)
		return nil
	}

	// 2. Initialize Runtime Manager from Registry
	logging.Debug("Initializing LLM runtime manager...")
	registry := dataMgr.Registry().Get()
	settings := dataMgr.Settings().Get()
	secretsStore := dataMgr.Secrets()
	manager := llm.NewManagerFromRegistry(registry, sys, settings, secretsStore, func() models.RegistryData {
		return dataMgr.Registry().Get()
	})

	logging.Debug("Creating server context...")
	appCtx := NewServer(manager, dataMgr)
	appCtx.cliEnableRuns = enableRuns || recordEnabled
	runtime := appCtx.Manager()

	logging.Debug("Bootstrap phase complete", "root", dataMgr.RootDir())

	runsDir := ""
	if recordEnabled {
		runsDir = filepath.Join(dataMgr.RootDir(), "runs")
	}

	runLoggingEnabled := enableRuns

	return &Container{
		Core: Core{
			AppCtx:  appCtx,
			Runtime: runtime,
		},
		Infra: Infra{
			Logger:     logger,
			Clock:      clock,
			NodeHerder: nodeHerder,
		},
		RecordDir:   runsDir,
		RunLoggingEnabled: runLoggingEnabled,
	}
}

// BuildDispatcher creates the new dispatcher subsystem.
// It uses the persistence layer directly (not the old workspace.Manager).
func (c *Container) BuildDispatcher(svc api.AssistantService) (*automation.Dispatcher, error) {
	persistenceMgr := svc.Persistence()

	exec := automation.NewLLMTaskExecutor(svc)

	d, err := automation.NewDispatcher(persistenceMgr, exec, c.Infra.Logger,

		automation.WithWorkerCount(1),
	)
	if err != nil {
		return nil, err
	}

	return d, nil
}

func configureMCP(dataMgr *storage.DataManager, logger logging.Logger, dialer func(context.Context, string, string) (net.Conn, error)) (nodeherder.MCPService, error) {
	// Initialize MCP Orchestrator
	orchestrator := mcp.NewOrchestrator(logger)
	orchestrator.DialContext = dialer

	// Initialize Resource Mirror
	mirror := mcp.NewResourceMirror()

	// Subscribe Registry Updates -> MCP Orchestrator
	dataMgr.Registry().OnChange(func(reg models.RegistryData) {
		sys := dataMgr.System().Get()
		orchestrator.Reload(context.Background(), translateMCPServers(reg.MCPServers), sys.Server.Bind)
	})

	// Initial Load
	currentReg := dataMgr.Registry().Get()
	sys := dataMgr.System().Get()
	orchestrator.Reload(context.Background(), translateMCPServers(currentReg.MCPServers), sys.Server.Bind)

	// Register prompt updates handled by Orchestrator (which propagates to Clients)
	orchestrator.OnPromptUpdate(func(prompt string) {
		mirror.SetSystemPrompt(prompt)
	})

	// Subscribe to system prompt to receive updates
	orchestrator.Subscribe(context.Background(), "nodeherder://system-prompt")

	return mcp.NewMCPNodeHerder(orchestrator, mirror, logger), nil
}

// translateMCPServers converts registry servers to internal model configs (Bridge logic)
func translateMCPServers(reg []models.MCPServerRegistryEntry) []models.MCPServerConfig {
	out := make([]models.MCPServerConfig, len(reg))
	for i, s := range reg {
		out[i] = models.MCPServerConfig{
			Name:      s.Name,
			URL:       s.URL,
			Enabled:   s.Enabled,
			TLSCACert: s.TLSCACert,
		}
	}
	return out
}

func buildHTTP(s *AppServices, disp *automation.Dispatcher, buildInfo *buildinfo.Info) http.Handler {
	assistant := api.NewAssistantMessageHandler(s)

	adminHandlers := api.NewAdminHandlers(s.Runtime, s.AppCtx, s.Logger(), buildInfo, s.AppCtx.Orchestrator())
	proxyHandlers := api.NewProxyHandlers(s.Runtime)

	var dispatcherHandlers *api.DispatcherHandlers
	if disp != nil {
		dispatcherHandlers = api.NewDispatcherHandlers(disp, s.logger)
	}

	recordingHandlers := api.NewRecordingHandlers(s.RecordingStore)

	var memoryHandlers *api.MemoryHandlers
	if store := s.AppCtx.MemoryStore(); store != nil {
		memoryHandlers = api.NewMemoryHandlers(store)
	}

	return buildRouter(adminHandlers, proxyHandlers, assistant, dispatcherHandlers, recordingHandlers, memoryHandlers)
}

func buildRouter(
	admin *api.AdminHandlers,
	proxyHandlers *api.ProxyHandlers,
	assistant *api.AssistantMessageHandler,
	dispatcherHandlers *api.DispatcherHandlers,
	recordings *api.RecordingHandlers,
	memoryHandlers *api.MemoryHandlers,
) http.Handler {

	router := api.NewRouter()

	jsonMethodNotAllowed := api.WithMethodNotAllowed(http.HandlerFunc(api.MethodNotAllowedJSON))
	textMethodNotAllowed := api.WithMethodNotAllowed(http.HandlerFunc(api.MethodNotAllowedText))

	// Admin
	router.Get("/admin/api/version", admin.AdminVersionHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/state", admin.AdminStateHandler, textMethodNotAllowed)
	router.Post("/admin/api/start", admin.AdminStartHandler, jsonMethodNotAllowed)
	router.Post("/admin/api/stop", admin.AdminStopHandler, textMethodNotAllowed)
	router.Post("/admin/api/models", admin.AdminAddModelHandler, jsonMethodNotAllowed)
	router.Put("/admin/api/models", admin.AdminUpdateModelHandler, jsonMethodNotAllowed)
	router.Delete("/admin/api/models", admin.AdminDeleteModelHandler, jsonMethodNotAllowed)
	router.Delete("/admin/api/models/all", admin.AdminDeleteAllModelsHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/config", admin.AdminConfigHandler, jsonMethodNotAllowed)
	router.Put("/admin/api/config", admin.AdminConfigUpdateHandler, jsonMethodNotAllowed)
	router.Post("/admin/api/system/restart", admin.AdminRestartHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/host", admin.AdminHostSettingsHandler, jsonMethodNotAllowed)
	router.Put("/admin/api/host", admin.AdminHostSettingsPutHandler, jsonMethodNotAllowed)
	router.Post("/admin/api/host/terminal/reset", admin.AdminTerminalResetHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/host/terminal/sessions", admin.AdminTerminalSessionsHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/logs", admin.AdminLogsHandler, jsonMethodNotAllowed)
	router.Delete("/admin/api/logs", admin.AdminLogsClearHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/log-level", admin.AdminLogLevelHandler, jsonMethodNotAllowed)
	router.Put("/admin/api/log-level", admin.AdminLogLevelUpdateHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/app-logs", admin.AdminAppLogsHandler, textMethodNotAllowed)
	router.Delete("/admin/api/app-logs", admin.AdminAppLogsClearHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/app-logs/tail", admin.AdminAppLogsTailHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/metrics", admin.AdminMetricsHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/runtime/processes", admin.AdminProcessesHandler, jsonMethodNotAllowed)
	router.Post("/admin/api/runtime/processes/{pid}/stop", admin.AdminProcessKillHandler, jsonMethodNotAllowed)

	// MCP
	router.Get("/admin/api/mcp", admin.AdminMCPListHandler, jsonMethodNotAllowed)
	router.Post("/admin/api/mcp", admin.AdminMCPAddHandler, jsonMethodNotAllowed)
	router.Put("/admin/api/mcp", admin.AdminMCPUpdateHandler, jsonMethodNotAllowed)
	router.Delete("/admin/api/mcp", admin.AdminMCPRemoveHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/providers/models", admin.AdminListProviderModelsHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/providers/manifests", admin.AdminListProviderManifestsHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/providers/test", admin.AdminTestProviderConnectionHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/orchestrator/balance", admin.AdminICUBalanceHandler, jsonMethodNotAllowed)

	// Templates
	router.Get("/admin/api/templates", admin.ListTemplatesHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/templates/", admin.GetTemplateHandler, jsonMethodNotAllowed)

	// Secrets — provider API keys
	router.Get("/admin/api/secrets/keys", admin.AdminProviderKeysHandler, jsonMethodNotAllowed)
	router.Put("/admin/api/secrets/keys", admin.AdminProviderKeysPutHandler, jsonMethodNotAllowed)
	router.Delete("/admin/api/secrets/keys", admin.AdminProviderKeyDeleteHandler, jsonMethodNotAllowed)

	// Secrets — tool secrets (search, communication, etc.)
	router.Get("/admin/api/secrets/tools", admin.AdminToolSecretHandler, jsonMethodNotAllowed)
	router.Put("/admin/api/secrets/tools", admin.AdminToolSecretPutHandler, jsonMethodNotAllowed)

	// Recordings
	if recordings != nil {
		router.Get("/admin/api/recordings", recordings.List, jsonMethodNotAllowed)
		router.Get("/admin/api/recordings/status", recordings.Status, jsonMethodNotAllowed)
		router.Get("/admin/api/recordings/{id}", recordings.Get, jsonMethodNotAllowed)
		router.Delete("/admin/api/recordings/{id}", recordings.Delete, jsonMethodNotAllowed)
	}

	// Dispatcher
	if dispatcherHandlers != nil {
		router.Get("/admin/api/dispatcher/automations", dispatcherHandlers.ListAutomations, jsonMethodNotAllowed)
		router.Post("/admin/api/dispatcher/trigger/{"+models.WorkspaceIDParam+"}/{automation}", dispatcherHandlers.TriggerAutomation, jsonMethodNotAllowed)
		router.Post("/admin/api/dispatcher/stop/{"+models.WorkspaceIDParam+"}", dispatcherHandlers.StopAutomation, jsonMethodNotAllowed)
		router.Get("/admin/api/dispatcher/metrics", dispatcherHandlers.GetDispatcherMetrics, jsonMethodNotAllowed)
		router.Get("/admin/api/dispatcher/activity", dispatcherHandlers.GetGlobalActivity, jsonMethodNotAllowed)

		router.Get("/admin/api/dispatcher/workspaces", dispatcherHandlers.ListWorkspaces, jsonMethodNotAllowed)
		router.Post("/admin/api/dispatcher/workspaces", dispatcherHandlers.CreateWorkspace, jsonMethodNotAllowed)
		router.Post("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/automations", dispatcherHandlers.CreateAutomation, jsonMethodNotAllowed)
		router.Put("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/automations/{automation}", dispatcherHandlers.UpdateAutomation, jsonMethodNotAllowed)
		router.Delete("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/automations/{automation}", dispatcherHandlers.DeleteAutomation, jsonMethodNotAllowed)
		router.Get("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/files", dispatcherHandlers.ListWorkspaceFiles, jsonMethodNotAllowed)
		router.Get("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/files/{file}", dispatcherHandlers.ReadWorkspaceFile, jsonMethodNotAllowed)
		router.Put("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/files/{file}", dispatcherHandlers.WriteWorkspaceFile, jsonMethodNotAllowed)
		router.Delete("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/files/{file}", dispatcherHandlers.DeleteWorkspaceFile, jsonMethodNotAllowed)
		router.Get("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/state", dispatcherHandlers.GetWorkspaceState, jsonMethodNotAllowed)
		router.Get("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/config", dispatcherHandlers.GetWorkspaceConfig, jsonMethodNotAllowed)
		router.Put("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/config", dispatcherHandlers.UpdateWorkspaceConfig, jsonMethodNotAllowed)
		router.Get("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/live", dispatcherHandlers.StreamWorkspaceEvents, textMethodNotAllowed)
		router.Get("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/processlogs", admin.AdminWorkspaceProcessLogsHandler, jsonMethodNotAllowed)
		router.Delete("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}", dispatcherHandlers.DeleteWorkspace, jsonMethodNotAllowed)
	}

	router.Get("/admin/", admin.AdminPageHandler, textMethodNotAllowed)

	// Proxy
	router.Any("/v1/chat/completions", http.HandlerFunc(proxyHandlers.EnsureModelProxyHandler))

	// Conversation API
	router.Any("/admin/api/conversation/message", assistant)
	router.Post("/admin/api/conversation/guardrail-decision", assistant.GuardrailDecisionHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/conversation/sessions/{"+models.WorkspaceIDParam+"}", assistant.ListSessions, jsonMethodNotAllowed)
	router.Get("/admin/api/conversation/sessions/{"+models.WorkspaceIDParam+"}/{session}", assistant.GetSession, jsonMethodNotAllowed)
	router.Delete("/admin/api/conversation/sessions/{"+models.WorkspaceIDParam+"}/{session}", assistant.DeleteSession, jsonMethodNotAllowed)

	// Memory API
	if memoryHandlers != nil {
		router.Get("/admin/api/memory/{"+models.WorkspaceIDParam+"}", memoryHandlers.ListMemories, jsonMethodNotAllowed)
		router.Post("/admin/api/memory/{"+models.WorkspaceIDParam+"}/search", memoryHandlers.SearchMemories, jsonMethodNotAllowed)
		router.Get("/admin/api/memory/{"+models.WorkspaceIDParam+"}/{id}", memoryHandlers.GetMemory, jsonMethodNotAllowed)
		router.Put("/admin/api/memory/{"+models.WorkspaceIDParam+"}/{id}", memoryHandlers.UpdateMemory, jsonMethodNotAllowed)
		router.Delete("/admin/api/memory/{"+models.WorkspaceIDParam+"}/{id}", memoryHandlers.DeleteMemory, jsonMethodNotAllowed)
		router.Delete("/admin/api/memory/{"+models.WorkspaceIDParam+"}", memoryHandlers.ClearWorkspace, jsonMethodNotAllowed)
	}

	return router
}
