package app

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"path/filepath"

	"llm-proxy/internal/api"
	"llm-proxy/internal/assistant"
	"llm-proxy/internal/buildinfo"
	"llm-proxy/internal/config"
	"llm-proxy/internal/dispatcher"
	"llm-proxy/internal/llm"
	"llm-proxy/internal/logging"
	"llm-proxy/internal/mcp"
	"llm-proxy/internal/nodeherder"
	"llm-proxy/internal/persistence"
	"llm-proxy/internal/proxy"
	"llm-proxy/internal/ratelimiter"
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
	Core   Core
	Infra  Infra
	Dispatcher *dispatcher.Dispatcher
}

type AssistantService interface {
	NodeHerder() nodeherder.MCPService
	ClientProvider() proxy.LLMClientProvider
	Limiter() ratelimiter.Limiter
	Logger() logging.Logger
	DefaultModel() (string, error)
}

func (c *Container) BuildAppServices() AppServices {
	s := AppServices{
		Runtime:    c.Core.Runtime,
		AppCtx:     c.Core.AppCtx,
		nodeHerder: c.Infra.NodeHerder,
		logger:     c.Infra.Logger,
		Clock:      c.Infra.Clock,
	}

	factory := func(baseURL string) proxy.Client {
		return proxy.NewLLMClient(baseURL, nil)
	}

	s.clientProvider = proxy.NewRuntimeClientProvider(s, c.Core.Runtime, factory)
	s.engine = assistant.NewEngine(s.nodeHerder, s.logger)

	return s
}

type AppServices struct {
	Runtime        llm.RuntimeManager
	AppCtx         *AppContext
	nodeHerder     nodeherder.MCPService
	clientProvider proxy.LLMClientProvider
	engine         assistant.Engine
	logger         logging.Logger
	Clock          utils.Clock
}

func (s AppServices) NodeHerder() nodeherder.MCPService {
	return s.nodeHerder
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

func (s AppServices) Engine() assistant.Engine {
	return s.engine
}

func bootstrap(cfgMgr *config.ConfigManager, logger logging.Logger) *Container {
	if logger == nil {
		log.Fatal("Logger is required")
	}
	clock := utils.NewRealClock()

	// Configure MCP Service
	nodeHerder, err := configureMCP(cfgMgr, logger)
	if err != nil {
		logger.Error("Failed to configure MCP service", "error", err)
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
func (c *Container) BuildDispatcher(svc AssistantService) (*dispatcher.Dispatcher, error) {
	baseDir := filepath.Join(c.Core.AppCtx.ModelDir(), "..", "workspaces")
	persistenceMgr := persistence.NewWorkspaceManager(baseDir)

	// Phase 3: Using LLM-backed executor
	exec := dispatcher.NewLLMTaskExecutor(svc)

	d, err := dispatcher.NewDispatcher(persistenceMgr, exec, slog.Default(),
		dispatcher.WithWorkerCount(1),
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

func buildHTTP(s AppServices, disp *dispatcher.Dispatcher, buildInfo *buildinfo.Info) http.Handler {
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

	// Dispatcher (Phase 2 & 4)
	if dispatcherHandlers != nil {
		router.Get("/admin/api/dispatcher/automations", dispatcherHandlers.ListAutomations, jsonMethodNotAllowed)
		router.Post("/admin/api/dispatcher/trigger/{workspace}/{automation}", dispatcherHandlers.TriggerAutomation, jsonMethodNotAllowed)
		router.Get("/admin/api/dispatcher/metrics", dispatcherHandlers.GetDispatcherMetrics, jsonMethodNotAllowed)
		
		router.Get("/admin/api/dispatcher/workspaces", dispatcherHandlers.ListWorkspaces, jsonMethodNotAllowed)
		router.Post("/admin/api/dispatcher/workspaces", dispatcherHandlers.CreateWorkspace, jsonMethodNotAllowed)
		router.Get("/admin/api/dispatcher/workspaces/{workspace}/files", dispatcherHandlers.ListWorkspaceFiles, jsonMethodNotAllowed)
		router.Get("/admin/api/dispatcher/workspaces/{workspace}/files/{file}", dispatcherHandlers.ReadWorkspaceFile, jsonMethodNotAllowed)
		router.Put("/admin/api/dispatcher/workspaces/{workspace}/files/{file}", dispatcherHandlers.WriteWorkspaceFile, jsonMethodNotAllowed)
	}

	router.Get("/admin/", admin.AdminPageHandler, textMethodNotAllowed)

	// Proxy
	router.Any("/v1/chat/completions", http.HandlerFunc(proxyHandlers.EnsureModelProxyHandler))

	// Conversation API
	router.Any("/api/conversation/message", assistant)

	return router
}
