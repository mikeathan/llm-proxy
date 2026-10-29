package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"llm-proxy/internal/platform/auth"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"time"

	"llm-proxy/internal/core/egress"
	"llm-proxy/internal/core/netaudit"

	assistantPkg "llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/assistant/profile"
	"llm-proxy/internal/core/automation"
	"llm-proxy/internal/core/delegation"
	"llm-proxy/internal/core/llm"
	"llm-proxy/internal/core/nodeherder"
	"llm-proxy/internal/core/orchestrator"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/core/proxy/recorder"
	"llm-proxy/internal/core/reminders"
	"llm-proxy/internal/core/runlane"
	"llm-proxy/internal/core/toolregistry"
	"llm-proxy/internal/core/tools"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/memory"
	"llm-proxy/internal/platform/metrics"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/ratelimiter"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/internal/recordings"
	"llm-proxy/internal/shell"
	handlers "llm-proxy/internal/transport/http/handlers"
	"llm-proxy/models"
	"llm-proxy/utils"
)

type Core struct {
	AppCtx  *AppContext
	Runtime llm.RuntimeManager
}

type Infra struct {
	Logger       logging.Logger
	Clock        utils.Clock
	NodeHerder   nodeherder.MCPService
	ShellManager shell.ShellProvider
}

type Container struct {
	Core       Core
	Infra      Infra
	Dispatcher *automation.Dispatcher
	RunLane    *runlane.Scheduler
	RecordDir  string // absolute path to runs directory, empty when recording is disabled
}

// Building automation task executor
func (c *Container) BuildTaskExecutor(svc handlers.AssistantService) automation.TaskExecutor {
	exec := automation.NewLLMTaskExecutor(svc).(*automation.LLMTaskExecutor)
	exec.SetShellPool(c.Infra.ShellManager)
	reporter := netaudit.NewRunReporter(svc.Persistence()).WithAcks(svc.Persistence()).WithFixes(svc.Persistence()).WithExpectations(svc.Persistence())
	if v, ok := svc.(interface{ NetauditVulns() netaudit.VulnSource }); ok && v.NetauditVulns() != nil {
		reporter.WithVulns(v.NetauditVulns())
	}
	exec.SetReportHook(reporter)
	return exec
}

func (c *Container) BuildAppServices() *AppServices {
	resolver := storage.NewPathResolver(c.Core.AppCtx.RootDir(), c.Core.AppCtx.WorkspacesDir(), c.Core.AppCtx.MetadataDir())
	s := &AppServices{
		Runtime:        c.Core.Runtime,
		AppCtx:         c.Core.AppCtx,
		nodeHerder:     c.Infra.NodeHerder,
		logger:         c.Infra.Logger,
		Clock:          c.Infra.Clock,
		persistence:    persistence.NewWorkspaceManager(resolver),
		profiles:       profile.MustCatalog(resolver.ProfilesDir(), c.Infra.Logger),
		limiter:        ratelimiter.NewLimiter(c.Infra.Clock),
		RecordingStore: c.newRecordingStore(),
		recordDir:      c.RecordDir,
	}
	// One "is this workload local?" authority (model host + cached local
	// interface IPs), shared by the client factory and the reasoning wire.
	s.workloadClassifier = models.NewWorkloadClassifier(c.modelHost(), models.LocalInterfaceIPs())

	c.initRunScheduler(s)
	s.clientProvider = proxy.NewRuntimeClientProvider(s, c.Core.Runtime, c.newClientFactory(s.workloadClassifier))
	s.dispatcher = c.Dispatcher
	s.guardrailDecisionStore = assistantPkg.NewGuardrailDecisionStore()

	// Shell/terminal subsystem, then the agent egress proxy (plan D2 / Phase 1):
	// when sandboxing.egress_proxy > 0, run the loopback-only forward proxy and
	// route agent tool + shell egress through it. Domain policy is built inside
	// startEgressProxy from host settings — absent lists = allow-all.
	shellManager, streamObserver := c.initShellOrchestrator(s)
	egressProxyURL, egressEnv, err := startEgressProxy(s)
	if err != nil {
		logging.Warn("agent egress proxy disabled", "error", err.Error())
	}

	// Unified tool providers and engines (Local Registry + Remote MCP). Services
	// travel together in one deps struct (rule: ≤3 params).
	s.toolProvider, s.engine, s.guardrailEngine = toolregistry.InitializeAgentStack(
		s.AppCtx,
		s.nodeHerder,
		toolregistry.AgentStackDeps{
			Persistence:  s.persistence,
			Logger:       s.logger,
			ShellManager: shellManager,
			Observer:     streamObserver,
			EgressProxy:  egressProxyURL,
			EgressEnv:    egressEnv,
		},
	)
	s.wireNetaudit(c.Core.AppCtx.dataMgr)
	s.wireAuth(c.Core.AppCtx.dataMgr.RootDir())
	c.watchConnectorConfig(s)
	c.initReminders(s)
	// The sub-agent runner gets a copy of the services taken now, before it is set: a child run's own services have
	// no runner, so delegation is one level deep by construction.
	s.subRunner = delegation.New(*s)

	return s
}

