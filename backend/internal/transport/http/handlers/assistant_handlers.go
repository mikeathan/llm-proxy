package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	assistantPkg "llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/automation"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/ratelimiter"
	"llm-proxy/models"
)

type AssistantMessage struct {
	WorkspaceID    string   `json:"workspace_id"`
	ConversationID string   `json:"conversation_id"`
	ContextVersion string   `json:"context_version,omitempty"`
	Message        string   `json:"message"`
	Timezone       string   `json:"timezone,omitempty"`
	ExcludeTools   []string `json:"exclude_tools,omitempty"`
}

type AssistantMessageHandler struct {
	provider    assistantPkg.ToolProvider
	client      proxy.LLMClientProvider
	limiter     ratelimiter.Limiter
	logger      logging.Logger
	engine      assistantPkg.Engine
	guardrails  *guardrails.GuardrailEngine
	persistence *persistence.WorkspaceManager
	svc         AssistantService
	service     assistantPkg.ConversationService
	running     sync.Map
}

type runningAgent struct {
	cancel         context.CancelFunc
	done           chan struct{}
	conversationID string
}

func NewAssistantMessageHandler(service AssistantService) *AssistantMessageHandler {
	return &AssistantMessageHandler{
		provider:    service.ToolProvider(),
		client:      service.ClientProvider(),
		limiter:     service.Limiter(),
		logger:      service.Logger(),
		engine:      service.Engine(),
		guardrails:  service.GuardrailEngine(),
		persistence: service.Persistence(),
		svc:         service,
		service:     assistantPkg.NewConversationService(service, service.Persistence()),
	}
}

// ServeHTTP accepts an assistant message and starts the agent run as a
// detached background job. It returns immediately with 202 Accepted and a
// lightweight status payload; the run outlives this HTTP request and is
// observed by the client over the SSE event bus (reasoning/content stream,
// lifecycle completion). This keeps a run alive across page refreshes and
// client disconnects — only the explicit /assistant/cancel endpoint stops it.
func (h *AssistantMessageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	payload, log, ok := h.prepareRequest(w, r)
	if !ok {
		return
	}

	conversationID := assistantPkg.NormalizeConversationID(payload.ConversationID)
	go func() {
		_, _ = h.RunWithCancel(r.Context(), payload.WorkspaceID, payload, log)
	}()

	w.WriteHeader(http.StatusAccepted)
	respondJSON(w, map[string]any{
		"status": "running",
		// conversationID is resolved BEFORE the run goroutine starts: the goroutine
		// mutates payload.ConversationID (RunWithCancel re-resolves it idempotently),
		// so the response must use this local copy instead of the shared struct field
		// to stay race-free under -race.
		"conversation_id": conversationID,
	})
}

// cancelPriorForWorkspace signals any in-flight agent for the same workspace
// to stop and waits up to 2 seconds for it to fully exit.  Idempotent.
func (h *AssistantMessageHandler) cancelPriorForWorkspace(workspaceID string, log logging.Logger) {
	if workspaceID == "" {
		return
	}
	v, ok := h.running.LoadAndDelete(workspaceID)
	if !ok {
		return
	}
	ra := v.(*runningAgent)
	ra.cancel()
	log.Info("canceled prior in-flight assistant request for workspace", "workspace", workspaceID)
	select {
	case <-ra.done:
	case <-time.After(2 * time.Second):
		log.Warn("prior assistant request did not exit within 2s; proceeding anyway", "workspace", workspaceID)
	}
}

// CancelAgent signals the running agent for the given workspace to stop.
// Returns true if a running agent was found and canceled, false otherwise.
func (h *AssistantMessageHandler) CancelAgent(workspaceID, conversationID string) bool {
	if workspaceID == "" {
		return false
	}
	v, ok := h.running.LoadAndDelete(workspaceID)
	if !ok {
		return false
	}
	ra := v.(*runningAgent)
	ra.cancel()
	return true
}

