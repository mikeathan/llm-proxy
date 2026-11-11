package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/netip"
	"runtime"
	"strings"

	"llm-proxy/internal/core/netaudit/traceroute"
	"llm-proxy/models"
)

// maxRouteTraceBody bounds the request: one address.
const maxRouteTraceBody = 1 << 10

// RouteTracer is what the handlers use: the trace manager.
type RouteTracer interface {
	Start(netip.Addr) error
	Traces() []models.RouteTrace
}

// NetauditRouteTraceHandlers serve the route trace (Constitution I.14): the operator presses for one internet address the last traffic look
// saw, and the screen polls the kept traces while it fills in.
type NetauditRouteTraceHandlers struct {
	tracer RouteTracer
}

func NewNetauditRouteTraceHandlers(t RouteTracer) *NetauditRouteTraceHandlers {
	return &NetauditRouteTraceHandlers{tracer: t}
}

type routeTracesView struct {
	// Supported says this system can send the probes without privilege (Linux only).
	Supported bool                `json:"supported"`
	Traces    []models.RouteTrace `json:"traces"`
}

type routeTraceRequest struct {
	Address string `json:"address"`
}

func (h *NetauditRouteTraceHandlers) view() routeTracesView {
	return routeTracesView{Supported: runtime.GOOS == "linux", Traces: h.tracer.Traces()}
}

// Get answers the kept traces, newest first.
func (h *NetauditRouteTraceHandlers) Get(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, h.view())
}

// Start begins a trace and answers the kept traces with it running. A refusal says why in the words of the error, which are fixed.
func (h *NetauditRouteTraceHandlers) Start(w http.ResponseWriter, r *http.Request) {
	var req routeTraceRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRouteTraceBody)).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "the request must name one address")
		return
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(req.Address))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "that is not an address")
		return
	}
	if err := h.tracer.Start(addr); err != nil {
		writeJSONError(w, routeTraceStatus(err), err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(h.view())
}

// routeTraceStatus is the HTTP status of a refusal: busy is a conflict, anything about the address or the settings is forbidden.
func routeTraceStatus(err error) int {
	switch {
	case errors.Is(err, traceroute.ErrBusy):
		return http.StatusConflict
	case errors.Is(err, traceroute.ErrNotRunning):
		return http.StatusServiceUnavailable
	case errors.Is(err, traceroute.ErrNotInternet):
		return http.StatusBadRequest
	}
	return http.StatusForbidden
}
