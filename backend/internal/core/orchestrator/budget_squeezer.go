package orchestrator

// budget_squeezer implements the density-based adaptive squeezing
// algorithm and the ICU weight resolvers.  When a request would exceed
// the remaining budget, the squeezer reduces max_tokens and reasoning
// budget using a quadratic decay curve with a hard floor at 20% to
// prevent empty responses.  The weight functions convert OpenRouter
// pricing to ICU multipliers and local model parameter counts to weights.

import (
	"math"
	"strconv"
	"strings"

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
		contextLen = 8192
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

// ApplyMetadataDefaults sets max_tokens, context_budget, and
// reasoning_budget from the model's context length when available
// and the field hasn't been explicitly configured.  For local models
// the GGUF scanner provides context length; for OpenRouter it comes
// from the limits block in the model list response.
//
// Rules:
//
//	max_tokens       = context / 4       (leave 75% for prompt + history)
//	context_budget   = context * 2       (chars at ~2 chars/token)
//	reasoning_budget = context / 8       only when name suggests reasoning
func ApplyMetadataDefaults(cfg *models.ModelConfig) {
	ctxLen := resolveContextLength(cfg)
	if ctxLen <= 0 {
		return
	}

	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = ctxLen / 4
	}
	if cfg.ContextBudget == 0 {
		cfg.ContextBudget = ctxLen * 2
	}
	if cfg.ReasoningBudget == 0 {
		name := strings.ToLower(cfg.Name + " " + cfg.Filename)
		if strings.Contains(name, "thinking") || strings.Contains(name, "reason") || strings.Contains(name, "r1") || strings.Contains(name, "o3") || strings.Contains(name, "o4") {
			cfg.ReasoningBudget = ctxLen / 8
		}
	}
}

var providerCtxDefaults = map[string]int{
	"gemini":     1_048_576,
	"openai":     128_000,
	"nvidia":     128_000,
	"vertex":     1_048_576,
	"openrouter": 128_000,
	"mulerouter": 128_000,
	"local":      8192,
}

var knownCtx = map[string]int{
	// Exceptional models — context length differs from their provider default.
	"deepseek-v3":    64_000,   // V3 has 64K, all other DeepSeek models default to 128K
	"claude-sonnet":  200_000,  // Claude 4 Sonnet
	"claude-opus":    200_000,  // Claude 4 Opus
	"claude-3.5":     200_000,  // Claude 3.5 Sonnet
	"o3":             200_000,  // o-series has 200K, not standard OpenAI 128K
	"o4":             200_000,
	"gemini-1.5-pro": 2_097_152, // 2M — only Gemini model above the 1M provider default
	"mistral-small":  32_000,    // 32K — unusually small in Mistral family
}

func resolveContextLength(cfg *models.ModelConfig) int {
	if cfg.Metadata != nil && cfg.Metadata.ContextLength > 0 {
		return cfg.Metadata.ContextLength
	}

	name := strings.ToLower(cfg.Name + " " + cfg.Filename)
	for fragment, ctx := range knownCtx {
		if strings.Contains(name, fragment) {
			return ctx
		}
	}

	if ctx, ok := providerCtxDefaults[cfg.Provider]; ok {
		return ctx
	}
	return 0
}
// 1. Explicit ProviderConfig.InternalCreditWeight override
// 2. Local models: derived from GGUF metadata parameter count
// 3. Default 1.0
func ResolveICUWeight(cfg models.ModelConfig) float64 {
	if cfg.ProviderConfig != nil && cfg.ProviderConfig.InternalCreditWeight > 0 {
		return cfg.ProviderConfig.InternalCreditWeight
	}
	if cfg.Provider == "local" && cfg.Metadata != nil && cfg.Metadata.Parameters > 0 {
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
