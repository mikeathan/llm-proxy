package app

import (
	"context"
	"log"
	"net/http"

	"llm-proxy/internal/buildinfo"
	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/automation"
	"llm-proxy/internal/core/llm"
	"llm-proxy/internal/core/mcp"
	"llm-proxy/internal/core/nodeherder"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/ratelimiter"
	"llm-proxy/internal/platform/storage"
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
	Core       Core
	Infra      Infra
	Dispatcher *automation.Dispatcher
}

// Building automation task executor
func (c *Container) BuildTaskExecutor(svc api.AssistantService) automation.TaskExecutor {
	return automation.NewLLMTaskExecutor(svc)
}

func (c *Container) BuildAppServices() *AppServices {
	s := &AppServices{
		Runtime:     c.Core.Runtime,
		AppCtx:      c.Core.AppCtx,
		nodeHerder:  c.Infra.NodeHerder,
		logger:      c.Infra.Logger,
		Clock:       c.Infra.Clock,
		persistence: persistence.NewWorkspaceManager(c.Core.AppCtx.WorkspacesDir()),
	}

	factory := func(baseURL string, model string, headers http.Header) proxy.Client {
		return proxy.NewLLMClient(baseURL, model, nil, headers)
	}

	s.clientProvider = proxy.NewRuntimeClientProvider(s, c.Core.Runtime, factory)
	s.dispatcher = c.Dispatcher

	// Initialize unified tool providers and engines (Local Registry + Remote MCP)
	s.toolProvider, s.engine, s.guardrailEngine = assistant.InitializeAgentStack(s.AppCtx, s.nodeHerder, s.logger)

	return s
}

type AppServices struct {
	Runtime         llm.RuntimeManager
	AppCtx          *AppContext
	nodeHerder      nodeherder.MCPService
	toolProvider    assistant.ToolProvider
	clientProvider  proxy.LLMClientProvider
	engine          assistant.Engine
	guardrailEngine *assistant.GuardrailEngine
	persistence     *persistence.WorkspaceManager
	logger          logging.Logger
	Clock           utils.Clock
	dispatcher      *automation.Dispatcher
}

