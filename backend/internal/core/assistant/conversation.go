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

// ConversationService encapsulates the stateful agent turn execution.
// The HTTP handler decodes/validates the request, calls Execute, then encodes the response.
type ConversationService interface {
	Execute(ctx context.Context, workspaceID, conversationID, message, contextVersion, timezone string, excludeTools []string, log logging.Logger, provider ToolProvider, client proxy.Client, engine Engine, events EventPublisher, recorder EventRecorder) (ExecuteResult, error)
}
