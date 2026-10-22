// run_options.go — the one place a run's agent options are resolved, for every kind of run (a chat turn, an
// automation). The precedence is the same for each setting: the run's own choice, then its profile, then the model's
// config, then the global default.
package assistant

import (
	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/assistant/profile"
	"llm-proxy/internal/core/orchestrator"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/memory"
	"llm-proxy/models"
)

// RunOptionDeps is what resolving a run's options reads: the model's config, the shared engines and the settings.
type RunOptionDeps interface {
	ModelConfig(modelName string) (models.ModelConfig, bool)
	GuardrailEngine() *guardrails.GuardrailEngine
	GuardrailDecisionStore() *GuardrailDecisionStore
	Orchestrator() *orchestrator.Orchestrator
	MemoryStore() *memory.Store
	MemorySettings() *models.MemoryConfig
	AgentSettings() *models.AgentConfig
}

// RunOverrides are the choices made for one run, above its profile: an automation's own fields, a workspace's
// setting for its chats. Zero values defer to the profile and the defaults.
type RunOverrides struct {
	LoopStrategy  LoopStrategyName
	AllowedTools  []string // an allow list narrows the tools (unattended runs)
	ExcludedTools []string // a deny list removes tools (the chat's UI)
	Memory        models.MemoryMode
}

// RunSpec describes one run to resolve options for.
type RunSpec struct {
	Kind           profile.Kind
	WorkspaceID    string
	ConversationID string // a chat's conversation; empty for an automation
	Model          string
	Profile        profile.Profile
	Overrides      RunOverrides
	Logger         logging.Logger
	Observer       Observer
	// DecisionEvents publishes guardrail approval events; nil publishes them through Observer.
	DecisionEvents Observer
}

// ResolveRunOptions returns the agent options for one run.
func ResolveRunOptions(deps RunOptionDeps, spec RunSpec) AgentOptions {
	channel := channelFor(spec.Kind)
	decisionEvents := spec.DecisionEvents
	if decisionEvents == nil {
		decisionEvents = spec.Observer
	}
	hot := HotMemoryActive(deps, spec)
	opts := AgentOptions{
		Logger:                   spec.Logger,
		MaxSteps:                 DefaultMaxSteps,
		Guardrails:               deps.GuardrailEngine(),
		WorkspaceID:              spec.WorkspaceID,
		ConversationID:           spec.ConversationID,
		Channel:                  channel,
		Orchestrator:             deps.Orchestrator(),
		ModelName:                spec.Model,
		Observer:                 spec.Observer,
		GuardrailDecisionHandler: NewGuardrailDecisionCallback(deps.GuardrailDecisionStore(), decisionEvents, channel),
		AllowedTools:             spec.Overrides.AllowedTools,
		ExcludedTools:            spec.Overrides.ExcludedTools,
		Profile:                  spec.Profile,
		AdaptiveResultCap:        deps.AgentSettings().AdaptiveResultCapEnabled(),
		EnableHotMemory:          hot,
	}
	// A chat keeps the store with hot memory off: memory_search reads it. An automation has memory only when on.
	if hot || spec.Kind == profile.KindAssistant {
		opts.MemoryStore = deps.MemoryStore()
	}
	if cfg, ok := deps.ModelConfig(spec.Model); ok && spec.Model != "" {
		opts.ApplyModelConfig(cfg)
		if spec.Logger != nil {
			spec.Logger.Info("ModelConfig loaded", "model", spec.Model, "max_tokens", cfg.MaxTokens, "reasoning_budget", cfg.ReasoningBudget, "context_budget", cfg.ContextBudget, "provider", cfg.Provider)
		}
	}
	// The run's own loop beats the model's. Unknown values are rejected at the HTTP boundary (400); parsing here is
	// defense-in-depth.
	if spec.Overrides.LoopStrategy != "" {
		opts.LoopStrategy = ParseLoopStrategy(spec.Overrides.LoopStrategy)
	}
	return opts
}

// HotMemoryActive reports whether the run gets hot memory: its own choice, else its profile's, else the global
// default for its kind — and a store to read. A prompt that mentions memory asks the same question.
func HotMemoryActive(deps RunOptionDeps, spec RunSpec) bool {
	settings := deps.MemorySettings()
	globalDefault := settings.AssistantHotDefault()
	if spec.Kind == profile.KindAutomation {
		globalDefault = settings.AutomationHotDefault()
	}
	return spec.Overrides.Memory.Effective(spec.Profile.Memory.Effective(globalDefault)) && deps.MemoryStore() != nil
}

// channelFor is the event stream a kind of run publishes on.
func channelFor(kind profile.Kind) EventChannel {
	if kind == profile.KindAutomation {
		return ChannelAutomation
	}
	return ChannelAssistant
}