// RunningExists reports whether an agent is currently running for the workspace.
// Unlike CancelAgent it does NOT remove or cancel the entry — it is a read-only
// check that can be used in tests or observability without side-effects.
func (h *AssistantMessageHandler) RunningExists(workspaceID string) bool {
	_, ok := h.running.Load(workspaceID)
	return ok
}

func (h *AssistantMessageHandler) prepareRequest(w http.ResponseWriter, r *http.Request) (*AssistantMessage, logging.Logger, bool) {
	var payload AssistantMessage
	if err := decodeJSON(w, r, &payload); err != nil {
		if errors.Is(err, ErrUnsupportedContentType) {
			writeJSONError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		} else {
			h.logger.Error("failed to decode request body", "error", err)
			writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		}
		return nil, nil, false
	}

	traceID := getTraceID(payload.ConversationID, r.RemoteAddr)
	log := h.logger.With("trace", traceID)

	if !h.limiter.Allow(traceID, time.Second) {
		log.Warn("assistant rate limit")
		writeJSONError(w, http.StatusTooManyRequests, "rate limit exceeded")
		return nil, nil, false
	}

	log.Info(
		"assistant request",
		"context_version", payload.ContextVersion,
		"timezone", payload.Timezone,
		"message_len", len(payload.Message),
	)

	return &payload, log, true
}

func (h *AssistantMessageHandler) handleAssistant(ctx context.Context, payload *AssistantMessage, log logging.Logger) (any, *handlerError) {
	client, herr := h.getLLMClient(ctx, log)
	if herr != nil {
		// Surface on the SSE bus so the client renders a visible error instead of hanging.
		h.publishRunError(payload, herr.Message)
		return nil, herr
	}

	// Resolve the conversation ID up front: a brand-new conversation carries an
	// empty ID, and the recording run directory must be created under a
	// well-formed {model}/{conversation} path (an empty ID would otherwise
	// collapse the task segment in NewRunDir, orphaning the directory directly
	// beneath the model). Passing the resolved ID through to Execute also
	// ensures the session uses the same ID.
	conversationID := assistantPkg.NormalizeConversationID(payload.ConversationID)

	// Set up recording infrastructure (run directory, event sink). The workspace
	// process log is used directly; no per-run duplicate log is written.
	runID := assistantPkg.GenerateRunID()
	// Thread the run ID into the context so the service's setupRun reuses it
	// instead of generating a second one: a mismatched ID left a recording file
	// + fd + sync goroutine open forever (CloseRun never found the orphaned
	// state under the service-generated ID).
	ctx = models.WithRunID(ctx, runID)
	eventSink, procLog := h.setupRecording(client, payload.WorkspaceID, conversationID, modelName(payload, h.svc), runID)
	defer func() {
		if eventSink != nil {
			eventSink.Close()
		}
		if rcl, ok := client.(interface{ CloseRun(string) }); ok {
			rcl.CloseRun(runID)
		}
	}()

	var recorder assistantPkg.EventRecorder
	if eventSink != nil {
		recorder = eventSink
	}

	result, execErr := h.service.Execute(ctx, payload.WorkspaceID, conversationID, payload.Message, payload.ContextVersion, payload.Timezone, payload.ExcludeTools, procLog, h.provider, client, h.engine, h.svc.Events(), recorder)
	if execErr != nil {
		if errors.Is(execErr, context.Canceled) {
			return result, nil
		}
		return nil, &handlerError{Status: http.StatusInternalServerError, Message: execErr.Error()}
	}
	return result, nil
}

