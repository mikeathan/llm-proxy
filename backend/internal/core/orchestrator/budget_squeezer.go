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

// ApplyMetadataDefaults sets max_tokens, context_budget, and tool_call_format
// from the model's context length when available and the field hasn't been
// explicitly configured. For local models the GGUF scanner provides context
// length; for OpenRouter it comes from the limits block in the model list
// response.
//
// Context length resolution priority (resolveContextLength):
//   1. cfg.Metadata.Nctx           — serving context from llama.cpp /slots
//   2. cfg.Metadata.ContextLength  — training context from GGUF metadata
//   3. knownCtx model-name match   — exceptional models by name
//   4. providerCtxDefaults         — per-provider fallback (e.g. openai=128K)
//   5. 0                           — unknown
//
// Rules:
//
//	max_tokens       = context / 3                  (leave 2/3 for prompt + history)
//	context_budget   = (context - maxTokens) * 2    (chars at ~2 chars/token, reserve response space)
//	tool_call_format = "native" when empty for cloud; local/GGUF workloads stay "" (XML)
//
// Reasoning enablement is NOT derived here. The think-token budget for local
// (ModeThinkTokens) providers is auto-computed from max_tokens via
// DefaultReasoningBudget (assistant/agent.go resolveReasoningSpec), which keeps
// it tied to the server's serving context. It is NEVER inferred from the model
// name (the old name-heuristic gate was removed — it caused false
// positives/negatives). Cloud providers use effort/object/enable_thinking
// (ReasoningSpec), not a numeric budget.
func ApplyMetadataDefaults(cfg *models.ModelConfig) {
	ctxLen := resolveContextLength(cfg)
	if ctxLen <= 0 {
		return
	}

	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = ctxLen / 3
	}
	if cfg.ContextBudget == 0 {
		// Reserve max_tokens space in the context window for the response.
		// At ~2 chars/token, the prompt can safely use (ctxLen - maxTokens) * 2 chars.
		availableCtx := ctxLen - cfg.MaxTokens
		if availableCtx <= 0 {
			availableCtx = ctxLen / 2
		}
		cfg.ContextBudget = availableCtx * 2
	}
	// Reasoning enable params are driven by the provider tier table
	// (assistant/reasoning_param.go) and explicit per-model configuration
	// (ModelConfig.ReasoningBudget for local think-tokens), NOT by fragile
	// name heuristics.  No automatic reasoning-budget derivation from model
	// name happens here.
	// Empty format: cloud APIs default to native tool calling. Local/GGUF workloads
	// (including openai-compat llama.cpp with a .gguf file) stay empty → XML text
	// mode so fat tool-arg JSON is not parsed server-side. Explicit overrides win.
	if cfg.ToolCallFormat == "" && !isLocalWorkload(cfg) {
		cfg.ToolCallFormat = "native"
	}
}

// isLocalWorkload reports whether cfg is a local/GGUF model even when the
// catalogue provider string is "openai" (openai-compat llama.cpp).
func isLocalWorkload(cfg *models.ModelConfig) bool {
	if cfg == nil {
		return false
	}
	if strings.EqualFold(cfg.Provider, "local") {
		return true
	}
	return hasGGUFArtifact(cfg.Filename) || hasGGUFArtifact(cfg.Path) || hasGGUFArtifact(cfg.Name)
}

func hasGGUFArtifact(s string) bool {
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(s)), ".gguf")
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
	// Priority 1: serving context from llama.cpp /slots endpoint
	// (n_ctx, captured by OpenAICompatibleProvider.fetchSlotsContext).
	if cfg.Metadata != nil && cfg.Metadata.Nctx > 0 {
		ctxLen := cfg.Metadata.Nctx
		if maxCtx, ok := providerCtxDefaults[cfg.Provider]; ok && ctxLen > maxCtx {
			return maxCtx
		}
		return ctxLen
	}

	// Priority 2: training context from GGUF metadata (n_ctx_train).
	// Guard against inflated training-context values that exceed the
	// provider's known max — cap to the provider default.
	if cfg.Metadata != nil && cfg.Metadata.ContextLength > 0 {
		ctxLen := cfg.Metadata.ContextLength
		if maxCtx, ok := providerCtxDefaults[cfg.Provider]; ok && ctxLen > maxCtx {
			return maxCtx
		}
		return ctxLen
	}

	// Priority 3: exceptional models keyed by name fragment.
	name := strings.ToLower(cfg.Name + " " + cfg.Filename)
	for fragment, ctx := range knownCtx {
		if strings.Contains(name, fragment) {
			return ctx
		}
	}

	// Priority 4: per-provider default (e.g. openai=128K, local=8K).
	if ctx, ok := providerCtxDefaults[cfg.Provider]; ok {
		return ctx
	}

	// Priority 5: unknown — caller defaults to 0 (no budget applied).
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
