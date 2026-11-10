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
	paths     func() bool
	touches   DecoyTouches
	now       func() time.Time
}

func NewNetauditOverviewHandlers(devices DeviceInventory, presence PresenceControl, series OverviewSeries, baselines NetauditBaselines) *NetauditOverviewHandlers {
	return &NetauditOverviewHandlers{devices: devices, presence: presence, series: series, baselines: baselines, now: time.Now}
}

// WithAttackPaths has the overview list the attack paths while enabled returns true, reading the decoys' touches for the entry.
func (h *NetauditOverviewHandlers) WithAttackPaths(enabled func() bool, touches DecoyTouches) *NetauditOverviewHandlers {
	h.paths, h.touches = enabled, touches
	return h
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
	var acks []models.ScanAck
	if ws, automation := q.Get("workspace_id"), q.Get("automation"); ws != "" && automation != "" {
		if base, err = h.baselines.ReadBaseline(ws, automation); err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		acks = h.acceptances(ws)
	}
	series, err := h.series.Series(days)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	in := netaudit.OverviewInput{Now: h.now(), Inventory: inv, Baseline: base, Acks: acks, Presence: h.presence.Status(), Series: series, Life: h.lifeOf(q.Get("workspace_id"))}
	if h.paths != nil && h.paths() {
		in.AttackPaths = true
		if h.touches != nil {
			in.Touches, _ = h.touches.Read() // unreadable touches only mean the entry is not called "already probing"
		}
	}
	respondJSON(w, netaudit.OverviewOf(in))
}

// lifeOf is the workspace's finding history, when the store keeps it; unreadable history only means no standing is shown.
func (h *NetauditOverviewHandlers) lifeOf(workspaceID string) []models.FindingLife {
	reader, ok := h.baselines.(interface {
		ReadLife(workspaceID string) ([]models.FindingLife, error)
	})
	if !ok || workspaceID == "" {
		return nil
	}
	life, err := reader.ReadLife(workspaceID)
	if err != nil {
		return nil
	}
	return life
}

// acceptances are the workspace's accepted findings, when the store keeps them. Acceptances that cannot be read hide nothing.
func (h *NetauditOverviewHandlers) acceptances(workspaceID string) []models.ScanAck {
	reader, ok := h.baselines.(interface {
		ReadAcks(workspaceID string) ([]models.ScanAck, error)
	})
	if !ok {
		return nil
	}
	acks, err := reader.ReadAcks(workspaceID)
	if err != nil {
		return nil
	}
	return acks
}
