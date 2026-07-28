// agent.go — Agent struct, options, constructors, guardrail decision store,
// repetition detection, and the Execute entry point (thin wrapper delegating
// to runSession).  Also holds message preparation helpers shared across all
// LLM-calling paths.
package assistant

import (
	"context"
	"fmt"
	"hash/fnv"
	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/orchestrator"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/memory"
	"llm-proxy/internal/platform/storage"
	"llm-proxy/models"
	"sync"
	"time"
)

const (
	DefaultMaxSteps              = 25
	DefaultContextBudget         = 8000 // chars, not tokens — rough heuristic for context window pressure
	DefaultMaxTokens             = 3072
	DefaultAutomationTemperature = 0.1  // low temperature for deterministic automation tasks
	MinReasoningStuckThreshold   = 2000 // chars; floor for stuck detection even at small max_tokens
	DefaultStarvationLimit       = 15
	AgentGlobalTimeout           = 30 * time.Minute // total wall-clock for one Execute call
	AgentTurnTimeout             = 10 * time.Minute // per-LLM-call timeout (stream or Chat)
	AgentRetryTimeout            = 5 * time.Minute  // tool-support-fallback retry timeout

	// Safety timeout defaults (all configurable per-model via AgentOptions / ModelConfig)
	DefaultToolTimeout              = 2 * time.Minute  // per-tool execution timeout
	DefaultFilesystemToolTimeout    = 30 * time.Second // filesystem I/O timeout
	DefaultMaxPlanDuration          = 15 * time.Minute // plan execution wall-clock
	DefaultMaxPlanSteps             = 50               // max steps per plan
	DefaultGuardrailTimeout         = 5 * time.Second  // guardrail validation timeout
	DefaultGuardrailTimeoutBehavior = "fail-open"      // fail-open | fail-closed
)

// DefaultReasoningBudget returns the auto-computed reasoning budget for a given
// max_tokens value. Divisor 3 gives ~910 tokens for 2730 max_tokens — enough to
// review history and plan the next tool call. Shared by stream.go (runtime) and
// admin_view.go (API response) so the computation is in one place.
func DefaultReasoningBudget(maxTokens int) int {
	if maxTokens <= 0 {
		return 0
	}
	return maxTokens / 3
}

// ReasoningBudgetExceeded returns true when accumulated reasoning content
// (in characters) exceeds the token budget by a wide enough margin to indicate
// the server is not enforcing the limit. The factor of 4 converts tokens to
// approximate characters (~4 chars per token for typical text).
func ReasoningBudgetExceeded(reasoningChars int, budgetTokens int) bool {
	if budgetTokens <= 0 {
		return false
	}
	return reasoningChars > budgetTokens*4
}

type ProviderTuningDefaults struct {
	MaxSteps        int
	ContextBudget   int
	MaxTokens       int
	ToolCallFormat  string
	Prefill         bool
	ReasoningBudget int
}

// providerTiers is the frozen baseline of per-provider agent-tuning defaults.
// It is allocated once at package init; callers must treat the returned map as
// read-only (ProviderTiers returns the shared instance, never a copy). The
// reasoning-budget wire field is intentionally NOT here — that is a property of
// the upstream API contract and is resolved via proxy.Client.ReasoningField().
var providerTiers = map[string]ProviderTuningDefaults{
	"local":      {MaxSteps: 25, ContextBudget: 8000, MaxTokens: 2048, ToolCallFormat: "", Prefill: false, ReasoningBudget: 0},
	"gemini":     {MaxSteps: 35, ContextBudget: 50000, MaxTokens: 4096, ToolCallFormat: "native", Prefill: false, ReasoningBudget: 8192},
	"vertex":     {MaxSteps: 35, ContextBudget: 50000, MaxTokens: 4096, ToolCallFormat: "native", Prefill: false, ReasoningBudget: 8192},
	"openai":     {MaxSteps: 35, ContextBudget: 50000, MaxTokens: 4096, ToolCallFormat: "native", Prefill: false, ReasoningBudget: 8192},
	"openrouter": {MaxSteps: 30, ContextBudget: 30000, MaxTokens: 2048, ToolCallFormat: "native", Prefill: false, ReasoningBudget: 4096},
	"mulerouter": {MaxSteps: 30, ContextBudget: 30000, MaxTokens: 2048, ToolCallFormat: "native", Prefill: false, ReasoningBudget: 4096},
	"nvidia":     {MaxSteps: 30, ContextBudget: 20000, MaxTokens: 2048, ToolCallFormat: "native", Prefill: false, ReasoningBudget: 2048},
}

