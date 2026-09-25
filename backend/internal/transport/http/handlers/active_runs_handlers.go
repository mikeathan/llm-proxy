package handlers

import (
	"net/http"

	"llm-proxy/internal/core/runlane"
	"llm-proxy/models"
)

// ActiveRunsResponse aggregates the authoritative "currently executing" state
// for a workspace. It is the single ground-truth source the frontend polls to
// drive "running" notifications (assistant glow, automation indicators, future
// surfaces) instead of trusting sticky client-side flags that can miss a
// completion event.
//
// Run-scheduler lane state is deliberately NOT here: the lane snapshot is
// global, so it would be the same bytes for every workspace. It lives only on
// GlobalActiveRunsResponse.
type ActiveRunsResponse struct {
	AssistantRunning  bool `json:"assistant_running"`
	AutomationRunning bool `json:"automation_running"`
	// AssistantConversationID is the conversation ID of the agent currently
	// running for the workspace, or "" when none is running. The frontend uses
	// it to mark the correct history row as running after a refresh — the
	// per-session flag itself is not persisted.
	AssistantConversationID string `json:"assistant_conversation_id"`
	// AssistantQueued is true while a chat run is registered but not yet
	// admitted to the run scheduler (assistant_running stays true while it
	// waits), so the UI can show "waiting" instead of a running glow.
	AssistantQueued bool `json:"assistant_queued"`
}

// GlobalActiveRunsResponse is the workspace-independent slice of the active
// state: the run-scheduler lanes across every workspace. It backs the
// always-visible header indicator, which has no workspace context to poll.
// Every run occupies a lane (automations via Submit, chat via ClaimInteractive),
// so the snapshot is a complete "something is running, anywhere" signal.
type GlobalActiveRunsResponse struct {
	LaneHolders []runlane.Holder `json:"lane_holders,omitempty"`
	Queued      []runlane.Entry  `json:"queued,omitempty"`
}

// QueueKeyParam is the path parameter naming a queued inbound caller; the key
// comes from the run-scheduler gate, so the UI can act on a row it listed.
const QueueKeyParam = "queue_key"

// ActiveRunsSources groups the authoritative running-state sources the handler
// delegates to. All funcs are read-only and must be safe for concurrent use,
// except the two operator actions, which mutate scheduler state on purpose.
type ActiveRunsSources struct {
	AssistantRunning        func(workspaceID string) bool
	AutomationRunning       func(workspaceID string) bool
	AssistantConversationID func(workspaceID string) string
	AssistantQueued         func(workspaceID string) bool
	LaneSnapshot            func() runlane.Snapshot
	// PromoteModelWait serves a queued inbound caller by cancelling the run that
	// holds the model it waits for (the waiter is granted once that run
	// unwinds). CancelModelWait drops a queued caller outright.
	PromoteModelWait func(key string) error
	CancelModelWait  func(key string) bool
}

// ActiveRunsHandler reports what is executing per workspace by delegating to the
// authoritative running-state sources of each subsystem, keeping the handler
// decoupled and trivially testable.
type ActiveRunsHandler struct {
	sources ActiveRunsSources
}

func NewActiveRunsHandler(sources ActiveRunsSources) *ActiveRunsHandler {
	return &ActiveRunsHandler{sources: sources}
}

// ServeHTTP returns the aggregated running state for the requested workspace.
func (h *ActiveRunsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	vals, ok := requirePathParams(w, r, models.WorkspaceIDParam)
	if !ok {
		return
	}
	workspaceID := vals[0]

	// Every source is optional so a partially wired handler (tests, future
	// surfaces) reads as "not running" instead of panicking on a nil func.
	resp := ActiveRunsResponse{
		AssistantRunning:        callBool(h.sources.AssistantRunning, workspaceID),
		AutomationRunning:       callBool(h.sources.AutomationRunning, workspaceID),
		AssistantConversationID: callString(h.sources.AssistantConversationID, workspaceID),
		AssistantQueued:         callBool(h.sources.AssistantQueued, workspaceID),
	}

	respondJSON(w, resp)
}

// callBool invokes fn when wired, defaulting to false otherwise.
func callBool(fn func(string) bool, workspaceID string) bool {
	if fn == nil {
		return false
	}
	return fn(workspaceID)
}

// callString invokes fn when wired, defaulting to "" otherwise.
func callString(fn func(string) string, workspaceID string) string {
	if fn == nil {
		return ""
	}
	return fn(workspaceID)
}

// ServeGlobalHTTP returns the run-scheduler state across all workspaces. No
// workspace path parameter is required because the lane snapshot is inherently
// global — this is the only route that carries lane state, so the UI polls it
// once for the always-visible header indicator and reuses it for the per-chat
// "waiting for a lane" hint.
func (h *ActiveRunsHandler) ServeGlobalHTTP(w http.ResponseWriter, r *http.Request) {
	holders, queued := h.laneState()
	respondJSON(w, GlobalActiveRunsResponse{LaneHolders: holders, Queued: queued})
}

// ServePromoteHTTP serves a queued inbound caller now, by cancelling the run
// holding the model it waits for. This is the operator's "push it forward": the
// only path that evicts, and never silent — the caller is granted once that run
// unwinds, and the run's own cancellation is reported through its normal state.
func (h *ActiveRunsHandler) ServePromoteHTTP(w http.ResponseWriter, r *http.Request) {
	key, ok := h.queueKey(w, r)
	if !ok {
		return
	}
	if h.sources.PromoteModelWait == nil {
		http.Error(w, "inbound queue is not enabled", http.StatusServiceUnavailable)
		return
	}
	if err := h.sources.PromoteModelWait(key); err != nil {
		// No queued caller with that key: it was already served, cancelled, or
		// its wait expired.
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	respondJSON(w, map[string]string{"status": "promoted", "key": key})
}

// ServeCancelHTTP drops a queued inbound caller without serving it.
func (h *ActiveRunsHandler) ServeCancelHTTP(w http.ResponseWriter, r *http.Request) {
	key, ok := h.queueKey(w, r)
	if !ok {
		return
	}
	if h.sources.CancelModelWait == nil {
		http.Error(w, "inbound queue is not enabled", http.StatusServiceUnavailable)
		return
	}
	if !h.sources.CancelModelWait(key) {
		http.Error(w, "no queued caller with that key", http.StatusNotFound)
		return
	}
	respondJSON(w, map[string]string{"status": "cancelled", "key": key})
}

// queueKey reads the caller's queue key from the path.
func (h *ActiveRunsHandler) queueKey(w http.ResponseWriter, r *http.Request) (string, bool) {
	vals, ok := requirePathParams(w, r, QueueKeyParam)
	if !ok {
		return "", false
	}
	return vals[0], true
}

// laneState flattens the scheduler's per-lane read model into the two flat
// lists the UI consumes. Nil when no snapshot source is wired; empty when every
// lane is idle.
func (h *ActiveRunsHandler) laneState() (holders []runlane.Holder, queued []runlane.Entry) {
	if h.sources.LaneSnapshot == nil {
		return nil, nil
	}
	snap := h.sources.LaneSnapshot()
	for _, lane := range snap.Lanes {
		holders = append(holders, lane.Holders...)
		queued = append(queued, lane.Queued...)
	}
	// Inbound callers are rows too: holders are being served by the local model
	// right now, waiters are queued for it. The waiters are what the operator
	// acts on (promote / cancel), and both explain a busy answer to an external
	// client. Positions are relative to their own queue (Kind distinguishes).
	holders = append(holders, snap.ModelHolders...)
	queued = append(queued, snap.ModelWaiters...)
	return holders, queued
}
