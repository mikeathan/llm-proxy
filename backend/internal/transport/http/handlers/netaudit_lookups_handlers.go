package handlers

import (
	"net/http"
	"time"

	"llm-proxy/models"
)

// DeviceLookupsSource is what the screen of lookups reads: the phone-home watch's names, judged. Enabled says the watch is on.
type DeviceLookupsSource interface {
	DeviceLookups(now time.Time) (enabled bool, since time.Time, rows []models.DeviceLookups)
}

// NetauditLookupsHandlers serve what each device on the network asked the router's DNS for (Constitution I.8, DNS query lines). It is text read
// from memory: nothing is looked up, sent or run, and a name is shown as text only.
type NetauditLookupsHandlers struct {
	source func() DeviceLookupsSource
}

// NewNetauditLookupsHandlers takes the source as a function because the routes are registered before the services are wired.
func NewNetauditLookupsHandlers(source func() DeviceLookupsSource) *NetauditLookupsHandlers {
	return &NetauditLookupsHandlers{source: source}
}

type deviceLookupsView struct {
	Enabled bool                   `json:"enabled"`
	Since   *time.Time             `json:"since,omitempty"`
	Devices []models.DeviceLookups `json:"devices"`
}

// Get answers one row per device, flagged devices first.
func (h *NetauditLookupsHandlers) Get(w http.ResponseWriter, _ *http.Request) {
	view := deviceLookupsView{Devices: []models.DeviceLookups{}}
	if src := h.source(); src != nil {
		enabled, since, rows := src.DeviceLookups(time.Now())
		view.Enabled = enabled
		if !since.IsZero() {
			view.Since = &since
		}
		if rows != nil {
			view.Devices = rows
		}
	}
	respondJSON(w, view)
}
