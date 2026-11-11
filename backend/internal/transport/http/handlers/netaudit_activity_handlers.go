package handlers

import (
	"net/http"
	"net/url"
	"strconv"
	"time"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/core/netaudit/findingsview"
	"llm-proxy/models"
)

// activityDays is how much of the daily history the timeline reads.
const activityDays = 30

// NetauditActivityHandlers serve the Network page's Activity tab: one timeline built from records the program already keeps.
// A request starts no scan and no probe, and keeps nothing.
type NetauditActivityHandlers struct {
	devices   DeviceInventory
	alerts    NetworkAlertLog
	series    OverviewSeries
	baselines NetauditBaselines
	lifeStore findingsview.Life // optional: without it no finding's recheck or return is a row
	now       func() time.Time
}

// WithLife adds the findings' history, so a recheck's fix and a return become timeline rows.
func (h *NetauditActivityHandlers) WithLife(life findingsview.Life) *NetauditActivityHandlers {
	h.lifeStore = life
	return h
}

// life is the workspace's finding history; none when the store is absent or unreadable (a note, not part of the timeline's core).
func (h *NetauditActivityHandlers) life(workspaceID string) []models.FindingLife {
	if h.lifeStore == nil || workspaceID == "" {
		return nil
	}
	records, err := h.lifeStore.ReadLife(workspaceID)
	if err != nil {
		return nil
	}
	return records
}

func NewNetauditActivityHandlers(devices DeviceInventory, alerts NetworkAlertLog, series OverviewSeries, baselines NetauditBaselines) *NetauditActivityHandlers {
	return &NetauditActivityHandlers{devices: devices, alerts: alerts, series: series, baselines: baselines, now: time.Now}
}

// Get returns the timeline. limit is how many rows (a positive number; the server bounds it), and workspace_id with automation
// name the audit whose last result may add a row.
func (h *NetauditActivityHandlers) Get(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := 0
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeJSONError(w, http.StatusBadRequest, "limit must be a positive number")
			return
		}
		limit = n
	}
	activity, err := h.build(q, limit)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, activity)
}

// build reads the records the timeline is made of and joins them. q carries the audit (workspace_id and automation).
func (h *NetauditActivityHandlers) build(q url.Values, limit int) (netaudit.Activity, error) {
	inv, err := h.devices.View()
	if err != nil {
		return netaudit.Activity{}, err
	}
	alerts, err := h.alerts.After(0, netaudit.MaxActivityRows)
	if err != nil {
		return netaudit.Activity{}, err
	}
	series, err := h.series.Series(activityDays)
	if err != nil {
		return netaudit.Activity{}, err
	}
	var base models.ScanBaseline
	if ws, automation := q.Get("workspace_id"), q.Get("automation"); ws != "" && automation != "" {
		if base, err = h.baselines.ReadBaseline(ws, automation); err != nil {
			return netaudit.Activity{}, err
		}
	}
	return netaudit.ActivityOf(netaudit.ActivityInput{Now: h.now(), Inventory: inv, Alerts: alerts, Series: series, Baseline: base, Life: h.life(q.Get("workspace_id"))}, limit), nil
}
