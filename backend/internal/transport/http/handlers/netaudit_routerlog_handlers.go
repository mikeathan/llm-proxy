package handlers

import (
	"net/http"

	"llm-proxy/internal/core/netaudit/routerlog"
)

// RouterLogRuntime is what the listener is doing: *routerlog.Manager.
type RouterLogRuntime interface {
	Status() routerlog.Status
	Poke()
}

// NetauditRouterLogHandlers serve the router log listener's switch and status.
type NetauditRouterLogHandlers struct {
	control SwitchControl
	runtime RouterLogRuntime
}

func NewNetauditRouterLogHandlers(control SwitchControl, runtime RouterLogRuntime) *NetauditRouterLogHandlers {
	return &NetauditRouterLogHandlers{control: control, runtime: runtime}
}

type routerLogResponse struct {
	Enabled bool `json:"enabled"`
	routerlog.Status
	// Port is the unprivileged port the program listens on; the router's own setting (514) is redirected to it.
	Port int `json:"port"`
}

// Get returns the switch, whether the program listens, the trouble opening the port, and when the router was last heard.
func (h *NetauditRouterLogHandlers) Get(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, routerLogResponse{Enabled: h.control.Enabled(), Status: h.runtime.Status(), Port: routerlog.Port})
}

// Put switches the listener on or off. Switching on opens a listening socket, so it is a step-up route.
func (h *NetauditRouterLogHandlers) Put(w http.ResponseWriter, r *http.Request) {
	var req switchState
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
