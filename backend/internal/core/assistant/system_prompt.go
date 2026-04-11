package assistant

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

CRITICAL: Your final response MUST be a natural language answer. 
DO NOT echo the tool call JSON, tags, or return any JSON structures in your final output. 
Just give the plain text answer to the user's question.`,
		systemPrompt,
		conversationID,
		contextVersion,
		timezone,
		currentTimeUTC,
	)
}
