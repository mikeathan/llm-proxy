package handlers

import (
	"errors"
	"net/http"

	"llm-proxy/internal/core/netaudit"
	"llm-proxy/internal/core/netaudit/nearby"
)

// NearbyControl is the Nearby switch: *netaudit.NearbyConfig.
type NearbyControl interface {
	Set(enabled bool) error
}

// NearbyLooks asks for a look and reads what the looks kept: the collector, joined with the device list by the application.
type NearbyLooks interface {
	Request() error
	View() (netaudit.NearbyView, error)
}

// NetauditNearbyHandlers serve the Nearby card: the devices that said what they are, the switch, and Check now.
type NetauditNearbyHandlers struct {
	control NearbyControl
	looks   NearbyLooks
}

func NewNetauditNearbyHandlers(control NearbyControl, looks NearbyLooks) *NetauditNearbyHandlers {
	return &NetauditNearbyHandlers{control: control, looks: looks}
}

// Get returns the switch, whether a look is running, the latest outcome and the devices.
func (h *NetauditNearbyHandlers) Get(w http.ResponseWriter, _ *http.Request) {
	view, err := h.looks.View()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, view)
}

type nearbyRequest struct {
	Enabled *bool `json:"enabled"`
}

// Put switches Nearby on or off. Switching it on lets the host send two multicast questions and read the answers, so it is a
// step-up route (Constitution I.9).
func (h *NetauditNearbyHandlers) Put(w http.ResponseWriter, r *http.Request) {
	var req nearbyRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Enabled == nil {
		writeJSONError(w, http.StatusBadRequest, "enabled is required")
		return
	}
	if err := h.control.Set(*req.Enabled); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.Get(w, r)
}

// Look starts a look and answers at once; the card reads the result when it ends. The refusals are checked on the server: a
// second look while one runs, or within seconds of the last, is not started.
func (h *NetauditNearbyHandlers) Look(w http.ResponseWriter, _ *http.Request) {
	switch err := h.looks.Request(); {
	case errors.Is(err, nearby.ErrOff):
		writeJSONError(w, http.StatusConflict, "Nearby is switched off. Switch it on first.")
		return
	case errors.Is(err, nearby.ErrBusy):
		writeJSONError(w, http.StatusConflict, "A look is already running.")
		return
	case errors.Is(err, nearby.ErrTooSoon):
		writeJSONError(w, http.StatusTooManyRequests, "The last look was moments ago. Wait a few seconds and try again.")
		return
	case err != nil:
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	view, err := h.looks.View()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusAccepted)
	respondJSON(w, view)
}
