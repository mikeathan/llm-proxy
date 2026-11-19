package handlers

import (
	"context"
	"net/http"

	"llm-proxy/internal/platform/datausage"
)

// StorageHandlers serve the data management page: what the service keeps, and how big it is. It only reads.
type StorageHandlers struct {
	report func(ctx context.Context) (datausage.Report, error)
}

// NewStorageHandlers builds the handlers over the report.
func NewStorageHandlers(report func(ctx context.Context) (datausage.Report, error)) *StorageHandlers {
	return &StorageHandlers{report: report}
}

// Get answers the report. A data folder that cannot be read is a 500 with a fixed message.
func (h *StorageHandlers) Get(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.report == nil {
		respondError(w, http.StatusServiceUnavailable, "the storage report is not available")
		return
	}
	rep, err := h.report(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, "the data folder could not be read")
		return
	}
	respondJSON(w, rep)
}
