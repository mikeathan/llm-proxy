package handlers

import "net/http"

// SwitchControl is one on/off setting of the network feature: *netaudit.SummaryConfig, *netaudit.NamesConfig.
type SwitchControl interface {
	Enabled() bool
	Set(enabled bool) error
}

// NetauditSwitchHandlers serve one on/off setting (the weekly summary, asking devices their names). Each is a quiet
// feature that probes nothing beyond what is stated beside its switch, so an ordinary signed-in request is enough.
type NetauditSwitchHandlers struct{ control SwitchControl }

func NewNetauditSwitchHandlers(control SwitchControl) *NetauditSwitchHandlers {
	return &NetauditSwitchHandlers{control: control}
}

type switchState struct {
	Enabled bool `json:"enabled"`
}

// Get returns whether the setting is on.
func (h *NetauditSwitchHandlers) Get(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, switchState{Enabled: h.control.Enabled()})
}

// Put switches the setting on or off and answers with its new state.
func (h *NetauditSwitchHandlers) Put(w http.ResponseWriter, r *http.Request) {
	var req switchState
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if err := h.control.Set(req.Enabled); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.Get(w, r)
}
