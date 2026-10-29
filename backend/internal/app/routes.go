package app

import (
	"llm-proxy/internal/core/chat"
	"llm-proxy/internal/core/eventbus"
	"net/http"
	"os"

	"llm-proxy/internal/buildinfo"
	"llm-proxy/internal/core/automation"
	"llm-proxy/internal/core/tools"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/ratelimiter"
	api "llm-proxy/internal/transport/http"
	handlers "llm-proxy/internal/transport/http/handlers"
	"llm-proxy/models"
	"llm-proxy/utils"
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
	Turns      *chat.Service
	Assistant  *handlers.AssistantMessageHandler
	ActiveRuns *handlers.ActiveRunsHandler
	Dispatcher *handlers.DispatcherHandlers
	Recordings *handlers.RecordingHandlers
	Memory     *handlers.MemoryHandlers
	Reminders  *handlers.ReminderHandlers
	Profiles   *handlers.ProfileHandlers
	Helpers    *handlers.HelpersHandlers
	Netaudit   *handlers.NetauditHandlers
	VulnData   *handlers.VulnDataHandlers
	Commands   *handlers.CommandHandlers
	Webhook    *handlers.WebhookHandler
	Auth       *handlers.AuthHandlers
	Exposure   *handlers.ExposureHandlers
}

// wireHandlers constructs all HTTP handler types from AppServices + Dispatcher.
func wireHandlers(s *AppServices, disp *automation.Dispatcher, buildInfo *buildinfo.Info) *HandlerSet {
	hs := &HandlerSet{}
	if s.auth != nil {
		trusted := func() []string { return s.AppCtx.GetSystem().Server.TrustedProxies }
		warnInvalidTrustedProxies(trusted())
		hs.Auth = handlers.NewAuthHandlers(s.auth).WithTrustedProxies(trusted).WithSessionLength(
			func() models.AuthSettings { return s.AppCtx.GetSystem().Server.Auth }, s.AppCtx.SetAuthSettings)
		hs.Exposure = handlers.NewExposureHandlers(handlers.ExposureSource{
			Auth:           s.auth,
			Bind:           func() string { return bindAddress(s.AppCtx.GetSystem()) },
			Registry:       s.AppCtx.GetRegistry,
			Host:           s.AppCtx.HostSettings,
			TrustedProxies: trusted,
		})
	}
	ownerZone := func() string { return s.AppCtx.GetSettings().Reminders.Effective().Timezone }
	hs.Turns = chat.New(s, ownerZone)
	hs.Assistant = handlers.NewAssistantMessageHandler(s, hs.Turns)
	// Operator actions on queued inbound callers. These are the only paths that
	// cancel a run to free the local model, alongside the host's explicit
	// inbound-preempt policy.
	var promoteModelWait func(string) error
	var cancelModelWait func(string) bool
	if lane := s.Lane(); lane != nil {
		promoteModelWait = lane.PromoteModelWait
		cancelModelWait = lane.CancelModelWait
	}
	// Authoritative per-workspace "running" source, aggregating the assistant
	// and automation subsystems so the frontend has one endpoint to poll.
	hs.ActiveRuns = handlers.NewActiveRunsHandler(handlers.ActiveRunsSources{
		AssistantRunning:        hs.Turns.Running,
		AutomationRunning:       disp.IsAutomationRunning,
		AssistantConversationID: hs.Turns.RunningConversation,
		AssistantQueued:         hs.Turns.Queued,
		LaneSnapshot:            disp.LaneSnapshot,
		PromoteModelWait:        promoteModelWait,
		CancelModelWait:         cancelModelWait,
	})
	hs.Admin = handlers.NewAdminHandlers(s.Runtime, s.AppCtx, s.Logger(), buildInfo, s.AppCtx.Orchestrator())
	hs.System = handlers.NewSystemHandlers(s.AppCtx, s.Logger(), buildInfo)
	hs.Secrets = handlers.NewSecretsHandlers(s.AppCtx)
	hs.Process = handlers.NewProcessHandlers(s.Runtime, s.AppCtx, s.Logger())
	hs.MCP = handlers.NewMCPHandlers(s.AppCtx)
	hs.Model = handlers.NewModelHandlers(s.Runtime, s.AppCtx)
	hs.Proxy = handlers.NewProxyHandlers(s.Runtime)
	// External /v1 callers go through the model-residency gate: a request that
	// would evict the model a run is using waits or is refused instead of
	// killing the run. Policy comes from the live scheduler settings.
	if s.Lane() != nil {
		hs.Proxy.SetInboundGate(s.gate())
	}
	if disp != nil {
		wsSvc := handlers.NewWorkspaceService(s.Persistence())
		hs.Dispatcher = handlers.NewDispatcherHandlers(disp, wsSvc, s.logger).WithProfiles(s.Profiles())
	}
	hs.Profiles = handlers.NewProfileHandlers(s.Profiles())
	hs.Helpers = handlers.NewHelpersHandlers(s)
	hs.Netaudit = handlers.NewNetauditHandlers(s.netaudit.Scope, s.netaudit.Readiness, s.Persistence(), s.netaudit.Acks).WithFixes(s.netaudit.Fixes).WithExpectations(s.netaudit.Expectations).WithDeepChecks(s.netaudit.Deep)
	hs.VulnData = handlers.NewVulnDataHandlers(s.netaudit.Vulns)

	// Register recording routes unconditionally. The handler safely returns
	// enabled:false when no --record is configured.
	hs.Recordings = handlers.NewRecordingHandlers(s.RecordingStore, s.RecordDir(), tools.RedactForRecording)

	if store := s.AppCtx.MemoryStore(); store != nil {
		hs.Memory = handlers.NewMemoryHandlers(store).WithModelResolver(s.ModelConfig).WithSessionRetention(s.AppCtx.SessionRetention)
	}

	if s.Reminders != nil {
		hs.Reminders = handlers.NewReminderHandlers(reminderZone(s), s.Reminders)
	}

	commTools := communicationTools(s.ToolProvider())

	registry := buildCommands(s, disp)
	hs.Commands = handlers.NewCommandHandlers(registry, ownerZone)
	var automationEvents *eventbus.Bus
	if disp != nil {
		automationEvents = disp.Events()
	}
	hs.Webhook = &handlers.WebhookHandler{
		Commands:         registry,
		Registry:         s.AppCtx.GetRegistry,
		Persistence:      s.Persistence(),
		Events:           s.Events(),
		CommTools:        commTools,
		AutomationEvents: automationEvents,
		Turns:            hs.Turns,
		Logger:           s.Logger(),
		Limiter:          ratelimiter.NewLimiter(utils.NewRealClock()),
		// Connector messages carry no browser timezone; the owner's configured zone
		// keeps "tomorrow morning" meaning the owner's morning.
		OwnerTimezone: ownerZone,
	}

	return hs
}

