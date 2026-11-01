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
	Address() string
	SetAddress(raw string) error
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
	Enabled    bool   `json:"enabled"`
	URL        string `json:"url"`
	DefaultURL string `json:"default_url"`
	// Address is the public address the operator entered, or empty.
	Address string                  `json:"address"`
	Last    netaudit.ExposureStatus `json:"last"`
}

type outsideLookupRequest struct {
	Enabled *bool   `json:"enabled"`
	URL     *string `json:"url"`
	Address *string `json:"address"`
}

// Get returns the switch, the service and the last look.
func (h *NetauditOutsideLookupHandlers) Get(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, outsideLookupResponse{Enabled: h.control.Enabled(), URL: h.control.URL(), DefaultURL: h.control.DefaultURL(), Address: h.control.Address(), Last: h.status.Status()})
}

// Put changes what is sent: the switch, the service address, the entered public address, or any of them. The address is checked first, so a refused one changes
// nothing. Switching on lets the audit send the network's public address to the service, so it is a step-up route.
func (h *NetauditOutsideLookupHandlers) Put(w http.ResponseWriter, r *http.Request) {
	var req outsideLookupRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Address != nil {
		if err := h.control.SetAddress(*req.Address); err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
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
