package app

import (
	"net/http"

	"llm-proxy/internal/buildinfo"
	assistantPkg "llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/automation"
	"llm-proxy/internal/core/tools"
	api "llm-proxy/internal/transport/http"
	handlers "llm-proxy/internal/transport/http/handlers"
	"llm-proxy/models"
)

// HandlerSet bundles all constructed HTTP handler types.
type HandlerSet struct {
	Admin      *handlers.AdminHandlers
	System     *handlers.SystemHandlers
	Secrets    *handlers.SecretsHandlers
	Process    *handlers.ProcessHandlers
	MCP        *handlers.MCPHandlers
	Model      *handlers.ModelHandlers
	Proxy      *handlers.ProxyHandlers
	Assistant  *handlers.AssistantMessageHandler
	ActiveRuns *handlers.ActiveRunsHandler
	Dispatcher *handlers.DispatcherHandlers
	Recordings *handlers.RecordingHandlers
	Memory     *handlers.MemoryHandlers
	Webhook    *handlers.WebhookHandler
}

// wireHandlers constructs all HTTP handler types from AppServices + Dispatcher.
func wireHandlers(s *AppServices, disp *automation.Dispatcher, buildInfo *buildinfo.Info) *HandlerSet {
	hs := &HandlerSet{}
	hs.Assistant = handlers.NewAssistantMessageHandler(s)
	// Authoritative per-workspace "running" source, aggregating the assistant
	// and automation subsystems so the frontend has one endpoint to poll.
	hs.ActiveRuns = handlers.NewActiveRunsHandler(
		hs.Assistant.RunningExists,
		disp.IsAutomationRunning,
		hs.Assistant.RunningConversationID,
	)
	hs.Admin = handlers.NewAdminHandlers(s.Runtime, s.AppCtx, s.Logger(), buildInfo, s.AppCtx.Orchestrator())
	hs.System = handlers.NewSystemHandlers(s.AppCtx, s.Logger(), buildInfo)
	hs.Secrets = handlers.NewSecretsHandlers(s.AppCtx)
	hs.Process = handlers.NewProcessHandlers(s.Runtime, s.AppCtx, s.Logger())
	hs.MCP = handlers.NewMCPHandlers(s.AppCtx)
	hs.Model = handlers.NewModelHandlers(s.Runtime, s.AppCtx)
	hs.Proxy = handlers.NewProxyHandlers(s.Runtime)
	if disp != nil {
		wsSvc := handlers.NewWorkspaceService(s.Persistence())
		hs.Dispatcher = handlers.NewDispatcherHandlers(disp, wsSvc, s.logger)
	}

	// Register recording routes unconditionally. The handler safely returns
	// enabled:false when no --record is configured.
	hs.Recordings = handlers.NewRecordingHandlers(s.RecordingStore)

	if store := s.AppCtx.MemoryStore(); store != nil {
		hs.Memory = handlers.NewMemoryHandlers(store)
	}

	// Extract CommunicationTools from the tool provider chain
	var commTools *tools.CommunicationTools
	if mtp, ok := s.ToolProvider().(*assistantPkg.MultiToolProvider); ok {
		for _, p := range mtp.Providers {
			if ltr, ok := p.(*assistantPkg.LocalToolRegistry); ok {
				commTools = ltr.Communication
				break
			}
		}
	}

	hs.Webhook = &handlers.WebhookHandler{
		Registry:    s.AppCtx.GetRegistry,
		Persistence: s.Persistence(),
		Events:      s.Events(),
		CommTools:   commTools,
		Dispatcher:  disp,
		Assistant:   hs.Assistant,
		Logger:      s.Logger(),
	}

	return hs
}

func buildHTTP(s *AppServices, disp *automation.Dispatcher, buildInfo *buildinfo.Info) http.Handler {
	hs := wireHandlers(s, disp, buildInfo)

	// Register the clear-runtime-data active-work guard: refuse to clear runtime
	// state while any assistant run or automation is executing (Phase 10). The
	// checker fails closed if workspaces cannot be enumerated.
	if disp != nil {
		s.AppCtx.SetActiveWorkChecker(func() bool {
			workspaces, err := s.Persistence().ListWorkspaces()
			if err != nil {
				return true
			}
			for _, ws := range workspaces {
				if hs.Assistant.RunningExists(ws.ID) || disp.IsAutomationRunning(ws.ID) {
					return true
				}
			}
			return false
		})
	}

	return buildRouter(hs)
}

