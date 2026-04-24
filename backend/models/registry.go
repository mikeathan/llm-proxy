package models

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
	ID           string `json:"id"`
	Name         string `json:"name"`
	ProviderID   string `json:"provider_id"`
	ModelID      string `json:"model_id"`
	CredentialID string `json:"credential_id,omitempty"`
}

type MCPServerRegistryEntry struct {
	Name      string `json:"name"`
	URL       string `json:"url"`
	Enabled   bool   `json:"enabled"`
	TLSCACert string `json:"tls_ca_cert,omitempty"`
}
