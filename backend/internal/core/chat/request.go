package chat

// Request is one chat message and how to run it. The web chat and every connector build one and give it to Service.Run.
type Request struct {
	WorkspaceID    string   `json:"workspace_id"`
	ConversationID string   `json:"conversation_id"`
	ContextVersion string   `json:"context_version,omitempty"`
	Message        string   `json:"message"`
	Timezone       string   `json:"timezone,omitempty"`
	ExcludeTools   []string `json:"exclude_tools,omitempty"`
	// Model is a registry model chosen for this message; empty runs the primary.
	Model string `json:"model,omitempty"`
	// Profile is the prompt profile chosen for this message, which the conversation then keeps; empty keeps the
	// conversation's own or the workspace's.
	Profile string `json:"profile,omitempty"`
	// Agent is a profile for this message only (a turn command such as /research): the turn runs under it and the
	// conversation keeps its own. It outranks Profile for the turn; empty runs the conversation's profile.
	Agent string `json:"agent,omitempty"`
	// UseHelpers asks the assistant to hand the independent parts of this message to helpers (sub-agents).
	UseHelpers bool `json:"use_helpers,omitempty"`
}