func buildRouter(hs *HandlerSet) http.Handler {

	router := api.NewRouter()

	jsonMethodNotAllowed := api.WithMethodNotAllowed(http.HandlerFunc(api.MethodNotAllowedJSON))
	textMethodNotAllowed := api.WithMethodNotAllowed(http.HandlerFunc(api.MethodNotAllowedText))

	// Admin — state, page, balance, webhook
	router.Get("/admin/api/state", hs.Admin.AdminStateHandler, textMethodNotAllowed)
	router.Get("/admin/api/orchestrator/balance", hs.Admin.AdminICUBalanceHandler, jsonMethodNotAllowed)
	router.Post("/admin/api/connectors/{name}/webhook", hs.Admin.AdminConnectorWebhookHandler, jsonMethodNotAllowed)
	router.Get("/admin/", hs.Admin.AdminPageHandler, textMethodNotAllowed)

	// System — version, config, host, terminal, restart
	router.Get("/admin/api/version", hs.System.AdminVersionHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/config", hs.System.AdminConfigHandler, jsonMethodNotAllowed)
	router.Put("/admin/api/config", hs.System.AdminConfigUpdateHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/system", hs.System.AdminSystemHandler, jsonMethodNotAllowed)
	router.Put("/admin/api/system", hs.System.AdminSystemPutHandler, jsonMethodNotAllowed)
	router.Post("/admin/api/system/restart", hs.System.AdminRestartHandler, jsonMethodNotAllowed)
	router.Post("/admin/api/system/factory-reset", hs.System.AdminFactoryResetHandler, jsonMethodNotAllowed)
	router.Post("/admin/api/system/clear-runtime-data", hs.System.AdminClearRuntimeDataHandler, jsonMethodNotAllowed)
	router.Post("/admin/api/system/wipeout", hs.System.AdminWipeoutHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/host", hs.System.AdminHostSettingsHandler, jsonMethodNotAllowed)
	router.Put("/admin/api/host", hs.System.AdminHostSettingsPutHandler, jsonMethodNotAllowed)
	router.Post("/admin/api/host/terminal/reset", hs.System.AdminTerminalResetHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/host/terminal/sessions", hs.System.AdminTerminalSessionsHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/logs", hs.Process.AdminLogsHandler, jsonMethodNotAllowed)
	router.Delete("/admin/api/logs", hs.Process.AdminLogsClearHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/log-level", hs.Process.AdminLogLevelHandler, jsonMethodNotAllowed)
	router.Put("/admin/api/log-level", hs.Process.AdminLogLevelUpdateHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/app-logs", hs.Process.AdminAppLogsHandler, textMethodNotAllowed)
	router.Delete("/admin/api/app-logs", hs.Process.AdminAppLogsClearHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/app-logs/tail", hs.Process.AdminAppLogsTailHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/metrics", hs.Process.AdminMetricsHandler, jsonMethodNotAllowed)
	router.Post("/admin/api/start", hs.Process.AdminStartHandler, jsonMethodNotAllowed)
	router.Post("/admin/api/stop", hs.Process.AdminStopHandler, textMethodNotAllowed)
	router.Get("/admin/api/runtime/processes", hs.Process.AdminProcessesHandler, jsonMethodNotAllowed)
	router.Post("/admin/api/runtime/processes/{pid}/stop", hs.Process.AdminProcessKillHandler, jsonMethodNotAllowed)

	// MCP
	router.Get("/admin/api/mcp", hs.MCP.AdminMCPListHandler, jsonMethodNotAllowed)
	router.Post("/admin/api/mcp", hs.MCP.AdminMCPAddHandler, jsonMethodNotAllowed)
	router.Put("/admin/api/mcp", hs.MCP.AdminMCPUpdateHandler, jsonMethodNotAllowed)
	router.Delete("/admin/api/mcp", hs.MCP.AdminMCPRemoveHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/providers/models", hs.Model.AdminListProviderModelsHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/providers/manifests", hs.Model.AdminListProviderManifestsHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/providers/test", hs.Model.AdminTestProviderConnectionHandler, jsonMethodNotAllowed)

	// Model registry — CRUD + registry view
	router.Post("/admin/api/models", hs.Model.AdminAddModelHandler, jsonMethodNotAllowed)
	router.Put("/admin/api/models", hs.Model.AdminUpdateModelHandler, jsonMethodNotAllowed)
	router.Delete("/admin/api/models", hs.Model.AdminDeleteModelHandler, jsonMethodNotAllowed)
	router.Delete("/admin/api/models/all", hs.Model.AdminDeleteAllModelsHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/registry", hs.Model.AdminRegistryHandler, jsonMethodNotAllowed)
	router.Put("/admin/api/registry", hs.Model.AdminRegistryPutHandler, jsonMethodNotAllowed)

	// Templates
	router.Get("/admin/api/templates", hs.Admin.ListTemplatesHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/templates/", hs.Admin.GetTemplateHandler, jsonMethodNotAllowed)

	// Secrets — provider API keys
	router.Get("/admin/api/secrets/keys", hs.Secrets.AdminProviderKeysHandler, jsonMethodNotAllowed)
	router.Put("/admin/api/secrets/keys", hs.Secrets.AdminProviderKeysPutHandler, jsonMethodNotAllowed)
	router.Delete("/admin/api/secrets/keys", hs.Secrets.AdminProviderKeyDeleteHandler, jsonMethodNotAllowed)

	// Secrets — tool secrets (search, communication, etc.)
	router.Get("/admin/api/secrets/tools", hs.Secrets.AdminToolSecretHandler, jsonMethodNotAllowed)
	router.Put("/admin/api/secrets/tools", hs.Secrets.AdminToolSecretPutHandler, jsonMethodNotAllowed)

	// Recordings
	if hs.Recordings != nil {
		router.Get("/admin/api/recordings", hs.Recordings.List, jsonMethodNotAllowed)
		router.Get("/admin/api/recordings/status", hs.Recordings.Status, jsonMethodNotAllowed)
		router.Get("/admin/api/recordings/{id}", hs.Recordings.Get, jsonMethodNotAllowed)
		router.Delete("/admin/api/recordings/{id}", hs.Recordings.Delete, jsonMethodNotAllowed)
	}

	// Dispatcher
	if hs.Dispatcher != nil {
		router.Get("/admin/api/dispatcher/automations", hs.Dispatcher.ListAutomations, jsonMethodNotAllowed)
		router.Post("/admin/api/dispatcher/trigger/{"+models.WorkspaceIDParam+"}/{automation}", hs.Dispatcher.TriggerAutomation, jsonMethodNotAllowed)
		router.Post("/admin/api/dispatcher/stop/{"+models.WorkspaceIDParam+"}", hs.Dispatcher.StopAutomation, jsonMethodNotAllowed)
		router.Get("/admin/api/dispatcher/metrics", hs.Dispatcher.GetDispatcherMetrics, jsonMethodNotAllowed)
		router.Get("/admin/api/dispatcher/activity", hs.Dispatcher.GetGlobalActivity, jsonMethodNotAllowed)

		router.Get("/admin/api/dispatcher/workspaces", hs.Dispatcher.ListWorkspaces, jsonMethodNotAllowed)
		router.Post("/admin/api/dispatcher/workspaces", hs.Dispatcher.CreateWorkspace, jsonMethodNotAllowed)
		router.Post("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/automations", hs.Dispatcher.CreateAutomation, jsonMethodNotAllowed)
		router.Put("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/automations/{automation}", hs.Dispatcher.UpdateAutomation, jsonMethodNotAllowed)
		router.Delete("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/automations/{automation}", hs.Dispatcher.DeleteAutomation, jsonMethodNotAllowed)
		router.Get("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/files", hs.Dispatcher.ListWorkspaceFiles, jsonMethodNotAllowed)
		router.Get("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/files/{file}", hs.Dispatcher.ReadWorkspaceFile, jsonMethodNotAllowed)
		router.Put("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/files/{file}", hs.Dispatcher.WriteWorkspaceFile, jsonMethodNotAllowed)
		router.Delete("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/files/{file}", hs.Dispatcher.DeleteWorkspaceFile, jsonMethodNotAllowed)
		router.Get("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/state", hs.Dispatcher.GetWorkspaceState, jsonMethodNotAllowed)
		router.Get("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/config", hs.Dispatcher.GetWorkspaceConfig, jsonMethodNotAllowed)
		router.Put("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/config", hs.Dispatcher.UpdateWorkspaceConfig, jsonMethodNotAllowed)
		router.Get("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/live", hs.Dispatcher.StreamWorkspaceEvents, textMethodNotAllowed)
		router.Get("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/processlogs", hs.Process.AdminWorkspaceProcessLogsHandler, jsonMethodNotAllowed)
		router.Delete("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}", hs.Dispatcher.DeleteWorkspace, jsonMethodNotAllowed)
		router.Delete("/admin/api/dispatcher/runs/{"+models.WorkspaceIDParam+"}/run/{run}", hs.Dispatcher.DeleteRun, jsonMethodNotAllowed)
		router.Delete("/admin/api/dispatcher/runs/{"+models.WorkspaceIDParam+"}/{automation}", hs.Dispatcher.DeleteAutomationRuns, jsonMethodNotAllowed)
	}

	router.Get("/admin/", hs.Admin.AdminPageHandler, textMethodNotAllowed)

	// Proxy
	router.Any("/v1/chat/completions", http.HandlerFunc(hs.Proxy.EnsureModelProxyHandler))
	// OpenAI-compatible model catalogue for external OpenAI-format clients
	// pointed at this proxy (metadata discovery; see ProxyHandlers.ModelsListHandler).
	router.Get("/v1/models", http.HandlerFunc(hs.Proxy.ModelsListHandler), jsonMethodNotAllowed)

	// Conversation API
	router.Any("/admin/api/conversation/message", hs.Assistant)
	router.Post("/admin/api/conversation/cancel", hs.Assistant.CancelAssistantHandler, jsonMethodNotAllowed)
	router.Post("/admin/api/conversation/guardrail-decision", hs.Assistant.GuardrailDecisionHandler, jsonMethodNotAllowed)
	router.Get("/admin/api/conversation/sessions/{"+models.WorkspaceIDParam+"}", hs.Assistant.ListSessions, jsonMethodNotAllowed)
	router.Get("/admin/api/conversation/sessions/{"+models.WorkspaceIDParam+"}/{session}", hs.Assistant.GetSession, jsonMethodNotAllowed)
	router.Delete("/admin/api/conversation/sessions/{"+models.WorkspaceIDParam+"}/{session}", hs.Assistant.DeleteSession, jsonMethodNotAllowed)
	router.Delete("/admin/api/conversation/sessions/{"+models.WorkspaceIDParam+"}", hs.Assistant.DeleteAllSessions, jsonMethodNotAllowed)
	router.Patch("/admin/api/conversation/sessions/{"+models.WorkspaceIDParam+"}/{session}", hs.Assistant.RenameSession, jsonMethodNotAllowed)
	router.Get("/admin/api/workspaces/{"+models.WorkspaceIDParam+"}/active-runs", hs.ActiveRuns.ServeHTTP, jsonMethodNotAllowed)

	// Memory API
	if hs.Memory != nil {
		router.Get("/admin/api/memory/{"+models.WorkspaceIDParam+"}", hs.Memory.ListMemories, jsonMethodNotAllowed)
		router.Post("/admin/api/memory/{"+models.WorkspaceIDParam+"}/search", hs.Memory.SearchMemories, jsonMethodNotAllowed)
		router.Get("/admin/api/memory/{"+models.WorkspaceIDParam+"}/{id}", hs.Memory.GetMemory, jsonMethodNotAllowed)
		router.Put("/admin/api/memory/{"+models.WorkspaceIDParam+"}/{id}", hs.Memory.UpdateMemory, jsonMethodNotAllowed)
		router.Delete("/admin/api/memory/{"+models.WorkspaceIDParam+"}/{id}", hs.Memory.DeleteMemory, jsonMethodNotAllowed)
		router.Delete("/admin/api/memory/{"+models.WorkspaceIDParam+"}", hs.Memory.ClearWorkspace, jsonMethodNotAllowed)
	}

	// Public webhooks — external platforms POST here (no admin auth)
	if hs.Webhook != nil {
		router.Post("/api/v1/webhooks/{connector_name}", hs.Webhook.ServeHTTP, jsonMethodNotAllowed)
	}

	return router
}
