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

// ToolEffect classifies what a tool call can change. The agent loop uses it to
// decide whether the rest of a tool-call batch may still run after one call
// fails, and whether a turn's text beside its calls can be the run's report.
type ToolEffect int

const (
	// EffectMutating means the call may change workspace, host or external
	// state (files, shell, notifications, MCP servers). This is the default, so
	// an unlisted or unknown tool is never assumed safe to run past.
	EffectMutating ToolEffect = iota
	// EffectReadOnly means the call only reads: a later call in the same batch
	// cannot depend on what it changed, because it changes nothing.
	EffectReadOnly
	// EffectHousekeeping means the call records run bookkeeping (saving a
	// memory, writing the automation journal) rather than doing the task, so
	// text written beside it may be the run's report.
	EffectHousekeeping
)

// toolEffects is the explicit per-tool side-effect table. Only built-in tools
// whose behaviour is known appear here; everything else defaults to mutating.
var toolEffects = map[string]ToolEffect{
	models.ToolNetworkFetch:      EffectReadOnly,
	models.ToolInternetSearch:    EffectReadOnly,
	models.ToolFileRead:          EffectReadOnly,
	models.ToolDirectoryList:     EffectReadOnly,
	models.ToolMemorySearch:      EffectReadOnly,
	models.ToolNetworkInfo:       EffectReadOnly,
	models.ToolNetworkScan:       EffectReadOnly,
	models.ToolMemoryUpdate:      EffectHousekeeping,
	models.ToolAutomationJournal: EffectHousekeeping,
}

// EffectFor returns the side-effect class of a tool. An unlisted tool
// (including every MCP tool) is EffectMutating.
func EffectFor(name string) ToolEffect {
	if e, ok := toolEffects[name]; ok {
		return e
	}
	return EffectMutating
}
