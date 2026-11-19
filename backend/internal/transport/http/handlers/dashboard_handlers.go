package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"llm-proxy/internal/core/dashboard"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
)

// maxDashboardBody bounds the layout's request body: 12 boards of 40 cards is far under it.
const maxDashboardBody = 128 << 10

// DashboardLayout is the owner's dashboard layout: *dashboard.Manager.
type DashboardLayout interface {
	Get() (models.Dashboard, error)
	Put(models.Dashboard) (models.Dashboard, error)
	Reset() error
}

// DashboardHandlers serve the dashboard layout: the boards, their cards and the size of each. One layout belongs to the
// server, so it follows the owner to every browser. It holds no stack data and changes no setting, so it is an ordinary
// signed-in request, not a step-up action.
type DashboardHandlers struct{ layout DashboardLayout }

func NewDashboardHandlers(layout DashboardLayout) *DashboardHandlers {
	return &DashboardHandlers{layout: layout}
}

// Get handles GET /admin/api/dashboard. Nothing stored answers {"boards": []}: the screen then shows its defaults.
func (h *DashboardHandlers) Get(w http.ResponseWriter, _ *http.Request) {
	d, err := h.layout.Get()
	if err != nil {
		logging.Warn("dashboard layout", "error", err.Error())
		writeJSONError(w, http.StatusInternalServerError, "failed to read the dashboard layout")
		return
	}
	respondJSON(w, d)
}

// Put handles PUT /admin/api/dashboard {"boards": [...]}: it replaces the layout, or refuses it with the reason.
func (h *DashboardHandlers) Put(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxDashboardBody+1))
	if err != nil || len(body) > maxDashboardBody {
		writeJSONError(w, http.StatusRequestEntityTooLarge, "the dashboard layout is too large")
		return
	}
	var in models.Dashboard
	if err := json.Unmarshal(body, &in); err != nil {
		writeJSONError(w, http.StatusBadRequest, "the body must be {\"boards\": [...]}")
		return
	}
	saved, err := h.layout.Put(in)
	switch {
	case errors.Is(err, dashboard.ErrInvalid):
		writeJSONError(w, http.StatusBadRequest, err.Error())
	case err != nil:
		logging.Warn("dashboard layout", "error", err.Error())
		writeJSONError(w, http.StatusInternalServerError, "failed to save the dashboard layout")
	default:
		respondJSON(w, saved)
	}
}

// Delete handles DELETE /admin/api/dashboard: it forgets the layout, so the screen shows its defaults again.
func (h *DashboardHandlers) Delete(w http.ResponseWriter, _ *http.Request) {
	if err := h.layout.Reset(); err != nil {
		logging.Warn("dashboard layout", "error", err.Error())
		writeJSONError(w, http.StatusInternalServerError, "failed to reset the dashboard layout")
		return
	}
	respondJSON(w, models.Dashboard{Boards: []models.DashboardBoard{}})
}
