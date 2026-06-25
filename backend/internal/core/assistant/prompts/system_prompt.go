package prompts

import (
	"fmt"
	"llm-proxy/models"
	"time"
)

func BuildSystemMessage(systemPrompt string, useNativeTools bool, conversationID string, contextVersion string, timezone string) string {
	currentTimeUTC := time.Now().UTC().Format(time.RFC3339)

	finalAnswerInstruction := "CRITICAL: When you have finished your task or have a final answer for the user, your response MUST be a clear natural language or Markdown answer. \nDO NOT include raw technical data structures in the final answer you provide to the user."
	if useNativeTools {
		finalAnswerInstruction = "CRITICAL: In native tool-calling mode, your final answer MUST be delivered through the '" + models.ToolSubmitFinalAnswer + "' tool's 'summary' argument. Keep any freeform content brief — do not write the full answer as text before calling the tool."
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
