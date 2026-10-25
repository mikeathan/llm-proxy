package models

import (
	"errors"
	"sort"
)

// ErrLocalOnly marks a run whose profile allows only a local model but whose model client is not local. The run
// stops before any prompt is sent.
var ErrLocalOnly = errors.New("this run may only use a local model")

// ErrToolUnavailable marks an operator-actionable tool failure: the tool cannot
// succeed until configuration is fixed (missing or rejected credential, disabled
// integration). Tools wrap it with %w where the failure is detected; the agent
// loop classifies it via errors.Is and stops retrying that tool (see
// docs/PLANS/cross-cutting/tool-error-classification.md). Transient and
// input/content errors stay plain — they are model-actionable.
var ErrToolUnavailable = errors.New("tool unavailable: operator action required")

// ToolUnavailableError is ErrToolUnavailable with a short reason for people (e.g. "brave rejected the API key
// (HTTP 422)"); Cause keeps the full detail (the provider's response) for logs and the model. errors.Is matches it to
// ErrToolUnavailable, so the loop classifies it the same way.
type ToolUnavailableError struct {
	Reason string
	Cause  error // optional
}

func (e *ToolUnavailableError) Error() string {
	msg := ErrToolUnavailable.Error() + ": " + e.Reason
	if e.Cause != nil {
		msg += ": " + e.Cause.Error()
	}
	return msg
}

func (e *ToolUnavailableError) Unwrap() []error {
	if e.Cause == nil {
		return []error{ErrToolUnavailable}
	}
	return []error{ErrToolUnavailable, e.Cause}
}

// UnavailableReason is the short reason a ToolUnavailableError in err's chain carries, or err's own text when there is
// none. Warnings shown to the operator use it.
func UnavailableReason(err error) string {
	var unavailable *ToolUnavailableError
	if errors.As(err, &unavailable) && unavailable.Reason != "" {
		return unavailable.Reason
	}
	return err.Error()
}

// Tool Names
const (
	// Terminal
	ToolTerminalExecute = "execute_terminal_command"

	// Search
	ToolInternetSearch = "internet_search"

	// Communication
	ToolNotifyUser = "notify_user"

	// Reminders
	ToolScheduleReminder = "schedule_reminder"
	ToolListReminders    = "list_reminders"
	ToolCancelReminder   = "cancel_reminder"

	// FileSystem
	ToolFileRead      = "read_file"
	ToolFileWrite     = "write_file"
	ToolFileAppend    = "append_file"
	ToolFileEditBlock = "edit_file_block"
	ToolDirectoryList = "list_directory"

	// Network
	ToolNetworkFetch = "fetch_url"
	ToolNetworkScan  = "network_scan" // the netaudit category
	ToolNetworkInfo  = "get_network_info"

	// Security/Admin
	ToolApplyGuardrails = "security_guardrails"

	// Memory
	ToolMemorySearch = "memory_search"
	ToolMemoryUpdate = "memory_update"
	ToolMemoryForget = "memory_forget"

	// Automation (unattended runs only)
	ToolAutomationJournal = "automation_journal"

	// System
	ToolSystemError = "system_error"

	// Delegation: hand questions to sub-agents that run on a cloud model and return a summary
	ToolDelegate = "delegate_task"
)

// Tool Categories
const (
	CategoryTerminal      = "terminal"
	CategorySearch        = "search"
	CategoryCommunication = "communication"
	CategoryFileSystem    = "filesystem"
	CategoryNetwork       = "network"
	CategoryNetaudit      = "netaudit"
	CategoryGlobal        = "security"
	CategoryMemory        = "memory"
	CategoryReminders     = "reminders"
	CategoryAutomation    = "automation"
	CategorySystem        = "system"
	// CategoryDelegation is governed by a profile's delegates list, not by its tools list.
	CategoryDelegation = "delegation"
	// CategoryGeneral labels a tool outside the built-in table (an MCP tool); it is not a category a profile can name.
	CategoryGeneral = "general"
)

// Tool error message format strings (use with ToolFileWrite / ToolFileAppend via fmt.Sprintf).
const (
	ToolMissingForAppendMsg = "file does not exist: use %s first, then %s to add more content"
	ToolMissingForEditMsg   = "file does not exist: use %s to create it, or verify the path"
)

// CategoryOf returns the category a tool belongs to, or CategoryGeneral for a tool outside the built-in table.
func CategoryOf(toolName string) string {
	if category, ok := index.categoryByTool[toolName]; ok {
		return category
	}
	return CategoryGeneral
}

// ToolCategories lists every built-in tool category, sorted.
func ToolCategories() []string {
	out := make([]string, 0, len(index.toolsByCategory))
	for category := range index.toolsByCategory {
		out = append(out, category)
	}
	sort.Strings(out)
	return out
}

// ToolsInCategory returns the tool names of a category (nil for an unknown one). The slice is a copy.
func ToolsInCategory(category string) []string {
	return append([]string(nil), index.toolsByCategory[category]...)
}

// IsToolCategory reports whether category names a built-in tool category.
func IsToolCategory(category string) bool {
	_, ok := index.toolsByCategory[category]
	return ok
}
