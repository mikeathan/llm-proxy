package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"llm-proxy/internal/core/netaudit/routerconn"
	"llm-proxy/internal/platform/routerssh"
	"llm-proxy/models"
)

// RouterConnControl is the router connection table as the handlers use it: *routerconn.Manager.
type RouterConnControl interface {
	View() routerconn.View
	SetTarget(address, user string) error
	SetKey(key string) error
	ClearKey() error
	Probe(ctx context.Context) (string, error)
	Pin(ctx context.Context, confirm string) error
	Read(ctx context.Context) (*models.RouterConnections, error)
}

// routerConnLimit bounds one request that reaches the router.
const routerConnLimit = 30 * time.Second

// NetauditRouterConnHandlers serve the router's connection table (Constitution I.15): the setup (address, user, key, the router's pinned key)
// and the read the operator presses for. Every route that changes the setup or reaches the router is a step-up route; the key is never
// answered back.
type NetauditRouterConnHandlers struct{ control func() RouterConnControl }

// NewNetauditRouterConnHandlers takes the control as a function because the routes are registered before the services are wired.
func NewNetauditRouterConnHandlers(control func() RouterConnControl) *NetauditRouterConnHandlers {
	return &NetauditRouterConnHandlers{control: control}
}

func (h *NetauditRouterConnHandlers) ready(w http.ResponseWriter) RouterConnControl {
	c := h.control()
	if c == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "the router connection table is not available until the service has started")
	}
	return c
}

// Get answers the setup and the last read. It reaches nothing.
func (h *NetauditRouterConnHandlers) Get(w http.ResponseWriter, _ *http.Request) {
	if c := h.ready(w); c != nil {
		respondJSON(w, c.View())
	}
}

type routerTarget struct {
	Address string `json:"address"`
	User    string `json:"user"`
}

// PutTarget saves the router's address and user.
func (h *NetauditRouterConnHandlers) PutTarget(w http.ResponseWriter, r *http.Request) {
	c := h.ready(w)
	var req routerTarget
	if c == nil || !decodeJSONBody(w, r, &req) {
		return
	}
	if err := c.SetTarget(req.Address, req.User); err != nil {
		routerConnFailure(w, err)
		return
	}
	respondJSON(w, c.View())
}

type routerKey struct {
	Key string `json:"key"`
}

// PutKey saves the router key. It is a secret, so the route is step-up and the key is never answered back.
func (h *NetauditRouterConnHandlers) PutKey(w http.ResponseWriter, r *http.Request) {
	c := h.ready(w)
	var req routerKey
	if c == nil || !decodeJSONBody(w, r, &req) {
		return
	}
	if err := c.SetKey(req.Key); err != nil {
		routerConnFailure(w, err)
		return
	}
	respondJSON(w, c.View())
}

// DeleteKey removes the saved key and the last read.
func (h *NetauditRouterConnHandlers) DeleteKey(w http.ResponseWriter, _ *http.Request) {
	c := h.ready(w)
	if c == nil {
		return
	}
	if err := c.ClearKey(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "the key could not be removed")
		return
	}
	respondJSON(w, c.View())
}

type routerFingerprint struct {
	Fingerprint string `json:"fingerprint"`
}

// Fingerprint asks the router for its SSH key, sends no credential and saves nothing, and answers its fingerprint for the operator to compare.
func (h *NetauditRouterConnHandlers) Fingerprint(w http.ResponseWriter, r *http.Request) {
	c := h.ready(w)
	if c == nil {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), routerConnLimit)
	defer cancel()
	fp, err := c.Probe(ctx)
	if err != nil {
		routerConnFailure(w, err)
		return
	}
	respondJSON(w, routerFingerprint{Fingerprint: fp})
}

// Pin saves the router's SSH key when it still has the fingerprint the operator confirmed.
func (h *NetauditRouterConnHandlers) Pin(w http.ResponseWriter, r *http.Request) {
	c := h.ready(w)
	var req routerFingerprint
	if c == nil || !decodeJSONBody(w, r, &req) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), routerConnLimit)
	defer cancel()
	if err := c.Pin(ctx, req.Fingerprint); err != nil {
		routerConnFailure(w, err)
		return
	}
	respondJSON(w, c.View())
}

// Read reads the router's table once and answers the setup with the result.
func (h *NetauditRouterConnHandlers) Read(w http.ResponseWriter, r *http.Request) {
	c := h.ready(w)
	if c == nil {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), routerConnLimit)
	defer cancel()
	if _, err := c.Read(ctx); err != nil {
		routerConnFailure(w, err)
		return
	}
	respondJSON(w, c.View())
}

// routerConnFailure answers a refusal in fixed words. An error that is not one of the program's own says only that the router could not be
// reached or did not accept the key: the text of a network or server error is not passed on.
func routerConnFailure(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, routerconn.ErrBusy):
		writeJSONError(w, http.StatusConflict, err.Error())
	case errors.Is(err, routerssh.ErrHostKeyChanged), errors.Is(err, routerssh.ErrTooMuch):
		writeJSONError(w, http.StatusBadGateway, err.Error())
	case errors.Is(err, routerconn.ErrNotSet), errors.Is(err, routerconn.ErrBadUser), errors.Is(err, routerconn.ErrBadKey), errors.Is(err, routerconn.ErrNotConfirmed),
		errors.Is(err, routerssh.ErrNotPrivate), errors.Is(err, routerssh.ErrNoHostKey):
		writeJSONError(w, http.StatusBadRequest, err.Error())
	default:
		writeJSONError(w, http.StatusBadGateway, "the router could not be reached, or it did not accept the key")
	}
}
