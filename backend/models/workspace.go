package models

import "time"

// WorkspaceConfig represents the metadata from workspaces/{id}/config.yaml
type WorkspaceConfig struct {
	CronSchedule string  `yaml:"cron_schedule" json:"cron_schedule"`
	Model        string  `yaml:"model" json:"model"`
	Temperature  float64 `yaml:"temperature" json:"temperature"`
}

// AgentState represents the execution history and state from workspaces/{id}/state.json
type AgentState struct {
	LastOutput       string    `json:"last_output"`
	LastError        string    `json:"last_error"`
	NextRunPredicted time.Time `json:"next_run_predicted"`
	IsRunning        bool      `json:"is_running"`
}

// Workspace represents an entire workspace object
type Workspace struct {
	ID        string          `json:"id"`
	Config    WorkspaceConfig `json:"config"`
	State     AgentState      `json:"state"`
	Heartbeat string          `json:"heartbeat"` // The prompt in heartbeat.md
}
