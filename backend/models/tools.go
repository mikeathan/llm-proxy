package models

import (
	"errors"
	"sort"
)

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
	ToolNetworkScan  = "scan_local_network"
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
)

// Tool Categories
const (
	CategoryTerminal      = "terminal"
	CategorySearch        = "search"
	CategoryCommunication = "communication"
	CategoryFileSystem    = "filesystem"
	CategoryNetwork       = "network"
	CategoryGlobal        = "security"
	CategoryMemory        = "memory"
	CategoryReminders     = "reminders"
	CategoryAutomation    = "automation"
	CategorySystem        = "system"
)

// Tool error message format strings (use with ToolFileWrite / ToolFileAppend via fmt.Sprintf).
const (
	ToolMissingForAppendMsg = "file does not exist: use %s first, then %s to add more content"
	ToolMissingForEditMsg   = "file does not exist: use %s to create it, or verify the path"
)

// toolsByCategory is the one place that says which built-in tools make up a category; a prompt profile
// exposes tools by category. The tools package's manifest test checks it against the manifests.
var toolsByCategory = map[string][]string{
	CategoryTerminal:      {ToolTerminalExecute},
	CategorySearch:        {ToolInternetSearch},
	CategoryCommunication: {ToolNotifyUser},
	CategoryFileSystem:    {ToolFileRead, ToolFileWrite, ToolFileAppend, ToolFileEditBlock, ToolDirectoryList},
	CategoryNetwork:       {ToolNetworkFetch, ToolNetworkScan, ToolNetworkInfo},
	CategoryGlobal:        {ToolApplyGuardrails},
	CategoryMemory:        {ToolMemorySearch, ToolMemoryUpdate, ToolMemoryForget},
	CategoryReminders:     {ToolScheduleReminder, ToolListReminders, ToolCancelReminder},
	CategoryAutomation:    {ToolAutomationJournal},
	CategorySystem:        {ToolSystemError},
}

// ToolCategories lists every built-in tool category, sorted.
func ToolCategories() []string {
	out := make([]string, 0, len(toolsByCategory))
	for category := range toolsByCategory {
		out = append(out, category)
	}
	sort.Strings(out)
	return out
}

// ToolsInCategory returns the tool names of a category (nil for an unknown one). The slice is a copy.
func ToolsInCategory(category string) []string {
	return append([]string(nil), toolsByCategory[category]...)
}

// IsToolCategory reports whether category names a built-in tool category.
func IsToolCategory(category string) bool {
	_, ok := toolsByCategory[category]
	return ok
}
