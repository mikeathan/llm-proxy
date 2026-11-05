package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"llm-proxy/internal/core/netaudit/knownbad"
)

// downloadKnownBadLimit bounds the operator's download of the list.
const downloadKnownBadLimit = time.Minute

// KnownBadList is what the handlers use: the list and the operator's saved address for it. The address holds the operator's key, so it
// is only ever set or cleared, never read back.
type KnownBadList interface {
	Status() knownbad.Status
	HasAddress() bool
	SetAddress(address string) error
	ClearAddress() error
	Download(ctx context.Context) error
	Ready() (bool, string)
}

// NetauditKnownBadHandlers serve the list of malware host names (abuse.ch URLhaus): what is held, whether an address is saved, saving and
// removing the address (a step-up route, because it is a secret), and the operator's download.
type NetauditKnownBadHandlers struct{ list KnownBadList }

func NewNetauditKnownBadHandlers(l KnownBadList) *NetauditKnownBadHandlers {
	return &NetauditKnownBadHandlers{list: l}
}

type knownBadView struct {
	knownbad.Status
	HasAddress bool   `json:"has_address"`
	Ready      bool   `json:"ready"`
	Reason     string `json:"reason,omitempty"`
}

func (h *NetauditKnownBadHandlers) view() knownBadView {
	ready, why := h.list.Ready()
	return knownBadView{Status: h.list.Status(), HasAddress: h.list.HasAddress(), Ready: ready, Reason: why}
}

// Get answers what is held and what is missing, never the saved address.
func (h *NetauditKnownBadHandlers) Get(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, h.view())
}

type knownBadAddress struct {
	Address string `json:"address"`
}

// PutAddress saves the operator's address for the list. It must be an https address on abuse.ch.
func (h *NetauditKnownBadHandlers) PutAddress(w http.ResponseWriter, r *http.Request) {
	var req knownBadAddress
	if !decodeJSONBody(w, r, &req) {
		return
	}
	address := strings.TrimSpace(req.Address)
	if err := knownbad.ValidAddress(address); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.list.SetAddress(address); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "the address could not be saved")
		return
	}
	respondJSON(w, h.view())
}

// DeleteAddress removes the saved address. The list already downloaded stays until the operator replaces it.
func (h *NetauditKnownBadHandlers) DeleteAddress(w http.ResponseWriter, _ *http.Request) {
	if err := h.list.ClearAddress(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "the address could not be removed")
		return
	}
	respondJSON(w, h.view())
}

// Download fetches the list now with the saved address and answers with the new status.
func (h *NetauditKnownBadHandlers) Download(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), downloadKnownBadLimit)
	defer cancel()
	if err := h.list.Download(ctx); err != nil {
		writeJSONError(w, http.StatusBadGateway, err.Error())
		return
	}
	respondJSON(w, h.view())
}
