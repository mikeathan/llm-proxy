// reasoning_param.go — Single source of truth for provider reasoning-wire
// knowledge. Consolidates what was previously split across client.go
// (ReasoningField), agent.go (providerTiers.ReasoningBudget), stream.go
// (SetReasoningBudget) and budget_squeezer.go into ONE typed table plus ONE
// resolver strategy. The local override keys on the shared WorkloadClassifier
// (models.WorkloadClass), the same classifier that drives budget and ICU
// selection. No per-provider wire logic lives anywhere else.
package assistant

import (
	"fmt"

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
	// ModeEffort — openai / gemini: reasoning_effort string.
	ModeEffort
	// ModeObject — openrouter: reasoning object (effort + enabled).
	ModeObject
	// ModeEnableThinking — nvidia / Poolside NIM: chat_template_kwargs.enable_thinking.
	ModeEnableThinking
)

// reasoningModeNames maps wire mechanisms to their API string spelling.
var reasoningModeNames = map[ReasoningMode]string{
	ModeThinkTokens:    "think_tokens",
	ModeEffort:         "effort",
	ModeObject:         "object",
	ModeEnableThinking: "enable_thinking",
}

// String returns the API spelling for the reasoning mode.
func (m ReasoningMode) String() string {
	if s, ok := reasoningModeNames[m]; ok {
		return s
	}
	return "unknown"
}

// ReasoningEffort is the discrete effort level sent on the wire (low|medium|high).
type ReasoningEffort int

const (
	EffortLow ReasoningEffort = iota
	EffortMedium
	EffortHigh
	// EffortNone — explicit "disabled" for effort-mode providers. Sent as an
	// omitted reasoning_effort on the wire (see effortResolver), never as the
	// "medium" sentinel. Validate forbids EffortNone outside ModeEffort, so it
	// can only reach the effort path.
	EffortNone
)

// String returns the wire spelling for the effort level.
func (e ReasoningEffort) String() string {
	switch e {
	case EffortLow:
		return "low"
	case EffortHigh:
		return "high"
	case EffortNone:
		return ""
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

// Validate rejects internally inconsistent specs. EffortNone is only valid in
// ModeEffort (it means "omit reasoning_effort"); other modes require a concrete
// effort or no effort at all.
func (s ReasoningSpec) Validate() error {
	switch s.Mode {
	case ModeEffort:
		if s.Enabled {
			return errReasoningConflict("effort mode must not set Enabled")
		}
		// EffortNone (omit) is valid here; low/medium/high also valid.
	case ModeObject:
		if s.Effort == EffortNone {
			return errReasoningConflict("object mode requires a concrete effort (not none)")
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
	// EffortNone means "omit reasoning_effort entirely" (provider default).
	// Short-circuit BEFORE any String() call so the disabled state is explicit
	// and never leaks the "medium" sentinel.
	if spec.Effort == EffortNone {
		resolverNoop.Apply(req, spec)
		return
	}
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
	enabled := spec.Enabled
	req.Reasoning = &models.ReasoningObject{
		Effort:  spec.Effort.String(),
		Enabled: &enabled,
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
	resolverEffort         = effortResolver{}
	resolverObject         = objectResolver{}
	resolverEnableThinking = enableThinkingResolver{}
	resolverThinkTokens    = thinkTokensResolver{}
	resolverNoop           = noopResolver{}
)

// providerReasoningCapabilities is the single provider-type-keyed table: it
// carries BOTH the wire spec (Mode/Effort/Enabled/Budget — the provider's
// native default reasoning parameters) AND the toggle capability descriptor
// (DefaultEnabled/Toggleable) surfaced to the frontend via the admin API.
// The local override (host detection) is applied ONCE in NewReasoningResolver;
// this table is the only place per-provider reasoning knowledge lives.
var providerReasoningCapabilities = map[string]ReasoningCapability{
	models.ProviderLocal:      {Mode: ModeThinkTokens, Effort: EffortMedium, Budget: 0, DefaultEnabled: false, Toggleable: false},
	models.ProviderGemini:     {Mode: ModeEffort, Effort: EffortMedium, DefaultEnabled: true, Toggleable: true},
	models.ProviderOpenAI:     {Mode: ModeEffort, Effort: EffortMedium, DefaultEnabled: true, Toggleable: true},
	models.ProviderOpenRouter: {Mode: ModeObject, Effort: EffortMedium, DefaultEnabled: true, Toggleable: true, Enabled: true},
	models.ProviderNVIDIA:     {Mode: ModeEnableThinking, Effort: EffortMedium, DefaultEnabled: true, Toggleable: true, Enabled: true},
}

// ReasoningCapability is the declarative, provider-type-keyed descriptor of a
// provider's reasoning wire mechanism and whether (and how) it can be toggled.
// This is the single source of truth surfaced to the frontend via the admin
// API; adding a provider means adding one row here (plus, only if the wire
// mechanism is new, one resolver). It is keyed by wire protocol (provider
// type), never by vendor/model slug.
type ReasoningCapability struct {
	// Mode is the wire mechanism for this provider family.
	Mode ReasoningMode
	// Effort is the default effort level used when reasoning is enabled.
	Effort ReasoningEffort
	// Enabled is the provider's native wire default for flag/object modes
	// (nvidia/openrouter default true; effort modes leave it false).
	Enabled bool
	// Budget is the provider's native think-token budget (ModeThinkTokens
	// only; resolved to the derived value at agent-build time).
	Budget int
	// DefaultEnabled is the provider's native default for reasoning.
	DefaultEnabled bool
	// Toggleable reports whether a disabled state is expressible on the wire
	// (false for local llama.cpp — no toggle, no wire change).
	Toggleable bool
}

// Spec returns the wire ReasoningSpec implied by the capability's native
// defaults. Wire fields (Enabled/Budget) are resolved into ReasoningSpec at
// compose time; buildChatRequest then applies the resolver.
func (c ReasoningCapability) Spec() ReasoningSpec {
	return ReasoningSpec{Mode: c.Mode, Effort: c.Effort, Enabled: c.Enabled, Budget: c.Budget}
}

// ReasoningCapabilityFor returns the reasoning capability descriptor for a
// provider type. Unknown provider types are reported as non-toggleable (the
// safe default — no UI toggle, no wire change).
func ReasoningCapabilityFor(providerType string) ReasoningCapability {
	if c, ok := providerReasoningCapabilities[providerType]; ok {
		return c
	}
	return ReasoningCapability{Mode: ModeEffort, Effort: EffortMedium, DefaultEnabled: false, Toggleable: false}
}

// NewReasoningResolver returns the resolver for the given provider type,
// applying the single local-override rule: a workload classified WorkloadLocal
// (via the shared WorkloadClassifier — the same classifier that drives budget
// and ICU) always uses thinking_budget_tokens regardless of the cloud slug.
// This preserves the working local path and is the ONLY place workload
// classification is consulted for the reasoning wire.  configuredBudget is the
// user-set think-token budget (used only for the local override path).
func NewReasoningResolver(workload models.WorkloadClass, providerType string, configuredBudget int) ReasoningParamResolver {
	if workload == models.WorkloadLocal {
		// Local llama.cpp always uses thinking_budget_tokens, never the cloud
		// enable params. Budget comes from explicit config (0 = server default).
		return &localOverrideResolver{budget: configuredBudget}
	}
	if capability, ok := providerReasoningCapabilities[providerType]; ok {
		return resolverForMode(capability.Mode)
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
