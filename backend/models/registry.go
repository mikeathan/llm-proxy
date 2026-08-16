package models

import "fmt"

// RegistryData represents the dynamic application state (Tier 3: registry.json)
type RegistryData struct {
	Providers     map[string]ProviderRegistryEntry `json:"providers"`
	Catalogue     []ModelRegistryEntry             `json:"catalogue"`
	MCPServers    []MCPServerRegistryEntry         `json:"mcp_servers"`
	PrimaryModel  string                           `json:"primary_model,omitempty"`
	FallbackModel string                           `json:"fallback_model,omitempty"`
	Communication CommunicationConfig              `json:"communication,omitempty"`
	Search        SearchConfig                     `json:"search,omitempty"`
}

type ProviderRegistryEntry struct {
	Type                string `json:"type"`
	DefaultCredentialID string `json:"default_credential_id,omitempty"`
	BaseURL             string `json:"base_url,omitempty"`
}

type ModelRegistryEntry struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	ProviderID     string         `json:"provider_id"`
	ModelID        string         `json:"model_id"`
	CredentialID   string         `json:"credential_id,omitempty"`
	Port           int            `json:"port,omitempty"`
	Args           []string       `json:"args,omitempty"`
	Prefill        *bool          `json:"prefill,omitempty"`
	TimeoutMinutes int            `json:"timeout_minutes,omitempty"`
	Metadata       *ModelMetadata `json:"metadata,omitempty"`
}

type MCPServerRegistryEntry struct {
	Name      string `json:"name"`
	URL       string `json:"url"`
	Enabled   bool   `json:"enabled"`
	TLSCACert string `json:"tls_ca_cert,omitempty"`
}

// ClearDanglingModelRefs ensures PrimaryModel/FallbackModel never dangle: if
// either references a model that is no longer in the catalogue, it is cleared so
// selection logic cannot resolve to a non-existent model. Call after any
// catalogue rebuild.
func ClearDanglingModelRefs(reg *RegistryData) {
	names := make(map[string]bool, len(reg.Catalogue))
	for _, m := range reg.Catalogue {
		names[m.Name] = true
	}
	if reg.PrimaryModel != "" && !names[reg.PrimaryModel] {
		reg.PrimaryModel = ""
	}
	if reg.FallbackModel != "" && !names[reg.FallbackModel] {
		reg.FallbackModel = ""
	}
}

// ModelExists reports whether a model with the given name is present in the
// catalogue. Used to validate primary/fallback references at set time.
func ModelExists(reg *RegistryData, name string) bool {
	for _, m := range reg.Catalogue {
		if m.Name == name {
			return true
		}
	}
	return false
}

// ModelNotFoundError is returned when a primary/fallback reference names a model
// that is not present in the catalogue. Surfaced to clients as an HTTP 400
// (bad request) rather than a server error.
type ModelNotFoundError struct {
	Role      string // "primary" or "fallback"
	ModelName string
}

func (e *ModelNotFoundError) Error() string {
	return fmt.Sprintf("%s model %q does not exist", e.Role, e.ModelName)
}
