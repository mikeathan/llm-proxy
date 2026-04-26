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

CRITICAL: Your final response MUST be a natural language or Markdown answer. 
DO NOT include raw JSON, tool call XML tags (<function-name>, <args-json-object>), or return any technical data structures in your final output. 
Provide only the raw markdown or text answer for the user.`,
		systemPrompt,
		conversationID,
		contextVersion,
		timezone,
		currentTimeUTC,
	)
}