// ProviderTiers returns the shared provider-tuning table. Treat as read-only.
func ProviderTiers() map[string]ProviderTuningDefaults {
	return providerTiers
}

// TierForProvider returns the tuning defaults for a provider type, or a safe
// OpenAI-compatible default for unknown providers. Agent tuning only — the
// reasoning-budget wire field is resolved per-request from the client.
func TierForProvider(providerType string) ProviderTuningDefaults {
	if t, ok := providerTiers[providerType]; ok {
		return t
	}
	return ProviderTuningDefaults{}
}

// AgentConfig holds immutable per-agent data from user/model config.
type AgentConfig struct {
	MaxSteps        int
	ContextBudget   int
	MaxTokens       int
	ReasoningBudget int
	Temperature     float64
	ICUWeight       float64
	GlobalTimeout   time.Duration
	UseNativeTools  bool
	UsePrefill      bool
	WorkspaceID     string
	ModelName       string
	ProviderType    string
	SkipStuckCheck  bool
	EnableHotMemory bool
	// Channel is the event stream this agent publishes to (assistant vs
	// automation). It is stamped onto every AgentEvent so the EventBus can
	// route and the SSE handler can serve a single channel per connection.
	Channel EventChannel
	// ConversationID scopes assistant events to a specific chat session.
	ConversationID string
	// Safety timeouts — per-model overrides for unattended run hardening.
	ToolTimeout              time.Duration
	FilesystemToolTimeout    time.Duration
	MaxPlanDuration          time.Duration
	MaxPlanSteps             int
	GuardrailTimeout         time.Duration
	GuardrailTimeoutBehavior string
}

// AgentRuntimeDeps holds shared services injected into every Agent.
type AgentRuntimeDeps struct {
	Client       proxy.Client
	Provider     ToolProvider
	Engine       Engine
	Guardrails   *guardrails.GuardrailEngine
	Logger       logging.Logger
	Observer     Observer
	Orchestrator *orchestrator.Orchestrator
	PlanStrategy *ExecutionPlanStrategy
	MemoryStore  *memory.Store

	OnGuardrail GuardrailDecisionCallback
}

// Agent drives the assistant loop: prompt, execute, repeat.
type Agent struct {
	config AgentConfig
	deps   AgentRuntimeDeps

	runS *runSession // current execution session; nil outside Execute

	cachedToolManual    string // cached BuildToolManual output
	cachedToolReference string // cached BuildNativeToolReference output
	toolsHash           uint64 // fingerprint of tool set used to build cache
}

// prefillDisabled returns whether prefill has been disabled at runtime.
func (a *Agent) prefillDisabled() bool {
	if a.runS == nil {
		return false
	}
	return a.runS.prefillDisabled
}

// setPrefillDisabled sets the runtime prefill-disabled flag.
func (a *Agent) setPrefillDisabled(v bool) {
	if a.runS != nil {
		a.runS.prefillDisabled = v
	}
}

// memoryInjected returns whether hot memory has been injected this session.
func (a *Agent) memoryInjected() bool {
	if a.runS == nil {
		return false
	}
	return a.runS.memoryInjected
}

// setMemoryInjected sets the memory-injected flag.
func (a *Agent) setMemoryInjected(v bool) {
	if a.runS != nil {
		a.runS.memoryInjected = v
	}
}

