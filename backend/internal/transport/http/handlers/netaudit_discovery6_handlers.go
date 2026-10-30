package handlers

import "net/http"

// Discovery6Control is the switch for active IPv6 discovery: *netaudit.Discovery6Config.
type Discovery6Control interface {
	Enabled() bool
	Set(enabled bool) error
}

// Discovery6Status says why the last discovery did not work: *netaudit.Inventory.
type Discovery6Status interface {
	Discovery6Problem() string
}

// NetauditDiscovery6Handlers serve the switch for active IPv6 discovery and its last problem, as a code the screen words.
type NetauditDiscovery6Handlers struct {
	control Discovery6Control
	status  Discovery6Status
}

func NewNetauditDiscovery6Handlers(control Discovery6Control, status Discovery6Status) *NetauditDiscovery6Handlers {
	return &NetauditDiscovery6Handlers{control: control, status: status}
}

type discovery6Response struct {
	Enabled bool   `json:"enabled"`
	Problem string `json:"problem,omitempty"`
}

// Get returns the switch and the problem code of the last discovery, if there was one.
func (h *NetauditDiscovery6Handlers) Get(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, discovery6Response{Enabled: h.control.Enabled(), Problem: h.status.Discovery6Problem()})
}

type discovery6Request struct {
	Enabled bool `json:"enabled"`
}

// Put switches the discovery on or off. It sends one fixed link-local packet per look, so an ordinary signed-in request is
// enough; it opens no listening socket.
func (h *NetauditDiscovery6Handlers) Put(w http.ResponseWriter, r *http.Request) {
	var req discovery6Request
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if err := h.control.Set(req.Enabled); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.Get(w, r)
}
