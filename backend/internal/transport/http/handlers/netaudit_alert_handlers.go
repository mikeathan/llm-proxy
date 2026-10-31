package handlers

import (
	"net/http"
	"strconv"

	"llm-proxy/internal/platform/logging"
	"llm-proxy/models"
)

const (
	alertEventsDefaultLimit = 20
	// alertEventsMaxLimit is the size of the log itself, so no request can ask for more than there is.
	alertEventsMaxLimit = 50
)

// NetworkAlertLog is the Network alert log: *netaudit.AlertLog.
type NetworkAlertLog interface {
	After(id int64, limit int) ([]models.NetworkAlert, error)
	Latest() (int64, error)
}

// NetauditAlertHandlers tell the web UI what the Network module has alerted about.
type NetauditAlertHandlers struct{ log NetworkAlertLog }

func NewNetauditAlertHandlers(log NetworkAlertLog) *NetauditAlertHandlers {
	return &NetauditAlertHandlers{log: log}
}

type alertView struct {
	ID       int64  `json:"id"`
	Kind     string `json:"kind"`
	Message  string `json:"message"`
	Delivery string `json:"delivery"`
	Count    int    `json:"count"`
	// At is when the alert was raised, in unix milliseconds.
	At int64 `json:"at"`
}

// Events handles GET /admin/api/netaudit/alerts?after=<id>&limit=<n>: the entries newer than the cursor, newest first
// (the newest n when more exist), and the cursor to ask from next (always the log's own newest id, so a client holding a
// stale one after a reset recovers). With no cursor it returns no items, only the newest id, so a client that has just
// opened announces what happens from then on and not the history.
func (h *NetauditAlertHandlers) Events(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := alertEventsDefaultLimit
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > alertEventsMaxLimit {
			writeJSONError(w, http.StatusBadRequest, "limit must be a number from 1 to "+strconv.Itoa(alertEventsMaxLimit))
			return
		}
		limit = n
	}
	latest, err := h.log.Latest()
	if err != nil {
		logging.Warn("network alerts failed", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to read the network alerts")
		return
	}
	items := []alertView{}
	if raw := q.Get("after"); raw != "" {
		after, perr := strconv.ParseInt(raw, 10, 64)
		if perr != nil || after < 0 {
			writeJSONError(w, http.StatusBadRequest, "after must be an alert id")
			return
		}
		found, ferr := h.log.After(after, limit)
		if ferr != nil {
			logging.Warn("network alerts failed", "error", ferr)
			writeJSONError(w, http.StatusInternalServerError, "failed to read the network alerts")
			return
		}
		for _, a := range found {
			items = append(items, alertView{ID: a.ID, Kind: string(a.Kind), Message: a.Message, Delivery: string(a.Delivery), Count: a.Count, At: a.At.UnixMilli()})
		}
	}
	respondJSON(w, map[string]any{"cursor": latest, "items": items})
}