type AgentOptions struct {
	MaxSteps                 int
	ContextBudget            int
	MaxResponseTokens        int
	ReasoningBudget          int
	Temperature              float64
	ICUWeight                float64
	Logger                   logging.Logger
	Guardrails               *guardrails.GuardrailEngine
	Observer                 Observer
	WorkspaceID              string
	UseNativeTools           *bool
	UsePrefill               bool
	GuardrailDecisionHandler GuardrailDecisionCallback
	Orchestrator             *orchestrator.Orchestrator
	ModelName                string
	ProviderType             string
	GlobalTimeout            time.Duration
	PlanStrategy             *ExecutionPlanStrategy
	MemoryStore              *memory.Store // nil when memory is disabled

	EnableHotMemory bool // inject hot memory at session start
	// Channel is the event stream this agent publishes to. Defaults to
	// ChannelAssistant; automation sets ChannelAutomation.
	Channel EventChannel
	// ConversationID scopes this agent's events to a specific chat session.
	ConversationID string

	// Safety timeouts — per-model overrides for unattended run hardening.
	// Zero means "use global default" (set in applyDefaults).
	ToolTimeout              time.Duration // default 2 min; 0 = disabled
	FilesystemToolTimeout    time.Duration // default 30 sec
	MaxPlanDuration          time.Duration // default 15 min
	MaxPlanSteps             int           // default 50
	GuardrailTimeout         time.Duration // default 5 sec
	GuardrailTimeoutBehavior string        // "fail-open" | "fail-closed"
}

type GuardrailDecisionStore struct {
	mu      sync.Mutex
	pending map[string]chan GuardrailDecision
}

func NewGuardrailDecisionStore() *GuardrailDecisionStore {
	return &GuardrailDecisionStore{
		pending: make(map[string]chan GuardrailDecision),
	}
}

func (s *GuardrailDecisionStore) Register(id string, ch chan GuardrailDecision) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending[id] = ch
}

func (s *GuardrailDecisionStore) Resolve(id string, decision GuardrailDecision) bool {
	s.mu.Lock()
	ch, ok := s.pending[id]
	if ok {
		delete(s.pending, id)
	}
	s.mu.Unlock()

	if !ok {
		return false
	}
	select {
	case ch <- decision:
		return true
	default:
		return false
	}
}

func (s *GuardrailDecisionStore) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pending, id)
}

func NewGuardrailDecisionCallback(store *GuardrailDecisionStore, observer Observer) GuardrailDecisionCallback {
	return func(ctx context.Context, payload GuardrailBlockedPayload) (GuardrailDecision, error) {
		ch := make(chan GuardrailDecision, 1)
		store.Register(payload.DecisionID, ch)

		if observer != nil {
			observer(AgentEvent{
				Type:      EventGuardrailBlocked,
				Payload:   payload,
				Timestamp: time.Now(),
			})
		}

		select {
		case decision := <-ch:
			if observer != nil {
				observer(AgentEvent{
					Type: EventGuardrailInvalidated,
					Payload: GuardrailInvalidatedPayload{
						DecisionID: payload.DecisionID,
						Reason:     "decision_resolved",
					},
					Timestamp: time.Now(),
				})
			}
			return decision, nil
		case <-ctx.Done():
			store.Remove(payload.DecisionID)
			if observer != nil {
				observer(AgentEvent{
					Type: EventGuardrailInvalidated,
					Payload: GuardrailInvalidatedPayload{
						DecisionID: payload.DecisionID,
						Reason:     "context_cancelled",
					},
					Timestamp: time.Now(),
				})
			}
			return GuardrailDecision{Allow: false}, ctx.Err()
		}
	}
}

func (o *AgentOptions) applyDefaults() {
	if o.MaxSteps <= 0 {
		o.MaxSteps = DefaultMaxSteps
	}
	if o.ContextBudget <= 0 {
		o.ContextBudget = DefaultContextBudget
	}
	if o.MaxResponseTokens <= 0 {
		o.MaxResponseTokens = DefaultMaxTokens
	}
	if o.ICUWeight <= 0 {
		o.ICUWeight = 1.0
	}
	if o.Logger == nil {
		o.Logger = logging.NewNopLogger()
	}
	if o.GlobalTimeout <= 0 {
		o.GlobalTimeout = AgentGlobalTimeout
	}
	if o.Channel == "" {
		o.Channel = ChannelAssistant
	}
	if o.ToolTimeout <= 0 {
		o.ToolTimeout = DefaultToolTimeout
	}
	if o.FilesystemToolTimeout <= 0 {
		o.FilesystemToolTimeout = DefaultFilesystemToolTimeout
	}
	if o.MaxPlanDuration <= 0 {
		o.MaxPlanDuration = DefaultMaxPlanDuration
	}
	if o.MaxPlanSteps <= 0 {
		o.MaxPlanSteps = DefaultMaxPlanSteps
	}
	if o.GuardrailTimeout <= 0 {
		o.GuardrailTimeout = DefaultGuardrailTimeout
	}
	if o.GuardrailTimeoutBehavior == "" {
		o.GuardrailTimeoutBehavior = DefaultGuardrailTimeoutBehavior
	}
}

