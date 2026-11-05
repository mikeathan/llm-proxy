package handlers

import (
	"errors"
	"net/http"

	"llm-proxy/internal/core/stack"
	api "llm-proxy/internal/transport/http"
)

// StackRegistry is what the stack handlers read and switch: *stack.Registry.
type StackRegistry interface {
	Stacks(stalled map[string]bool) []stack.StackSummary
	View(id stack.StackID, stalled map[string]bool) (stack.StackView, error)
	Module(id stack.StackID, module string) (stack.Module, bool)
	Switch(id stack.StackID, module string, enabled bool) (stack.Outcome, error)
}

// StackHandlers serve the stacks and their modules (ADR-0028): the list, one stack with its layers, and the one route that
// switches a module. The passphrase is asked per module here, because a single path cannot be listed per module in the
// middleware's table of step-up routes.
type StackHandlers struct {
	registry StackRegistry
	stalled  func() []string
}

// NewStackHandlers builds the handlers. stalled returns the ids of the monitored parts that have stopped.
func NewStackHandlers(registry StackRegistry, stalled func() []string) *StackHandlers {
	return &StackHandlers{registry: registry, stalled: stalled}
}

func (h *StackHandlers) stalledSet() map[string]bool {
	set := map[string]bool{}
	if h.stalled == nil {
		return set
	}
	for _, id := range h.stalled() {
		set[id] = true
	}
	return set
}

// List answers every stack with its counts.
func (h *StackHandlers) List(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, map[string]any{"stacks": h.registry.Stacks(h.stalledSet())})
}

// Get answers one stack: its layers, top first, with every module's state, health and what blocks it.
func (h *StackHandlers) Get(w http.ResponseWriter, r *http.Request) {
	view, err := h.registry.View(stack.StackID(r.PathValue("stack")), h.stalledSet())
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "no such stack")
		return
	}
	respondJSON(w, view)
}

type moduleRequest struct {
	Enabled *bool `json:"enabled"`
}

// switchedStack is the stack as it is after a switch, and the modules the change left without something they need.
type switchedStack struct {
	stack.StackView
	Blocked []string `json:"blocked"`
}

// PutModule switches one module on or off. A module that needs the passphrase is refused without it before anything is read
// or changed; switching on while a module it needs is off is a conflict that names the need.
func (h *StackHandlers) PutModule(w http.ResponseWriter, r *http.Request) {
	id, moduleID := stack.StackID(r.PathValue("stack")), r.PathValue("id")
	m, ok := h.registry.Module(id, moduleID)
	if !ok {
		writeJSONError(w, http.StatusNotFound, "no such module")
		return
	}
	if m.StepUp && !api.StepUpGranted(r.Context()) {
		api.RefuseStepUp(w, r)
		return
	}
	var req moduleRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Enabled == nil {
		writeJSONError(w, http.StatusBadRequest, "enabled is required")
		return
	}
	out, err := h.registry.Switch(id, moduleID, *req.Enabled)
	switch {
	case errors.Is(err, stack.ErrNeedOff):
		writeJSONError(w, http.StatusConflict, err.Error())
		return
	case errors.Is(err, stack.ErrNotFound):
		writeJSONError(w, http.StatusNotFound, "no such module")
		return
	case err != nil:
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	view, err := h.registry.View(id, h.stalledSet())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, switchedStack{StackView: view, Blocked: out.Blocked})
}
