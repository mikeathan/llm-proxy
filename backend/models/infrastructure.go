package models

const (
	AddrAllInterfaces     = "0.0.0.0"
	AddrLocalhost         = "127.0.0.1"
	DefaultAppPort        = "4001"
	DefaultModelPortStart = 8081
)

// SystemServerConfig is the infrastructure-level server section of
// SystemConfig. It embeds the persisted AppServerConfig (bind/host/limits/env)
// and surfaces the canonical top-level AppConfig RunLogging field as
// Server.RunLogging. A named type (not an inline anonymous struct) so
// projections and the model share one definition and cannot drift.
type SystemServerConfig struct {
	AppServerConfig
	RunLogging *RunLoggingConfig `json:"run_logging,omitempty"`
}

// SystemConfig represents the infrastructure-level settings (Tier 1: settings.yml)
type SystemConfig struct {
	Server SystemServerConfig `json:"server"`

	WorkspacesDir string        `json:"workspaces_dir"`
	Metrics       MetricsConfig `json:"metrics,omitempty"`
}

type LocalSettings struct {
	LlamaServerBinary string   `yaml:"llama_server_binary" json:"llama_server_binary"`
	ModelDir          string   `yaml:"model_dir" json:"model_dir"`
	DefaultArgs       []string `yaml:"default_args" json:"default_args"`
}

// ModelOverride stores per-model agent tuning fields that override
// the base values from the registry catalogue.
//
// Zero-value semantics: every field uses omitempty, so a zero value is never
// serialized.  This is what makes RESET work without nullable pointers — the
// handler writes the whole entry, and any field deliberately reset to zero
// (or left at its derived baseline) is simply dropped from the serialized
// entry on the next save.  Local workloads always write zero budget fields
// (MaxTokens/ContextBudget), so a stale persisted budget override is removed
// by writing the entry without those fields — explicit map-entry replacement,
// never zero-value guessing about deletion.
type ModelOverride struct {
	MaxSteps         int     `yaml:"max_steps,omitempty" json:"max_steps,omitempty"`
	ContextBudget    int     `yaml:"context_budget,omitempty" json:"context_budget,omitempty"`
	MaxTokens        int     `yaml:"max_tokens,omitempty" json:"max_tokens,omitempty"`
	Temperature      float64 `yaml:"temperature,omitempty" json:"temperature,omitempty"`
	ToolCallFormat   string  `yaml:"tool_call_format,omitempty" json:"tool_call_format,omitempty"`
	Prefill          *bool   `yaml:"prefill,omitempty" json:"prefill,omitempty"`
	ReasoningEnabled *bool   `yaml:"reasoning_enabled,omitempty" json:"reasoning_enabled,omitempty"`
	ReasoningBudget  int     `yaml:"reasoning_budget,omitempty" json:"reasoning_budget,omitempty"`
	SlotTimeout      int     `yaml:"slot_timeout,omitempty" json:"slot_timeout,omitempty"`
	ICUWeight        float64 `yaml:"icu_weight,omitempty" json:"icu_weight,omitempty"`
	TimeoutMinutes   int     `yaml:"timeout_minutes,omitempty" json:"timeout_minutes,omitempty"`

	ToolTimeoutSeconds           int          `yaml:"tool_timeout_seconds,omitempty" json:"tool_timeout_seconds,omitempty"`
	FilesystemToolTimeoutSeconds int          `yaml:"filesystem_tool_timeout_seconds,omitempty" json:"filesystem_tool_timeout_seconds,omitempty"`
	MaxPlanDurationMinutes       int          `yaml:"max_plan_duration_minutes,omitempty" json:"max_plan_duration_minutes,omitempty"`
	MaxPlanSteps                 int          `yaml:"max_plan_steps,omitempty" json:"max_plan_steps,omitempty"`
	GuardrailTimeoutSeconds      int          `yaml:"guardrail_timeout_seconds,omitempty" json:"guardrail_timeout_seconds,omitempty"`
	GuardrailTimeoutBehavior     string       `yaml:"guardrail_timeout_behavior,omitempty" json:"guardrail_timeout_behavior,omitempty"`
	GuardrailApprovalTimeoutSecs int          `yaml:"guardrail_approval_timeout_seconds,omitempty" json:"guardrail_approval_timeout_seconds,omitempty"`
	LoopStrategy                 LoopStrategy `yaml:"loop_strategy,omitempty" json:"loop_strategy,omitempty"`
}

