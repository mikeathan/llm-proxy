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
	ConfigFilename    = "config.yaml"
	StateFilename     = "state.json"
	HeartbeatFilename = "heartbeat.md"
	RulesFilename     = "AGENTS.md"
	InternalDirName   = ".internal"
	LockFilename      = ".lock"
	WorkspacesDirName = "workspaces"

	// System root files
	SystemConfigFilename = "config.json"
	SecretsFilename      = "secrets.json"
	RegistryFilename     = "registry.json"
	SettingsFilename     = "settings.yml"
	ProcessLogFilename   = "process.log"

	// API Parameter Names
	WorkspaceIDParam = "workspace"

	// Sandbox execution paths
	SandboxTmpDir = "/tmp"
	SandboxRunDir = "/run"
)

// TreeEntryType is the kind of a workspace tree entry.
type TreeEntryType string

const (
	TreeEntryFile TreeEntryType = "file"
	TreeEntryDir  TreeEntryType = "dir"
)

// TreeEntry is one path in a workspace file tree (GET …/workspaces/{ws}/tree).
// Path is workspace-root-relative and slash-separated. Collapsed marks a heavy
// directory (node_modules, .venv, …) that is listed but not descended into.
type TreeEntry struct {
	Path      string        `json:"path"`
	Type      TreeEntryType `json:"type"`
	Collapsed bool          `json:"collapsed,omitempty"`
}

// WorkspaceTree is a bounded recursive listing of a workspace, sorted by path.
// Truncated reports that the entry cap cut off deeper entries.
type WorkspaceTree struct {
	Entries   []TreeEntry `json:"entries"`
	Truncated bool        `json:"truncated"`
}

// TriggerConfig describes a trigger for an automation.
type TriggerConfig struct {
	Type  TriggerType `yaml:"type"  json:"type"`  // "cron" | "interval" | "manual"
	Value string      `yaml:"value" json:"value"` // "*/5 * * * *" | "15m" | ""
}

// NetworkScope is the per-run agent network scope (plan D1/D8 grant model).
// The zero value "" means inherit — the run follows the workspace (L1) scope
// instead of overriding it. Explicit values name the reachable network:
// none (off), lan (LAN only), internet_only (internet, no LAN), internet (LAN
// + internet). The host (L0) sandboxing.network switch remains the ceiling above
// all of them.
type NetworkScope string

const (
	NetworkScopeInherit      NetworkScope = ""              // default: follow the workspace L1 scope
	NetworkScopeNone         NetworkScope = "none"          // no agent egress
	NetworkScopeLan          NetworkScope = "lan"           // local network only
	NetworkScopeInternet     NetworkScope = "internet"      // local network + internet
	NetworkScopeInternetOnly NetworkScope = "internet_only" // internet, local network blocked
)

// Valid reports whether s is a known scope value (empty = inherit is valid).
func (s NetworkScope) Valid() bool {
	switch s {
	case NetworkScopeInherit, NetworkScopeNone, NetworkScopeLan, NetworkScopeInternet, NetworkScopeInternetOnly:
		return true
	}
	return false
}

// NetworkOn reports whether a RESOLVED scope permits any agent egress. It is
// fail-safe: only explicit lan/internet/internet_only return true; none AND
// unresolved inherit/unknown return false, so an accidentally-unresolved grant
// can never open the network. The OS shell pool key in the plan (D8) derives
// from this. Note the shell cannot split LAN from internet (D8) — internet_only
// blocks the in-process LAN tools, not shell sockets.
func (s NetworkScope) NetworkOn() bool {
	return s == NetworkScopeLan || s == NetworkScopeInternet || s == NetworkScopeInternetOnly
}

