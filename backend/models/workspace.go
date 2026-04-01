package models

import "time"

// TriggerConfig describes a trigger for an automation.
type TriggerConfig struct {
	Type  string `yaml:"type"  json:"type"`  // "cron" | "interval" | "manual"
	Value string `yaml:"value" json:"value"` // "*/5 * * * *" | "15m" | ""
}

// Automation binds a Trigger to a Task file with an execution strategy.
type Automation struct {
	Name      string        `yaml:"name"      json:"name"`
	Trigger   TriggerConfig `yaml:"trigger"   json:"trigger"`
	TaskFile  string        `yaml:"task_file" json:"task_file"`
	Strategy  string        `yaml:"strategy"  json:"strategy"` // "isolated" | "persistent"
}

// WorkspaceConfig represents the metadata from workspaces/{id}/config.yaml
type WorkspaceConfig struct {
	// Legacy single cron schedule (for backward compatibility)
	CronSchedule string        `yaml:"cron_schedule" json:"cron_schedule"`
	Model        string        `yaml:"model" json:"model"`
	Temperature  float64       `yaml:"temperature" json:"temperature"`
	// New: automations array (N:M model)
	Automations []*Automation `yaml:"automations" json:"automations"`
}

// AgentState represents the execution history and state from workspaces/{id}/state.json
type AgentState struct {
	LastOutput string    `json:"last_output"`
	LastError  string    `json:"last_error"`
	NextRunAt  time.Time `json:"next_run_at"`
	IsRunning  bool      `json:"is_running"`
	LastPulse  time.Time `json:"last_pulse"` // For HEARTBEAT_OK suppression
}

// Workspace represents an entire workspace object
type Workspace struct {
	ID        string          `json:"id"`
	Config    WorkspaceConfig `json:"config"`
	State     AgentState      `json:"state"`
	Heartbeat string          `json:"heartbeat"`
}
