package models

// AppConfig is the single persisted configuration root (Tier 1+2 merged,
// written to settings.yml). It carries the full operator+user configuration in
// one hand-editable document: server, workspaces_dir, metrics, sandboxing,
// local, guardrails, model_overrides, memory, run_logging.
type AppConfig struct {
	Server         AppServerConfig          `yaml:"server" json:"server"`
	WorkspacesDir  string                   `yaml:"workspaces_dir" json:"workspaces_dir"`
	Metrics        MetricsConfig            `yaml:"metrics" json:"metrics"`
	Sandboxing     HostSandboxingConfig     `yaml:"sandboxing" json:"sandboxing"`
	Local          LocalSettings            `yaml:"local" json:"local"`
	Guardrails     *AgentGuardrailsConfig   `yaml:"guardrails,omitempty" json:"guardrails,omitempty"`
	ModelOverrides map[string]ModelOverride `yaml:"model_overrides,omitempty" json:"model_overrides,omitempty"`
	Memory         *MemoryConfig            `yaml:"memory,omitempty" json:"memory,omitempty"`
	RunLogging     *RunLoggingConfig        `yaml:"run_logging,omitempty" json:"run_logging,omitempty"`
}

// AppServerConfig is the infrastructure-level server section of AppConfig
// (the former SystemConfig.Server, minus run_logging which is a canonical
// top-level AppConfig field).
type AppServerConfig struct {
	Bind            string            `yaml:"bind" json:"bind"`
	ModelHost       string            `yaml:"model_host" json:"model_host"`
	IdleTimeoutSecs int               `yaml:"idle_timeout_seconds" json:"idle_timeout_seconds"`
	Environment     map[string]string `yaml:"environment,omitempty" json:"environment,omitempty"`
	LogLevel        string            `yaml:"log_level,omitempty" json:"log_level,omitempty"`
}

// DefaultAppConfig returns the canonical first-run configuration. It mirrors
// the historical defaults: bind all interfaces on :4001, metrics GPU auto, and
// the shipped host/memory/run-logging defaults. workspaces_dir is left empty so
// the "unset → default location" rule applies.
func DefaultAppConfig() AppConfig {
	return AppConfig{
		Server: AppServerConfig{
			Bind:            AddrAllInterfaces + ":" + DefaultAppPort,
			ModelHost:       AddrAllInterfaces,
			IdleTimeoutSecs: 1800,
		},
		// Metrics defaults are baked in here so the GPU gauge "just works"
		// without any UI configuration: the background sampler runs at 10s and
		// the display uses the default EMA smoothing (0.3). The sample-interval
		// and smoothing UI knobs were removed 2026-08-28; operators can still
		// override via settings.yml.
		Metrics:    MetricsConfig{GPU: GPUConfig{Provider: "auto"}, GPUSampleIntervalSec: 10, GPUSmoothingAlpha: 0.3},
		Sandboxing: DefaultHostSettings().Sandboxing,
		Memory:     ptr(DefaultMemoryConfig()),
		RunLogging: ptr(DefaultRunLoggingConfig()),
	}
}

func ptr[T any](v T) *T {
	return &v
}
