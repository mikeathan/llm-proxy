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
