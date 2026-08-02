package models

// Provider registry — single source of truth for the canonical set of provider
// keys. The numeric tuning table (tuning.go) and the reasoning wire/capability
// tables (assistant/reasoning_param.go) key off these IDs; the drift test
// enforces that every table covers every ID (and nothing else). Adding a
// provider = add a const + a row in each table; CI fails on omission.
//
// This lives in the leaf models package on purpose: assistant imports models,
// never the reverse, so a shared key set here avoids an import cycle while
// still being importable by every downstream package.
const (
	ProviderLocal      = "local"
	ProviderGemini     = "gemini"
	ProviderOpenAI     = "openai"
	ProviderOpenRouter = "openrouter"
	ProviderNVIDIA     = "nvidia"
)

// ProviderIDs returns the canonical, ordered list of provider keys.
func ProviderIDs() []string {
	return []string{
		ProviderLocal,
		ProviderGemini,
		ProviderOpenAI,
		ProviderOpenRouter,
		ProviderNVIDIA,
	}
}

// SupportsBaseURL reports whether a provider accepts a per-key custom base URL
// (the OpenAI-compatible wire). Gemini uses project_id/region instead; local
// has no cloud base URL. This is a provider capability, surfaced to the UI via
// provider_defaults, so the frontend never hardcodes the provider list.
func SupportsBaseURL(id string) bool {
	switch id {
	case ProviderOpenAI, ProviderOpenRouter, ProviderNVIDIA:
		return true
	default:
		return false
	}
}
