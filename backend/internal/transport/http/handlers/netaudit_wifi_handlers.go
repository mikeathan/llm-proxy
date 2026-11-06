package handlers

import (
	"context"
	"net/http"

	"llm-proxy/internal/core/netaudit"
)

// WifiService is what the Wi-Fi look-alikes card needs: *netaudit.WifiWatch for the look, the card's state and the confirmation, and
// *netaudit.WifiConfig for the operator's names. The switch itself is a module on the Modules tab.
type WifiService interface {
	View() netaudit.WifiView
	Look(ctx context.Context) error
	Confirm(name string, addresses []string) error
}

// WifiNames are the operator's network names.
type WifiNames interface {
	AddNetwork(name string) error
	RemoveNetwork(name string) error
}

// NetauditWifiHandlers serve the Wi-Fi look-alikes card.
type NetauditWifiHandlers struct {
	service WifiService
	names   WifiNames
	// ready says whether the look can run and, when it cannot, why in plain words.
	ready func() (bool, string)
}

func NewNetauditWifiHandlers(service WifiService, names WifiNames, ready func() (bool, string)) *NetauditWifiHandlers {
	return &NetauditWifiHandlers{service: service, names: names, ready: ready}
}

type wifiResponse struct {
	netaudit.WifiView
	// Ready is false, with Why, while the look cannot run (nmcli is not installed, no network name is set).
	Ready bool   `json:"ready"`
	Why   string `json:"why,omitempty"`
}

func (h *NetauditWifiHandlers) state() wifiResponse {
	ok, why := h.ready()
	return wifiResponse{WifiView: h.service.View(), Ready: ok, Why: why}
}

// Get returns the switch, each network name with its confirmed access points and what the last look saw.
func (h *NetauditWifiHandlers) Get(w http.ResponseWriter, _ *http.Request) { respondJSON(w, h.state()) }

type wifiNameRequest struct {
	Name string `json:"name"`
}

// AddNetwork adds one of the operator's network names to watch. It sends nothing and trusts nothing new, so it is not a step-up route.
func (h *NetauditWifiHandlers) AddNetwork(w http.ResponseWriter, r *http.Request) {
	var req wifiNameRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if err := h.names.AddNetwork(req.Name); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.Get(w, r)
}

// RemoveNetwork stops watching a name and forgets the access points confirmed for it. It silences a watch, so it is a step-up route.
func (h *NetauditWifiHandlers) RemoveNetwork(w http.ResponseWriter, r *http.Request) {
	if err := h.names.RemoveNetwork(r.URL.Query().Get("name")); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.Get(w, r)
}

// Look reads the access points once now, so the operator can see which advertise their name. It changes nothing.
func (h *NetauditWifiHandlers) Look(w http.ResponseWriter, r *http.Request) {
	if ok, why := h.ready(); !ok {
		writeJSONError(w, http.StatusConflict, why)
		return
	}
	if err := h.service.Look(r.Context()); err != nil {
		writeJSONError(w, http.StatusBadGateway, err.Error())
		return
	}
	h.Get(w, r)
}

type wifiConfirmRequest struct {
	Name      string   `json:"name"`
	Addresses []string `json:"addresses"`
}

// Confirm says these access points are the operator's own. It decides what is trusted, so it is a step-up route; only addresses the last
// look saw advertising that name are taken.
func (h *NetauditWifiHandlers) Confirm(w http.ResponseWriter, r *http.Request) {
	var req wifiConfirmRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if err := h.service.Confirm(req.Name, req.Addresses); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.Get(w, r)
}