// setupRecording creates the run directory and event sink for recording
// infrastructure. The workspace process logger is returned for execution; no
// per-run duplicate log is created. Returns zero values when recording is
// disabled.
func (h *AssistantMessageHandler) setupRecording(client proxy.Client, workspaceID, conversationID, modelName, runID string) (*automation.EventSink, logging.Logger) {
	procLog := h.svc.ProcessLogger(workspaceID)
	if !h.svc.RunLoggingEnabled() || modelName == "" {
		return nil, procLog
	}
	parent := filepath.Join(h.svc.RootDir(), "runs")
	rd, rErr := automation.NewRunDir(parent, workspaceID, conversationID, modelName)
	if rErr != nil {
		h.logger.Error("failed to create run dir", "error", rErr)
		return nil, procLog
	}
	es, esErr := automation.NewEventSink(rd.EventsPath())
	var eventSink *automation.EventSink
	if esErr == nil {
		eventSink = es
	}
	if rcl, ok := client.(interface{ SetDirForRun(string, string) }); ok {
		rcl.SetDirForRun(runID, rd.Root)
	} else if rcl, ok := client.(interface{ SetDir(string) }); ok {
		rcl.SetDir(rd.Root)
	}
	return eventSink, procLog
}

// modelName extracts the model name from the assistant request.
func modelName(payload *AssistantMessage, svc AssistantService) string {
	primary, _ := svc.SelectModels()
	return primary
}

// Logic fix for appendToolResult structure:
// Refactoring processToolCall to first append the assistant message.

func (h *AssistantMessageHandler) getLLMClient(ctx context.Context, log logging.Logger) (proxy.Client, *handlerError) {
	client, err := h.client.GetClient(ctx)
	if err != nil {
		if errors.Is(err, models.ErrModelStarting) {
			return nil, &handlerError{
				Status:  http.StatusServiceUnavailable,
				Message: "model is starting, try again shortly",
			}
		}
		log.Error("get LLM client failed", "error", err)
		// Actionable message when no model is selected, so the user gets a clear next step instead of a silent failure.
		if primary, _ := h.svc.SelectModels(); primary == "" {
			return nil, &handlerError{
				Status:  http.StatusInternalServerError,
				Message: "No primary model is configured. Add and select a model in Settings → Providers, then retry.",
			}
		}
		return nil, &handlerError{
			Status:  http.StatusInternalServerError,
			Message: "failed to get LLM client",
		}
	}
	return client, nil
}

// publishRunError emits an SSE error event for the conversation. Early client-acquisition failures occur before h.service.Execute (where run events are normally published), so they must be surfaced here to avoid a silent hang.
//
// The event is published on the workspace channel alone: the SSE stream is
// workspace-scoped, so we only require WorkspaceID (always present from the
// request routing). RunWithCancel resolves the conversation ID up front, so
// ConversationID may already be populated here; it is preserved when present
// and never a delivery prerequisite.
func (h *AssistantMessageHandler) publishRunError(payload *AssistantMessage, message string) {
	if payload.WorkspaceID == "" {
		return
	}
	h.svc.Events().Publish(payload.WorkspaceID, assistantPkg.AgentEvent{
		ID:             fmt.Sprintf("run_err_%d", time.Now().UnixNano()),
		Type:           assistantPkg.EventError,
		Channel:        assistantPkg.ChannelAssistant,
		ConversationID: payload.ConversationID,
		Payload:        map[string]any{"error": message},
		Timestamp:      time.Now(),
	})
}

// Session Management Handlers

