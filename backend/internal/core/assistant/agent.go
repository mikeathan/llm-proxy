// agent.go — Agent struct, options, constructors, guardrail decision store,
// repetition detection, and the Execute entry point (thin wrapper delegating
// to runSession).  Also holds message preparation helpers shared across all
// LLM-calling paths.
package assistant

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/assistant/reasoning"
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
	DefaultMaxSteps      = 25
	DefaultContextBudget = 8000 // chars, not tokens — rough heuristic for context window pressure
	// DefaultMaxTokens is the GLOBAL agent-loop fallback: applied in
	// applyDefaults() when an agent is constructed with no per-model max_tokens,
	// and surfaced as the display fallback in modelViewTuning.  It is
	// deliberately distinct from the per-provider MaxTokens rows in
	// models/tuning.go (which drive cloud output-cap clamps) — this is the
	// "no provider info" default, not a provider's preferred cap.  Do not
	// collapse the two: unifying them would change cloud cap clamps or the
	// unconfigured-model behaviour (stuck-detection sensitivity).
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
	// DefaultGuardrailApprovalTimeout bounds how long the agent waits for a
	// human allow/deny decision after a guardrail block (Constitution II.10 and
	// SPEC guardrails: "Agent blocks on channel for up to 60s"). On expiry the
	// call is treated as denied and the run continues with the violation
	// recorded. Distinct from DefaultGuardrailTimeout (the validation bound) —
	// 5s is far too short for a human to read and answer an approval prompt.
	// 300s matches Hermes' approval default: 60s proved too tight in practice
	// (approvals arrive as notifications the user may not see for a couple of
	// minutes, so the wait failed closed before they answered).
	DefaultGuardrailApprovalTimeout = 5 * time.Minute
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

// resolveReasoningSpec builds the per-agent ReasoningSpec from the provider
// tier table, combining the resolved wire Mode with the think-token budget for
// local (ModeThinkTokens) providers. This is the single source of truth for the
// resolved spec; the resolver later decides the wire field.
//
// Local reasoning budget derivation (SSOT): when no explicit budget is
// configured, it is derived from the model's max_tokens via
// DefaultReasoningBudget (max_tokens/3). max_tokens itself is derived from the
// server's serving context (ctxLen/3 in ApplyMetadataDefaults), so the budget
// tracks the context size the user launched the server with. Derivation is NEVER
// based on model name (the old name-heuristic gate was removed — it caused
// false positives/negatives). Explicit configuration always wins.
func resolveReasoningSpec(providerType string, configuredBudget, maxTokens int) reasoning.ReasoningSpec {
	tier, ok := providerTiers[providerType]
	if !ok {
		return reasoning.ReasoningSpec{Mode: reasoning.ModeEffort, Effort: reasoning.EffortMedium}
	}
	spec := tier.Reasoning
	if spec.Mode == reasoning.ModeThinkTokens {
		if configuredBudget > 0 {
			spec.Budget = configuredBudget
		} else if maxTokens > 0 {
			spec.Budget = DefaultReasoningBudget(maxTokens)
		}
	}
	return spec
}

// applyReasoningEnabledOverride reconciles the per-model reasoning_enabled
// toggle with the provider's capability and workload class. It is capability-
// and workload-driven (not provider-name-driven): local workloads never take
// the override, and effort-mode providers map a disabled toggle to omitting
// reasoning_effort rather than sending a concrete effort.
func applyReasoningEnabledOverride(spec reasoning.ReasoningSpec, enabled *bool, workload models.WorkloadClass) reasoning.ReasoningSpec {
	if workload == models.WorkloadLocal || enabled == nil {
		return spec
	}
	if spec.Mode == reasoning.ModeThinkTokens { // non-toggleable; guard for safety
		return spec
	}
	switch spec.Mode {
	case reasoning.ModeObject, reasoning.ModeEnableThinking:
		spec.Enabled = *enabled // today's behaviour
	case reasoning.ModeEffort:
		if *enabled {
			spec.Effort = reasoning.EffortMedium // medium
		} else {
			spec.Effort = reasoning.EffortNone // omit on the wire
		}
	}
	return spec
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
	// ProviderTuning carries the numeric agent-tuning defaults shared with the
	// leaf models package (MaxSteps, ContextBudget, MaxTokens, ToolCallFormat,
	// Prefill, DefaultContext) — embedded to avoid duplicating those fields.
	models.ProviderTuning
	Reasoning reasoning.ReasoningSpec
}

