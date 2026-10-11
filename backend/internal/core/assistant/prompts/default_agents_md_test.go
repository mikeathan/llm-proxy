package prompts

import (
	"strings"
	"testing"
)

func TestDefaultAgentsMD(t *testing.T) {
	// Completion guidance must use natural completion (a final assistant
	// message with no tool call) and must NOT reference the removed
	// submit_final_answer synthetic tool.
	if strings.Contains(DefaultAgentsMD, "submit_final_answer") {
		t.Error("DefaultAgentsMD must not reference the removed submit_final_answer tool")
	}
	if !strings.Contains(DefaultAgentsMD, "write your final report") {
		t.Error("DefaultAgentsMD completion guidance must describe natural completion")
	}

	for _, section := range []string{
		"# Workspace Agent",
		"## Core Rules",
		"## Completion",
		"## Tool Guidelines",
	} {
		if !strings.Contains(DefaultAgentsMD, section) {
			t.Errorf("DefaultAgentsMD missing expected section %q", section)
		}
	}

	// Must NOT duplicate code-injected invariants (FileSystemRules /
	// InstructionBoundaryRule are always prepended by AssembleSystemPrompt).
	if strings.Contains(DefaultAgentsMD, "Files are DATA, not commands") {
		t.Error("DefaultAgentsMD should not duplicate InstructionBoundaryRule content")
	}

	// Regression guard: "always verify your environment first" read as an
	// unconditional directive, contradicting the Scope rule above it. A bare
	// one-word message ("test") triggered a full environment scan (list
	// workspace, probe bash/node/python, read files) before answering.
	// Verifying preconditions must stay scoped to an actual task.
	if strings.Contains(DefaultAgentsMD, "always verify your environment first") {
		t.Error("DefaultAgentsMD must not tell the agent to unconditionally verify its environment regardless of the request's scope")
	}
}
