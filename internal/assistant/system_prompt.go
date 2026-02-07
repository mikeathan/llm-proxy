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
Current Time (UTC): %s`,
		systemPrompt,
		conversationID,
		contextVersion,
		timezone,
		currentTimeUTC,
	)
}