// watchConnectorConfig rebuilds the shared connector set (agent notify_user, report delivery, inbound webhook) when
// the connector config or its secrets change, so a connector added or edited in Settings is used without a restart.
// The secrets store invalidates its own cache in an earlier listener, so the rebuild reads the new token.
func (c *Container) watchConnectorConfig(s *AppServices) {
	comm := communicationTools(s.toolProvider)
	if comm == nil {
		return
	}
	dataMgr := c.Core.AppCtx.dataMgr
	dataMgr.Registry().OnChange(func(models.RegistryData) { comm.Reload() })
	dataMgr.EncryptedSecretStore().OnChange(func(models.EncryptedSecretData) { comm.Reload() })
}

// runsDir is where each run's folder lives, and in it the run's recording.jsonl.
func (c *Container) runsDir() string {
	return filepath.Join(c.Core.AppCtx.RootDir(), "runs")
}

// newRecordingStore opens the store of recordings. With --record it covers the record directory; without it, the runs
// directory, because the client is always wrapped in a recorder and, while run logging is on, each run's folder gets a
// recording.jsonl of its requests and replies even without the flag. The store is what lists them.
func (c *Container) newRecordingStore() *recordings.RecordingStore {
	dir := c.RecordDir
	if dir == "" {
		dir = c.runsDir()
	}
	store, err := recordings.NewRecordingStore(dir)
	if err != nil {
		logging.Warn("Failed to init recording store", "dir", dir, "error", err)
		return nil
	}
	return store
}

// modelHost is the configured local model host, "" when no runtime is wired.
func (c *Container) modelHost() string {
	if c.Core.Runtime == nil {
		return ""
	}
	return c.Core.Runtime.ModelHost()
}

// initRunScheduler builds the one run scheduler and wires everything that must
// see it at boot: the container (automations), AppServices (chat), and the model
// manager's residency guard — plus live reconfiguration on settings changes.
func (c *Container) initRunScheduler(s *AppServices) {
	cfg := c.Core.AppCtx.GetSettings().Scheduler
	if cfg == nil {
		def := models.DefaultSchedulerConfig()
		cfg = &def
	}
	preempt := cfg.PreemptAutomations == nil || *cfg.PreemptAutomations
	lane := runlane.New(runlane.Limits{Local: cfg.LocalConcurrency, Cloud: cfg.CloudConcurrency}, preempt)
	// One scheduler for every admission path: the container serves automations
	// (dispatcher) and AppServices serves chat (the handler's Lane()). Without
	// this, chat runs skip the lane entirely — they record no holder, so the
	// header run indicator stays hidden and chat preemption never happens.
	c.RunLane = lane
	s.runLane = lane
	// The model manager refuses local-model evictions that would stop an admitted
	// run or inbound caller — the lane is the single authority (runlane gate).
	// Runtime is nil in some tests, so this is optional.
	if setter, ok := c.Core.Runtime.(interface{ SetResidencyGuard(llm.ResidencyGuard) }); ok {
		setter.SetResidencyGuard(s.gate())
	}
	c.Core.AppCtx.dataMgr.Settings().OnChange(func(set models.UserSettings) {
		if set.Scheduler == nil {
			return
		}
		lane.SetLimits(runlane.Limits{Local: set.Scheduler.LocalConcurrency, Cloud: set.Scheduler.CloudConcurrency})
		lane.SetPreempt(set.Scheduler.PreemptAutomations == nil || *set.Scheduler.PreemptAutomations)
	})
}

