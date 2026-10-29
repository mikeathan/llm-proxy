package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"llm-proxy/internal/core/netaudit/vulnfeed"
	"llm-proxy/internal/platform/logging"
)

// VulnData is the part of the vulnerability-data manager the handlers use. *vulnfeed.Manager satisfies it.
type VulnData interface {
	Status() vulnfeed.Status
	Start(restart bool) error
	Import(source string, year int, r io.Reader) error
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

// importReadWindow is how long one imported file may take to arrive.
const importReadWindow = 10 * time.Minute

// Import reads one vulnerability data file the operator supplies in the request body, for a host that cannot download
// it: `?source=nvd&year=2025` for NVD's yearly file (gzip or plain JSON), `?source=kev` for CISA's list. It answers when
// the file has been read and saved, with the status. It is a step-up route: it replaces what every audit compares with,
// so a token must not be able to call it. The body is capped, and nothing of the file's text is ever repeated back.
func (h *VulnDataHandlers) Import(w http.ResponseWriter, r *http.Request) {
	year := 0
	if raw := r.URL.Query().Get("year"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, vulnfeed.ErrBadYear.Error())
			return
		}
		year = n
	}
	// The server reads a request for 30 seconds; a file of tens of megabytes over a slow link needs longer. Only this route
	// is given more, and the body is still capped.
	if err := http.NewResponseController(w).SetReadDeadline(time.Now().Add(importReadWindow)); err != nil {
		logging.Warn("vulnerability data import: the read deadline could not be extended; a slow upload may be cut at the server default", "error", err.Error())
	}
	body := http.MaxBytesReader(w, r.Body, vulnfeed.MaxFileBytes)
	err := h.data.Import(r.URL.Query().Get("source"), year, body)
	switch {
	case err == nil:
		respondJSON(w, h.data.Status())
	case errors.Is(err, vulnfeed.ErrBusy):
		writeJSONError(w, http.StatusConflict, err.Error())
	case errors.Is(err, vulnfeed.ErrBadSource), errors.Is(err, vulnfeed.ErrBadYear), errors.Is(err, vulnfeed.ErrBadImport):
		writeJSONError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, vulnfeed.ErrNotReady):
		writeJSONError(w, http.StatusServiceUnavailable, err.Error())
	default:
		writeJSONError(w, http.StatusInternalServerError, "The file could not be imported.")
	}
}
