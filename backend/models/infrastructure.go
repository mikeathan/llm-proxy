package models

// SystemConfig represents the infrastructure-level settings (Tier 1: config.json)
type SystemConfig struct {
	Server struct {
		Bind            string `json:"bind"`
		ModelHost       string `json:"model_host"`
		IdleTimeoutSecs int    `json:"idle_timeout_seconds"`
		PrimaryModel    string            `json:"primary_model,omitempty"`
		FallbackModel   string            `json:"fallback_model,omitempty"`
		Environment     map[string]string `json:"environment,omitempty"`
	} `json:"server"`

	// Local Infrastructure settings
	Local struct {
		LlamaServerBinary string   `json:"llama_server_binary"`
		ModelDir          string   `json:"model_dir"`
		DefaultArgs       []string `json:"default_args"`
	} `json:"local"`

	WorkspacesDir string `json:"workspaces_dir"`
	Communication CommunicationConfig `json:"communication"`
	Search        SearchConfig        `json:"search"`
}

// SystemUpdatePayload represents a unified request to update system, registry, and environment settings.
type SystemUpdatePayload struct {
	WorkspacesDir       string                         `json:"workspaces_dir,omitempty"`
	ModelHost           string                         `json:"model_host,omitempty"`
	IdleTimeoutSecs     int                            `json:"idle_timeout_seconds,omitempty"`
	GPUProvider         string                         `json:"gpu_provider,omitempty"`
	GPUBinary           string                         `json:"gpu_binary,omitempty"`
	GPUIndex            *int                           `json:"gpu_index,omitempty"`
	ServiceClientID     string                         `json:"service_client_id,omitempty"`
	ServiceClientSecret string                         `json:"service_client_secret,omitempty"`
	Environment         map[string]string              `json:"environment,omitempty"`
	DefaultArgs         []string                       `json:"default_args,omitempty"`
	PrimaryModel        string                         `json:"primary_model,omitempty"`
	FallbackModel       string                         `json:"fallback_model,omitempty"`
	Providers           map[string]ProviderItem        `json:"providers,omitempty"`
	Guardrails          *AgentGuardrailsConfig         `json:"guardrails,omitempty"`
	Bind                string                         `json:"bind,omitempty"` // For AdminSystemHandler specifically
}

