package chat

import (
	"context"

	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
)

// ExecuteResult is the typed return value from ConversationService.Execute.
type ExecuteResult struct {
	Reply          string `json:"reply"`
	ConversationID string `json:"conversation_id"`
	WorkspaceID    string `json:"workspace_id"`
	Canceled       bool   `json:"canceled,omitempty"`
}

// ExecuteRequest is one chat turn handed to the conversation service: what was asked, and the parts the turn runs with.
type ExecuteRequest struct {
	WorkspaceID    string
	ConversationID string
	Message        string
	ContextVersion string
	Timezone       string
	// Agent runs this turn under that profile and leaves the conversation's own as it was (a turn command); ""
	// runs the conversation's profile.
	Agent        string
	ExcludeTools []string
	Log          logging.Logger
	Provider     assistant.ToolProvider
	Client       proxy.Client
	Engine       assistant.Engine
	Events       assistant.EventPublisher
	Recorder     assistant.EventRecorder // nil: the turn is not recorded
}

// ConversationService encapsulates the stateful agent turn execution.
// The chat service (core/chat) admits the turn and calls Execute; the HTTP handler only decodes and encodes.
type ConversationService interface {
	Execute(ctx context.Context, req ExecuteRequest) (ExecuteResult, error)
}
