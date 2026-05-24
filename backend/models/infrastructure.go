package models

const (
	AddrAllInterfaces = "0.0.0.0"
	AddrLocalhost     = "127.0.0.1"
	DefaultAppPort    = "4001"
	DefaultModelPortStart = 8081
)

// SystemConfig represents the infrastructure-level settings (Tier 1: config.json)
type SystemConfig struct {
	Server struct {
		Bind            string            `json:"bind"`
		ModelHost       string            `json:"model_host"`
		IdleTimeoutSecs int               `json:"idle_timeout_seconds"`
		Environment     map[string]string `json:"environment,omitempty"`
		LogLevel        string            `json:"log_level,omitempty"`
	} `json:"server"`

	WorkspacesDir string `json:"workspaces_dir"`
	Metrics       MetricsConfig `json:"metrics,omitempty"`
}

type LocalSettings struct {
	LlamaServerBinary string   `yaml:"llama_server_binary" json:"llama_server_binary"`
	ModelDir          string   `yaml:"model_dir" json:"model_dir"`
	DefaultArgs       []string `yaml:"default_args" json:"default_args"`
}

// ModelOverride stores per-model agent tuning fields that override
// the base values from the registry catalogue.
type ModelOverride struct {
	MaxSteps        int     `yaml:"max_steps,omitempty" json:"max_steps,omitempty"`
	ContextBudget   int     `yaml:"context_budget,omitempty" json:"context_budget,omitempty"`
	MaxTokens       int     `yaml:"max_tokens,omitempty" json:"max_tokens,omitempty"`
	ToolCallFormat  string  `yaml:"tool_call_format,omitempty" json:"tool_call_format,omitempty"`
	Prefill         *bool   `yaml:"prefill,omitempty" json:"prefill,omitempty"`
	ReasoningBudget int     `yaml:"reasoning_budget,omitempty" json:"reasoning_budget,omitempty"`
	SlotTimeout     int     `yaml:"slot_timeout,omitempty" json:"slot_timeout,omitempty"`
	ICUWeight       float64 `yaml:"icu_weight,omitempty" json:"icu_weight,omitempty"`
	TimeoutMinutes  int     `yaml:"timeout_minutes,omitempty" json:"timeout_minutes,omitempty"`
}

// UserSettings represents the user-level settings (Tier 2: settings.yml)
type UserSettings struct {
	Local          LocalSettings               `yaml:"local" json:"local"`
	Guardrails     *AgentGuardrailsConfig      `yaml:"guardrails,omitempty" json:"guardrails,omitempty"`
	ModelOverrides map[string]ModelOverride    `yaml:"model_overrides,omitempty" json:"model_overrides,omitempty"`
}

// SystemUpdatePayload represents a unified request to update system, registry, and environment settings.
type SystemUpdatePayload struct {
	WorkspacesDir       string                         `json:"workspaces_dir,omitempty"`
	ModelHost           string                         `json:"model_host,omitempty"`
	IdleTimeoutSecs     int                            `json:"idle_timeout_seconds,omitempty"`
	GPUProvider         string                         `json:"gpu_provider,omitempty"`
	GPUBinary           string                         `json:"gpu_binary,omitempty"`
	GPUIndex            *int                           `json:"gpu_index,omitempty"`
	GPUSysfsPath        string                         `json:"gpu_sysfs_path,omitempty"`
	ServiceClientID     string                         `json:"service_client_id,omitempty"`
	ServiceClientSecret string                         `json:"service_client_secret,omitempty"`
	Environment         map[string]string              `json:"environment,omitempty"`
	DefaultArgs         []string                       `json:"default_args,omitempty"`
	PrimaryModel        string                         `json:"primary_model,omitempty"`
	FallbackModel       string                         `json:"fallback_model,omitempty"`
	Communication       *CommunicationConfig           `json:"communication,omitempty"`
	Search              *SearchConfig                  `json:"search,omitempty"`
	Providers           map[string]ProviderItem        `json:"providers,omitempty"`
	Guardrails          *AgentGuardrailsConfig         `json:"guardrails,omitempty"`
	Bind                string                         `json:"bind,omitempty"` // For AdminSystemHandler specifically
}

