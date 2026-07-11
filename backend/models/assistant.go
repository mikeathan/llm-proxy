package models

import (
	"strings"
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
	// Source identifies how the session originated.  Webhook sessions carry
	// their connector platform in the value ("webhook-telegram",
	// "webhook-slack", ...) so the UI can label them; manual sessions are
	// "manual".  Derived from the session ID by SessionSource — the only place
	// that knows the ID format.
	Source string `json:"source"`
}

// SessionSource derives the origin from a session ID.  It is the single source
// of truth: webhook IDs embed the connector platform type (wb_{type}_...), so
// the value is extracted from the connector rather than hardcoded per platform.
// The frontend only maps the resulting string to icons/labels and never parses
// the ID itself.
func SessionSource(id string) string {
	if !strings.HasPrefix(id, "wb_") {
		return "manual"
	}
	platform := strings.SplitN(strings.TrimPrefix(id, "wb_"), "_", 2)[0]
	if platform == "" {
		return "manual"
	}
	return "webhook-" + platform
}
