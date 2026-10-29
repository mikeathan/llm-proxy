package models

// AppConfig is the single persisted configuration root (Tier 1+2 merged,
// written to settings.yml). It carries the full operator+user configuration in
// one hand-editable document: server, workspaces_dir, metrics, sandboxing,
// local, guardrails, model_overrides, memory, scheduler, run_logging.
type AppConfig struct {
	Server         AppServerConfig          `yaml:"server" json:"server"`
	WorkspacesDir  string                   `yaml:"workspaces_dir" json:"workspaces_dir"`
	Metrics        MetricsConfig            `yaml:"metrics" json:"metrics"`
	Sandboxing     HostSandboxingConfig     `yaml:"sandboxing" json:"sandboxing"`
	Local          LocalSettings            `yaml:"local" json:"local"`
	Guardrails     *AgentGuardrailsConfig   `yaml:"guardrails,omitempty" json:"guardrails,omitempty"`
	ModelOverrides map[string]ModelOverride `yaml:"model_overrides,omitempty" json:"model_overrides,omitempty"`
	Memory         *MemoryConfig            `yaml:"memory,omitempty" json:"memory,omitempty"`
	Scheduler      *SchedulerConfig         `yaml:"scheduler,omitempty" json:"scheduler,omitempty"`
	Reminders      *RemindersConfig         `yaml:"reminders,omitempty" json:"reminders,omitempty"`
	Agent          *AgentConfig             `yaml:"agent,omitempty" json:"agent,omitempty"`
	RunLogging     *RunLoggingConfig        `yaml:"run_logging,omitempty" json:"run_logging,omitempty"`
	Netaudit       *NetauditConfig          `yaml:"netaudit,omitempty" json:"netaudit,omitempty"`
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
	// AllowedHosts are names the admin interface may be reached under, besides IP addresses, localhost and this machine's
	// own name (a reverse proxy or a custom DNS name). Any other name is refused, which stops DNS rebinding.
	AllowedHosts []string `yaml:"allowed_hosts,omitempty" json:"allowed_hosts,omitempty"`
	// TrustedProxies are the addresses (10.0.0.5, ::1) or ranges (10.0.0.0/8) of reverse proxies in front of the server,
	// the only connections whose X-Forwarded-Proto is believed. Behind a TLS front end that lists it, the session cookie is
	// Secure and the sign-in page reports an encrypted connection. Empty (the default): no header is believed.
	TrustedProxies []string `yaml:"trusted_proxies,omitempty" json:"trusted_proxies,omitempty"`
	// Auth sets how long a sign-in lasts. Zero means the default, so an existing settings file needs no change.
	Auth AuthSettings `yaml:"auth,omitempty" json:"auth,omitempty"`
}

// AuthSettings are the lifetimes of a signed-in session (platform/auth bounds them: a session always ends).
type AuthSettings struct {
	// IdleHours: a session unused for this long ends. 0 = the default (24).
	IdleHours int `yaml:"idle_hours,omitempty" json:"idle_hours,omitempty"`
	// AbsoluteDays: no session lasts longer than this, however active. 0 = the default (14).
	AbsoluteDays int `yaml:"absolute_days,omitempty" json:"absolute_days,omitempty"`
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
		Memory:     new(DefaultMemoryConfig()),
		Scheduler:  new(DefaultSchedulerConfig()),
		RunLogging: new(DefaultRunLoggingConfig()),
	}
}
