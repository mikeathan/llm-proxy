package prompts

import (
	"strings"
	"testing"
	"time"
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

func TestLocalTimeBlock(t *testing.T) {
	// 19:29 UTC on Mon 12 Oct 2026 is 20:29 BST in London.
	now := time.Date(2026, 10, 12, 19, 29, 0, 0, time.UTC)
	block := LocalTimeBlock(now, "Europe/London")
	for _, want := range []string{"Mon 12 Oct 2026 20:29", "UTC+01:00", "Sun 18 Oct", "Mon 19 Oct", "Europe/London"} {
		if !strings.Contains(block, want) {
			t.Errorf("block missing %q:\n%s", want, block)
		}
	}
}

func TestLocalTimeBlock_UnknownZoneFallsBackToUTC(t *testing.T) {
	now := time.Date(2026, 10, 12, 19, 29, 0, 0, time.UTC)
	for _, zone := range []string{"", "Mars/Base"} {
		block := LocalTimeBlock(now, zone)
		if !strings.Contains(block, "Mon 12 Oct 2026 19:29") || !strings.Contains(block, "UTC+00:00") {
			t.Errorf("zone %q: want UTC local time, got:\n%s", zone, block)
		}
	}
}

func TestBuildSystemMessage_HasNoClockThatCouldGoStale(t *testing.T) {
	system := BuildSystemMessage("test", true, "conv-1", "v1", "Europe/London")
	if strings.Contains(system, "Next 14 days:") {
		t.Errorf("the local clock rides on each turn, not in the system prompt:\n%s", system)
	}
}

func TestTurnTimeNote(t *testing.T) {
	note := TurnTimeNote(time.Date(2026, 10, 14, 19, 29, 0, 0, time.UTC), "Europe/London")
	if !strings.HasPrefix(note, LocalTimeNoteHeader) || !strings.Contains(note, "Wed 14 Oct 2026 20:29") {
		t.Errorf("note = %q", note)
	}
}