// newClientFactory routes each model to its client by the actual upstream
// destination, artifact and the model's own workload class — a remote llama.cpp
// is a local workload (SPEC-005) — and wraps every client in the recorder.
func (c *Container) newClientFactory(classifier models.WorkloadClassifier) func(string, string, http.Header, bool) proxy.Client {
	return func(baseURL string, model string, headers http.Header, local bool) proxy.Client {
		var client proxy.Client
		if local || classifier.ClassifyClient(baseURL, model) {
			client = proxy.NewLLMClientForLocal(baseURL, model, nil, headers)
		} else {
			client = proxy.NewLLMClient(baseURL, model, nil, headers)
		}
		// Always wrap in RecordingClient so a run-specific recording.jsonl is
		// supported; the dir is only set when recording is globally enabled.
		// A recording is every prompt of a run, so what is written is masked; what the model and agent see is not.
		client = recorder.New(client, c.RecordDir, model, recorder.WithRedactor(tools.RedactForRecording))
		if c.RecordDir != "" {
			logging.Debug("recording LLM responses", "model", model, "dir", c.RecordDir)
		}
		return client
	}
}

// egressProxyUser is the Basic-auth username the agent credentials use; the
// secret half is the per-process token.
const egressProxyUser = "agent"

// newEgressToken returns a 128-bit hex shared secret for the loopback egress
// proxy (never logged; delivered to agent transports/shells only).
func newEgressToken() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// startEgressProxy enables the agent egress proxy when sandboxing.egress_proxy
// is a valid port, returning its proxy URL and the env vars shells need. The
// returned cancel func is invoked by AppServices.Shutdown.
func startEgressProxy(s *AppServices) (*url.URL, func() []string, error) {
	port := s.AppCtx.HostSettings().Sandboxing.EgressProxy
	if port <= 0 {
		return nil, nil, nil // disabled
	}
	if port > 65535 {
		return nil, nil, fmt.Errorf("invalid egress_proxy port %d", port)
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.egressCancel = cancel

	sb := s.AppCtx.HostSettings().Sandboxing
	// Operator domain policy (plan §4.5/1d): allow list non-empty ⇒ default
	// deny (only listed hosts); deny always wins.
	allow, deny, defaultDeny := sb.EgressPolicy()
	policy := egress.HostListPolicy{Allow: allow, Deny: deny, DefaultDeny: defaultDeny}
	srv := egress.New(policy)

	// Local-abuse hardening (plan §9 T5): a per-process shared secret. Only the
	// agent's own transports and shells receive it, so other local processes
	// cannot ride the proxy. Per-RUN scoping remains a documented follow-up (the
	// proxy transport is pooled per process).
	token, tokenErr := newEgressToken()
	if tokenErr != nil {
		return nil, nil, fmt.Errorf("egress proxy token: %w", tokenErr)
	}
	srv.SetToken(token)

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	safeGo := func() {
		if err := srv.Serve(ctx, addr); err != nil && ctx.Err() == nil {
			logging.Error("agent egress proxy stopped unexpectedly", "addr", addr, "error", err.Error())
		}
	}
	// safe.Go lives in platform/safe; bootstrap uses a plain goroutine tethered
	// to ctx (cancelled in Shutdown) — Constitution II.14 termination path.
	go safeGo()

	proxyURL := &url.URL{Scheme: "http", Host: addr, User: url.UserPassword(egressProxyUser, token)}
	host := proxyURL.Host
	authHost := egressProxyUser + ":" + token + "@" + host
	noProxy := "127.0.0.1,localhost,::1"
	env := func() []string {
		return []string{
			"HTTP_PROXY=http://" + authHost, "http_proxy=http://" + authHost,
			"HTTPS_PROXY=http://" + authHost, "https_proxy=http://" + authHost,
			"NO_PROXY=" + noProxy, "no_proxy=" + noProxy,
		}
	}
	logging.Info("Agent egress proxy enabled", "addr", addr)
	return proxyURL, env, nil
}

// initShellOrchestrator spins up the background persistent shell manager
// and configures the streaming T-Junction metrics observer for the frontend.
func (c *Container) initShellOrchestrator(s *AppServices) (shell.ShellProvider, tools.StreamObserver) {
	var shellManager shell.ShellProvider

	settings := s.AppCtx.HostSettings()
	if !settings.Sandboxing.Enabled {
		// Plan §4.1 decision: the master switch no longer bricks the service.
		// With persistent terminals disabled agents fall back to one-shot
		// executeLocal commands (no session state); host-level containment
		// switches still apply. Historically this was log.Fatal — the loud
		// warning is the documented replacement.
		logging.Warn("[SECURITY] sandboxing.enabled is false — persistent terminals are disabled; agent commands run one-shot (no session state). Set sandboxing.enabled = true for full agentic execution.")
		return nil, nil
	}

	if sm, err := shell.NewHostShellManager(); err == nil {
		shellManager = sm
		c.Infra.ShellManager = sm
		c.Infra.Logger.Debug("Host Shell Manager initialized successfully")
	} else {
		log.Fatalf("[SECURITY] Failed to start Host Shell Manager: %v", err)
	}

	streamObserver := func(streamType string, chunk []byte) {
		s.Events().Publish("global", assistantPkg.AgentEvent{
			Type: assistantPkg.EventToolStream, // Emits to the frontend console via EventBus
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
	Runtime                llm.RuntimeManager
	AppCtx                 *AppContext
	nodeHerder             nodeherder.MCPService
	toolProvider           assistantPkg.ToolProvider
	clientProvider         proxy.LLMClientProvider
	engine                 assistantPkg.Engine
	guardrailEngine        *guardrails.GuardrailEngine
	persistence            *persistence.WorkspaceManager
	profiles               *profile.Catalog
	logger                 logging.Logger
	Clock                  utils.Clock
	dispatcher             *automation.Dispatcher
	runLane                *runlane.Scheduler
	workloadClassifier     models.WorkloadClassifier
	limiter                ratelimiter.Limiter
	guardrailDecisionStore *assistantPkg.GuardrailDecisionStore
	RecordingStore         *recordings.RecordingStore
	recordDir              string             // the --record directory; empty when the flag is off, whatever the store lists
	egressCancel           context.CancelFunc // stops the agent egress proxy (Phase 1c)
	// Reminders delivers stored reminders; nil when the database or the connector
	// registry is unavailable. Its Run loop is started by the app lifecycle.
	Reminders *reminders.Service
	// subRunner runs delegated tasks. The runner keeps its own copy of these services taken before this is set, so a
	// child run has none: delegation is one level deep by construction.
	subRunner assistantPkg.SubRunner
	// netaudit is the network-audit scope and readiness the admin API serves; the scan tool is registered with it.
	netaudit *netauditServices
	// auth is the operator's sign-in; the middleware around the router and the /admin/api/auth routes use it.
	auth *auth.Service
}

func (s AppServices) Shutdown(ctx context.Context) {
	if s.egressCancel != nil {
		logging.Info("Stopping agent egress proxy...")
		s.egressCancel()
	}
	if s.netaudit != nil && s.netaudit.Vulns != nil {
		s.netaudit.Vulns.Close()
	}
	if s.Runtime != nil {
		logging.Info("Shutting down LLM runtime...")
		s.Runtime.Shutdown()
	}
	if s.guardrailEngine != nil {
		logging.Info("Stopping guardrail override reaper...")
		s.guardrailEngine.Stop()
	}
	if s.AppCtx != nil {
		s.AppCtx.Shutdown(ctx)
	}
}

func (s AppServices) GetClientForModel(ctx context.Context, modelName string) (proxy.Client, error) {
	return s.clientProvider.GetClientForModel(ctx, modelName)
}

func (s AppServices) NodeHerder() nodeherder.MCPService {
	return s.nodeHerder
}

func (s AppServices) ToolProvider() assistantPkg.ToolProvider {
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

func (s AppServices) Engine() assistantPkg.Engine {
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

// EffectiveToolCallFormat resolves the model's tool_call_format, probing local
// endpoints for native tool support when unset (cached). See
// LLMRuntimeManager.EffectiveToolCallFormat.
func (s AppServices) EffectiveToolCallFormat(ctx context.Context, modelName string) string {
	if s.Runtime == nil {
		return ""
	}
	return s.Runtime.EffectiveToolCallFormat(ctx, modelName)
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

// Profiles is the prompt-profile catalog: the built-ins plus the user's files in the data root's profiles folder.
func (s AppServices) Profiles() *profile.Catalog {
	return s.profiles
}

func (s AppServices) ProcessLogger(workspaceID string) logging.Logger {
	return s.AppCtx.ProcessLogger(workspaceID)
}

func (s AppServices) RootDir() string {
	return s.AppCtx.RootDir()
}

func (s *AppServices) Events() assistantPkg.EventPublisher {
	if s.dispatcher == nil {
		return nil
	}
	return s.dispatcher.Events()
}

func (s *AppServices) SetDispatcher(d *automation.Dispatcher) {
	s.dispatcher = d
}

func (s AppServices) GuardrailDecisionStore() *assistantPkg.GuardrailDecisionStore {
	return s.guardrailDecisionStore
}

func (s AppServices) MemoryStore() *memory.Store {
	return s.AppCtx.MemoryStore()
}

// RecordDir is the directory --record writes to, "" without the flag. Run recordings written while run logging is on
// are not under it, so an automation's recording reference is only set when the flag is on, as before.
func (s AppServices) RecordDir() string {
	return s.recordDir
}

// AgentSettings are the agent-loop settings, read each time so a change applies to the next run without a restart.
func (s AppServices) AgentSettings() *models.AgentConfig {
	return s.AppCtx.GetSettings().Agent
}

// SubRunner runs delegated tasks; nil before it is wired and inside a sub-agent's own services.
func (s AppServices) SubRunner() assistantPkg.SubRunner { return s.subRunner }

func (s AppServices) MemorySettings() *models.MemoryConfig {
	return s.AppCtx.GetSettings().Memory
}

func (s AppServices) RunLoggingEnabled() bool {
	return s.AppCtx.RunLoggingEnabled()
}

func (s AppServices) GetPlaybackClient(ctx context.Context, ref string) (proxy.Client, error) {
	if s.RecordingStore == nil {
		return nil, fmt.Errorf("recording store not available (start server with --record)")
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

	// Inject the dedicated provider infrastructure HTTP client (pooled
	// SharedTransport, 45s timeout).  Provider traffic (catalogue listing,
	// /slots probes, connection tests) is a separate class from agent tools —
	// it never inherits agent-tool LAN/internet guardrails (C1 / Constitution
	// I.2 amendment).
	manager.Registrar().SetHTTPDoer(&http.Client{
		Transport: proxy.SharedTransport,
		Timeout:   45 * time.Second,
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
		RecordDir: runsDir,
	}
}

// BuildDispatcher creates the new dispatcher subsystem.
// It uses the persistence layer directly (not the old workspace.Manager).
func (c *Container) BuildDispatcher(svc handlers.AssistantService) (*automation.Dispatcher, error) {
	var opts []automation.Option
	if comm := communicationTools(svc.ToolProvider()); comm != nil {
		opts = append(opts, automation.WithNotifier(connectorNotifier{comm: comm}))
	}
	// An audit waits on its scan, so its run may take longer than other automations'.
	opts = append(opts, automation.WithProfileTimeout(netaudit.AuditorProfileID, netaudit.AuditRunCap))
	if pf, ok := svc.(interface{ NetauditPreflight() automation.Preflight }); ok && pf.NetauditPreflight() != nil {
		opts = append(opts, automation.WithPreflight(pf.NetauditPreflight()))
	}
	d, err := automation.NewDispatcher(automation.DispatcherDeps{
		Persistence: svc.Persistence(),
		Executor:    c.BuildTaskExecutor(svc),
		Logger:      c.Infra.Logger,
		Lane:        c.RunLane,
		LaneKeyFor:  svc.LaneKeyFor,
	}, opts...)
	if err != nil {
		return nil, err
	}

	return d, nil
}
