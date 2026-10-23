package assistant

import (
	"context"

	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
)

// EventPublisher is the minimal interface for publishing agent events.
// This avoids a cyclic dependency with the automation package.
type EventPublisher interface {
	Publish(workspaceID string, event AgentEvent)
	Clear(workspaceID string, channel EventChannel)
}

// EventRecorder writes agent events to persistent storage (recording files).
// Nil-safe — the service skips recording when no recorder is provided.
type EventRecorder interface {
	Write(event AgentEvent) error
}

// ExecuteResult is the typed return value from ConversationService.Execute.
type ExecuteResult struct {
	Reply          string           `json:"reply"`
	ConversationID string           `json:"conversation_id"`
	WorkspaceID    string           `json:"workspace_id"`
	Events         []map[string]any `json:"events"`
	Canceled       bool             `json:"canceled,omitempty"`
}

// ExecuteRequest is one chat turn handed to the conversation service: what was asked, and the parts the turn runs with.
type ExecuteRequest struct {
	WorkspaceID    string
	ConversationID string
	Message        string
	ContextVersion string
	Timezone       string
	ExcludeTools   []string
	Log            logging.Logger
	Provider       ToolProvider
	Client         proxy.Client
	Engine         Engine
	Events         EventPublisher
	Recorder       EventRecorder // nil: the turn is not recorded
}

// ConversationService encapsulates the stateful agent turn execution.
// The chat service (core/chat) admits the turn and calls Execute; the HTTP handler only decodes and encodes.
type ConversationService interface {
	Execute(ctx context.Context, req ExecuteRequest) (ExecuteResult, error)
}
