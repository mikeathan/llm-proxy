package models

import "time"

type TriggerType string

const (
	TriggerCron     TriggerType = "cron"
	TriggerInterval TriggerType = "interval"
	TriggerManual   TriggerType = "manual"
)

// Workspace File and Directory Constants 
const (
	ConfigFilename     = "config.yaml"
	StateFilename      = "state.json"
	HeartbeatFilename  = "heartbeat.md"
	AgentPromptFilename = "agent.md"
	RulesFilename      = "rules.md"
	InternalDirName    = ".internal"
	LockFilename       = ".lock"
	WorkspacesDirName  = "workspaces"

	// System root files
	SystemConfigFilename = "config.json"
	SecretsFilename      = "secrets.json"
	RegistryFilename     = "registry.json"
	ProcessLogFilename   = "process.log"

	// API Parameter Names
	WorkspaceIDParam = "workspace"
)

// TriggerConfig describes a trigger for an automation.
type TriggerConfig struct {
	Type  TriggerType `yaml:"type"  json:"type"`  // "cron" | "interval" | "manual"
	Value string `yaml:"value" json:"value"` // "*/5 * * * *" | "15m" | ""
}

type Automation struct {
	Name     string        `yaml:"name"      json:"name"`
	Trigger  TriggerConfig `yaml:"trigger"   json:"trigger"`
	TaskFile string        `yaml:"task_file" json:"task_file"`
	Strategy string        `yaml:"strategy"  json:"strategy"` // "isolated" | "persistent"
	Model    string        `yaml:"model,omitempty" json:"model,omitempty"` // Model override for this automation
}

// WorkspaceConfig represents the metadata from workspaces/{id}/config.yaml
type WorkspaceConfig struct {
	// Legacy single cron schedule (for backward compatibility)
	CronSchedule string  `yaml:"cron_schedule" json:"cron_schedule"`
	Model        string  `yaml:"model" json:"model"`
	Temperature  float64 `yaml:"temperature" json:"temperature"`
	// automations array (N:M model)
	Automations []*Automation `yaml:"automations" json:"automations"`
	// per-workspace guardrail overrides
	Guardrails *AgentGuardrailsConfig `yaml:"guardrails,omitempty" json:"guardrails,omitempty"`
}

// AutomationRun represents a single execution of an automation.
type AutomationRun struct {
	ID             string    `json:"id"`
	WorkspaceID    string    `json:"workspace_id"`
	AutomationName string    `json:"automation_name"`
	Timestamp      time.Time `json:"timestamp"`
	Output         string    `json:"output"`
	Error          string    `json:"error"`
	DurationMs     int64     `json:"duration_ms"`
	Model          string    `json:"model"`
	Events         []any     `json:"events"` // Full event log for "Live Console" reconstruction
}

// AgentState represents the execution history and state from workspaces/{id}/state.json
type AgentState struct {
	LastOutput string    `json:"last_output"`
	LastError  string    `json:"last_error"`
	NextRunAt  time.Time `json:"next_run_at"`
	IsRunning  bool      `json:"is_running"`
	LastPulse  time.Time `json:"last_pulse"` // For HEARTBEAT_OK suppression

	// History and per-automation state
	History  []AutomationRun           `json:"history"`
	LastRuns map[string]*AutomationRun `json:"last_runs"`
}

// Workspace represents an entire workspace object
type Workspace struct {
	ID        string          `json:"id"`
	Config    WorkspaceConfig `json:"config"`
	State     AgentState      `json:"state"`
	Heartbeat string          `json:"heartbeat"`
}
