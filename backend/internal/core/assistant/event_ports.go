package assistant

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
