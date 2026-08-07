package orchestrator

// budget_squeezer implements the density-based adaptive squeezing
// algorithm and the ICU weight resolvers.  When a request would exceed
// the remaining budget, the squeezer reduces max_tokens and reasoning
// budget using a quadratic decay curve with a hard floor at 20% to
// prevent empty responses.  The weight functions convert OpenRouter
// pricing to ICU multipliers and local model parameter counts to weights.
//
// ApplyMetadataDefaults is now a THIN compatibility entry point: it resolves
// the effective workload/context, selects a BudgetPolicy by WorkloadClass, and
// applies the result.  All provider-specific context and cap knowledge lives in
// context_resolution.go / budget_policy.go / output_cap.go / models/tuning.go.

import (
	"math"
	"strconv"

	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
)

type SqueezeRequest struct {
	MaxTokens       int
	ReasoningBudget int
	ContextChars    int
	ModelContextLen int
	ICUWeight       float64
	RemainingICU    int64
}

type SqueezeResult struct {
	Allowed           bool
	SqueezeFactor     float64
	AdjustedMaxTokens int
	AdjustedReasoning int
	AllocatedICU      int64
	TransactionID     string
	Reason            string
}

// BudgetSqueezer reduces max_tokens and reasoning budget when the context
// window is crowded, using a density-based decay curve with a hard floor
// at 20% of the original budget to prevent empty responses.
type BudgetSqueezer struct {
	hardFloor float64
}

func NewBudgetSqueezer() *BudgetSqueezer {
	return &BudgetSqueezer{hardFloor: 0.2}
}

func (s *BudgetSqueezer) Squeeze(req SqueezeRequest) SqueezeResult {
	baseICU := int64(float64(req.ContextChars/2+req.MaxTokens+req.ReasoningBudget) * req.ICUWeight)

	if baseICU <= req.RemainingICU {
		return SqueezeResult{
			Allowed:           true,
			SqueezeFactor:     1.0,
			AdjustedMaxTokens: req.MaxTokens,
			AdjustedReasoning: req.ReasoningBudget,
			AllocatedICU:      baseICU,
		}
	}

	contextLen := req.ModelContextLen
	if contextLen <= 0 {
		contextLen = defaultLocalContextLength
	}

	density := float64(req.ContextChars) / float64(2*contextLen)
	if density > 1.0 {
		density = 1.0
	}

	squeezeFactor := 1.0 - (density * density * 0.75)
	squeezeFactor = math.Max(squeezeFactor, s.hardFloor)

	squeezedReasoning := int(math.Floor(float64(req.ReasoningBudget) * squeezeFactor))
	squeezedMaxTokens := int(math.Floor(float64(req.MaxTokens) * squeezeFactor))
	squeezedICU := int64(float64(req.ContextChars/2+squeezedMaxTokens+squeezedReasoning) * req.ICUWeight)

	if squeezedICU <= req.RemainingICU {
		return SqueezeResult{
			Allowed:           true,
			SqueezeFactor:     squeezeFactor,
			AdjustedMaxTokens: squeezedMaxTokens,
			AdjustedReasoning: squeezedReasoning,
			AllocatedICU:      squeezedICU,
		}
	}

	return SqueezeResult{Allowed: false, SqueezeFactor: squeezeFactor, Reason: "ICU cap exceeded"}
}

// ComputeICUWeightFromPricing converts OpenRouter dollar pricing to an
// ICU multiplier: (prompt + completion) / 0.000001.  Returns 0 if the
// pricing strings can't be parsed (nil safety, malformed input).
func ComputeICUWeightFromPricing(pricing *models.ModelPricing) float64 {
	if pricing == nil {
		return 0
	}
	prompt, err1 := strconv.ParseFloat(pricing.Prompt, 64)
	completion, err2 := strconv.ParseFloat(pricing.Completion, 64)
	if err1 != nil || err2 != nil {
		return 0
	}
	totalPerToken := prompt + completion
	if totalPerToken <= 0 {
		return 0
	}
	return totalPerToken / 0.000001
}