// providerTiers is the composed per-provider agent-tuning table: numeric
// defaults come from the leaf models.ProviderTuningDefaults table (single
// source — see models/tuning.go), reasoning wire from the capability table
// (reasoning.ReasoningCapabilityFor).
// It is allocated once at package init; callers must treat the returned map as
// read-only (ProviderTiers returns the shared instance, never a copy).
var providerTiers = composeProviderTiers()

func composeProviderTiers() map[string]ProviderTuningDefaults {
	numeric := models.ProviderTuningDefaults()
	out := make(map[string]ProviderTuningDefaults, len(numeric))
	for provider, n := range numeric {
		spec := reasoning.ReasoningCapabilityFor(provider).Spec()
		out[provider] = ProviderTuningDefaults{
			ProviderTuning: models.ProviderTuning{
				MaxSteps:       n.MaxSteps,
				ContextBudget:  n.ContextBudget,
				MaxTokens:      n.MaxTokens,
				ToolCallFormat: n.ToolCallFormat,
				Prefill:        n.Prefill,
				DefaultContext: n.DefaultContext,
			},
			Reasoning: spec,
		}
	}
	return out
}

// ProviderTiers returns the shared provider-tuning table. Treat as read-only.
func ProviderTiers() map[string]ProviderTuningDefaults {
	return providerTiers
}

// ReasoningCapabilityFor re-exports the reasoning capability lookup so
// transport handlers (which import assistant, not the leaf reasoning package)
// keep a stable accessor.
func ReasoningCapabilityFor(providerType string) reasoning.ReasoningCapability {
	return reasoning.ReasoningCapabilityFor(providerType)
}

// AgentConfig holds immutable per-agent data from user/model config.
type AgentConfig struct {
	MaxSteps        int
	ContextBudget   int
	MaxTokens       int
	ReasoningSpec   reasoning.ReasoningSpec
	ReasoningBudget int // local think-token budget (ModeThinkTokens); 0 for effort/object/enabled modes
	Temperature     float64
	ICUWeight       float64
	GlobalTimeout   time.Duration
	UseNativeTools  bool
	UsePrefill      bool
	WorkspaceID     string
	ModelName       string
	ProviderType    string
	WorkloadClass   models.WorkloadClass
	SkipStuckCheck  bool
	EnableHotMemory bool
	// LoopStrategy selects the agent loop archetype. Empty = provider default /
	// react; the resolver applies the deterministic precedence.
	LoopStrategy LoopStrategyName
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
	GuardrailApprovalTimeout time.Duration
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
	ReasoningBudget          int // configured think-token budget (local mode); 0 => tier default
	ReasoningEnabled         *bool
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
	WorkloadClass            models.WorkloadClass
	GlobalTimeout            time.Duration
	LoopStrategy             LoopStrategyName
	MemoryStore              *memory.Store // nil when memory is disabled

	EnableHotMemory bool // inject hot memory at session start
	// Channel is the event stream this agent publishes to. Defaults to
	// ChannelAssistant; automation sets ChannelAutomation.
	Channel EventChannel
	// ConversationID scopes this agent's events to a specific chat session.
	ConversationID string

	// AllowedTools / ExcludedTools constrain the tool schema this agent can
	// see. They are combined with the guardrail-derived static exclusions in
	// NewAgent (resolveToolProvider) — the single narrow waist for tool
	// availability. allow ∩ exclude ∩ guardrail-disabled.
	AllowedTools  []string
	ExcludedTools []string

	// Safety timeouts — per-model overrides for unattended run hardening.
	// Zero means "use global default" (set in applyDefaults).
	ToolTimeout              time.Duration // default 2 min; 0 = disabled
	FilesystemToolTimeout    time.Duration // default 30 sec
	MaxPlanDuration          time.Duration // default 15 min
	MaxPlanSteps             int           // default 50
	GuardrailTimeout         time.Duration // default 5 sec
	GuardrailTimeoutBehavior string        // "fail-open" | "fail-closed"
	GuardrailApprovalTimeout time.Duration // default 5 min
}