// RunWithCancel registers the workspace in the running map (so the
// /assistant/cancel endpoint can cancel this invocation), cancels any prior
// in-flight agent for the same workspace, executes handleAssistant, then
// cleans up the running map entry.
//
// The execution context is derived from context.Background(), NOT the caller's
// context. This is intentional: a client disconnect (page refresh / tab close /
// aborted fetch) must NOT kill an in-flight assistant run. The run outlives the
// triggering HTTP request and is observed via the SSE event bus instead. The
// only way to stop a run is the explicit /assistant/cancel endpoint, which
// calls the stored cancel func. This mirrors the automation subsystem, which
// already runs detached via context.Background().
//
// The ctx parameter is retained for signature stability and is only used for
// request-scoped logging/values; it does not cancel the run.
func (h *AssistantMessageHandler) RunWithCancel(ctx context.Context, workspaceID string, payload *AssistantMessage, log logging.Logger) (any, *handlerError) {
	h.cancelPriorForWorkspace(workspaceID, log)

	// Resolve the conversation ID up front so the running agent can report it
	// (e.g. to /active-runs for the UI to mark the session as running after a
	// refresh). NormalizeConversationID is idempotent, so handleAssistant
	// re-resolving the same value is a no-op.
	conversationID := assistantPkg.NormalizeConversationID(payload.ConversationID)
	payload.ConversationID = conversationID

	execCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	h.running.Store(workspaceID, &runningAgent{cancel: cancel, done: done, conversationID: conversationID})
	defer func() {
		h.running.Delete(workspaceID)
		cancel()
		close(done)
	}()
	return h.handleAssistant(execCtx, payload, log)
}

// RunningConversationID reports the conversation ID of the agent currently
// running for the workspace, or "" when none is running. It backs the
// /active-runs endpoint so the UI can reconcile per-session running state
// after a refresh without trusting client-side flags.
func (h *AssistantMessageHandler) RunningConversationID(workspaceID string) string {
	if v, ok := h.running.Load(workspaceID); ok {
		if ra, ok := v.(*runningAgent); ok {
			return ra.conversationID
		}
	}
	return ""
}