// ApplyMetadataDefaults sets max_tokens, context_budget, and tool_call_format
// from the model's workload class and resolved context when the field hasn't
// been explicitly configured.
//
// Workload is classified via cfg.WorkloadClass when the runtime boundary has
// already resolved it (manager Sync), otherwise via a pure classifier over the
// provider label, GGUF artifact, and effective endpoint host (§2.2).  A
// workload that reaches a local serving endpoint must use the local policy —
// this is what keeps `openai`-slugged local models on the ctx/3 math.
//
// Selection:
//
//	WorkloadLocal → LocalBudgetPolicy  (verbatim ctx/3, (ctx-mt)*2)
//	WorkloadCloud → CloudBudgetPolicy  (clamp-first, data-driven)
//
// Rules:
//
//	max_tokens       = context / 3                  (local — leave 2/3 for prompt + history)
//	context_budget   = (context - maxTokens) * 2    (local — chars at ~2 chars/token)
//	tool_call_format = "native" when empty for cloud; local workloads stay "" (XML)
//
// Reasoning enablement is NOT derived here. The think-token budget for local
// (ModeThinkTokens) providers is auto-computed from max_tokens via
// DefaultReasoningBudget (assistant/agent.go resolveReasoningSpec), which keeps
// it tied to the server's serving context. Cloud providers use
// effort/object/enable_thinking (ReasoningSpec), not a numeric budget.
func ApplyMetadataDefaults(cfg *models.ModelConfig) {
	if cfg == nil {
		return
	}

	class := cfg.WorkloadClass
	if class == "" {
		class = classifyWorkload(*cfg)
		cfg.WorkloadClass = class
	}

	var ctx ContextResolution
	switch class {
	case models.WorkloadLocal:
		ctx = ContextResolution{ServingContext: ResolveLocalContext(cfg)}
	default:
		ctx = ContextResolution{
			PublishedContext: resolvePublishedContext(*cfg),
			OutputCap:        ResolveOutputCap(*cfg),
		}
	}

	budget, err := budgetPolicyFor(class).Derive(*cfg, ctx)
	if err != nil {
		// Registration-edge typed error only — the runtime local path is
		// numeric-only and never reaches here.  Surface it as an actionable
		// structured warning (no payloads, per Constitution) so an operator can
		// fix the contradictory configuration; fall back to unset derived fields
		// rather than silently zeroing them.
		logging.Warn("[budget] capability impossible: derived budget rejected",
			"provider", cfg.Provider,
			"model", cfg.Name,
			"published_context", ctx.PublishedContext,
			"error", err.Error(),
		)
		return
	}

	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = budget.MaxTokens
	}
	if cfg.ContextBudget == 0 {
		cfg.ContextBudget = budget.ContextBudget
	}
	// Empty format: cloud APIs default to native tool calling. Local/GGUF
	// workloads stay empty → XML text mode so fat tool-arg JSON is not parsed
	// server-side. Explicit overrides win.
	if cfg.ToolCallFormat == "" && class == models.WorkloadCloud {
		cfg.ToolCallFormat = "native"
	}
}

// classifyWorkload is the pure budget-path classifier.  The runtime boundary
// pre-sets cfg.WorkloadClass using the full classifier (modelHost + local
// interface IPs); this fallback covers provider/artifact/loopback and the
// effective endpoint host carried on ProviderConfig.BaseURL.  No DNS, no
// network calls.
func classifyWorkload(cfg models.ModelConfig) models.WorkloadClass {
	return models.NewWorkloadClassifier("", nil).Classify(cfg)
}

// 1. Explicit ProviderConfig.InternalCreditWeight override
// 2. Local workloads: derived from GGUF metadata parameter count
// 3. Default 1.0
func ResolveICUWeight(cfg models.ModelConfig) float64 {
	if cfg.ProviderConfig != nil && cfg.ProviderConfig.InternalCreditWeight > 0 {
		return cfg.ProviderConfig.InternalCreditWeight
	}
	class := cfg.WorkloadClass
	if class == "" {
		class = classifyWorkload(cfg)
	}
	if class == models.WorkloadLocal && cfg.Metadata != nil && cfg.Metadata.Parameters > 0 {
		switch {
		case cfg.Metadata.Parameters < 4_000_000_000:
			return 0.5
		case cfg.Metadata.Parameters < 8_000_000_000:
			return 1.0
		case cfg.Metadata.Parameters < 20_000_000_000:
			return 1.5
		case cfg.Metadata.Parameters < 40_000_000_000:
			return 2.5
		default:
			return 4.0
		}
	}
	return 1.0
}