type GuardrailDecisionStore struct {
	mu      sync.Mutex
	pending map[string]chan GuardrailDecision
	// retained keeps the last N blocked payloads (tombstones) after their
	// approval channel is removed (resolved or timed out). They let a late
	// "allow & remember" decision persist an override even though the agent's
	// wait already expired (SPEC guardrails: persist override, current tool
	// skipped). Bounded so an unbounded stream of blocks cannot leak memory.
	retained    map[string]GuardrailBlockedPayload
	retainedOrd []string
}

const maxRetainedGuardrailDecisions = 100

func NewGuardrailDecisionStore() *GuardrailDecisionStore {
	return &GuardrailDecisionStore{
		pending:     make(map[string]chan GuardrailDecision),
		retained:    make(map[string]GuardrailBlockedPayload),
		retainedOrd: make([]string, 0, maxRetainedGuardrailDecisions),
	}
}

func (s *GuardrailDecisionStore) Register(id string, ch chan GuardrailDecision) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending[id] = ch
}

// Retain stores a blocked payload tombstone keyed by decision id so a late
// decision can still persist an override. Keeps the most recent
// maxRetainedGuardrailDecisions entries to bound memory.
func (s *GuardrailDecisionStore) Retain(payload GuardrailBlockedPayload) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.retained[payload.DecisionID]; exists {
		return
	}
	if len(s.retainedOrd) >= maxRetainedGuardrailDecisions {
		oldest := s.retainedOrd[0]
		s.retainedOrd = s.retainedOrd[1:]
		delete(s.retained, oldest)
	}
	s.retainedOrd = append(s.retainedOrd, payload.DecisionID)
	s.retained[payload.DecisionID] = payload
}

