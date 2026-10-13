package prompts

import (
	"fmt"
	"strings"
	"time"
)

func BuildSystemMessage(systemPrompt string, useNativeTools bool, conversationID string, contextVersion string, timezone string) string {
	currentTimeUTC := time.Now().UTC().Format(time.RFC3339)

	// Termination is implicit: a normal assistant message with no tool calls
	// signals completion. In native tool-calling mode the model must write its
	// final answer as text and stop, not call a synthetic submission tool.
	finalAnswerInstruction := "CRITICAL: When you have finished your task or have a final answer for the user, your response MUST be a clear natural language or Markdown answer. \nDO NOT include raw technical data structures. Never end a turn with a promise of future action — if you need more tools, call them immediately. When you are done, respond with only your final answer as a normal message. Do NOT call the write_file tool for the answer unless the task explicitly asks to save a file."
	if useNativeTools {
		finalAnswerInstruction = "CRITICAL: In native tool-calling mode, write your final answer as a regular assistant message and stop calling tools. Never end a turn with a promise of future action — if you need more tools, call them immediately. When you are done, respond with only your final answer. Do NOT call the write_file tool for the answer unless the task explicitly asks to save a file."
	}

	return fmt.Sprintf(
		`%s

Conversation ID: %s
Context Version: %s
Timezone: %s
Current Time (UTC): %s

%s`,
		systemPrompt,
		conversationID,
		contextVersion,
		timezone,
		currentTimeUTC,
		finalAnswerInstruction,
	)
}

// TurnTimeNote is the user's local date, time and calendar as a note appended to
// the turn's user message for the model only (never stored), so it is fresh on every
// turn of a conversation that stays open for days and the system prompt's prefix
// stays byte-identical for the model server's cache.
func TurnTimeNote(now time.Time, zone string) string {
	return LocalTimeNoteHeader + "\n" + LocalTimeBlock(now, zone)
}

// LocalTimeBlock is the user's local date and time plus the next days' calendar, in
// zone (UTC when it is empty or unknown). The prompt gives the model a calendar so
// "Sunday the 18th" is read off it, not worked out.
func LocalTimeBlock(now time.Time, zone string) string {
	loc, err := time.LoadLocation(zone)
	if err != nil || zone == "" {
		loc = time.UTC
	}
	local := now.In(loc)
	_, offset := local.Zone()
	sign := "+"
	if offset < 0 {
		sign, offset = "-", -offset
	}
	offsetText := fmt.Sprintf("%s%02d:%02d", sign, offset/3600, offset%3600/60)
	days := make([]string, 0, LocalCalendarDays)
	for i := 1; i <= LocalCalendarDays; i++ {
		days = append(days, local.AddDate(0, 0, i).Format(LocalDayLayout))
	}
	return fmt.Sprintf(LocalNowLine, loc, local.Format(LocalNowLayout), offsetText) + "\n" +
		fmt.Sprintf(LocalCalendarLine, LocalCalendarDays, strings.Join(days, ", "))
}
