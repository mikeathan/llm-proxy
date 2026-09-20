package toolpolicy

import "llm-proxy/models"

// FailurePolicy is the run-fatality decision for a terminal tool failure —
// distinct from terminality itself (the tool marks that with
// models.ErrToolUnavailable). See
// docs/PLANS/cross-cutting/tool-error-classification.md.
type FailurePolicy int

const (
	// FatalOnTerminalError means a terminal failure of this tool ends an
	// unattended (automation) run: an essential capability is down and there is
	// nobody to fix it. This is the default.
	FatalOnTerminalError FailurePolicy = iota
	// WarnOnTerminalError records a warning and lets the run continue — for
	// delivery/side-effect tools whose failure does not invalidate the result
	// (e.g. a notification channel being down).
	WarnOnTerminalError
)

// failurePolicies is the explicit per-tool policy table. Only tools that opt out
// of run-fatality appear here; everything else defaults to Fatal.
var failurePolicies = map[string]FailurePolicy{
	models.ToolNotifyUser: WarnOnTerminalError,
}

// FailurePolicyFor returns the terminal-failure policy for a tool. An unlisted
// tool is FatalOnTerminalError so an unknown tool can never silently degrade a
// run.
func FailurePolicyFor(name string) FailurePolicy {
	if p, ok := failurePolicies[name]; ok {
		return p
	}
	return FatalOnTerminalError
}