func (s AppServices) Shutdown() {
	if s.Runtime != nil {
		logging.Info("Shutting down LLM runtime...")
		s.Runtime.Shutdown()
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
	return ratelimiter.NewLimiter(s.Clock)
}

func (s AppServices) SelectModels() (string, string) {
	return s.AppCtx.SelectModels()
}

func (s AppServices) Engine() assistant.Engine {
	return s.engine
}

func (s AppServices) GuardrailEngine() *assistant.GuardrailEngine {
	return s.guardrailEngine
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

func bootstrap(dataMgr *storage.DataManager, logger logging.Logger) *Container {
	if logger == nil {
		log.Fatal("Logger is required")
	}
	clock := utils.NewRealClock()

	// 1. Load System Config for MCP/Runtime defaults
	logging.Info("Loading system configuration...")
	sys := dataMgr.System().Get()

	// Configure MCP Service (Bridge logic: we still pass sys config parts)
	logging.Info("Configuring MCP services...")
	nodeHerder, err := configureMCP(dataMgr, logger)
	if err != nil {
		logging.Error("Failed to configure MCP service", "error", err)
		return nil
	}

	// 2. Initialize Runtime Manager from Registry
	logging.Info("Initializing LLM runtime manager...")
	registry := dataMgr.Registry().Get()
	secretsStore := storage.NewSecretStore(dataMgr.Secrets())
	manager := llm.NewManagerFromRegistry(registry, sys, secretsStore)

	logging.Info("Creating server context...")
	appCtx := NewServer(manager, dataMgr)
	runtime := appCtx.Manager()

	logging.Info("Bootstrap phase complete", "root", dataMgr.RootDir())

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

func configureMCP(dataMgr *storage.DataManager, logger logging.Logger) (nodeherder.MCPService, error) {
	// Initialize MCP Orchestrator
	orchestrator := mcp.NewOrchestrator(logger)

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
			Name:    s.Name,
			URL:     s.URL,
			Enabled: s.Enabled,
		}
	}
	return out
}

func buildHTTP(s *AppServices, disp *automation.Dispatcher, buildInfo *buildinfo.Info) http.Handler {
	assistant := api.NewAssistantMessageHandler(s)

	adminHandlers := api.NewAdminHandlers(s.Runtime, s.AppCtx, s.Logger(), buildInfo)
	proxyHandlers := api.NewProxyHandlers(s.Runtime)

	var dispatcherHandlers *api.DispatcherHandlers
	if disp != nil {
		dispatcherHandlers = api.NewDispatcherHandlers(disp)
	}

	return buildRouter(adminHandlers, proxyHandlers, assistant, dispatcherHandlers)
}

func buildRouter(
	admin *api.AdminHandlers,
	proxyHandlers *api.ProxyHandlers,
	assistant *api.AssistantMessageHandler,
	dispatcherHandlers *api.DispatcherHandlers,
) http.Handler {

	router := api.NewRouter()

	jsonMethodNotAllowed := api.WithMethodNotAllowed(http.HandlerFunc(api.MethodNotAllowedJSON))
	textMethodNotAllowed := api.WithMethodNotAllowed(http.HandlerFunc(api.MethodNotAllowedText))

	// Admin
	router.Get("/admin/api/state", admin.AdminStateHandler, textMethodNotAllowed)
	router.Post("/admin/api/start", admin.AdminStartHandler, jsonMethodNotAllowed)
	router.Post("/admin/api/stop", admin.AdminStopHandler, textMethodNotAllowed)
	router.Post("/admin/api/models", admin.AdminAddModelHandler, jsonMethodNotAllowed)
	router.Put("/admin/api/models", admin.AdminUpdateModelHandler, jsonMethodNotAllowed)
	router.Delete("/admin/api/models", admin.AdminDeleteModelHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/config", admin.AdminConfigHandler, jsonMethodNotAllowed)
	router.Put("/admin/api/config", admin.AdminConfigUpdateHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/logs", admin.AdminLogsHandler, jsonMethodNotAllowed)
	router.Delete("/admin/api/logs", admin.AdminLogsClearHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/log-level", admin.AdminLogLevelHandler, jsonMethodNotAllowed)
	router.Put("/admin/api/log-level", admin.AdminLogLevelUpdateHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/app-logs", admin.AdminAppLogsHandler, textMethodNotAllowed)
	router.Delete("/admin/api/app-logs", admin.AdminAppLogsClearHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/app-logs/tail", admin.AdminAppLogsTailHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/metrics", admin.AdminMetricsHandler, jsonMethodNotAllowed)

	// MCP
	router.Get("/admin/api/mcp", admin.AdminMCPListHandler, jsonMethodNotAllowed)
	router.Post("/admin/api/mcp", admin.AdminMCPAddHandler, jsonMethodNotAllowed)
	router.Put("/admin/api/mcp", admin.AdminMCPUpdateHandler, jsonMethodNotAllowed)
	router.Delete("/admin/api/mcp", admin.AdminMCPRemoveHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/providers/models", admin.AdminListProviderModelsHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/providers/manifests", admin.AdminListProviderManifestsHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/providers/test", admin.AdminTestProviderConnectionHandler, jsonMethodNotAllowed)

	// Secrets — provider API keys
	router.Get("/admin/api/secrets/keys", admin.AdminProviderKeysHandler, jsonMethodNotAllowed)
	router.Put("/admin/api/secrets/keys", admin.AdminProviderKeysPutHandler, jsonMethodNotAllowed)
	router.Delete("/admin/api/secrets/keys", admin.AdminProviderKeyDeleteHandler, jsonMethodNotAllowed)

	// Secrets — tool secrets (search, communication, etc.)
	router.Get("/admin/api/secrets/tools", admin.AdminToolSecretHandler, jsonMethodNotAllowed)
	router.Put("/admin/api/secrets/tools", admin.AdminToolSecretPutHandler, jsonMethodNotAllowed)

	// Dispatcher
	if dispatcherHandlers != nil {
		router.Get("/admin/api/dispatcher/automations", dispatcherHandlers.ListAutomations, jsonMethodNotAllowed)
		router.Post("/admin/api/dispatcher/trigger/{workspace}/{automation}", dispatcherHandlers.TriggerAutomation, jsonMethodNotAllowed)
		router.Get("/admin/api/dispatcher/metrics", dispatcherHandlers.GetDispatcherMetrics, jsonMethodNotAllowed)
		router.Get("/admin/api/dispatcher/activity", dispatcherHandlers.GetGlobalActivity, jsonMethodNotAllowed)

		router.Get("/admin/api/dispatcher/workspaces", dispatcherHandlers.ListWorkspaces, jsonMethodNotAllowed)
		router.Post("/admin/api/dispatcher/workspaces", dispatcherHandlers.CreateWorkspace, jsonMethodNotAllowed)
		router.Post("/admin/api/dispatcher/workspaces/{workspace}/automations", dispatcherHandlers.CreateAutomation, jsonMethodNotAllowed)
		router.Put("/admin/api/dispatcher/workspaces/{workspace}/automations/{automation}", dispatcherHandlers.UpdateAutomation, jsonMethodNotAllowed)
		router.Delete("/admin/api/dispatcher/workspaces/{workspace}/automations/{automation}", dispatcherHandlers.DeleteAutomation, jsonMethodNotAllowed)
		router.Get("/admin/api/dispatcher/workspaces/{workspace}/files", dispatcherHandlers.ListWorkspaceFiles, jsonMethodNotAllowed)
		router.Get("/admin/api/dispatcher/workspaces/{workspace}/files/{file}", dispatcherHandlers.ReadWorkspaceFile, jsonMethodNotAllowed)
		router.Put("/admin/api/dispatcher/workspaces/{workspace}/files/{file}", dispatcherHandlers.WriteWorkspaceFile, jsonMethodNotAllowed)
		router.Delete("/admin/api/dispatcher/workspaces/{workspace}/files/{file}", dispatcherHandlers.DeleteWorkspaceFile, jsonMethodNotAllowed)
		router.Get("/admin/api/dispatcher/workspaces/{workspace}/state", dispatcherHandlers.GetWorkspaceState, jsonMethodNotAllowed)
		router.Get("/admin/api/dispatcher/workspaces/{workspace}/live", dispatcherHandlers.StreamWorkspaceEvents, textMethodNotAllowed)
		router.Get("/admin/api/dispatcher/workspaces/{workspace}/processlogs", admin.AdminWorkspaceProcessLogsHandler, jsonMethodNotAllowed)
		router.Delete("/admin/api/dispatcher/workspaces/{workspace}", dispatcherHandlers.DeleteWorkspace, jsonMethodNotAllowed)
	}

	router.Get("/admin/", admin.AdminPageHandler, textMethodNotAllowed)

	// Proxy
	router.Any("/v1/chat/completions", http.HandlerFunc(proxyHandlers.EnsureModelProxyHandler))

	// Conversation API
	router.Any("/admin/api/conversation/message", assistant)
	router.Get("/admin/api/conversation/sessions/{workspace}", assistant.ListSessions, jsonMethodNotAllowed)
	router.Get("/admin/api/conversation/sessions/{workspace}/{session}", assistant.GetSession, jsonMethodNotAllowed)
	router.Delete("/admin/api/conversation/sessions/{workspace}/{session}", assistant.DeleteSession, jsonMethodNotAllowed)

	return router
}
