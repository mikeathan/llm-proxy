// subrun.go — the port a run delegates through. The agent owns the delegate tool (its schema, its arguments, the
// parent's events and result cap); a SubRunner, injected at bootstrap, runs each task as a child run on a cloud model
// and returns a summary. The child never comes back through this port, so delegation is one level deep.
package assistant

import "context"

// SubRunner runs one delegated task as a sub-agent.
type SubRunner interface {
	// Enabled reports whether delegation is set up (a cloud model is named); the delegate tool is offered only then.
	Enabled() bool
	// Run runs the task to the end and reports how it went; it never returns an error, a failure is a status.
	Run(ctx context.Context, req SubRunRequest) SubRunResult
}

// SubRunRequest is one task for one helper profile.
type SubRunRequest struct {
	WorkspaceID string
	Agent       string // the helper profile id, one the parent's profile lists in delegates
	Task        string
	Context     string
	// Emit reports the child's progress to the parent's event stream; the parent serialises calls.
	Emit func(SubagentEvent)
}

// SubRunStatus is how a delegated task ended.
type SubRunStatus string

const (
	SubRunDone    SubRunStatus = "done"
	SubRunBusy    SubRunStatus = "busy"    // the sub-agent pool stayed full
	SubRunRefused SubRunStatus = "refused" // delegation is off, the helper is unknown, or the model is not a cloud model
	SubRunFailed  SubRunStatus = "failed"
	SubRunStopped SubRunStatus = "stopped" // the parent was cancelled
)

// SubRunResult is what a delegated task returned.
type SubRunResult struct {
	RunID            string
	Model            string
	Status           SubRunStatus
	Summary          string
	Error            string
	DurationMs       int64
	PromptTokens     int
	CompletionTokens int
}

// SubagentEventKind is the coarse step a child reports to its parent; reasoning and streamed text are never forwarded.
type SubagentEventKind string

const (
	SubagentStarted    SubagentEventKind = "started"
	SubagentToolCall   SubagentEventKind = "tool_call"
	SubagentToolResult SubagentEventKind = "tool_result"
	SubagentDone       SubagentEventKind = "done"
)

// SubagentEvent is the payload of an EventSubagent: one coarse step of one child, nested under the parent's turn.
type SubagentEvent struct {
	RunID  string            `json:"run_id"`
	Agent  string            `json:"agent"`
	Kind   SubagentEventKind `json:"kind"`
	Tool   string            `json:"tool,omitempty"`
	Detail string            `json:"detail,omitempty"`
}
