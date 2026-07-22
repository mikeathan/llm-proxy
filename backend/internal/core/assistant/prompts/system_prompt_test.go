package prompts

import (
	"strings"
	"testing"
)

func TestBuildSystemMessage_NativeMode(t *testing.T) {
	system := BuildSystemMessage("test", true, "conv-1", "v1", "UTC")
	if strings.Contains(system, "clear natural language or Markdown answer") {
		t.Error("native mode should NOT contain 'clear natural language' instruction")
	}
	if !strings.Contains(system, "write your final answer as a regular assistant message") {
		t.Error("native mode should contain natural completion instruction")
	}
	if !strings.Contains(system, "stop calling tools") {
		t.Error("native mode should mention stopping tool calls")
	}
	if strings.Contains(system, "submit_final_answer") {
		t.Error("native mode should NOT reference the removed submit_final_answer tool")
	}
}

func TestBuildSystemMessage_NonNativeMode(t *testing.T) {
	system := BuildSystemMessage("test", false, "conv-1", "v1", "UTC")
	if !strings.Contains(system, "clear natural language or Markdown answer") {
		t.Error("non-native mode should contain 'clear natural language' instruction")
	}
}