// UserSettings represents the user-level settings (Tier 2: settings.yml)
type UserSettings struct {
	Local          LocalSettings            `yaml:"local" json:"local"`
	Guardrails     *AgentGuardrailsConfig   `yaml:"guardrails,omitempty" json:"guardrails,omitempty"`
	ModelOverrides map[string]ModelOverride `yaml:"model_overrides,omitempty" json:"model_overrides,omitempty"`
	Memory         *MemoryConfig            `yaml:"memory,omitempty" json:"memory,omitempty"`
	RunOutput      *RunLoggingConfig        `yaml:"run_logging,omitempty" json:"run_logging,omitempty"`
}

type RunLoggingConfig struct {
	Enabled bool `yaml:"enabled" json:"enabled"`
}

// DefaultRunLoggingConfig returns the shipped first-run default. Run logging
// is enabled by default to match the historical shipped config.json
// (run_logging.enabled = true) that predates the single-root config relocation;
// fresh installs must keep producing per-run events.jsonl and final reports.
func DefaultRunLoggingConfig() RunLoggingConfig {
	return RunLoggingConfig{Enabled: true}
}

type MemoryConfig struct {
	Enabled        bool    `yaml:"enabled" json:"enabled"`
	SearchTopK     int     `yaml:"search_top_k,omitempty" json:"search_top_k,omitempty"`
	FlushThreshold float64 `yaml:"flush_threshold,omitempty" json:"flush_threshold,omitempty"`
	RetentionDays  int     `yaml:"retention_days,omitempty" json:"retention_days,omitempty"`
}

func DefaultMemoryConfig() MemoryConfig {
	return MemoryConfig{
		Enabled:        true,
		SearchTopK:     5,
		FlushThreshold: 0.7,
		RetentionDays:  90,
	}
}

// SystemUpdatePayload represents a unified request to update system, registry, and environment settings.
type SystemUpdatePayload struct {
	WorkspacesDir string `json:"workspaces_dir,omitempty"`
	ModelHost     string `json:"model_host,omitempty"`
	// IdleTimeoutSecs is a pointer so an explicit 0/-1 ("never stop the model
	// automatically") can be distinguished from "not provided". -1 (or any
	// value <= 0) means never reap — the only form that survives the
	// defaults-merge on reload (the merge treats 0 as "unset").
	IdleTimeoutSecs      *int                    `json:"idle_timeout_seconds,omitempty"`
	GPUProvider          string                  `json:"gpu_provider,omitempty"`
	GPUBinary            string                  `json:"gpu_binary,omitempty"`
	GPUIndex             *int                    `json:"gpu_index,omitempty"`
	GPUSysfsPath         string                  `json:"gpu_sysfs_path,omitempty"`
	GPUSampleIntervalSec int                     `json:"gpu_sample_interval_seconds,omitempty"`
	GPUSmoothingAlpha    float64                 `json:"gpu_smoothing_alpha,omitempty"`
	ServiceClientID      string                  `json:"service_client_id,omitempty"`
	ServiceClientSecret  string                  `json:"service_client_secret,omitempty"`
	Environment          map[string]string       `json:"environment,omitempty"`
	DefaultArgs          []string                `json:"default_args,omitempty"`
	PrimaryModel         string                  `json:"primary_model,omitempty"`
	FallbackModel        string                  `json:"fallback_model,omitempty"`
	Communication        *CommunicationConfig    `json:"communication,omitempty"`
	Search               *SearchConfig           `json:"search,omitempty"`
	Providers            map[string]ProviderItem `json:"providers,omitempty"`
	Guardrails           *AgentGuardrailsConfig  `json:"guardrails,omitempty"`
	Bind                 string                  `json:"bind,omitempty"` // For AdminSystemHandler specifically
	RunLogging           *RunLoggingConfig       `json:"run_logging,omitempty"`
}
