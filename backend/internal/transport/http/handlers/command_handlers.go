package handlers

import (
	"net/http"

	"llm-proxy/internal/core/commands"
	"llm-proxy/internal/core/reminders"
)

// CommandHandlers serves the slash commands to the web chat: the list for its
// menu, a live preview of what the text typed so far would do, and running one.
type CommandHandlers struct {
	registry *commands.Registry
	// ownerZone returns the owner's configured IANA zone, or "" when unset.
	ownerZone func() string
}

func NewCommandHandlers(registry *commands.Registry, ownerZone func() string) *CommandHandlers {
	return &CommandHandlers{registry: registry, ownerZone: ownerZone}
}

type commandRequest struct {
	Message  string `json:"message"`
	Timezone string `json:"timezone"`
}

type commandRunResponse struct {
	Handled bool   `json:"handled"`
	Reply   string `json:"reply,omitempty"`
	// Failed is true when the command was refused; Reply then says why.
	Failed bool `json:"failed,omitempty"`
	// Turn is set by a turn command (/research, /agent): the chat sends Turn.Message with these run choices, for
	// this message only, after showing Reply when there is one.
	Turn *commands.TurnSpec `json:"turn,omitempty"`
}

type commandPreviewResponse struct {
	Matched bool `json:"matched"`
	commands.Preview
}

func (h *CommandHandlers) env(r commandRequest) commands.Env {
	owner := ""
	if h.ownerZone != nil {
		owner = h.ownerZone()
	}
	return commands.Env{Zone: reminders.ChatZone(owner, r.Timezone)}
}

// List handles GET /admin/api/commands.
func (h *CommandHandlers) List(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, map[string]any{"commands": h.registry.List()})
}

// Run handles POST /admin/api/commands/run. handled is false for text that is not
// a command, which the chat then sends to the assistant as usual; a turn command
// answers with the turn to send instead.
func (h *CommandHandlers) Run(w http.ResponseWriter, r *http.Request) {
	var req commandRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	reply, handled := h.registry.Execute(r.Context(), req.Message, h.env(req))
	respondJSON(w, commandRunResponse{Handled: handled, Reply: reply.Text, Failed: reply.Failed, Turn: reply.Turn})
}

// Preview handles POST /admin/api/commands/preview.
func (h *CommandHandlers) Preview(w http.ResponseWriter, r *http.Request) {
	var req commandRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	p, matched := h.registry.Preview(r.Context(), req.Message, h.env(req))
	respondJSON(w, commandPreviewResponse{Matched: matched, Preview: p})
}