func buildHTTP(s *AppServices, disp *automation.Dispatcher, buildInfo *buildinfo.Info) http.Handler {
	hs := wireHandlers(s, disp, buildInfo)

	// Register the clear-runtime-data active-work guard: refuse to clear runtime
	// state while any assistant run or automation is executing (Phase 10). The
	// checker fails closed if workspaces cannot be enumerated.
	if disp != nil {
		s.AppCtx.SetActiveWorkChecker(activeWorkChecker(workProbe{
			listWorkspaces: func() ([]string, error) {
				workspaces, err := s.Persistence().ListWorkspaces()
				ids := make([]string, 0, len(workspaces))
				for _, ws := range workspaces {
					ids = append(ids, ws.ID)
				}
				return ids, err
			},
			assistantBusy:  func(id string) bool { return hs.Turns.Running(id) || hs.Turns.Queued(id) },
			automationBusy: disp.IsAutomationRunning,
			// Queued lane entries are active work too: the lanes are in-memory, so clearing runtime data with a
			// queued run would silently drop it.
			laneQueued: func() bool {
				for _, lane := range disp.LaneSnapshot().Lanes {
					if len(lane.Queued) > 0 {
						return true
					}
				}
				return false
			},
		}))
	}

	hostname, _ := os.Hostname()
	var inner http.Handler = buildRouter(hs)
	if s.auth != nil {
		inner = api.Authenticate(inner, s.auth)
	}
	return api.Guard(inner, api.GuardConfig{
		Hostname:     hostname,
		AllowedHosts: func() []string { return s.AppCtx.GetSystem().Server.AllowedHosts },
	})
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
	router.Post("/admin/api/providers/test", hs.Model.AdminTestProviderConnectionHandler, jsonMethodNotAllowed)

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
	router.Delete("/admin/api/secrets/tools", hs.Secrets.AdminToolSecretDeleteHandler, jsonMethodNotAllowed)

	// Recordings
	if hs.Recordings != nil {
		router.Get("/admin/api/recordings", hs.Recordings.List, jsonMethodNotAllowed)
		router.Get("/admin/api/recordings/status", hs.Recordings.Status, jsonMethodNotAllowed)
		router.Get("/admin/api/recordings/content/{id...}", hs.Recordings.Content, jsonMethodNotAllowed)
		router.Get("/admin/api/recordings/{id}", hs.Recordings.Get, jsonMethodNotAllowed)
		router.Delete("/admin/api/recordings/{id}", hs.Recordings.Delete, jsonMethodNotAllowed)
	}

	// Dispatcher
	if hs.Dispatcher != nil {
		router.Get("/admin/api/dispatcher/automations", hs.Dispatcher.ListAutomations, jsonMethodNotAllowed)
		router.Post("/admin/api/dispatcher/trigger/{"+models.WorkspaceIDParam+"}/{automation}", hs.Dispatcher.TriggerAutomation, jsonMethodNotAllowed)
		router.Delete("/admin/api/dispatcher/queue/{"+models.WorkspaceIDParam+"}/{automation}", hs.Dispatcher.CancelQueuedHandler, jsonMethodNotAllowed)
		router.Post("/admin/api/dispatcher/stop/{"+models.WorkspaceIDParam+"}", hs.Dispatcher.StopAutomation, jsonMethodNotAllowed)
		router.Post("/admin/api/dispatcher/schedule/preview", hs.Dispatcher.PreviewSchedule, jsonMethodNotAllowed)
		router.Get("/admin/api/dispatcher/metrics", hs.Dispatcher.GetDispatcherMetrics, jsonMethodNotAllowed)
		router.Get("/admin/api/dispatcher/activity", hs.Dispatcher.GetGlobalActivity, jsonMethodNotAllowed)

		router.Get("/admin/api/dispatcher/workspaces", hs.Dispatcher.ListWorkspaces, jsonMethodNotAllowed)
		router.Post("/admin/api/dispatcher/workspaces", hs.Dispatcher.CreateWorkspace, jsonMethodNotAllowed)
		router.Post("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/automations", hs.Dispatcher.CreateAutomation, jsonMethodNotAllowed)
		router.Put("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/automations/{automation}", hs.Dispatcher.UpdateAutomation, jsonMethodNotAllowed)
		router.Delete("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/automations/{automation}", hs.Dispatcher.DeleteAutomation, jsonMethodNotAllowed)
		router.Get("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/automations/{automation}/journal", hs.Dispatcher.GetAutomationJournal, jsonMethodNotAllowed)
		router.Delete("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/automations/{automation}/journal", hs.Dispatcher.ClearAutomationJournal, jsonMethodNotAllowed)
		router.Get("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/automations/{automation}/reports", hs.Dispatcher.GetAutomationReports, jsonMethodNotAllowed)
		router.Get("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/tree", hs.Dispatcher.ListWorkspaceTree, jsonMethodNotAllowed)
		// {file...} matches nested workspace-relative paths (plan D4).
		router.Get("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/files/{file...}", hs.Dispatcher.ReadWorkspaceFile, jsonMethodNotAllowed)
		router.Put("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/files/{file...}", hs.Dispatcher.WriteWorkspaceFile, jsonMethodNotAllowed)
		router.Delete("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/files/{file...}", hs.Dispatcher.DeleteWorkspaceFile, jsonMethodNotAllowed)
		router.Get("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/state", hs.Dispatcher.GetWorkspaceState, jsonMethodNotAllowed)
		router.Get("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/config", hs.Dispatcher.GetWorkspaceConfig, jsonMethodNotAllowed)
		router.Put("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/config", hs.Dispatcher.UpdateWorkspaceConfig, jsonMethodNotAllowed)
		router.Get("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/heartbeat", hs.Dispatcher.GetHeartbeat, jsonMethodNotAllowed)
		router.Put("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/heartbeat", hs.Dispatcher.PutHeartbeat, jsonMethodNotAllowed)
		router.Get("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/live", hs.Dispatcher.StreamWorkspaceEvents, textMethodNotAllowed)
		router.Get("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}/processlogs", hs.Process.AdminWorkspaceProcessLogsHandler, jsonMethodNotAllowed)
		router.Delete("/admin/api/dispatcher/workspaces/{"+models.WorkspaceIDParam+"}", hs.Dispatcher.DeleteWorkspace, jsonMethodNotAllowed)
		router.Delete("/admin/api/dispatcher/runs/{"+models.WorkspaceIDParam+"}/run/{run}", hs.Dispatcher.DeleteRun, jsonMethodNotAllowed)
		router.Delete("/admin/api/dispatcher/runs/{"+models.WorkspaceIDParam+"}/{automation}", hs.Dispatcher.DeleteAutomationRuns, jsonMethodNotAllowed)
	}

	router.Get("/admin/", hs.Admin.AdminPageHandler, textMethodNotAllowed)

	// Proxy
	router.Any("/v1/chat/completions", http.HandlerFunc(hs.Proxy.EnsureModelProxyHandler))
	// Out-of-band cancel for a caller whose request is held waiting for the
	// local model; the key is returned in the busy/queued answer.
	router.Delete("/v1/queue/{"+handlers.QueueKeyParam+"}", http.HandlerFunc(hs.Proxy.ServeQueueCancel), jsonMethodNotAllowed)
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
	router.Post("/admin/api/conversation/sessions/{"+models.WorkspaceIDParam+"}/{session}/memory-review", hs.Assistant.ReviewMemories, jsonMethodNotAllowed)
	router.Get("/admin/api/workspaces/{"+models.WorkspaceIDParam+"}/active-runs", hs.ActiveRuns.ServeHTTP, jsonMethodNotAllowed)
	// Workspace-independent lane state for the always-visible header indicator.
	router.Get("/admin/api/active-runs", hs.ActiveRuns.ServeGlobalHTTP, jsonMethodNotAllowed)
	// Operator actions on a queued inbound caller: serve it now (cancelling the
	// run holding the model) or drop it.
	router.Post("/admin/api/queue/{queue_key}/promote", hs.ActiveRuns.ServePromoteHTTP, jsonMethodNotAllowed)
	router.Post("/admin/api/queue/{queue_key}/cancel", hs.ActiveRuns.ServeCancelHTTP, jsonMethodNotAllowed)

	router.Get("/admin/api/profiles", hs.Profiles.List, jsonMethodNotAllowed)
	router.Get("/admin/api/profile-schema", hs.Profiles.Schema, jsonMethodNotAllowed)
	router.Get("/admin/api/profiles/{id}", hs.Profiles.Get, jsonMethodNotAllowed)
	router.Put("/admin/api/profiles/{id}", hs.Profiles.Put, jsonMethodNotAllowed)
	router.Delete("/admin/api/profiles/{id}", hs.Profiles.Delete, jsonMethodNotAllowed)
	router.Get("/admin/api/helpers", hs.Helpers.Status, jsonMethodNotAllowed)
	router.Get("/admin/api/netaudit/scope", hs.Netaudit.GetScope, jsonMethodNotAllowed)
	router.Put("/admin/api/netaudit/scope", hs.Netaudit.PutScope, jsonMethodNotAllowed)
	router.Post("/admin/api/netaudit/targets/check", hs.Netaudit.CheckTargets, jsonMethodNotAllowed)
	router.Get("/admin/api/netaudit/readiness", hs.Netaudit.GetReadiness, jsonMethodNotAllowed)
	router.Get("/admin/api/netaudit/vulndata", hs.VulnData.Get, jsonMethodNotAllowed)
	router.Delete("/admin/api/netaudit/vulndata", hs.VulnData.Clear, jsonMethodNotAllowed)
	router.Post("/admin/api/netaudit/vulndata/download", hs.VulnData.Download, jsonMethodNotAllowed)
	router.Post("/admin/api/netaudit/vulndata/cancel", hs.VulnData.Cancel, jsonMethodNotAllowed)
	router.Post("/admin/api/netaudit/vulndata/import", hs.VulnData.Import, jsonMethodNotAllowed)
	router.Get("/admin/api/netaudit/findings", hs.Netaudit.GetFindings, jsonMethodNotAllowed)
	router.Get("/admin/api/netaudit/acks", hs.Netaudit.ListAcks, jsonMethodNotAllowed)
	router.Post("/admin/api/netaudit/acks", hs.Netaudit.AcceptFinding, jsonMethodNotAllowed)
	router.Delete("/admin/api/netaudit/acks", hs.Netaudit.RevokeAck, jsonMethodNotAllowed)
	router.Post("/admin/api/netaudit/fixes", hs.Netaudit.MarkFixed, jsonMethodNotAllowed)
	router.Delete("/admin/api/netaudit/fixes", hs.Netaudit.UnmarkFixed, jsonMethodNotAllowed)
	router.Get("/admin/api/netaudit/deep", hs.Netaudit.ListDeepChecks, jsonMethodNotAllowed)
	router.Post("/admin/api/netaudit/deep", hs.Netaudit.RunDeepCheck, jsonMethodNotAllowed)
	router.Get("/admin/api/netaudit/expectations", hs.Netaudit.ListExpectations, jsonMethodNotAllowed)
	router.Put("/admin/api/netaudit/expectations", hs.Netaudit.SetExpectation, jsonMethodNotAllowed)
	router.Delete("/admin/api/netaudit/expectations", hs.Netaudit.RemoveExpectation, jsonMethodNotAllowed)

	if hs.Reminders != nil {
		router.Get("/admin/api/reminders", hs.Reminders.List, jsonMethodNotAllowed)
		router.Get("/admin/api/reminders/events", hs.Reminders.Events, jsonMethodNotAllowed)
		router.Post("/admin/api/reminders", hs.Reminders.Create, jsonMethodNotAllowed)
		router.Put("/admin/api/reminders/{id}", hs.Reminders.Edit, jsonMethodNotAllowed)
		router.Post("/admin/api/reminders/{id}/pause", hs.Reminders.Pause, jsonMethodNotAllowed)
		router.Post("/admin/api/reminders/{id}/resume", hs.Reminders.Resume, jsonMethodNotAllowed)
		router.Post("/admin/api/reminders/{id}/skip", hs.Reminders.Skip, jsonMethodNotAllowed)
		router.Delete("/admin/api/reminders/{id}", hs.Reminders.Delete, jsonMethodNotAllowed)
	}

	router.Get("/admin/api/commands", hs.Commands.List, jsonMethodNotAllowed)
	router.Post("/admin/api/commands/run", hs.Commands.Run, jsonMethodNotAllowed)
	router.Post("/admin/api/commands/preview", hs.Commands.Preview, jsonMethodNotAllowed)

	// Memory API
	if hs.Memory != nil {
		router.Get("/admin/api/memory/{"+models.WorkspaceIDParam+"}", hs.Memory.ListMemories, jsonMethodNotAllowed)
		router.Post("/admin/api/memory/{"+models.WorkspaceIDParam+"}", hs.Memory.CreateMemory, jsonMethodNotAllowed)
		router.Get("/admin/api/memory/{"+models.WorkspaceIDParam+"}/injection-preview", hs.Memory.InjectionPreview, jsonMethodNotAllowed)
		router.Get("/admin/api/memory/{"+models.WorkspaceIDParam+"}/export", hs.Memory.ExportMemory, jsonMethodNotAllowed)
		router.Post("/admin/api/memory/{"+models.WorkspaceIDParam+"}/import", hs.Memory.ImportMemory, jsonMethodNotAllowed)
		router.Get("/admin/api/memory/{"+models.WorkspaceIDParam+"}/notes", hs.Memory.GetNotes, jsonMethodNotAllowed)
		router.Put("/admin/api/memory/{"+models.WorkspaceIDParam+"}/notes", hs.Memory.PutNotes, jsonMethodNotAllowed)
		router.Post("/admin/api/memory/{"+models.WorkspaceIDParam+"}/search", hs.Memory.SearchMemories, jsonMethodNotAllowed)
		router.Get("/admin/api/memory/{"+models.WorkspaceIDParam+"}/{id}", hs.Memory.GetMemory, jsonMethodNotAllowed)
		router.Put("/admin/api/memory/{"+models.WorkspaceIDParam+"}/{id}", hs.Memory.UpdateMemory, jsonMethodNotAllowed)
		router.Delete("/admin/api/memory/{"+models.WorkspaceIDParam+"}/{id}", hs.Memory.DeleteMemory, jsonMethodNotAllowed)
		router.Delete("/admin/api/memory/{"+models.WorkspaceIDParam+"}", hs.Memory.ClearWorkspace, jsonMethodNotAllowed)
	}

	// Sign-in. Only status, setup, login and logout are open to someone not signed in (api.Authenticate decides that from
	// the path); the rest need a session, and creating a token needs the passphrase again.
	if hs.Auth != nil {
		router.Get("/admin/api/auth/status", hs.Auth.Status, jsonMethodNotAllowed)
		router.Post("/admin/api/auth/setup", hs.Auth.Setup, jsonMethodNotAllowed)
		router.Post("/admin/api/auth/login", hs.Auth.Login, jsonMethodNotAllowed)
		router.Post("/admin/api/auth/logout", hs.Auth.Logout, jsonMethodNotAllowed)
		router.Post("/admin/api/auth/reauth", hs.Auth.Reauth, jsonMethodNotAllowed)
		router.Get("/admin/api/auth/sessions", hs.Auth.Sessions, jsonMethodNotAllowed)
		router.Delete("/admin/api/auth/sessions", hs.Auth.RevokeSession, jsonMethodNotAllowed)
		router.Post("/admin/api/auth/sessions/revoke-all", hs.Auth.RevokeAllSessions, jsonMethodNotAllowed)
		router.Post("/admin/api/auth/passphrase", hs.Auth.ChangePassphrase, jsonMethodNotAllowed)
		router.Get("/admin/api/auth/tokens", hs.Auth.Tokens, jsonMethodNotAllowed)
		router.Post("/admin/api/auth/tokens", hs.Auth.CreateToken, jsonMethodNotAllowed)
		router.Delete("/admin/api/auth/tokens", hs.Auth.RevokeToken, jsonMethodNotAllowed)
		router.Get("/admin/api/auth/events", hs.Auth.Events, jsonMethodNotAllowed)
		router.Get("/admin/api/auth/exposure", hs.Exposure.Get, jsonMethodNotAllowed)
		router.Get("/admin/api/auth/session-length", hs.Auth.SessionLength, jsonMethodNotAllowed)
		router.Put("/admin/api/auth/session-length", hs.Auth.SetSessionLength, jsonMethodNotAllowed)
	}

	// Public webhooks — external platforms POST here (no admin auth)
	if hs.Webhook != nil {
		router.Post("/api/v1/webhooks/{connector_name}", hs.Webhook.ServeHTTP, jsonMethodNotAllowed)
	}

	return router
}

// workProbe is how the runtime-data guard asks what is running; it is narrow so the guard can be tested without handlers.
type workProbe struct {
	listWorkspaces func() ([]string, error)
	assistantBusy  func(workspaceID string) bool
	automationBusy func(workspaceID string) bool
	laneQueued     func() bool
}

// activeWorkChecker reports whether any assistant run or automation is executing or queued, and fails closed (busy) when
// the workspaces cannot be listed.
func activeWorkChecker(p workProbe) func() bool {
	return func() bool {
		ids, err := p.listWorkspaces()
		if err != nil {
			return true
		}
		for _, id := range ids {
			if p.assistantBusy(id) || p.automationBusy(id) {
				return true
			}
		}
		return p.laneQueued()
	}
}

// warnInvalidTrustedProxies names the server.trusted_proxies entries that are neither an address nor a range: they trust
// nothing, which would otherwise look like a working proxy setting that has no effect.
func warnInvalidTrustedProxies(entries []string) {
	if bad := api.InvalidProxyEntries(entries); len(bad) > 0 {
		logging.Warn("server.trusted_proxies has entries that are not an address or a range; they are ignored", "entries", bad)
	}
}
