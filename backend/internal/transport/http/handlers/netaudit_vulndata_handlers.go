package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"llm-proxy/internal/core/netaudit/vulnfeed"
)

// VulnData is the part of the vulnerability-data manager the handlers use. *vulnfeed.Manager satisfies it.
type VulnData interface {
	Status() vulnfeed.Status
	Start(restart bool) error
	Cancel()
	Clear() error
}

// VulnDataHandlers serve the "Vulnerability data" card: how much data is held, and the one-click download with its
// progress. Starting a download is an operator action and, like the rest of the admin API, relies on the unauthenticated
// admin API's convention (see the admin-api-authentication plan).
type VulnDataHandlers struct{ data VulnData }

func NewVulnDataHandlers(data VulnData) *VulnDataHandlers { return &VulnDataHandlers{data: data} }

// Get reports the data held and the download's progress.
func (h *VulnDataHandlers) Get(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, h.data.Status())
}

// Download starts the download of the files not yet held (or a refresh when all are held) and answers at once; the
// screen follows it with Get. `?restart=1` fetches every file again, not only the missing ones. A second start while one
// runs is a conflict. When everything is held and recent nothing starts and the answer is `200` with the status.
func (h *VulnDataHandlers) Download(w http.ResponseWriter, r *http.Request) {
	if err := h.data.Start(r.URL.Query().Get("restart") == "1"); errors.Is(err, vulnfeed.ErrUpToDate) {
		respondJSON(w, h.data.Status())
		return
	} else if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, vulnfeed.ErrBusy) {
			status = http.StatusConflict
		}
		writeJSONError(w, status, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(h.data.Status())
}

// Cancel stops a running download; the files already finished stay.
func (h *VulnDataHandlers) Cancel(w http.ResponseWriter, _ *http.Request) {
	h.data.Cancel()
	respondJSON(w, h.data.Status())
}

// Clear deletes all the vulnerability data (stopping a download that runs). Audits have nothing to compare with until it
// is downloaded again.
func (h *VulnDataHandlers) Clear(w http.ResponseWriter, _ *http.Request) {
	if err := h.data.Clear(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, h.data.Status())
}
