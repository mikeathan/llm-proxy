package models

// ProviderManifest defines the static metadata for a cloud provider.
// This is read from JSON files to dynamically register providers.
type ProviderManifest struct {
	ID             string             `json:"id"`               // unique slug (e.g., openai, nvidia)
	Name           string             `json:"name"`             // display name (e.g., OpenAI, NVIDIA NIM)
	DefaultBaseURL string             `json:"default_base_url"` // fallback URL if not configured
	Archetype      ProviderArchetype  `json:"archetype"`        // implementation template
	Auth           AuthProviderConfig `json:"auth"`             // authentication settings
	Endpoints      ProviderEndpoints  `json:"endpoints"`        // endpoint paths
	Icon           string             `json:"icon,omitempty"`   // emoji or icon slug
	// SessionHeader names a header the provider wants on every chat request, carrying a stable id per conversation
	// (OpenCode Go answers 400 MissingSessionID without x-opencode-session). Empty: none.
	SessionHeader string `json:"session_header,omitempty"`
}

type ProviderArchetype string

const (
	ArchetypeOpenAICompatible ProviderArchetype = "openai-compatible"
	ArchetypeGemini           ProviderArchetype = "gemini"
	ArchetypeCustom           ProviderArchetype = "custom"
)

type AuthProviderConfig struct {
	Type         string `json:"type"`                    // bearer, header, query
	HeaderName   string `json:"header_name,omitempty"`   // e.g., Authorization, X-API-Key
	HeaderPrefix string `json:"header_prefix,omitempty"` // e.g., Bearer
	Placeholder  string `json:"placeholder,omitempty"`   // help text
}

type ProviderEndpoints struct {
	Models string `json:"models,omitempty"` // default: /models
	Chat   string `json:"chat,omitempty"`   // default: /chat/completions
}
