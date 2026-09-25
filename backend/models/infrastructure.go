package models

import (
	"errors"
	"fmt"
)

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
	Scheduler      *SchedulerConfig         `yaml:"scheduler,omitempty" json:"scheduler,omitempty"`
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

// SchedulerConfig holds the run-scheduler knobs: per-workload-class
// concurrency limits, whether interactive claims may preempt running
// automations, and how external /v1 callers are admitted to the local model.
//
// The inbound fields are pointers because a settings.yml that already has a
// `scheduler:` block is not backfilled per field (only the whole block is) — a
// plain int would read as "explicitly zero" when it was simply absent. The two
// concurrency limits are plain ints because zero is not a meaningful value for
// them: Validate rejects it and the runtime clamps to 1.
type SchedulerConfig struct {
	LocalConcurrency   int   `yaml:"local_concurrency,omitempty" json:"local_concurrency,omitempty"`
	CloudConcurrency   int   `yaml:"cloud_concurrency,omitempty" json:"cloud_concurrency,omitempty"`
	PreemptAutomations *bool `yaml:"preempt_automations,omitempty" json:"preempt_automations,omitempty"`

	// InboundWaitSeconds is how long an external /v1 caller may wait for the
	// local model: the cap on a wait it requests with X-Queue-Wait, and the
	// park duration for a header-less caller when InboundWaitByDefault is on.
	// 0 refuses immediately; -1 waits until served, cancelled, or the operator
	// acts. Unlimited is bounded in practice by InboundMaxQueued, never by
	// memory.
	InboundWaitSeconds *int `yaml:"inbound_wait_seconds,omitempty" json:"inbound_wait_seconds,omitempty"`
	// InboundWaitByDefault parks a caller that sends no X-Queue-Wait header
	// (for up to InboundWaitSeconds) instead of refusing it. Off by default:
	// an unsolicited caller is refused with a retry hint rather than having a
	// connection held on its behalf. Parked callers are visible to the operator
	// (run-activity panel) and can cancel out of band.
	InboundWaitByDefault *bool `yaml:"inbound_wait_by_default,omitempty" json:"inbound_wait_by_default,omitempty"`
	// InboundMaxQueued bounds how many callers may wait at once. Always
	// enforced — it is what keeps an unlimited wait from becoming unbounded
	// held connections.
	InboundMaxQueued *int `yaml:"inbound_max_queued,omitempty" json:"inbound_max_queued,omitempty"`
	// InboundPreempt lets a contended inbound request serve by cancelling the
	// run holding the model. Off by default: clients may only ever ask to wait,
	// and promotion stays an operator action.
	InboundPreempt *bool `yaml:"inbound_preempt,omitempty" json:"inbound_preempt,omitempty"`
}

// minSchedulerConcurrency is the lowest limit that can still admit a run.
const minSchedulerConcurrency = 1

// Inbound-admission bounds. Unlimited waiting is expressible; a nonsensical wait
// and an empty queue are not.
const (
	minInboundWaitSeconds = -1
	minInboundMaxQueued   = 1
	defaultInboundWait    = 60
	defaultInboundQueued  = 32
)

// ErrSchedulerConcurrencyBelowMinimum is the save-boundary validation failure
// for SchedulerConfig concurrency limits. IsSchedulerConfigError classifies it
// so transport handlers map the whole class to a 400 in one predicate.
var ErrSchedulerConcurrencyBelowMinimum = errors.New("scheduler concurrency must be >= 1")

// ErrSchedulerInboundInvalid is the save-boundary validation failure for the
// inbound-admission knobs, classified the same way.
var ErrSchedulerInboundInvalid = errors.New("invalid inbound admission settings")

// DefaultSchedulerConfig returns the shipped first-run defaults: local runs
// serialize (one GPU / one llama.cpp slot), cloud runs parallelize, chat
// preempts automations, and an external caller is refused with a retry hint
// unless it asks to wait (X-Queue-Wait) or the host enables parking
// (queue depth 32, no client-side preemption).
func DefaultSchedulerConfig() SchedulerConfig {
	return SchedulerConfig{
		LocalConcurrency:     1,
		CloudConcurrency:     3,
		PreemptAutomations:   new(true),
		InboundWaitSeconds:   new(defaultInboundWait),
		InboundWaitByDefault: new(false),
		InboundMaxQueued:     new(defaultInboundQueued),
		InboundPreempt:       new(false),
	}
}

// InboundWait returns the effective cap on how long an external caller may wait
// for the local model: 0 refuses immediately, -1 is unbounded.
func (c SchedulerConfig) InboundWait() int {
	if c.InboundWaitSeconds != nil {
		return *c.InboundWaitSeconds
	}
	return defaultInboundWait
}

// InboundQueueDepth returns the effective bound on how many callers may wait.
func (c SchedulerConfig) InboundQueueDepth() int {
	if c.InboundMaxQueued != nil {
		return *c.InboundMaxQueued
	}
	return defaultInboundQueued
}

// InboundMayPark reports whether a caller that sends no X-Queue-Wait header
// is parked for InboundWaitSeconds instead of refused. Off unless explicitly
// enabled: unsolicited connections are never held by default.
func (c SchedulerConfig) InboundMayPark() bool {
	return c.InboundWaitByDefault != nil && *c.InboundWaitByDefault
}

// InboundMayPreempt reports whether a contended inbound request may serve by
// cancelling the run holding the model. Off unless explicitly enabled: clients
// may only ever ask to wait, and promotion otherwise stays an operator action.
func (c SchedulerConfig) InboundMayPreempt() bool {
	return c.InboundPreempt != nil && *c.InboundPreempt
}

// Validate rejects limits that could never admit a run, or inbound settings that
// could never admit a caller.
func (c SchedulerConfig) Validate() error {
	if c.LocalConcurrency < minSchedulerConcurrency {
		return fmt.Errorf("%w: local_concurrency %d", ErrSchedulerConcurrencyBelowMinimum, c.LocalConcurrency)
	}
	if c.CloudConcurrency < minSchedulerConcurrency {
		return fmt.Errorf("%w: cloud_concurrency %d", ErrSchedulerConcurrencyBelowMinimum, c.CloudConcurrency)
	}
	if c.InboundWaitSeconds != nil && *c.InboundWaitSeconds < minInboundWaitSeconds {
		return fmt.Errorf("%w: inbound_wait_seconds %d (-1 is the minimum: unlimited)", ErrSchedulerInboundInvalid, *c.InboundWaitSeconds)
	}
	if c.InboundMaxQueued != nil && *c.InboundMaxQueued < minInboundMaxQueued {
		return fmt.Errorf("%w: inbound_max_queued %d (at least %d)", ErrSchedulerInboundInvalid, *c.InboundMaxQueued, minInboundMaxQueued)
	}
	return nil
}

// IsSchedulerConfigError reports whether err is a scheduler-config validation
// failure.
func IsSchedulerConfigError(err error) bool {
	return errors.Is(err, ErrSchedulerConcurrencyBelowMinimum) || errors.Is(err, ErrSchedulerInboundInvalid)
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
	Scheduler            *SchedulerConfig        `json:"scheduler,omitempty"`
}
