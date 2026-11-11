package handlers

import (
	"context"
	"net/http"
	"time"

	"llm-proxy/internal/core/netaudit/ipowners"
)

// downloadDestinationDataLimit bounds the operator's download of the table.
const downloadDestinationDataLimit = 3 * time.Minute

// DestinationOwners is what the handlers use: a table the Traffic view names destinations with (the address owners, or the places they are used).
type DestinationOwners interface {
	Status() ipowners.Status
	Download(ctx context.Context) error
	Remove() error
}

// NetauditDestinationDataHandlers serve one table of destination data (the address owners, or the places): what is held, the operator's download (a step-up route, because it makes
// this computer ask an outside service), and removing it. Looking an address up never leaves this computer.
type NetauditDestinationDataHandlers struct {
	owners DestinationOwners
	// source is the public service this table comes from, named on the screen so the operator knows who is asked.
	source string
}

func NewNetauditDestinationDataHandlers(o DestinationOwners, source string) *NetauditDestinationDataHandlers {
	return &NetauditDestinationDataHandlers{owners: o, source: source}
}

type destinationDataView struct {
	ipowners.Status
	// Source is the service the table is downloaded from: the only one this computer is ever asked to contact for it.
	Source string `json:"source"`
}

func (h *NetauditDestinationDataHandlers) view() destinationDataView {
	return destinationDataView{Status: h.owners.Status(), Source: h.source}
}

// Get answers what is held.
func (h *NetauditDestinationDataHandlers) Get(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, h.view())
}

// Download fetches the table now and answers with the new status. A failed download keeps the table that was held.
func (h *NetauditDestinationDataHandlers) Download(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), downloadDestinationDataLimit)
	defer cancel()
	if err := h.owners.Download(ctx); err != nil {
		writeJSONError(w, http.StatusBadGateway, err.Error())
		return
	}
	respondJSON(w, h.view())
}

// Delete forgets the table and answers with the empty status.
func (h *NetauditDestinationDataHandlers) Delete(w http.ResponseWriter, _ *http.Request) {
	if err := h.owners.Remove(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "the table could not be removed")
		return
	}
	respondJSON(w, h.view())
}