// ApplyModelConfig copies model-level overrides from cfg into the options.
// Returns true when cfg.EnableExecutionPlan is true and the caller should
// set opts.PlanStrategy after this call (requires external dependencies).
func (o *AgentOptions) ApplyModelConfig(cfg models.ModelConfig) bool {
	if cfg.Provider != "" {
		o.ProviderType = cfg.Provider
	}
	if cfg.MaxSteps > 0 {
		o.MaxSteps = cfg.MaxSteps
	}
	if cfg.ContextBudget > 0 {
		o.ContextBudget = cfg.ContextBudget
	}
	if cfg.MaxTokens > 0 {
		o.MaxResponseTokens = cfg.MaxTokens
	}
	if cfg.ReasoningBudget > 0 {
		o.ReasoningBudget = cfg.ReasoningBudget
	}
	if cfg.Temperature > 0 {
		o.Temperature = cfg.Temperature
	}
	if cfg.ToolCallFormat == "native" {
		native := true
		o.UseNativeTools = &native
	}
	if cfg.Prefill != nil && *cfg.Prefill {
		o.UsePrefill = true
	}
	if cfg.TimeoutMinutes > 0 {
		o.GlobalTimeout = time.Duration(cfg.TimeoutMinutes) * time.Minute
	}
	if cfg.ToolTimeoutSeconds > 0 {
		o.ToolTimeout = time.Duration(cfg.ToolTimeoutSeconds) * time.Second
	}
	if cfg.FilesystemToolTimeoutSeconds > 0 {
		o.FilesystemToolTimeout = time.Duration(cfg.FilesystemToolTimeoutSeconds) * time.Second
	}
	if cfg.MaxPlanDurationMinutes > 0 {
		o.MaxPlanDuration = time.Duration(cfg.MaxPlanDurationMinutes) * time.Minute
	}
	if cfg.MaxPlanSteps > 0 {
		o.MaxPlanSteps = cfg.MaxPlanSteps
	}
	if cfg.GuardrailTimeoutSeconds > 0 {
		o.GuardrailTimeout = time.Duration(cfg.GuardrailTimeoutSeconds) * time.Second
	}
	if cfg.GuardrailTimeoutBehavior != "" {
		o.GuardrailTimeoutBehavior = cfg.GuardrailTimeoutBehavior
	}
	return cfg.EnableExecutionPlan
}

