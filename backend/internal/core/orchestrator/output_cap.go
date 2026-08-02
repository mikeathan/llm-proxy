// output_cap.go — OutputCapSource chain of responsibility (§2.6).  First known
// wins: published per-model cap → per-provider tier row → (extensibility)
// learned-from-400.  The chain lives here so CloudBudgetPolicy never grows a
// provider switch.
package orchestrator

import (
	"llm-proxy/models"
)

// OutputCapSource is one element of the output-cap chain.  Each element
// reports a known cap or declines.
type OutputCapSource interface {
	OutputCap(cfg models.ModelConfig) (tokens int, known bool)
}

// publishedOutputCap reads a cap the provider actually publishes for the model
// (max_completion_tokens / top_provider.max_completion_tokens), carried on
// ModelConfig.PublishedOutputCap, clamped to the per-provider tier row
// (min(published, tier) — §2.10 #5, so a per-tier raise never exceeds what a
// model publishes).  Falls back to the persisted ModelMetadata.MaxOutputTokens
// (Phase 2 carrier) so the clamp survives a restart.
type publishedOutputCap struct{}

func (publishedOutputCap) OutputCap(cfg models.ModelConfig) (int, bool) {
	cap := cfg.PublishedOutputCap
	if cap <= 0 && cfg.Metadata != nil && cfg.Metadata.MaxOutputTokens > 0 {
		cap = cfg.Metadata.MaxOutputTokens
	}
	if cap <= 0 {
		return 0, false
	}
	if tier := models.TuningFor(cfg.Provider).MaxTokens; tier > 0 && cap > tier {
		return tier, true
	}
	return cap, true
}

// providerTierCap is the per-provider row in models/tuning.go — data, not a
// global constant.  A provider known to support larger outputs raises its row
// independently; the chain still clamps to min(published, tier).
type providerTierCap struct{}

func (providerTierCap) OutputCap(cfg models.ModelConfig) (int, bool) {
	if cap := models.TuningFor(cfg.Provider).MaxTokens; cap > 0 {
		return cap, true
	}
	return 0, false
}

// outputCapChain is the ordered chain.  First known wins.
var outputCapChain = []OutputCapSource{
	publishedOutputCap{},
	providerTierCap{},
}

// ResolveOutputCap walks the chain and returns the first known output cap, or
// 0 when nothing is known (capless).
func ResolveOutputCap(cfg models.ModelConfig) int {
	for _, src := range outputCapChain {
		if cap, ok := src.OutputCap(cfg); ok {
			return cap
		}
	}
	return 0
}
