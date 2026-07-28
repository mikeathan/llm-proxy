// reasoning_param.go — Single source of truth for provider reasoning-wire
// knowledge. Consolidates what was previously split across client.go
// (ReasoningField), agent.go (providerTiers.ReasoningBudget), stream.go
// (SetReasoningBudget) and budget_squeezer.go (name gate) into ONE typed table
// plus ONE resolver strategy. No per-provider wire logic lives anywhere else.
package assistant

import (
	"fmt"

	"llm-proxy/internal/core/proxy"
	"llm-proxy/models"
)

// errReasoningConflict builds a typed validation error for an inconsistent spec.
func errReasoningConflict(msg string) error {
	return fmt.Errorf("reasoning spec invalid: %s", msg)
}

// ReasoningMode enumerates the wire mechanism each provider family uses to
// enable reasoning. Typed (not stringly-typed) so invalid combinations are
// unrepresentable.
type ReasoningMode int

const (
	// ModeThinkTokens — local llama.cpp: thinking_budget_tokens.
	ModeThinkTokens ReasoningMode = iota
	// ModeEffort — openai / gemini / vertex / mulerouter: reasoning_effort string.
	ModeEffort
	// ModeObject — openrouter: reasoning object (effort + enabled).
	ModeObject
	// ModeEnableThinking — nvidia / Poolside NIM: chat_template_kwargs.enable_thinking.
	ModeEnableThinking
)

// ReasoningEffort is the discrete effort level sent on the wire (low|medium|high).
type ReasoningEffort int

const (
	EffortLow ReasoningEffort = iota
	EffortMedium
	EffortHigh
)

// String returns the wire spelling for the effort level.
func (e ReasoningEffort) String() string {
	switch e {
	case EffortLow:
		return "low"
	case EffortHigh:
		return "high"
	default:
		return "medium"
	}
}

// ReasoningSpec is the resolved reasoning configuration for a single request.
// Invalid states (e.g. ModeEffort with Enabled=true) cannot be constructed
// through the tier table; Validate guards explicit construction in tests.
type ReasoningSpec struct {
	Mode    ReasoningMode
	Effort  ReasoningEffort // used by ModeEffort / ModeObject
	Enabled bool            // used by ModeEnableThinking
	Budget  int             // used by ModeThinkTokens (post-squeeze amount)
}

// Validate rejects internally inconsistent specs.
func (s ReasoningSpec) Validate() error {
	switch s.Mode {
	case ModeEffort, ModeObject:
		if s.Enabled {
			return errReasoningConflict("effort/object mode must not set Enabled")
		}
	case ModeEnableThinking:
		if s.Effort != EffortMedium {
			return errReasoningConflict("enable_thinking mode must not carry Effort")
		}
	case ModeThinkTokens:
		if s.Effort != EffortMedium || s.Enabled {
			return errReasoningConflict("think_tokens mode must not carry Effort/Enabled")
		}
	}
	return nil
}

// ReasoningParamResolver applies a ReasoningSpec to a ChatRequest. Callers
// (buildChatRequest) depend only on this interface — never on concrete
// providers (Dependency Inversion). Adding a provider means adding a resolver
// + table entry; buildChatRequest stays closed for modification.
type ReasoningParamResolver interface {
	Apply(req *models.ChatRequest, spec ReasoningSpec)
}

// --- Resolver implementations (one per mode, no provider awareness) ---

type effortResolver struct{}

func (effortResolver) Apply(req *models.ChatRequest, spec ReasoningSpec) {
	req.ReasoningEffort = spec.Effort.String()
	// Ensure no other reasoning fields leak onto the wire.
	req.Reasoning = nil
	req.ChatTemplateKwargs = nil
	if spec.Mode != ModeThinkTokens {
		req.ThinkingBudgetTokens = 0
		req.ReasoningBudget = 0
	}
}

type objectResolver struct{}

