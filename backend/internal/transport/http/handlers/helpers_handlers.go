package handlers

import (
	"net/http"

	"llm-proxy/internal/core/assistant"
	"llm-proxy/models"
)

// HelpersService is what the helpers status needs: whether the sub-runner is set up, and the settings that name its model.
type HelpersService interface {
	SubRunner() assistant.SubRunner
	AgentSettings() *models.AgentConfig
}

// HelpersHandlers tells the page whether a chat can use helpers.
type HelpersHandlers struct{ svc HelpersService }

func NewHelpersHandlers(svc HelpersService) *HelpersHandlers { return &HelpersHandlers{svc: svc} }

type helpersStatus struct {
	// Available is true when a cloud helper model is set up, so a chat's "Use helpers" can work.
	Available bool `json:"available"`
	// Model is the helper model Settings names ("" when none), even when it is not usable, so the page can say why.
	Model string `json:"model"`
}

// Status answers GET /admin/api/helpers.
func (h *HelpersHandlers) Status(w http.ResponseWriter, _ *http.Request) {
	runner := h.svc.SubRunner()
	respondJSON(w, helpersStatus{
		Available: runner != nil && runner.Enabled(),
		Model:     h.svc.AgentSettings().SubagentModelName(),
	})
}
