package prompts

import (
	"fmt"
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
