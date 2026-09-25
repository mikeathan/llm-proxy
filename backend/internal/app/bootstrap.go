package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"time"

	"llm-proxy/internal/core/egress"

	assistantPkg "llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/automation"
	"llm-proxy/internal/core/llm"
	"llm-proxy/internal/core/nodeherder"
	"llm-proxy/internal/core/orchestrator"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/core/proxy/recorder"
	"llm-proxy/internal/core/runlane"
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
	return exec
}

func (c *Container) BuildAppServices() *AppServices {
	s := &AppServices{
		Runtime:        c.Core.Runtime,
		AppCtx:         c.Core.AppCtx,
		nodeHerder:     c.Infra.NodeHerder,
		logger:         c.Infra.Logger,
		Clock:          c.Infra.Clock,
		persistence:    persistence.NewWorkspaceManager(storage.NewPathResolver(c.Core.AppCtx.RootDir(), c.Core.AppCtx.WorkspacesDir(), c.Core.AppCtx.MetadataDir())),
		limiter:        ratelimiter.NewLimiter(c.Infra.Clock),
		RecordingStore: c.newRecordingStore(),
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
	s.toolProvider, s.engine, s.guardrailEngine = assistantPkg.InitializeAgentStack(
		s.AppCtx,
		s.nodeHerder,
		assistantPkg.AgentStackDeps{
			Persistence:  s.persistence,
			Logger:       s.logger,
			ShellManager: shellManager,
			Observer:     streamObserver,
			EgressProxy:  egressProxyURL,
			EgressEnv:    egressEnv,
		},
	)

	return s
}

// newRecordingStore opens the per-run recording store, or nil when recording is
// disabled (a store that cannot be prepared must not block startup).
func (c *Container) newRecordingStore() *recordings.RecordingStore {
	if c.RecordDir == "" {
		return nil
	}
	store, err := recordings.NewRecordingStore(c.RecordDir)
	if err != nil {
		logging.Warn("Failed to init recording store", "dir", c.RecordDir, "error", err)
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
// destination and artifact — a remote llama.cpp serving GGUF is a local workload
// (SPEC-005) — and wraps every client in the recorder.
func (c *Container) newClientFactory(classifier models.WorkloadClassifier) func(string, string, http.Header) proxy.Client {
	return func(baseURL string, model string, headers http.Header) proxy.Client {
		var client proxy.Client
		if classifier.ClassifyClient(baseURL, model) {
			client = proxy.NewLLMClientForLocal(baseURL, model, nil, headers)
		} else {
			client = proxy.NewLLMClient(baseURL, model, nil, headers)
		}
		// Always wrap in RecordingClient so a run-specific recording.jsonl is
		// supported; the dir is only set when recording is globally enabled.
		client = recorder.New(client, c.RecordDir, model)
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
	logger                 logging.Logger
	Clock                  utils.Clock
	dispatcher             *automation.Dispatcher
	runLane                *runlane.Scheduler
	workloadClassifier     models.WorkloadClassifier
	limiter                ratelimiter.Limiter
	guardrailDecisionStore *assistantPkg.GuardrailDecisionStore
	RecordingStore         *recordings.RecordingStore
	egressCancel           context.CancelFunc // stops the agent egress proxy (Phase 1c)
}

func (s AppServices) Shutdown(ctx context.Context) {
	if s.egressCancel != nil {
		logging.Info("Stopping agent egress proxy...")
		s.egressCancel()
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
	d, err := automation.NewDispatcher(automation.DispatcherDeps{
		Persistence: svc.Persistence(),
		Executor:    c.BuildTaskExecutor(svc),
		Logger:      c.Infra.Logger,
		Lane:        c.RunLane,
		LaneKeyFor:  svc.LaneKeyFor,
	})
	if err != nil {
		return nil, err
	}

	return d, nil
}