func (h *AssistantMessageHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	vals, ok := requirePathParams(w, r, "workspace")
	if !ok {
		return
	}
	workspaceID := vals[0]

	sessions, err := h.persistence.ListSessions(workspaceID)
	if err != nil {
		h.logger.Error("failed to list sessions", "workspace", workspaceID, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to list sessions")
		return
	}

	respondJSON(w, sessions)
}

func (h *AssistantMessageHandler) GetSession(w http.ResponseWriter, r *http.Request) {
	vals, ok := requirePathParamsMsg(w, r, "workspace and session are required", "workspace", "session")
	if !ok {
		return
	}
	workspaceID, sessionID := vals[0], vals[1]

	session, err := h.persistence.ReadSession(workspaceID, sessionID)
	if err != nil {
		h.logger.Error("failed to read session", "workspace", workspaceID, "session", sessionID, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to read session")
		return
	}

	if session == nil {
		writeJSONError(w, http.StatusNotFound, "session not found")
		return
	}

	respondJSON(w, session)
}

func (h *AssistantMessageHandler) DeleteSession(w http.ResponseWriter, r *http.Request) {
	vals, ok := requirePathParamsMsg(w, r, "workspace and session are required", "workspace", "session")
	if !ok {
		return
	}
	workspaceID, sessionID := vals[0], vals[1]

	if err := h.persistence.DeleteSession(workspaceID, sessionID); err != nil {
		h.logger.Error("failed to delete session", "workspace", workspaceID, "session", sessionID, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to delete session")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *AssistantMessageHandler) DeleteAllSessions(w http.ResponseWriter, r *http.Request) {
	vals, ok := requirePathParams(w, r, "workspace")
	if !ok {
		return
	}
	workspaceID := vals[0]

	if err := h.persistence.DeleteAllSessions(workspaceID); err != nil {
		h.logger.Error("failed to delete all sessions", "workspace", workspaceID, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to delete all sessions")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type RenameSessionRequest struct {
	Title string `json:"title"`
}

func (h *AssistantMessageHandler) RenameSession(w http.ResponseWriter, r *http.Request) {
	vals, ok := requirePathParamsMsg(w, r, "workspace and session are required", "workspace", "session")
	if !ok {
		return
	}
	workspaceID, sessionID := vals[0], vals[1]

	var req RenameSessionRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Title == "" {
		writeJSONError(w, http.StatusBadRequest, "title is required")
		return
	}

	session, err := h.persistence.ReadSession(workspaceID, sessionID)
	if err != nil {
		h.logger.Error("failed to read session for rename", "workspace", workspaceID, "session", sessionID, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to read session")
		return
	}
	if session == nil {
		writeJSONError(w, http.StatusNotFound, "session not found")
		return
	}

	if session.Metadata == nil {
		session.Metadata = make(map[string]any)
	}
	session.Metadata["title"] = req.Title

	if err := h.persistence.WriteSession(workspaceID, session); err != nil {
		h.logger.Error("failed to write renamed session", "workspace", workspaceID, "session", sessionID, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to save session")
		return
	}

	respondJSON(w, session)
}

// CancelAssistantRequest is the body for POST /admin/api/conversation/cancel.
type CancelAssistantRequest struct {
	WorkspaceID    string `json:"workspace_id"`
	ConversationID string `json:"conversation_id"`
}

// CancelAssistantHandler signals the running agent for the given workspace to
// stop.  The conversation_id is optional: when empty, cancels by workspace
// (used by the frontend when the user clicks Stop before the first response
// returns a session_id).  Idempotent — returns 200 even if no agent was
// running.  The echoed conversation_id lets the frontend learn the session
// id from the cancel response if it didn't have one at click time.
func (h *AssistantMessageHandler) CancelAssistantHandler(w http.ResponseWriter, r *http.Request) {
	var req CancelAssistantRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.WorkspaceID == "" {
		writeJSONError(w, http.StatusBadRequest, "workspace_id is required")
		return
	}

	canceled := h.CancelAgent(req.WorkspaceID, req.ConversationID)
	h.logger.Info("assistant cancel requested",
		"workspace", req.WorkspaceID,
		"conversation", req.ConversationID,
		"canceled", canceled)

	respondJSON(w, map[string]any{
		"status":          "ok",
		"canceled":        canceled,
		"conversation_id": req.ConversationID,
	})
}

// GuardrailDecisionRequest is the request body for submitting a guardrail decision.
type GuardrailDecisionRequest struct {
	DecisionID string `json:"decision_id"`
	Allow      bool   `json:"allow"`
	Persist    bool   `json:"persist"`
}

// GuardrailDecisionHandler processes user decisions on blocked tool calls.
func (h *AssistantMessageHandler) GuardrailDecisionHandler(w http.ResponseWriter, r *http.Request) {
	var req GuardrailDecisionRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.DecisionID == "" {
		writeJSONError(w, http.StatusBadRequest, "decision_id is required")
		return
	}

	store := h.svc.GuardrailDecisionStore()
	if store == nil {
		writeJSONError(w, http.StatusInternalServerError, "guardrail decision store not available")
		return
	}

	if !store.Resolve(req.DecisionID, assistantPkg.GuardrailDecision{
		Allow:   req.Allow,
		Persist: req.Persist,
	}) {
		// The approval wait already expired (the agent moved on). Honor an
		// "allow & remember" decision by persisting the override so future
		// calls are not re-blocked; the current run's tool stays skipped.
		payload, found := store.Payload(req.DecisionID)
		if !found || !req.Allow || !req.Persist {
			writeJSONError(w, http.StatusNotFound, "decision not found or already resolved")
			return
		}
		if gr := h.svc.GuardrailEngine(); gr != nil && payload.WorkspaceID != "" {
			if pErr := gr.PersistOverride(payload.WorkspaceID, payload.Category, payload.Tool, payload.Args); pErr != nil {
				h.svc.Logger().Warn("failed to persist late guardrail override", "error", pErr)
				writeJSONError(w, http.StatusInternalServerError, "failed to persist guardrail override")
				return
			}
			respondJSON(w, map[string]string{"status": "ok", "late": "true"})
			return
		}
		writeJSONError(w, http.StatusNotFound, "decision not found or already resolved")
		return
	}

	respondJSON(w, map[string]string{"status": "ok"})
}
