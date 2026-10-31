package handlers

import (
	"net/http"

	"llm-proxy/internal/core/netaudit/decoy"
	"llm-proxy/models"
)

// DecoyControl is the switch for the decoy services: *netaudit.DecoyConfig.
type DecoyControl interface {
	Enabled() bool
	Set(enabled bool) error
}

// DecoyRuntime is what the listeners are doing: *decoy.Manager.
type DecoyRuntime interface {
	Status() decoy.Status
	Poke()
	// Clear empties the list of touches and the alert throttle.
	Clear() error
}

// DecoyTouches reads who connected to a decoy: *persistence.DecoyTouchStore.
type DecoyTouches interface {
	Read() ([]models.DecoyTouch, error)
}

// NetauditDecoyHandlers serve the decoy services' switch, status and the devices that touched them.
type NetauditDecoyHandlers struct {
	control DecoyControl
	runtime DecoyRuntime
	touches DecoyTouches
}

func NewNetauditDecoyHandlers(control DecoyControl, runtime DecoyRuntime, touches DecoyTouches) *NetauditDecoyHandlers {
	return &NetauditDecoyHandlers{control: control, runtime: runtime, touches: touches}
}

type decoyPort struct {
	Number int    `json:"number"`
	Name   string `json:"name"`
}

type decoyResponse struct {
	Enabled bool `json:"enabled"`
	decoy.Status
	Ports   []decoyPort         `json:"ports"`
	Touches []models.DecoyTouch `json:"touches"`
}

// Get returns the switch, which ports listen, the trouble with any that do not, and the recent touches.
func (h *NetauditDecoyHandlers) Get(w http.ResponseWriter, _ *http.Request) {
	touches, err := h.touches.Read()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := decoyResponse{Enabled: h.control.Enabled(), Status: h.runtime.Status(), Ports: make([]decoyPort, 0, len(decoy.Specs)), Touches: touches}
	for _, s := range decoy.Specs {
		resp.Ports = append(resp.Ports, decoyPort{Number: s.Number, Name: s.Name})
	}
	if resp.Touches == nil {
		resp.Touches = []models.DecoyTouch{}
	}
	respondJSON(w, resp)
}

type decoyRequest struct {
	Enabled bool `json:"enabled"`
}

// Put switches the decoys on or off. Switching on opens listening sockets, so it is a step-up route.
func (h *NetauditDecoyHandlers) Put(w http.ResponseWriter, r *http.Request) {
	var req decoyRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if err := h.control.Set(req.Enabled); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.runtime.Poke()
	h.Get(w, r)
}

// Clear empties the list of touches. Nothing else changes: the switch and the listeners stay as they are.
func (h *NetauditDecoyHandlers) Clear(w http.ResponseWriter, r *http.Request) {
	if err := h.runtime.Clear(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.Get(w, r)
}