func (objectResolver) Apply(req *models.ChatRequest, spec ReasoningSpec) {
	req.Reasoning = &models.ReasoningObject{
		Effort:  spec.Effort.String(),
		Enabled: true,
	}
	req.ReasoningEffort = ""
	req.ChatTemplateKwargs = nil
	req.ThinkingBudgetTokens = 0
	req.ReasoningBudget = 0
}

type enableThinkingResolver struct{}

func (enableThinkingResolver) Apply(req *models.ChatRequest, spec ReasoningSpec) {
	req.ChatTemplateKwargs = &models.ChatTemplateKwargs{EnableThinking: spec.Enabled}
	req.Reasoning = nil
	req.ReasoningEffort = ""
	req.ThinkingBudgetTokens = 0
	req.ReasoningBudget = 0
}

type thinkTokensResolver struct{}

func (thinkTokensResolver) Apply(req *models.ChatRequest, spec ReasoningSpec) {
	req.ThinkingBudgetTokens = spec.Budget
	req.ReasoningEffort = ""
	req.Reasoning = nil
	req.ChatTemplateKwargs = nil
	req.ReasoningBudget = 0
}

type noopResolver struct{}

func (noopResolver) Apply(_ *models.ChatRequest, _ ReasoningSpec) {
	// Send nothing — unknown provider or reasoning unsupported.
}

// resolverSingletons — shared per-mode instances. No per-request allocation.
var (
	resolverEffort          = effortResolver{}
	resolverObject          = objectResolver{}
	resolverEnableThinking  = enableThinkingResolver{}
	resolverThinkTokens     = thinkTokensResolver{}
	resolverNoop            = noopResolver{}
)

// providerReasoningTable maps provider type → base ReasoningSpec. The local
// override (host detection) is applied ONCE in NewReasoningResolver; this table
// is the only place per-provider wire knowledge lives.
var providerReasoningTable = map[string]ReasoningSpec{
	"local":      {Mode: ModeThinkTokens, Effort: EffortMedium, Budget: 0},
	"gemini":     {Mode: ModeEffort, Effort: EffortMedium},
	"vertex":     {Mode: ModeEffort, Effort: EffortMedium},
	"openai":     {Mode: ModeEffort, Effort: EffortMedium},
	"openrouter": {Mode: ModeObject, Effort: EffortMedium},
	"mulerouter": {Mode: ModeEffort, Effort: EffortMedium},
	"nvidia":     {Mode: ModeEnableThinking, Enabled: true},
}

// NewReasoningResolver returns the resolver for the given provider type,
// applying the single local-host override: if the client reports it talks to a
// local llama.cpp host (ReasoningField == ThinkTokens), use the think-tokens
// resolver with the configured budget regardless of the cloud slug. This
// preserves the working local path and is the ONLY place host detection is
// consulted. configuredBudget is the user-set think-token budget (used only for
// the local override path).
func NewReasoningResolver(providerType string, client proxy.Client, configuredBudget int) ReasoningParamResolver {
	if client != nil && client.ReasoningField() == proxy.ReasoningFieldThinkTokens {
		// Local llama.cpp always uses thinking_budget_tokens, never the cloud
		// enable params. Budget comes from explicit config (0 = server default).
		return &localOverrideResolver{budget: configuredBudget}
	}
	if spec, ok := providerReasoningTable[providerType]; ok {
		return resolverForMode(spec.Mode)
	}
	return resolverNoop
}

// localOverrideResolver forces think-tokens mode, ignoring the passed spec's
// mode (which may be a cloud tier when an "openai" slug fronts local llama.cpp).
type localOverrideResolver struct {
	budget int
}

func (r *localOverrideResolver) Apply(req *models.ChatRequest, _ ReasoningSpec) {
	resolverThinkTokens.Apply(req, ReasoningSpec{Mode: ModeThinkTokens, Effort: EffortMedium, Budget: r.budget})
}

func resolverForMode(mode ReasoningMode) ReasoningParamResolver {
	switch mode {
	case ModeThinkTokens:
		return resolverThinkTokens
	case ModeEffort:
		return resolverEffort
	case ModeObject:
		return resolverObject
	case ModeEnableThinking:
		return resolverEnableThinking
	default:
		return resolverNoop
	}
}
