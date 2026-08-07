// context_resolution.go — ContextResolution value + the context sources that
// feed budget policies.  Two independent chains per §2.4:
//
//   - LocalBudgetPolicy resolves the SERVING context workload-scoped
//     (priorities 1/2/3 + defaultLocalContextLength), NEVER providerCtxDefaults
//     (Fix 2 — a local workload is never given a cloud guess).
//   - CloudBudgetPolicy resolves the PUBLISHED context via a
//     PublishedContextSource chain (top_provider → top-level → knownCtx →
//     provider tier default).
//
// No network calls happen here; capability chains read cached catalog / model
// metadata only.
package orchestrator

import (
	"strings"

	"llm-proxy/models"
)

// ContextResolution carries the resolved context inputs a BudgetPolicy needs.
// Policies receive it read-only and never re-resolve.
type ContextResolution struct {
	// ServingContext is the resolved local serving window (n_ctx from /slots
	// or /v1/props, GGUF training context, or the numeric local default).
	ServingContext int
	// PublishedContext is the resolved published context length for cloud
	// workloads (0 when nothing is published and no tier default exists).
	PublishedContext int
	// OutputCap is the resolved per-model output cap (0 when unknown).
	OutputCap int
}

// --- Local serving context (workload-scoped, never providerCtxDefaults) ---

// ResolveLocalContext resolves the serving context for a local workload:
//
//	1. Metadata.Nctx               (serving context, /slots or /v1/props)
//	2. Metadata.ContextLength      capped by defaultLocalContextMax
//	3. defaultLocalContextLength   (8192) — the universal local fallback
//
// It never consults providerCtxDefaults, so a local workload can never leak
// into a 128K/1M cloud calculation.  Always returns a numeric value — the
// runtime local path never returns a typed unresolved-context error.
func ResolveLocalContext(cfg *models.ModelConfig) int {
	if cfg.Metadata != nil && cfg.Metadata.Nctx > 0 {
		return cfg.Metadata.Nctx
	}
	if cfg.Metadata != nil && cfg.Metadata.ContextLength > 0 {
		ctx := cfg.Metadata.ContextLength
		if ctx > defaultLocalContextMax {
			return defaultLocalContextMax
		}
		return ctx
	}
	return defaultLocalContextLength
}

// --- Published context source (cloud) ---

// PublishedContextSource is one element of the cloud published-context chain.
// Each element reports a known context or declines; the chain takes the first
// known value.
type PublishedContextSource interface {
	Context(cfg models.ModelConfig) (int, bool)
}

// publishedMetadataContext reads the published context from the live catalog
// (top_provider.context_length / context_length) carried on ModelConfig.
type publishedMetadataContext struct{}

func (publishedMetadataContext) Context(cfg models.ModelConfig) (int, bool) {
	if cfg.PublishedContextLength > 0 {
		return cfg.PublishedContextLength, true
	}
	return 0, false
}

// publishedServingMetadataContext falls back to the model's metadata context
// (n_ctx serving context, then n_ctx_train) when a cloud provider carries it.
type publishedServingMetadataContext struct{}

func (publishedServingMetadataContext) Context(cfg models.ModelConfig) (int, bool) {
	if cfg.Metadata == nil {
		return 0, false
	}
	if cfg.Metadata.Nctx > 0 {
		return cfg.Metadata.Nctx, true
	}
	if cfg.Metadata.ContextLength > 0 {
		return cfg.Metadata.ContextLength, true
	}
	return 0, false
}

// knownCtxContext matches exceptional model names (deepseek-v3, claude-opus,
// etc.) whose context differs from their provider default.
type knownCtxContext struct{}

func (knownCtxContext) Context(cfg models.ModelConfig) (int, bool) {
	name := strings.ToLower(cfg.Name + " " + cfg.Filename)
	for fragment, ctx := range knownCtx {
		if strings.Contains(name, fragment) {
			return ctx, true
		}
	}
	return 0, false
}

// tierDefaultContext is the per-provider default context row from the leaf
// tuning table — the last resort, data-driven (no global constant).
type tierDefaultContext struct{}

func (tierDefaultContext) Context(cfg models.ModelConfig) (int, bool) {
	if ctx := models.TuningFor(cfg.Provider).DefaultContext; ctx > 0 {
		return ctx, true
	}
	return 0, false
}

// publishedContextChain is the ordered chain of responsibility (§2.4).  First
// known wins: top_provider → top-level → knownCtx → tier default.
var publishedContextChain = []PublishedContextSource{
	publishedMetadataContext{},
	publishedServingMetadataContext{},
	knownCtxContext{},
	tierDefaultContext{},
}

// resolvePublishedContext walks the chain and returns the first known context
// length, or 0 when nothing is published/known (capless).
func resolvePublishedContext(cfg models.ModelConfig) int {
	for _, src := range publishedContextChain {
		if ctx, ok := src.Context(cfg); ok {
			return ctx
		}
	}
	return 0
}

// knownCtx lists exceptional models — context length differs from their
// provider default.  Data, not logic.
var knownCtx = map[string]int{
	"deepseek-v3":    64_000,   // V3 has 64K, all other DeepSeek models default to 128K
	"claude-sonnet":  200_000,  // Claude 4 Sonnet
	"claude-opus":    200_000,  // Claude 4 Opus
	"claude-3.5":     200_000,  // Claude 3.5 Sonnet
	"o3":             200_000,  // o-series has 200K, not standard OpenAI 128K
	"o4":             200_000,
	"gemini-1.5-pro": 2_097_152, // 2M — only Gemini model above the 1M provider default
	"mistral-small":  32_000,    // 32K — unusually small in Mistral family
}
