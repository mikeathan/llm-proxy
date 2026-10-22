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
	// Model is the helper model Settings names ("" when none), even when it is not usable, so the page can say which.
	Model string `json:"model"`
	// Problem says why helpers cannot run ("unset", "missing" when the model was removed, "not_cloud"); empty when they can.
	Problem assistant.HelperProblem `json:"problem,omitempty"`
}

// Status answers GET /admin/api/helpers.
func (h *HelpersHandlers) Status(w http.ResponseWriter, _ *http.Request) {
	problem := assistant.HelperUnset // no runner wired: helpers are not set up
	if runner := h.svc.SubRunner(); runner != nil {
		problem = runner.Problem()
	}
	respondJSON(w, helpersStatus{
		Available: problem == assistant.HelperNone,
		Model:     h.svc.AgentSettings().SubagentModelName(),
		Problem:   problem,
	})
}
