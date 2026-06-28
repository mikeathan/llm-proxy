package models

import (
	"time"
)

// AssistantSession represents a stateful conversation thread.
type AssistantSession struct {
	ID              string          `json:"id"`
	WorkspaceID     string          `json:"workspace_id"`
	ContextVersion  string          `json:"context_version,omitempty"`
	Timezone        string          `json:"timezone,omitempty"`
	History         []Message       `json:"history"`
	UpdatedAt       time.Time       `json:"updated_at"`
	Metadata        map[string]any  `json:"metadata,omitempty"`
	// CancelledIndices tracks every cancelled turn's user message index
	// (index into History).  Each cancel appends; the list survives reloads
	// so all cancelled turns can be marked and stripped from LLM context.
	CancelledIndices []int         `json:"cancelled_indices,omitempty"`
}


// SessionBrief provides a summary of a session for listing in the UI.
type SessionBrief struct {
	ID        string    `json:"id"`
	Snippet   string    `json:"snippet"`
	UpdatedAt time.Time `json:"updated_at"`
}
