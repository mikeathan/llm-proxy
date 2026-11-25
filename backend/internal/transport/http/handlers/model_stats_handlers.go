package handlers

import (
	"net/http"
	"strconv"

	"llm-proxy/internal/platform/modelstats"
)

// defaultModelStatsDays is how many days the report covers when the request does not say.
const defaultModelStatsDays = 7

// ModelStatsHandlers serves the model use counts (ADR-0085): numbers per day and per model, nothing else.
type ModelStatsHandlers struct {
	report func(days int) modelstats.Report
}

func NewModelStatsHandlers(report func(days int) modelstats.Report) *ModelStatsHandlers {
	return &ModelStatsHandlers{report: report}
}

// AdminModelStatsHandler answers GET /admin/api/model-stats?days=N: the last N days (default 7, at most the days kept) and today's models.
func (h *ModelStatsHandlers) AdminModelStatsHandler(w http.ResponseWriter, r *http.Request) {
	days := defaultModelStatsDays
	if raw := r.URL.Query().Get("days"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			writeJSONError(w, http.StatusBadRequest, "days must be a whole number of at least 1")
			return
		}
		days = n
	}
	respondJSON(w, h.report(days))
}
