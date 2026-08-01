package handlers

import (
	"context"
	"errors"
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
	cancel context.CancelFunc
	done   chan struct{}
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

func (h *AssistantMessageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	payload, log, ok := h.prepareRequest(w, r)
	if !ok {
		return
	}

	result, err := h.RunWithCancel(r.Context(), payload.WorkspaceID, payload, log)
	if err != nil {
		writeJSONError(w, err.Status, err.Message)
		return
	}

	respondJSON(w, result)
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
	client, err := h.getLLMClient(ctx, log)
	if err != nil {
		return nil, err
	}

	// Set up recording infrastructure (run directory, event sink, tee logger).
	runID := assistantPkg.GenerateRunID()
	eventSink, runLogCloser, runLog := h.setupRecording(client, payload.WorkspaceID, payload.ConversationID, modelName(payload, h.svc), runID)
	defer func() {
		if eventSink != nil {
			eventSink.Close()
		}
		if runLogCloser != nil {
			runLogCloser()
		}
		if rcl, ok := client.(interface{ CloseRun(string) }); ok {
			rcl.CloseRun(runID)
		}
	}()

	var recorder assistantPkg.EventRecorder
	if eventSink != nil {
		recorder = eventSink
	}

	result, execErr := h.service.Execute(ctx, payload.WorkspaceID, payload.ConversationID, payload.Message, payload.ContextVersion, payload.Timezone, payload.ExcludeTools, runLog, h.provider, client, h.engine, h.svc.Events(), recorder)
	if execErr != nil {
		if errors.Is(execErr, context.Canceled) {
			return result, nil
		}
		return nil, &handlerError{Status: http.StatusInternalServerError, Message: execErr.Error()}
	}
	return result, nil
}

// setupRecording creates the run directory, event sink, and tee logger for
// recording infrastructure. Returns zero values when recording is disabled.
func (h *AssistantMessageHandler) setupRecording(client proxy.Client, workspaceID, conversationID, modelName, runID string) (*automation.EventSink, func(), logging.Logger) {
	if !h.svc.RunLoggingEnabled() || modelName == "" {
		return nil, nil, h.svc.ProcessLogger(workspaceID)
	}
	parent := filepath.Join(h.svc.RootDir(), "runs")
	rd, rErr := automation.NewRunDir(parent, workspaceID, conversationID, modelName)
	if rErr != nil {
		h.logger.Error("failed to create run dir", "error", rErr)
		return nil, nil, h.svc.ProcessLogger(workspaceID)
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
	procLog := h.svc.ProcessLogger(workspaceID)
	tl, tlErr := automation.NewTeeLogger(procLog, rd.LogPath())
	if tlErr == nil {
		return eventSink, func() {
			if c, ok := tl.(interface{ Close() error }); ok {
				c.Close()
			}
		}, tl
	}
	return eventSink, nil, procLog
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
		return nil, &handlerError{
			Status:  http.StatusInternalServerError,
			Message: "failed to get LLM client",
		}
	}
	return client, nil
}

// Session Management Handlers

// RunWithCancel registers the workspace in the running map (so the
// /assistant/cancel endpoint can cancel this invocation), cancels any prior
// in-flight agent for the same workspace, executes handleAssistant, then
// cleans up the running map entry.
//
// The parent ctx is chained so that client disconnects (HTTP path) or
// explicit cancels propagate to the agent.  Webhook callers should pass
// context.Background() because the agent outlives the webhook HTTP request.
func (h *AssistantMessageHandler) RunWithCancel(ctx context.Context, workspaceID string, payload *AssistantMessage, log logging.Logger) (any, *handlerError) {
	h.cancelPriorForWorkspace(workspaceID, log)

	execCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	h.running.Store(workspaceID, &runningAgent{cancel: cancel, done: done})
	defer func() {
		h.running.Delete(workspaceID)
		cancel()
		close(done)
	}()
	return h.handleAssistant(execCtx, payload, log)
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
		writeJSONError(w, http.StatusNotFound, "decision not found or already resolved")
		return
	}

	respondJSON(w, map[string]string{"status": "ok"})
}