type Automation struct {
	Name         string        `yaml:"name"          json:"name"`
	Trigger      TriggerConfig `yaml:"trigger"       json:"trigger"`
	TaskFile     string        `yaml:"task_file"     json:"task_file"`
	Strategy     string        `yaml:"strategy"      json:"strategy"`                          // "isolated" | "persistent"
	Model        string        `yaml:"model,omitempty"  json:"model,omitempty"`                // Model override for this automation
	LoopStrategy LoopStrategy  `yaml:"loop_strategy,omitempty" json:"loop_strategy,omitempty"` // per-run loop archetype override; "" = model config default
	AllowedTools []string      `yaml:"allowed_tools,omitempty" json:"allowed_tools,omitempty"` // restrict tools for unattended runs
	RecordingRef string        `yaml:"recording_ref,omitempty" json:"recording_ref,omitempty"` // Recording file ID for playback
	// NetworkGrant overrides the workspace network scope for THIS automation's
	// runs only. Empty (default) = inherit the workspace L1 scope; explicit
	// none/lan/internet_only/internet tighten or loosen for this automation. The
	// host L0 switch remains the ceiling (plan §4.4 grant model).
	NetworkGrant NetworkScope `yaml:"network_grant,omitempty" json:"network_grant,omitempty"`
	// MemoryMode opts this automation into hot-memory injection. Empty/off (the
	// default) leaves the run without memory; hot injects the workspace's hot
	// entries once per run in the head system message.
	MemoryMode MemoryMode `yaml:"memory_mode,omitempty" json:"memory_mode,omitempty"`
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
	RecordingRef   string    `json:"recording_ref"`
	// RunDirName is the last path segment of the on-disk run directory
	// (data/runs/{workspace}/{model}/{automation}/{RunDirName}), used by the UI
	// to prune individual run artifacts. Empty when run logging is disabled.
	RunDirName string `json:"run_dir_name,omitempty"`
	Events     []any  `json:"events"` // Full event log for "Live Console" reconstruction
	// Warnings records non-fatal tool failures (e.g. a delivery connector being
	// down). A run with warnings but an empty Error is still a success — the
	// work product exists, only delivery was impaired (tool-error-classification).
	Warnings []string `json:"warnings,omitempty"`
}

// AgentState represents the execution history and state from workspaces/{id}/state.json
type AgentState struct {
	NextRunAt        time.Time `json:"next_run_at"`
	ActiveAutomation string    `json:"active_automation,omitempty"`
	LastPulse        time.Time `json:"last_pulse"` // For HEARTBEAT_OK suppression

	// History and per-automation state
	History  []AutomationRun           `json:"history"`
	LastRuns map[string]*AutomationRun `json:"last_runs"`
}

// IsRunning returns true if an automation is currently active.
func (s *AgentState) IsRunning() bool {
	return s.ActiveAutomation != ""
}

// SetRunning updates the active automation and ensures the state is consistent.
// Pass an empty string to clear the active status.
func (s *AgentState) SetRunning(name string) {
	s.ActiveAutomation = name
}

// MaxStateHistory caps AgentState.History so state.json stays small.
const MaxStateHistory = 30

// AppendRun records a finished run: it joins the capped history and becomes the
// automation's latest run (what the UI's "Last result" reads).
func (s *AgentState) AppendRun(run AutomationRun) {
	s.History = append(s.History, run)
	if len(s.History) > MaxStateHistory {
		s.History = s.History[len(s.History)-MaxStateHistory:]
	}
	if s.LastRuns == nil {
		s.LastRuns = make(map[string]*AutomationRun)
	}
	s.LastRuns[run.AutomationName] = &run
}

// Workspace represents an entire workspace object
type Workspace struct {
	ID        string          `json:"id"`
	Config    WorkspaceConfig `json:"config"`
	State     AgentState      `json:"state"`
	Heartbeat string          `json:"heartbeat"`
}

type TerminalSessionView struct {
	WorkspaceID string    `json:"workspace_id"`
	LastUsed    time.Time `json:"last_used"`
	HostPath    string    `json:"host_path"`
	NetworkOn   bool      `json:"network_on"` // D8: which pool key this session serves
}
