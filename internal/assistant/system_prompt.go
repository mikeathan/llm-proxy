package assistant

import "fmt"

func BuildSystemMessage(systemPrompt string, conversationID string, contextVersion string, timezone string) string {

	return fmt.Sprintf(
		`%s

Conversation ID: %s
Context Version: %s
Timezone: %s`,
		systemPrompt,
		conversationID,
		contextVersion,
		timezone,
	)
}
