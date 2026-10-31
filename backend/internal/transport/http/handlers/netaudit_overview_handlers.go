package handlers

import (
	"net/http"
	"strconv"
	"time"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/models"
)

// maxOverviewDays bounds the trend the Network page may ask for; defaultOverviewDays is what it gets without asking.
const (
	defaultOverviewDays = 30
	maxOverviewDays     = 400
)

// OverviewSeries is the daily history. *netaudit.History satisfies it.
type OverviewSeries interface {
	Series(n int) ([]models.NetworkDay, error)
}

// NetauditOverviewHandlers serve the Network page's first screen. Everything in it is read from data that already exists;
// a request starts no scan and no probe.
type NetauditOverviewHandlers struct {
	devices   DeviceInventory
	presence  PresenceControl
	series    OverviewSeries
	baselines NetauditBaselines
	now       func() time.Time
}

func NewNetauditOverviewHandlers(devices DeviceInventory, presence PresenceControl, series OverviewSeries, baselines NetauditBaselines) *NetauditOverviewHandlers {
	return &NetauditOverviewHandlers{devices: devices, presence: presence, series: series, baselines: baselines, now: time.Now}
}

// Get returns the overview. workspace_id and automation name the audit whose last result is counted; without them the
// overview has no findings.
func (h *NetauditOverviewHandlers) Get(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	days := defaultOverviewDays
	if v := q.Get("days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeJSONError(w, http.StatusBadRequest, "days must be a positive number")
			return
		}
		days = min(n, maxOverviewDays)
	}
	inv, err := h.devices.View()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var base models.ScanBaseline
	if ws, automation := q.Get("workspace_id"), q.Get("automation"); ws != "" && automation != "" {
		if base, err = h.baselines.ReadBaseline(ws, automation); err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	series, err := h.series.Series(days)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, netaudit.OverviewOf(netaudit.OverviewInput{Now: h.now(), Inventory: inv, Baseline: base, Presence: h.presence.Status(), Series: series}))
}