func NewAgent(client proxy.Client, provider ToolProvider, engine Engine, opts AgentOptions) *Agent {
	opts.applyDefaults()

	gr := opts.Guardrails
	if gr == nil {
		gr = guardrails.NewGuardrailEngine(func() models.AgentGuardrailsConfig { return models.AgentGuardrailsConfig{} }, storage.NewPathResolver("", "", ""), nil, nil)
	}

	useNative := provider.UseNativeTools()
	if opts.UseNativeTools != nil {
		useNative = *opts.UseNativeTools
	}

	usePrefill := true
	if opts.ProviderType != "" {
		if tier, ok := ProviderTiers()[opts.ProviderType]; ok {
			usePrefill = tier.Prefill
		}
	}
	if opts.UsePrefill {
		usePrefill = true
	}

	a := &Agent{
		deps: AgentRuntimeDeps{
			Client:       client,
			Provider:     provider,
			Engine:       engine,
			Guardrails:   gr,
			Logger:       opts.Logger,
			Observer:     opts.Observer,
			Orchestrator: opts.Orchestrator,
			PlanStrategy: opts.PlanStrategy,
			MemoryStore:  opts.MemoryStore,
			OnGuardrail:  opts.GuardrailDecisionHandler,
		},
		config: AgentConfig{
			MaxSteps:                 opts.MaxSteps,
			ContextBudget:            opts.ContextBudget,
			MaxTokens:                opts.MaxResponseTokens,
			ReasoningBudget:          opts.ReasoningBudget,
			Temperature:              opts.Temperature,
			ICUWeight:                opts.ICUWeight,
			GlobalTimeout:            opts.GlobalTimeout,
			UseNativeTools:           useNative,
			UsePrefill:               usePrefill,
			WorkspaceID:              opts.WorkspaceID,
			ModelName:                opts.ModelName,
			ProviderType:             opts.ProviderType,
			EnableHotMemory:          opts.EnableHotMemory,
			Channel:                  opts.Channel,
			ConversationID:           opts.ConversationID,
			ToolTimeout:              opts.ToolTimeout,
			FilesystemToolTimeout:    opts.FilesystemToolTimeout,
			MaxPlanDuration:          opts.MaxPlanDuration,
			MaxPlanSteps:             opts.MaxPlanSteps,
			GuardrailTimeout:         opts.GuardrailTimeout,
			GuardrailTimeoutBehavior: opts.GuardrailTimeoutBehavior,
		},
	}
	opts.Logger.Info("NewAgent: agent created", "max_tokens", a.config.MaxTokens, "reasoning_budget", a.config.ReasoningBudget, "max_steps", a.config.MaxSteps)
	return a
}

// watchdogGracePeriod is the extra time the watchdog waits past GlobalTimeout
// before force-cancelling the run. It gives legitimate long-running tools a
// small margin beyond the configured bound before the hard kill.
const watchdogGracePeriod = 5 * time.Minute

func (a *Agent) Execute(ctx context.Context, history []proxy.Message) (string, []proxy.Message, error) {
	execCtx, cancel := context.WithTimeout(ctx, a.config.GlobalTimeout)
	defer cancel()

	// Watchdog: if the global timeout fires but the run is still alive (a guardrail
	// eval, a stuck stream, or an un-cancellable syscall), force-cancel execCtx to
	// unblock any goroutines still observing it. The goroutine exits on execCtx.Done()
	// once the run completes normally, so it never leaks.
	a.startWatchdog(execCtx, cancel)

	execCtx = WithUsageTracker(execCtx)

	// Plan strategy short-circuits: if enabled, generate a plan for the last user
	// message and execute it step-by-step. Falls back to the agent loop on failure.
	if a.deps.PlanStrategy != nil {
		lastUserMsg := ""
		for i := len(history) - 1; i >= 0; i-- {
			if history[i].Role == proxy.UserRole {
				lastUserMsg = history[i].Content
				break
			}
		}
		if lastUserMsg != "" {
			tools, err := a.deps.Provider.ListTools(execCtx)
			if err == nil && len(tools) > 0 {
				plan, planErr := a.deps.PlanStrategy.Generate(execCtx, lastUserMsg)
				if planErr == nil {
					return a.executePlan(execCtx, history, plan)
				}
				a.deps.Logger.Warn("plan generation failed, falling back to normal loop", "error", planErr)
			}
		}
	}

	a.rebuildToolCache(execCtx)

	s := newRunSession(a, execCtx, history)
	return s.run()
}

// startWatchdog launches a goroutine that force-cancels the run if it outlives
// GlobalTimeout by watchdogGracePeriod — a backstop for guardrail evals, stuck
// streams, or syscalls that ignore context cancellation.
func (a *Agent) startWatchdog(execCtx context.Context, cancel context.CancelFunc) {
	a.startWatchdogGrace(execCtx, cancel, watchdogGracePeriod)
}

func (a *Agent) startWatchdogGrace(execCtx context.Context, cancel context.CancelFunc, grace time.Duration) {
	go func() {
		select {
		case <-time.After(a.config.GlobalTimeout + grace):
			a.deps.Logger.Error("watchdog: context still alive past global timeout, forcing shutdown",
				"globalTimeout", a.config.GlobalTimeout, "grace", grace)
			cancel() // idempotent; unblocks goroutines still observing execCtx
		case <-execCtx.Done():
			return
		}
	}()
}

func (a *Agent) prepareMessages(history []proxy.Message) []proxy.Message {
	return proxy.NormalizeHistory(history, a.config.UseNativeTools)
}

