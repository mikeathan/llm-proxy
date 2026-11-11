package handlers

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"llm-proxy/internal/core/netaudit/footprint"
)

// FootprintControl is the footprint as the handlers use it: *footprint.Manager.
type FootprintControl interface {
	View() (footprint.View, error)
	Apply(c footprint.Command) error
	Check(ctx context.Context, label string) error
	SetKey(key string) error
	ClearKey() error
}

// checkFootprintLimit bounds one operator-pressed look: it asks up to three services, each with its own time limit.
const checkFootprintLimit = 2 * time.Minute

// NetauditFootprintHandlers serve the Footprint page: the watched domains and addresses, which outside services may be asked and how
// often, the latest answers, the breach key (saved or cleared, never read back) and "Check now".
type NetauditFootprintHandlers struct{ footprint FootprintControl }

func NewNetauditFootprintHandlers(f FootprintControl) *NetauditFootprintHandlers {
	return &NetauditFootprintHandlers{footprint: f}
}

// Get answers the page. It asks nothing outside.
func (h *NetauditFootprintHandlers) Get(w http.ResponseWriter, _ *http.Request) {
	h.respond(w)
}

func (h *NetauditFootprintHandlers) respond(w http.ResponseWriter) {
	v, err := h.footprint.View()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "the footprint could not be read")
		return
	}
	respondJSON(w, v)
}

// Put makes one settings change (add or remove a domain or address, a frequency, an outside service on or off). It is a step-up
// route: a switch sends the watched names to an outside service.
func (h *NetauditFootprintHandlers) Put(w http.ResponseWriter, r *http.Request) {
	var c footprint.Command
	if !decodeJSONBody(w, r, &c) {
		return
	}
	switch err := h.footprint.Apply(c); {
	case err == nil:
		h.respond(w)
	case errors.Is(err, footprint.ErrNoSuchWatch):
		writeJSONError(w, http.StatusNotFound, err.Error())
	default:
		writeJSONError(w, http.StatusBadRequest, err.Error())
	}
}

type footprintKey struct {
	Key string `json:"key"`
}

// PutKey saves the breach service key. It is a secret, so the route is step-up and the key is never answered back.
func (h *NetauditFootprintHandlers) PutKey(w http.ResponseWriter, r *http.Request) {
	var req footprintKey
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if err := h.footprint.SetKey(req.Key); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.respond(w)
}

// DeleteKey removes the saved key.
func (h *NetauditFootprintHandlers) DeleteKey(w http.ResponseWriter, _ *http.Request) {
	if err := h.footprint.ClearKey(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "the key could not be removed")
		return
	}
	h.respond(w)
}

type footprintCheck struct {
	Label string `json:"label"`
}

// Check looks at one watch now and answers the page.
func (h *NetauditFootprintHandlers) Check(w http.ResponseWriter, r *http.Request) {
	var req footprintCheck
	if !decodeJSONBody(w, r, &req) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), checkFootprintLimit)
	defer cancel()
	var cool *footprint.CooldownError
	switch err := h.footprint.Check(ctx, req.Label); {
	case err == nil:
		h.respond(w)
	case errors.Is(err, footprint.ErrNoSuchWatch):
		writeJSONError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, footprint.ErrBusy):
		writeJSONError(w, http.StatusConflict, "A look is already running. Try again when it has finished.")
	case errors.As(err, &cool):
		w.Header().Set("Retry-After", strconv.Itoa(int(cool.Remaining/time.Second)+1))
		writeJSONError(w, http.StatusTooManyRequests, err.Error())
	default:
		writeJSONError(w, http.StatusInternalServerError, "the look could not be made")
	}
}
