// budget_policy.go — Budget value + BudgetPolicy strategy selected by
// WorkloadClass.  LocalBudgetPolicy is a verbatim move of the original
// ctx/3 local math (workload-scoped, numeric-only on the runtime path).
// CloudBudgetPolicy is clamp-first and data-driven (§2.4): it fails with a
// typed error only when a published context is so small no viable prompt
// reserve can fit; otherwise it clamps and succeeds.
package orchestrator

import (
	"errors"

	"llm-proxy/models"
)

// File-top constants (§2.4 / §3.3).  No hardcoded values in logic.
const (
	// defaultLocalContextLength is the universal local fallback serving context
	// when no metadata resolves.  Used only by LocalBudgetPolicy.
	defaultLocalContextLength = 8192
	// defaultLocalContextMax caps a local training-context value that exceeds
	// any real serving window.
	defaultLocalContextMax = 1_048_576

	// promptReserveGuess reserves a minimum amount of the window for the
	// prompt when clamping cloud output caps.
	promptReserveGuess = 2048
	// minHistoryBudgetChars is the floor for a cloud history budget so a tight
	// published context never collapses the sieve to near-zero.
	minHistoryBudgetChars = 8000
	// minViablySmallContext is the smallest published window that can fit any
	// viable prompt reserve.  publishedCtx <= this → ErrCapabilityImpossible.
	minViablySmallContext = 1024
)

// ErrCapabilityImpossible is a typed error returned when a published context
// window is too small to fit any viable prompt reserve — a contradictory
// configuration.  CloudBudgetPolicy clamps and succeeds for every larger
// window (§2.4 / §5 V3).
var ErrCapabilityImpossible = errors.New("capability impossible: published context too small for any viable prompt reserve")

// Budget is an immutable value object carrying the derived tuning numbers.
type Budget struct {
	MaxTokens      int
	ContextBudget  int
}

// BudgetPolicy derives the tuning budget for a workload class.  Consumers
// depend on this small interface; constructors return concrete types.
type BudgetPolicy interface {
	Derive(cfg models.ModelConfig, ctx ContextResolution) (Budget, error)
}

// LocalBudgetPolicy is a verbatim copy of the original local math: ctx/3 and
// (ctx - maxTokens) * 2.  Requires a resolved serving context; never consults
// providerCtxDefaults (Fix 2).  Numeric-only — no typed error on the runtime
// path.
type LocalBudgetPolicy struct{}

func (LocalBudgetPolicy) Derive(cfg models.ModelConfig, ctx ContextResolution) (Budget, error) {
	ctxLen := ctx.ServingContext
	if ctxLen <= 0 {
		ctxLen = defaultLocalContextLength
	}
	maxTokens := cfg.MaxTokens
	if maxTokens == 0 {
		maxTokens = ctxLen / 3
	}
	availableCtx := ctxLen - maxTokens
	if availableCtx <= 0 {
		availableCtx = ctxLen / 2
	}
	return Budget{MaxTokens: maxTokens, ContextBudget: availableCtx * 2}, nil
}

// CloudBudgetPolicy is clamp-first and data-driven.  The output cap comes from
// the OutputCapSource chain; the published context from the
// PublishedContextSource chain; both fall back to the per-provider tier row.
// The chain clamps to min(published, tier) so a per-tier raise never exceeds
// what a model publishes.
type CloudBudgetPolicy struct{}

func (CloudBudgetPolicy) Derive(cfg models.ModelConfig, ctx ContextResolution) (Budget, error) {
	tier := models.TuningFor(cfg.Provider)

	maxTokens := ctx.OutputCap
	if maxTokens <= 0 {
		maxTokens = tier.MaxTokens
	}

	publishedCtx := ctx.PublishedContext
	if publishedCtx > 0 {
		reserve := maxTokens
		if reserve < promptReserveGuess {
			reserve = promptReserveGuess
		}
		if publishedCtx <= minViablySmallContext {
			return Budget{}, ErrCapabilityImpossible
		}
		// CLAMP, do not fail: a tight-but-viable window keeps a working
		// (possibly small) output cap instead of bricking the agent.
		maxTokens = min(maxTokens, max(1, publishedCtx-reserve))
		historyBudget := publishedCtx - maxTokens
		if historyBudget < minHistoryBudgetChars {
			historyBudget = minHistoryBudgetChars
		}
		if tier.ContextBudget > 0 && historyBudget > tier.ContextBudget {
			historyBudget = tier.ContextBudget
		}
		return Budget{MaxTokens: maxTokens, ContextBudget: historyBudget}, nil
	}

	// Capless: nothing to clamp against — use the tier history budget.
	return Budget{MaxTokens: maxTokens, ContextBudget: tier.ContextBudget}, nil
}

// budgetPolicyFor selects the policy strategy by workload class.  A third class
// later is one map entry, not a growing if chain.
func budgetPolicyFor(class models.WorkloadClass) BudgetPolicy {
	switch class {
	case models.WorkloadLocal:
		return LocalBudgetPolicy{}
	default:
		return CloudBudgetPolicy{}
	}
}