func toolsFingerprint(tools []proxy.Tool) uint64 {
	h := fnv.New64a()
	for _, t := range tools {
		h.Write([]byte(t.Function.Name))
		h.Write([]byte{0})
		h.Write([]byte(t.Function.Description))
		h.Write([]byte{0})
		fmt.Fprintf(h, "%v", t.Function.Parameters)
		h.Write([]byte{0})
	}
	return h.Sum64()
}

func (a *Agent) rebuildToolCache(ctx context.Context) {
	tools, err := a.deps.Provider.ListTools(ctx)
	if err != nil {
		return
	}
	fp := toolsFingerprint(tools)
	if fp == a.toolsHash {
		return
	}

	info := make([]prompts.ToolInfo, len(tools))
	for i, t := range tools {
		info[i] = prompts.ToolInfo{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			Parameters:  t.Function.Parameters,
		}
	}
	a.cachedToolManual = prompts.BuildToolManual(info)
	a.cachedToolReference = prompts.BuildNativeToolReference(info)
	a.toolsHash = fp
}

// injectToolInstructions embeds XML tool definitions into the system prompt.
// Used when native API-level tools are disabled (local models, XML fallback).
func (a *Agent) injectToolInstructions(history []proxy.Message, tools []proxy.Tool) []proxy.Message {
	if len(tools) == 0 {
		return history
	}

	instructions := a.cachedToolManual
	if instructions == "" {
		info := make([]prompts.ToolInfo, len(tools))
		for i, t := range tools {
			info[i] = prompts.ToolInfo{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				Parameters:  t.Function.Parameters,
			}
		}
		instructions = prompts.BuildToolManual(info)
	}

	a.deps.Logger.Debug("injecting XML tool manual into system prompt",
		"tool_count", len(tools),
		"manual_chars", len(instructions),
		"has_manual", len(instructions) > 0,
	)
	newHistory := make([]proxy.Message, 0, len(history)+1)
	foundSystem := false
	for _, msg := range history {
		if !foundSystem && msg.Role == proxy.SystemRole {
			newMsg := msg
			hadManualBefore := prompts.HasToolManual(newMsg.Content)
			newMsg.Content = prompts.InjectToolManual(newMsg.Content, instructions)
			newHistory = append(newHistory, newMsg)
			foundSystem = true
			a.deps.Logger.Debug("tool manual injection result",
				"had_manual_before", hadManualBefore,
				"sys_prompt_chars", len(newMsg.Content),
			)
		} else {
			newHistory = append(newHistory, msg)
		}
	}
	if !foundSystem {
		newHistory = append([]proxy.Message{{
			Role:    proxy.SystemRole,
			Content: prompts.InjectToolManual("You are a powerful agentic AI.", instructions),
		}}, newHistory...)
	}
	return newHistory
}

// injectNativeToolReference injects a tool reference into the system prompt
// alongside API-level tool schemas.  Some models (Gemini) need the reference
// even when native tools are active.
func (a *Agent) injectNativeToolReference(history []proxy.Message, tools []proxy.Tool) []proxy.Message {
	if len(tools) == 0 {
		return history
	}

	reference := a.cachedToolReference
	if reference == "" {
		info := make([]prompts.ToolInfo, len(tools))
		for i, t := range tools {
			info[i] = prompts.ToolInfo{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				Parameters:  t.Function.Parameters,
			}
		}
		reference = prompts.BuildNativeToolReference(info)
	}
	newHistory := make([]proxy.Message, 0, len(history)+1)
	foundSystem := false
	for _, msg := range history {
		if !foundSystem && msg.Role == proxy.SystemRole {
			newMsg := msg
			newMsg.Content = prompts.InjectToolReference(newMsg.Content, reference)
			newHistory = append(newHistory, newMsg)
			foundSystem = true
		} else {
			newHistory = append(newHistory, msg)
		}
	}
	if !foundSystem {
		newHistory = append([]proxy.Message{{
			Role:    proxy.SystemRole,
			Content: prompts.InjectToolReference("You are a powerful agentic AI.", reference),
		}}, newHistory...)
	}
	return newHistory
}
