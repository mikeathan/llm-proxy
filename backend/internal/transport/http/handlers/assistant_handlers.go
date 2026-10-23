package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	assistantPkg "llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/assistant/profile"
	"llm-proxy/internal/core/chat"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/core/runlane"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/memory"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/ratelimiter"
	"llm-proxy/models"
)

// AssistantHandlerDeps is what the chat handlers use of the app; the turn lifecycle's own needs are chat.Deps.
type AssistantHandlerDeps interface {
	Limiter() ratelimiter.Limiter
	Logger() logging.Logger
	Persistence() *persistence.WorkspaceManager
	Lane() *runlane.Scheduler
	LaneKeyFor(model string) runlane.LaneKey
	ModelConfig(modelName string) (models.ModelConfig, bool)
	Profiles() *profile.Catalog
	GuardrailEngine() *guardrails.GuardrailEngine
	GuardrailDecisionStore() *assistantPkg.GuardrailDecisionStore
	GetClientForModel(ctx context.Context, modelName string) (proxy.Client, error)
	MemoryStore() *memory.Store
}

// AssistantMessageHandler serves the chat endpoints: it decodes and validates a message, starts its turn through the
// chat service and answers at once; the turn itself (admission, cold-model wait, recording, cancel and replace) is the
// chat service's.
type AssistantMessageHandler struct {
	limiter     ratelimiter.Limiter
	logger      logging.Logger
	persistence *persistence.WorkspaceManager
	svc         AssistantHandlerDeps
	turns       *chat.Service
	lane        *runlane.Scheduler
	// reviewTimeout bounds a memory review's wait plus model call; zero means memoryReviewTimeout.
	reviewTimeout time.Duration
}

func NewAssistantMessageHandler(service AssistantHandlerDeps, turns *chat.Service) *AssistantMessageHandler {
	return &AssistantMessageHandler{
		limiter:     service.Limiter(),
		logger:      service.Logger(),
		persistence: service.Persistence(),
		svc:         service,
		turns:       turns,
		lane:        service.Lane(),
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
		if _, err := h.turns.Run(r.Context(), payload, log); err != nil {
			log.Error("assistant run failed", "workspace", payload.WorkspaceID, "error", err)
		}
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

func (h *AssistantMessageHandler) prepareRequest(w http.ResponseWriter, r *http.Request) (*chat.Request, logging.Logger, bool) {
	var payload chat.Request
	if err := decodeJSON(w, r, &payload); err != nil {
		if errors.Is(err, ErrUnsupportedContentType) {
			writeJSONError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		} else {
			h.logger.Error("failed to decode request body", "error", err)
			writeJSONError(w, http.StatusBadRequest, "invalid JSON body")
		}
		return nil, nil, false
	}

	if payload.Model != "" {
		if _, known := h.svc.ModelConfig(payload.Model); !known {
			writeJSONError(w, http.StatusBadRequest, "unknown model: "+payload.Model)
			return nil, nil, false
		}
	}
	if err := validateProfileID(h.svc.Profiles(), "profile", payload.Profile); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
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

	canceled := h.turns.Cancel(req.WorkspaceID, h.logger)
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
