// tuning.go — immutable per-provider agent-tuning defaults, in the leaf models
// package so no import cycle exists (assistant → orchestrator → models).
// This is the ONLY place a default cloud output cap lives — data, not a global
// constant — so a provider known to support larger outputs raises its row
// independently.  The output-cap chain (§2.6) still clamps to
// min(published, tier), so a per-tier raise never exceeds what a model
// publishes.
package models

// ProviderTuning holds the numeric agent-tuning defaults for one provider.
// Reasoning wire configuration is NOT here — it lives in
// assistant/reasoning_param.go (ReasoningSpec), which stays provider-aware but
// never holds numeric output caps.
type ProviderTuning struct {
	MaxSteps       int
	ContextBudget  int
	MaxTokens      int
	ToolCallFormat string
	Prefill        bool
	// DefaultContext is the per-provider default context window (the
	// PublishedContextSource chain's last resort).  Cloud output-cap selection
	// never hardcodes a model's context; this is the tier-level fallback.
	DefaultContext int
}

// providerTuningDefaults is the frozen per-provider table.  Treat as read-only
// (ProviderTuningDefaults returns a copy).
var providerTuningDefaults = map[string]ProviderTuning{
	// "local" MaxTokens is a DISPLAY-ONLY prefill for the UI.  At runtime the
	// local budget is derived from the serving context (LocalBudgetPolicy.Derive
	// = ctxLen/3, ~2730 for the 8192 default), never from this row — the cloud
	// output-cap chain (output_cap.go) that reads this field only runs for cloud
	// workloads, so this value never enters local math.  It is intentionally
	// distinct from assistant.DefaultMaxTokens (3072), which is the global
	// agent-loop fallback for an unconfigured model; keep the two roles separate.
	ProviderLocal:      {MaxSteps: 25, ContextBudget: 8000, MaxTokens: 2048, ToolCallFormat: "", Prefill: false, DefaultContext: 8192},
	ProviderGemini:     {MaxSteps: 35, ContextBudget: 50000, MaxTokens: 8192, ToolCallFormat: "native", Prefill: false, DefaultContext: 1_048_576},
	ProviderOpenAI:     {MaxSteps: 35, ContextBudget: 50000, MaxTokens: 8192, ToolCallFormat: "native", Prefill: false, DefaultContext: 128_000},
	ProviderOpenRouter: {MaxSteps: 30, ContextBudget: 30000, MaxTokens: 8192, ToolCallFormat: "native", Prefill: false, DefaultContext: 128_000},
	ProviderNVIDIA:     {MaxSteps: 30, ContextBudget: 20000, MaxTokens: 8192, ToolCallFormat: "native", Prefill: false, DefaultContext: 128_000},
}

// ProviderTuningDefaults returns a copy of the per-provider tuning table.
func ProviderTuningDefaults() map[string]ProviderTuning {
	out := make(map[string]ProviderTuning, len(providerTuningDefaults))
	for k, v := range providerTuningDefaults {
		out[k] = v
	}
	return out
}

// TuningFor returns the tuning defaults for a provider, or a safe
// OpenAI-compatible default for unknown providers.  Agent tuning only — the
// reasoning-budget wire field is resolved per-request by the reasoning path.
func TuningFor(providerType string) ProviderTuning {
	if t, ok := providerTuningDefaults[providerType]; ok {
		return t
	}
	return ProviderTuning{}
}
