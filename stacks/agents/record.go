package agents

import "time"

// Kind says what an entry of the agent security record is. The strings are what the record file and the screen have always held.
type Kind string

const (
	// KindToolRefused is a tool call the guardrails refused (a path outside the workspace, a blocked command or address).
	KindToolRefused Kind = "tool_refused"
	// KindApprovalAsked is a tool call that waited for the operator's approval.
	KindApprovalAsked Kind = "approval_asked"
	// KindNewDestination is an automation fetching from a host it had not used before, after its learning week.
	KindNewDestination Kind = "new_destination"
	// KindSecretFound is a credential the nightly pass found in what an agent wrote.
	KindSecretFound Kind = "secret_found"
)

// Event is one entry of the record. It holds vetted plain tokens only: never arguments, a URL path, a query, an error text, a model's words
// or a secret.
type Event struct {
	At        time.Time `json:"at"`
	Kind      Kind      `json:"kind"`
	Workspace string    `json:"workspace"`
	// Automation names the automation of the run; empty for a chat.
	Automation string `json:"automation,omitempty"`
	// Tool is the tool's name (tool_refused, approval_asked); Host the destination's host name (new_destination); Where the
	// workspace-relative place of a secret (secret_found); Detail the kind of secret, in fixed words.
	Tool   string `json:"tool,omitempty"`
	Host   string `json:"host,omitempty"`
	Where  string `json:"where,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// Destinations are the hosts one automation fetched from, learned from its first fetch for a week. Many marks an automation that already
// reached many hosts while learning (a news or research job): its new hosts are recorded and never alerted.
type Destinations struct {
	Since time.Time `json:"since"`
	Hosts []string  `json:"hosts"`
	Many  bool      `json:"many,omitempty"`
}

// Record is the whole kept record, in the file `agent-watch.json`. Its keys are unchanged from before the stack existed, so a record written
// by an earlier version reads back as it was.
type Record struct {
	Events []Event `json:"events"`
	// Destinations is keyed by workspace and automation ("workspace/automation").
	Destinations map[string]Destinations `json:"destinations,omitempty"`
	// Alerted holds, per key, when the last alert for it was sent, so one automation raises at most one alert a day.
	Alerted map[string]time.Time `json:"alerted,omitempty"`
	// SecretsSeen holds the places the secrets pass already reported ("workspace/path|kind"), so each is reported once.
	SecretsSeen map[string]time.Time `json:"secrets_seen,omitempty"`
}
