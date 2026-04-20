package models

import (
	"time"
)

// AssistantSession represents a stateful conversation thread.
type AssistantSession struct {
	ID             string          `json:"id"`
	WorkspaceID    string          `json:"workspace_id"`
	ContextVersion string          `json:"context_version,omitempty"`
	Timezone       string          `json:"timezone,omitempty"`
	History        []Message       `json:"history"`
	UpdatedAt      time.Time       `json:"updated_at"`
	Metadata       map[string]any  `json:"metadata,omitempty"`
}


// SessionBrief provides a summary of a session for listing in the UI.
type SessionBrief struct {
	ID        string    `json:"id"`
	Snippet   string    `json:"snippet"`
	UpdatedAt time.Time `json:"updated_at"`
}
