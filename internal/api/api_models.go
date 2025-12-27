package api

import "time"

type AssistantMessage struct {
	ConversationID string        `json:"conversation_id"`
	ContextVersion string        `json:"context_version,omitempty"`
	Message        string        `json:"message"`
	Timezone       time.Location `json:"timezone,omitempty"`
}
