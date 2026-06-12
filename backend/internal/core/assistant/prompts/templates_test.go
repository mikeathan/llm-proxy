package prompts

import (
	"strings"
	"testing"
)

func TestAssembleSystemPrompt_ToolCallFormat(t *testing.T) {
	xmlPrompt := AssembleSystemPrompt("", false)
	nativePrompt := AssembleSystemPrompt("", true)

	if !strings.Contains(xmlPrompt, "<tool_call>") {
		t.Error("XML mode prompt should contain <tool_call>")
	}
	if strings.Contains(nativePrompt, "<tool_call>") {
		t.Error("native mode prompt should NOT contain <tool_call>")
	}

	// Both should still contain the fundamental rules.
	if !strings.Contains(xmlPrompt, "ReAct Loop") {
		t.Error("XML mode should contain ReAct Loop instruction")
	}
	if !strings.Contains(nativePrompt, "ReAct Loop") {
		t.Error("native mode should contain ReAct Loop instruction")
	}
}
