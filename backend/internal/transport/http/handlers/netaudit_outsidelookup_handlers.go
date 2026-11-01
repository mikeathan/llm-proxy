package handlers

import (
	"net/http"

	"llm-proxy/internal/core/netaudit"
)

// OutsideLookupControl is the setting for the outside lookup: *netaudit.OutsideLookupConfig.
type OutsideLookupControl interface {
	Enabled() bool
	URL() string
	DefaultURL() string
	Set(enabled bool) error
	SetURL(raw string) error
}

// OutsideLookupStatus is what the last look did: *netaudit.Exposure.
type OutsideLookupStatus interface {
	Status() netaudit.ExposureStatus
}

// NetauditOutsideLookupHandlers serve the switch for asking an outside scan database what the internet can reach, the
// service it asks, and what the last look did.
type NetauditOutsideLookupHandlers struct {
	control OutsideLookupControl
	status  OutsideLookupStatus
}

func NewNetauditOutsideLookupHandlers(control OutsideLookupControl, status OutsideLookupStatus) *NetauditOutsideLookupHandlers {
	return &NetauditOutsideLookupHandlers{control: control, status: status}
}

type outsideLookupResponse struct {
	Enabled    bool                    `json:"enabled"`
	URL        string                  `json:"url"`
	DefaultURL string                  `json:"default_url"`
	Last       netaudit.ExposureStatus `json:"last"`
}

type outsideLookupRequest struct {
	Enabled *bool   `json:"enabled"`
	URL     *string `json:"url"`
}

// Get returns the switch, the service and the last look.
func (h *NetauditOutsideLookupHandlers) Get(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, outsideLookupResponse{Enabled: h.control.Enabled(), URL: h.control.URL(), DefaultURL: h.control.DefaultURL(), Last: h.status.Status()})
}

// Put changes what is sent: the switch, the service address, or both. The address is checked first, so a refused one changes
// nothing. Switching on lets the audit send the network's public address to the service, so it is a step-up route.
func (h *NetauditOutsideLookupHandlers) Put(w http.ResponseWriter, r *http.Request) {
	var req outsideLookupRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.URL != nil {
		if err := h.control.SetURL(*req.URL); err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if req.Enabled != nil {
		if err := h.control.Set(*req.Enabled); err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	h.Get(w, r)
}
