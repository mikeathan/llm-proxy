package toolpolicy

import (
	"testing"

	"llm-proxy/models"
)

// TestFailurePolicyFor pins the terminal-failure policy table: notify_user is a
// delivery tool (warn), everything else defaults to fatal so an unlisted tool
// can never silently degrade a run.
func TestFailurePolicyFor(t *testing.T) {
	tests := []struct {
		tool string
		want FailurePolicy
	}{
		{models.ToolNotifyUser, WarnOnTerminalError},
		{models.ToolInternetSearch, FatalOnTerminalError},
		{models.ToolNetworkFetch, FatalOnTerminalError},
		{"unknown_tool", FatalOnTerminalError},
		{"", FatalOnTerminalError},
	}
	for _, tt := range tests {
		if got := FailurePolicyFor(tt.tool); got != tt.want {
			t.Errorf("FailurePolicyFor(%q) = %v, want %v", tt.tool, got, tt.want)
		}
	}
}

// TestEffectFor pins the side-effect table: only listed tools are read-only or
// housekeeping; every other name — a mutating built-in, an MCP server's tool,
// an unknown or empty name — is mutating, so a batch never runs past a failed
// call that may have changed state.
func TestEffectFor(t *testing.T) {
	tests := []struct {
		tool string
		want ToolEffect
	}{
		{models.ToolNetworkFetch, EffectReadOnly},
		{models.ToolInternetSearch, EffectReadOnly},
		{models.ToolFileRead, EffectReadOnly},
		{models.ToolDirectoryList, EffectReadOnly},
		{models.ToolMemorySearch, EffectReadOnly},
		{models.ToolNetworkInfo, EffectReadOnly},
		{models.ToolNetworkScan, EffectReadOnly},
		{models.ToolMemoryUpdate, EffectHousekeeping},
		{models.ToolAutomationJournal, EffectHousekeeping},
		{models.ToolFileWrite, EffectMutating},
		{models.ToolFileAppend, EffectMutating},
		{models.ToolFileEditBlock, EffectMutating},
		{models.ToolTerminalExecute, EffectMutating},
		{models.ToolNotifyUser, EffectMutating},
		{models.ToolApplyGuardrails, EffectMutating},
		{models.ToolSystemError, EffectMutating},
		{"create_issue", EffectMutating},     // MCP tools keep their server's raw name
		{"brave_web_search", EffectMutating}, // even a read-like MCP name is not trusted
		{"unknown_tool", EffectMutating},
		{"", EffectMutating},
	}
	for _, tt := range tests {
		if got := EffectFor(tt.tool); got != tt.want {
			t.Errorf("EffectFor(%q) = %v, want %v", tt.tool, got, tt.want)
		}
	}
}
