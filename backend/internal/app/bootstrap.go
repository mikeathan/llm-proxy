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
	"llm-proxy/internal/platform/config"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/ratelimiter"
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

func (c *Container) BuildAppServices() AppServices {
	s := AppServices{
		Runtime:    c.Core.Runtime,
		AppCtx:     c.Core.AppCtx,
		nodeHerder: c.Infra.NodeHerder,
		logger:     c.Infra.Logger,
		Clock:      c.Infra.Clock,
	}

	factory := func(baseURL string, model string, headers http.Header) proxy.Client {
		return proxy.NewLLMClient(baseURL, model, nil, headers)
	}

	s.clientProvider = proxy.NewRuntimeClientProvider(s, c.Core.Runtime, factory)

	// Initialize unified tool providers and engines (Local Registry + Remote MCP)
	s.toolProvider, s.engine, s.guardrailEngine = assistant.InitializeAgentStack(s.AppCtx, s.nodeHerder, s.logger)

	return s
}

type AppServices struct {
	Runtime        llm.RuntimeManager
	AppCtx         *AppContext
	nodeHerder     nodeherder.MCPService
	toolProvider    assistant.ToolProvider
	clientProvider  proxy.LLMClientProvider
	engine          assistant.Engine
	guardrailEngine *assistant.GuardrailEngine
	logger          logging.Logger
	Clock          utils.Clock
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

func (s AppServices) DefaultModel() (string, error) {
	return s.AppCtx.DefaultModel()
}

func (s AppServices) PrimaryModel() string {
	return s.AppCtx.PrimaryModel()
}

func (s AppServices) FallbackModel() string {
	return s.AppCtx.FallbackModel()
}

func (s AppServices) Engine() assistant.Engine {
	return s.engine
}

func (s AppServices) Config() *models.Config {
	return s.AppCtx.Config()
}

func (s AppServices) GuardrailEngine() *assistant.GuardrailEngine {
	return s.guardrailEngine
}

func bootstrap(cfgMgr *config.ConfigManager, logger logging.Logger) *Container {
	if logger == nil {
		log.Fatal("Logger is required")
	}
	clock := utils.NewRealClock()

	// Configure MCP Service
	nodeHerder, err := configureMCP(cfgMgr, logger)
	if err != nil {
		logging.Error("Failed to configure MCP service", "error", err)
		return nil
	}

	cfg := cfgMgr.GetConfig()
	manager := llm.NewManagerFromConfig(&cfg)

	appCtx := NewServer(manager, cfgMgr)
	runtime := appCtx.Manager()

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
	// Use workspaces_dir from config, or default to {rootDir}/workspaces
	baseDir := c.Core.AppCtx.WorkspacesDir()
	persistenceMgr := persistence.NewWorkspaceManager(baseDir)

	exec := automation.NewLLMTaskExecutor(svc)

	d, err := automation.NewDispatcher(persistenceMgr, exec, c.Infra.Logger,
		automation.WithWorkerCount(1),
	)
	if err != nil {
		return nil, err
	}

	return d, nil
}

func configureMCP(cfgMgr *config.ConfigManager, logger logging.Logger) (nodeherder.MCPService, error) {
	// Initialize MCP Orchestrator
	orchestrator := mcp.NewOrchestrator(logger)

	// Initialize Resource Mirror
	mirror := mcp.NewResourceMirror()

	// Subscribe ConfigManager -> MCP Orchestrator
	cfgMgr.OnChange(func(newCfg models.Config) {
		orchestrator.Reload(context.Background(), newCfg.MCPServers, newCfg.Server.Bind)
	})

	// Initial Load
	currentCfg := cfgMgr.GetConfig()
	orchestrator.Reload(context.Background(), currentCfg.MCPServers, currentCfg.Server.Bind)

	// Register prompt updates handled by Orchestrator (which propagates to Clients)
	// The Orchestrator's OnPromptUpdate is called when a client receives a notification.
	orchestrator.OnPromptUpdate(func(prompt string) {
		mirror.SetSystemPrompt(prompt)
	})

	// Subscribe to system prompt to receive updates
	// This ensures we get notified when NodeHerder loads devices or updates context
	orchestrator.Subscribe(context.Background(), "nodeherder://system-prompt")

	return mcp.NewMCPNodeHerder(orchestrator, mirror, logger), nil
}

func buildHTTP(s AppServices, disp *automation.Dispatcher, buildInfo *buildinfo.Info) http.Handler {
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
	assistant http.Handler,
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
	router.Get("/admin/api/metrics", admin.AdminMetricsHandler, jsonMethodNotAllowed)

	// MCP
	router.Get("/admin/api/mcp", admin.AdminMCPListHandler, jsonMethodNotAllowed)
	router.Post("/admin/api/mcp", admin.AdminMCPAddHandler, jsonMethodNotAllowed)
	router.Put("/admin/api/mcp", admin.AdminMCPUpdateHandler, jsonMethodNotAllowed)
	router.Delete("/admin/api/mcp", admin.AdminMCPRemoveHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/providers/models", admin.AdminListProviderModelsHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/providers/manifests", admin.AdminListProviderManifestsHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/providers/test", admin.AdminTestProviderConnectionHandler, jsonMethodNotAllowed)

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
		router.Delete("/admin/api/dispatcher/workspaces/{workspace}", dispatcherHandlers.DeleteWorkspace, jsonMethodNotAllowed)
	}

	router.Get("/admin/", admin.AdminPageHandler, textMethodNotAllowed)

	// Proxy
	router.Any("/v1/chat/completions", http.HandlerFunc(proxyHandlers.EnsureModelProxyHandler))

	// Conversation API
	router.Any("/api/conversation/message", assistant)

	return router
}