// Payload returns the retained blocked payload for a decision id, allowing the
// handler to persist a late override after the approval channel is gone.
func (s *GuardrailDecisionStore) Payload(id string) (GuardrailBlockedPayload, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.retained[id]
	return p, ok
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

// NewGuardrailDecisionCallback builds the approval-wait callback wired into
// agents as OnGuardrail. channel is the producer stream (assistant | automation)
// stamped on the emitted events — the event bus partitions by channel, so an
// assistant approval must be published on the assistant channel or the chat
// SSE never sees it and the wait burns the full GuardrailApprovalTimeout
// (observed 2026-08-31: `wc` block, no banner, 5-minute stall).
func NewGuardrailDecisionCallback(store *GuardrailDecisionStore, observer Observer, channel EventChannel) GuardrailDecisionCallback {
	return func(ctx context.Context, payload GuardrailBlockedPayload) (GuardrailDecision, error) {
		ch := make(chan GuardrailDecision, 1)
		store.Register(payload.DecisionID, ch)
		// Retain a tombstone so a late decision (after the wait expires) can
		// still persist an override for future calls.
		store.Retain(payload)

		if observer != nil {
			observer(AgentEvent{
				Type:      EventGuardrailBlocked,
				Channel:   channel,
				Payload:   payload,
				Timestamp: time.Now(),
			})
		}

		select {
		case decision := <-ch:
			if observer != nil {
				observer(AgentEvent{
					Type:    EventGuardrailInvalidated,
					Channel: channel,
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
			// Distinguish a bounded approval-wait timeout (the 60s guardrail
			// approval bound) from run cancellation so observers/logs can tell
			// "the user never answered" apart from "the run was cancelled".
			reason := "context_cancelled"
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				reason = "timeout"
			}
			if observer != nil {
				observer(AgentEvent{
					Type:    EventGuardrailInvalidated,
					Channel: channel,
					Payload: GuardrailInvalidatedPayload{
						DecisionID: payload.DecisionID,
						Reason:     reason,
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
	if o.GuardrailApprovalTimeout <= 0 {
		o.GuardrailApprovalTimeout = DefaultGuardrailApprovalTimeout
	}
}

// ApplyModelConfig copies model-level overrides from cfg into the options.
// The loop strategy is parsed here so callers never hold a raw config string.
func (o *AgentOptions) ApplyModelConfig(cfg models.ModelConfig) {
	if cfg.Provider != "" {
		o.ProviderType = cfg.Provider
	}
	if cfg.WorkloadClass != "" {
		o.WorkloadClass = cfg.WorkloadClass
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
	if cfg.ReasoningEnabled != nil {
		o.ReasoningEnabled = cfg.ReasoningEnabled
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
	if cfg.GuardrailApprovalTimeoutSecs > 0 {
		o.GuardrailApprovalTimeout = time.Duration(cfg.GuardrailApprovalTimeoutSecs) * time.Second
	}
	if cfg.LoopStrategy != "" {
		o.LoopStrategy = ParseLoopStrategy(cfg.LoopStrategy)
	}
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
			Provider:     resolveToolProvider(provider, gr, opts.WorkspaceID, opts.AllowedTools, opts.ExcludedTools),
			Engine:       engine,
			Guardrails:   gr,
			Logger:       opts.Logger,
			Observer:     opts.Observer,
			Orchestrator: opts.Orchestrator,
			MemoryStore:  opts.MemoryStore,
			OnGuardrail:  opts.GuardrailDecisionHandler,
		},
		config: AgentConfig{
			MaxSteps:                 opts.MaxSteps,
			ContextBudget:            opts.ContextBudget,
			MaxTokens:                opts.MaxResponseTokens,
			ReasoningSpec:            applyReasoningEnabledOverride(resolveReasoningSpec(opts.ProviderType, opts.ReasoningBudget, opts.MaxResponseTokens), opts.ReasoningEnabled, opts.WorkloadClass),
			ReasoningBudget:          opts.ReasoningBudget,
			Temperature:              opts.Temperature,
			ICUWeight:                opts.ICUWeight,
			GlobalTimeout:            opts.GlobalTimeout,
			UseNativeTools:           useNative,
			UsePrefill:               usePrefill,
			WorkspaceID:              opts.WorkspaceID,
			ModelName:                opts.ModelName,
			ProviderType:             opts.ProviderType,
			WorkloadClass:            opts.WorkloadClass,
			LoopStrategy:             opts.LoopStrategy,
			EnableHotMemory:          opts.EnableHotMemory,
			Channel:                  opts.Channel,
			ConversationID:           opts.ConversationID,
			ToolTimeout:              opts.ToolTimeout,
			FilesystemToolTimeout:    opts.FilesystemToolTimeout,
			MaxPlanDuration:          opts.MaxPlanDuration,
			MaxPlanSteps:             opts.MaxPlanSteps,
			GuardrailTimeout:         opts.GuardrailTimeout,
			GuardrailTimeoutBehavior: opts.GuardrailTimeoutBehavior,
			GuardrailApprovalTimeout: opts.GuardrailApprovalTimeout,
		},
	}

	// Keep the numeric ReasoningBudget in sync with the resolved spec for local
	// (ModeThinkTokens) providers, so downstream consumers (preflight ICU cost,
	// ReasoningBudgetExceeded stuck-check, interceptor budget) use the derived
	// value rather than the raw config (which is 0 when auto-derived). The spec
	// remains the single source of truth; this field is its numeric projection.
	if a.config.ReasoningSpec.Mode == reasoning.ModeThinkTokens {
		a.config.ReasoningBudget = a.config.ReasoningSpec.Budget
	}

	opts.Logger.Info("NewAgent: agent created", "max_tokens", a.config.MaxTokens, "reasoning_mode", int(a.config.ReasoningSpec.Mode), "max_steps", a.config.MaxSteps)
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
	execCtx = proxy.WithRetryObserver(execCtx, func(info proxy.RetryInfo) { a.notifyUpstream(info) })

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
