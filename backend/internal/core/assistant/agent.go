// agent.go — Agent struct, options, constructors, guardrail decision store,
// repetition detection, and the Execute entry point (thin wrapper delegating
// to runSession).  Also holds message preparation helpers shared across all
// LLM-calling paths.
package assistant

import (
	"context"
	"fmt"
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
	DefaultMaxSteps             = 25
	DefaultContextBudget        = 8000   // chars, not tokens — rough heuristic for context window pressure
	DefaultMaxTokens            = 3072
	DefaultAutomationTemperature = 0.1   // low temperature for deterministic automation tasks
	MinReasoningStuckThreshold  = 2000   // chars; floor for stuck detection even at small max_tokens
	DefaultStarvationLimit      = 15
	AgentGlobalTimeout          = 30 * time.Minute  // total wall-clock for one Execute call
	AgentTurnTimeout            = 10 * time.Minute  // per-LLM-call timeout (stream or Chat)
	AgentRetryTimeout           = 5 * time.Minute   // tool-support-fallback retry timeout
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

func ProviderTiers() map[string]ProviderTuningDefaults {
	return map[string]ProviderTuningDefaults{
		"local":      {MaxSteps: 25, ContextBudget: 8000, MaxTokens: 2048, ToolCallFormat: "", Prefill: false, ReasoningBudget: 0},
		"gemini":     {MaxSteps: 35, ContextBudget: 50000, MaxTokens: 4096, ToolCallFormat: "native", Prefill: false, ReasoningBudget: 8192},
		"vertex":     {MaxSteps: 35, ContextBudget: 50000, MaxTokens: 4096, ToolCallFormat: "native", Prefill: false, ReasoningBudget: 8192},
		"openai":     {MaxSteps: 35, ContextBudget: 50000, MaxTokens: 4096, ToolCallFormat: "native", Prefill: false, ReasoningBudget: 8192},
		"openrouter": {MaxSteps: 30, ContextBudget: 30000, MaxTokens: 2048, ToolCallFormat: "native", Prefill: false, ReasoningBudget: 4096},
		"mulerouter": {MaxSteps: 30, ContextBudget: 30000, MaxTokens: 2048, ToolCallFormat: "native", Prefill: false, ReasoningBudget: 4096},
		"nvidia":     {MaxSteps: 30, ContextBudget: 20000, MaxTokens: 2048, ToolCallFormat: "native", Prefill: false, ReasoningBudget: 2048},
	}
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

	// Per-execution state. These are mutated during Execute and MUST NOT
	// persist across calls. TODO(A8): move into runSession.
	prefillDisabled bool
	memoryInjected  bool
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
	MemoryStore              *memory.Store      // nil when memory is disabled

	EnableHotMemory bool // inject hot memory at session start
	// Channel is the event stream this agent publishes to. Defaults to
	// ChannelAssistant; automation sets ChannelAutomation.
	Channel EventChannel
	// ConversationID scopes this agent's events to a specific chat session.
	ConversationID string
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
	return cfg.EnableExecutionPlan
}

func NewAgent(client proxy.Client, provider ToolProvider, engine Engine, opts AgentOptions) *Agent {
	opts.applyDefaults()

	gr := opts.Guardrails
	if gr == nil {
		gr = guardrails.NewGuardrailEngine(func() models.AgentGuardrailsConfig { return models.AgentGuardrailsConfig{} }, storage.NewPathResolver("", "", ""), nil)
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
			MaxSteps:        opts.MaxSteps,
			ContextBudget:   opts.ContextBudget,
			MaxTokens:       opts.MaxResponseTokens,
			ReasoningBudget: opts.ReasoningBudget,
			Temperature:     opts.Temperature,
			ICUWeight:       opts.ICUWeight,
			GlobalTimeout:   opts.GlobalTimeout,
			UseNativeTools:  useNative,
			UsePrefill:      usePrefill,
			WorkspaceID:     opts.WorkspaceID,
			ModelName:       opts.ModelName,
			ProviderType:    opts.ProviderType,
			EnableHotMemory: opts.EnableHotMemory,
			Channel:         opts.Channel,
			ConversationID:  opts.ConversationID,
		},
	}
	opts.Logger.Info("NewAgent: agent created", "max_tokens", a.config.MaxTokens, "reasoning_budget", a.config.ReasoningBudget, "max_steps", a.config.MaxSteps)
	return a
}

type toolKey struct {
	name string
	args string
}

type repetitionDetector struct {
	recentCalls           []toolKey
	duplicateStreak       int
	lastTool              string
	consecutiveToolStreak int
}

func (rd *repetitionDetector) check(logger logging.Logger, toolCalls []proxy.ToolCall) (bool, string, error) {
		for _, tc := range toolCalls {
			key := toolKey{tc.Function.Name, tc.Function.Arguments}
			// system_error is a no-op bookkeeping tool and is expected to repeat.
			if tc.Function.Name != models.ToolSystemError {
				if len(rd.recentCalls) > 0 && rd.recentCalls[len(rd.recentCalls)-1] == key {
					rd.duplicateStreak++
					logger.Warn("duplicate action detected", "tool", key.name, "args", key.args, "streak", rd.duplicateStreak)
					if rd.duplicateStreak >= 3 {
						rd.duplicateStreak = 0
					rd.recentCalls = nil
					return true, "", fmt.Errorf("infinite loop detected: %s called 3+ times with identical args", key.name)
				}
				return true, prompts.AutomationDuplicateNagPrompt, nil
			}
			rd.duplicateStreak = 0

			// Catch same-tool-any-args spirals (e.g. memory_search with varying queries).
			if tc.Function.Name == rd.lastTool {
				rd.consecutiveToolStreak++
			if rd.consecutiveToolStreak >= 12 {
				rd.consecutiveToolStreak = 0
				rd.lastTool = ""
				rd.recentCalls = nil
				return true, "", fmt.Errorf("spiral detected: %s called %d+ consecutive times", key.name, 12)
				}
			} else {
				rd.consecutiveToolStreak = 0
				rd.lastTool = tc.Function.Name
			}

			if len(rd.recentCalls) >= 3 {
				rd.recentCalls = rd.recentCalls[1:]
			}
			rd.recentCalls = append(rd.recentCalls, key)
		}
	}
	return false, "", nil
}

func (a *Agent) Execute(ctx context.Context, history []proxy.Message) (string, []proxy.Message, error) {
	execCtx, cancel := context.WithTimeout(ctx, a.config.GlobalTimeout)
	defer cancel()

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

	s := newRunSession(a, execCtx, history)
	return s.run()
}

func (a *Agent) prepareMessages(history []proxy.Message) []proxy.Message {
	return proxy.NormalizeHistory(history, a.config.UseNativeTools)
}

// injectToolInstructions embeds XML tool definitions into the system prompt.
// Used when native API-level tools are disabled (local models, XML fallback).
func (a *Agent) injectToolInstructions(history []proxy.Message, tools []proxy.Tool) []proxy.Message {
	if len(tools) == 0 {
		return history
	}
	info := make([]prompts.ToolInfo, len(tools))
	for i, t := range tools {
		info[i] = prompts.ToolInfo{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			Parameters:  t.Function.Parameters,
		}
	}
	instructions := prompts.BuildToolManual(info)
	a.deps.Logger.Debug("injecting XML tool manual into system prompt",
		"tool_count", len(info),
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
	info := make([]prompts.ToolInfo, len(tools))
	for i, t := range tools {
		info[i] = prompts.ToolInfo{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			Parameters:  t.Function.Parameters,
		}
	}
	reference := prompts.BuildNativeToolReference(info)
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

