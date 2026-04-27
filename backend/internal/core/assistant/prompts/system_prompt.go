package prompts

import (
	"fmt"
	"time"
)

func BuildSystemMessage(systemPrompt string, conversationID string, contextVersion string, timezone string) string {
	currentTimeUTC := time.Now().UTC().Format(time.RFC3339)

	return fmt.Sprintf(
		`%s

Conversation ID: %s
Context Version: %s
Timezone: %s
Current Time (UTC): %s

CRITICAL: When you have finished your task or have a final answer for the user, your response MUST be a clear natural language or Markdown answer. 
DO NOT include raw technical data structures in the final answer you provide to the user.`,
		systemPrompt,
		conversationID,
		contextVersion,
		timezone,
		currentTimeUTC,
	)
}
