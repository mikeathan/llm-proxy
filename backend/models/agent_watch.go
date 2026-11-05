package models

import "time"

// AgentWatchKind says what an entry of the agent security record is.
type AgentWatchKind string

const (
	// AgentToolRefused is a tool call the guardrails refused (a path outside the workspace, a blocked command or address).
	AgentToolRefused AgentWatchKind = "tool_refused"
	// AgentApprovalAsked is a tool call that waited for the operator's approval.
	AgentApprovalAsked AgentWatchKind = "approval_asked"
	// AgentNewDestination is an automation fetching from a host it had not used before, after its learning week.
	AgentNewDestination AgentWatchKind = "new_destination"
	// AgentSecretFound is a credential the nightly pass found in what an agent wrote (a workspace file, a report, a memory).
	AgentSecretFound AgentWatchKind = "secret_found"
)

// Valid reports whether k is one of the known kinds.
func (k AgentWatchKind) Valid() bool {
	switch k {
	case AgentToolRefused, AgentApprovalAsked, AgentNewDestination, AgentSecretFound:
		return true
	}
	return false
}

// AgentWatchEvent is one entry of the record. It holds vetted plain tokens only: never arguments, a URL path, a query, an
// error text, a model's words or a secret.
type AgentWatchEvent struct {
	At        time.Time      `json:"at"`
	Kind      AgentWatchKind `json:"kind"`
	Workspace string         `json:"workspace"`
	// Automation names the automation of the run; empty for a chat.
	Automation string `json:"automation,omitempty"`
	// Tool is the tool's name (tool_refused, approval_asked); Host the destination's host name (new_destination); Where the
	// workspace-relative place of a secret, or "memory" (secret_found); Detail the kind of secret, in fixed words.
	Tool   string `json:"tool,omitempty"`
	Host   string `json:"host,omitempty"`
	Where  string `json:"where,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// AgentDestinations are the hosts one automation fetched from, learned from its first fetch for a week. Many marks an automation
// that already reached many hosts while learning (a news or research job): its new hosts are recorded and never alerted.
type AgentDestinations struct {
	Since time.Time `json:"since"`
	Hosts []string  `json:"hosts"`
	Many  bool      `json:"many,omitempty"`
}

// AgentWatch is the whole kept record.
type AgentWatch struct {
	Events []AgentWatchEvent `json:"events"`
	// Destinations is keyed by workspace and automation ("workspace/automation").
	Destinations map[string]AgentDestinations `json:"destinations,omitempty"`
	// Alerted holds, per key, when the last alert for it was sent, so one automation raises at most one alert a day.
	Alerted map[string]time.Time `json:"alerted,omitempty"`
	// SecretsSeen holds the places the secrets pass already reported ("workspace/path|kind"), so each is reported once.
	SecretsSeen map[string]time.Time `json:"secrets_seen,omitempty"`
}
