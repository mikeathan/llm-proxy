package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/assistant/guardrails"
	"llm-proxy/internal/core/assistant/prompts"
	"llm-proxy/internal/core/automation"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/ratelimiter"
	"llm-proxy/models"
)

const maxHistoryChars = 12 * 1024

type AssistantMessage struct {
	WorkspaceID    string `json:"workspace_id"`
	ConversationID string `json:"conversation_id"`
	ContextVersion string `json:"context_version,omitempty"`
	Message        string `json:"message"`
	Timezone       string `json:"timezone,omitempty"`
}

type AssistantMessageHandler struct {
	provider    assistant.ToolProvider
	client      proxy.LLMClientProvider
	limiter     ratelimiter.Limiter
	logger      logging.Logger
	engine      assistant.Engine
	guardrails  *guardrails.GuardrailEngine
	persistence *persistence.WorkspaceManager
	svc         AssistantService
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
	}
}

func (h *AssistantMessageHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	payload, log, ok := h.prepareRequest(w, r)
	if !ok {
		return
	}

	// Cancel any prior in-flight assistant request for the same workspace
	// before starting a new one.  This prevents an orphaned agent from
	// continuing to publish events to the workspace event bus after the user
	// has moved on.  Wait briefly (up to 2s) for the prior to finish draining
	// (cancel + persist partial session + close run-log) so its events stop
	// reaching the new SSE connection.
	h.cancelPriorForWorkspace(payload.WorkspaceID, log)

	// Use a cancellable background context for agent execution — not tied to
	// the HTTP request context (so the agent isn't killed by proxy idle
	// timeouts or browser disconnects) but cancellable via the explicit
	// /assistant/cancel endpoint or by a subsequent request for the same
	// workspace.  The agent has its own timeouts (GlobalTimeout,
	// AgentTurnTimeout) so it won't run forever.
	execCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	h.running.Store(payload.WorkspaceID, &runningAgent{cancel: cancel, done: done})
	defer func() {
		h.running.Delete(payload.WorkspaceID)
		cancel()
		close(done)
	}()

	result, err := h.handleAssistant(execCtx, payload, log)
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

// handleAssistant executes a stateful agent cycle.
func (h *AssistantMessageHandler) handleAssistant(ctx context.Context, payload *AssistantMessage, log logging.Logger) (any, *handlerError) {
	if payload.WorkspaceID == "" {
		payload.WorkspaceID = "default"
	}

	client, err := h.getLLMClient(ctx, log)
	if err != nil {
		return nil, err
	}

	session, hErr := h.resolveSession(payload, log)
	if hErr != nil {
		return nil, hErr
	}

	// 3. Load model config and resolve useNativeTools before building history
	// so the system prompt selects the correct tool format instructions.
	procLog := h.svc.ProcessLogger(payload.WorkspaceID)
	procLog.Info("Assistant request started", "conversation", payload.ConversationID, "message", payload.Message)

	modelName, _ := h.svc.SelectModels()
	useNativeTools := false
	if cfg, ok := h.svc.ModelConfig(modelName); ok {
		useNativeTools = cfg.ToolCallFormat == "native"
	}

	// 2. Build or Update History (after model config is loaded for system prompt).
	if len(session.History) == 0 {
		initial, bErr := h.buildInitialHistory(payload, useNativeTools)
		if bErr != nil {
			return nil, &handlerError{Status: http.StatusInternalServerError, Message: "failed to build history"}
		}
		session.History = initial
	} else {
		session.History = append(session.History, proxy.Message{
			Role:    proxy.UserRole,
			Content: payload.Message,
		})
	}

	session.History = h.truncateHistory(session.History)

	// Persist early so the session appears in the conversation sidebar
	// even while the agent is still executing.  The final write after
	// agent completion (or cancel) will overwrite with the full history.
	if pErr := h.persistence.WriteSession(payload.WorkspaceID, session); pErr != nil {
		log.Warn("failed to persist session on user message", "error", pErr)
	}
	h.publishSessionLifecycle(payload.WorkspaceID, session.ID, payload.Message, assistant.PhaseSessionStarted)

	// Generate run ID early so it's available for recording setup.
	runID := generateRunID()
	execCtx := models.WithTaskName(ctx, session.ID)
	execCtx = models.WithRunID(execCtx, runID)
	execCtx = assistant.WithUsageTracker(execCtx)

	_, eventSink, runLogCloser, runLog := h.setupRunInfra(client, payload, session, modelName, runID, procLog)

	// Clear stale events from previous runs so the new SSE connection
	// doesn't replay old tool_stream/message events (same pattern
	// as automation's dispatcher.go:478)
	h.svc.Events().Clear(payload.WorkspaceID)
	defer h.svc.Events().Clear(payload.WorkspaceID)

	// Build agent options using the shared builder.
	var collectedEvents []assistant.AgentEvent
	sessionStep := 0
	publishObs := func(ev assistant.AgentEvent) {
		collectedEvents = append(collectedEvents, ev)
		h.svc.Events().Publish(payload.WorkspaceID, ev)
		if eventSink != nil {
			eventSink.Write(ev)
		}
		if ev.Type == assistant.EventToolCall {
			sessionStep++
			h.publishSessionLifecycle(payload.WorkspaceID, session.ID, fmt.Sprintf("Step %d: %s", sessionStep, ev.Payload), assistant.PhaseSessionProgress)
		}
	}

	builder := NewAgentBuilder(h.svc).
		WithLogger(runLog).
		WithGuardrails().
		WithWorkspaceID(payload.WorkspaceID).
		WithModelName(modelName).
		WithHotMemory(true).
		WithObserver(publishObs).
		WithGuardrailDecisionHandler(assistant.NewGuardrailDecisionCallback(h.svc.GuardrailDecisionStore(), publishObs)).
		WithOrchestrator().
		WithModelConfig(ctx, modelName, h.provider, client)

	agent := builder.Build(client, h.provider, h.engine)

	// Strip cancelled turn messages from the LLM context so they don't
	// bleed into the next turn's response.  The display path (GetSession)
	// returns the unfiltered history.
	llmHistory := filterCancelledTurns(session.History, session.CancelledIndices)
	reply, updatedHistory, agErr := agent.Execute(execCtx, llmHistory)

	// Close recording and run log infrastructure.
	if eventSink != nil {
		eventSink.Close()
	}
	if runLogCloser != nil {
		runLogCloser()
	}
	if rcl, ok := client.(interface{ CloseRun(string) }); ok {
		rcl.CloseRun(runID)
	}

	if agErr != nil {
		if errors.Is(agErr, context.Canceled) {
			log.Info("assistant execution canceled by user", "error", agErr)

			// Append the messages the agent added to the full session
			// history (with cancelled content intact for display).  The
			// LLM was given a filtered history, but persistence preserves
			// the cancelled content too.
			if len(updatedHistory) >= len(llmHistory) {
				cancelNewMessages := updatedHistory[len(llmHistory):]
				session.History = append(session.History, cancelNewMessages...)
			} else {
				log.Warn("updatedHistory shorter than llmHistory (cancel)", "updated", len(updatedHistory), "llm", len(llmHistory))
			}

			_, canceledUserIdx := computeCancelIndices(session.History)
			if canceledUserIdx >= 0 {
				session.CancelledIndices = append(session.CancelledIndices, canceledUserIdx)
			}

			if pErr := h.persistence.WriteSession(payload.WorkspaceID, session); pErr != nil {
				log.Error("failed to save session after cancel", "error", pErr)
			}
			h.publishSessionLifecycle(payload.WorkspaceID, session.ID, "", assistant.PhaseSessionCompleted)
			return map[string]any{
				"reply":           reply,
				"conversation_id": session.ID,
				"workspace_id":    session.WorkspaceID,
				"events":          formatCollectedEvents(collectedEvents),
				"canceled":        true,
			}, nil
		}
		log.Error("assistant execution failed", "error", agErr)
		return nil, &handlerError{Status: http.StatusInternalServerError, Message: agErr.Error()}
	}

	// 4. Persistence: Save Updated History
	// Append the messages the agent added during this turn to the full
	// session history (which still contains cancelled turn messages for
	// display).  The LLM was given a filtered history, but the persisted
	// history keeps everything for the UI.
	if len(updatedHistory) >= len(llmHistory) {
		newMessages := updatedHistory[len(llmHistory):]
		session.History = append(session.History, newMessages...)
	} else {
		log.Warn("updatedHistory shorter than llmHistory", "updated", len(updatedHistory), "llm", len(llmHistory))
	}
	if pErr := h.persistence.WriteSession(payload.WorkspaceID, session); pErr != nil {
		log.Error("failed to save session", "error", pErr)
	}
	h.publishSessionLifecycle(payload.WorkspaceID, session.ID, "", assistant.PhaseSessionCompleted)

	return map[string]any{
		"reply":           reply,
		"conversation_id": session.ID,
		"workspace_id":    session.WorkspaceID,
		"events":          formatCollectedEvents(collectedEvents),
	}, nil
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

func (h *AssistantMessageHandler) buildInitialHistory(payload *AssistantMessage, useNativeTools bool) ([]proxy.Message, error) {
	// Read workspace-specific rules (same as automation reading "rules.md").
	customRules := ""
	if payload.WorkspaceID != "" && h.persistence != nil {
		rules, err := h.persistence.ReadTaskFile(payload.WorkspaceID, "rules.md")
		if err == nil && rules != "" {
			customRules = rules
		}
	}

	systemPrompt := prompts.AssembleSystemPrompt(customRules, useNativeTools)

	return []proxy.Message{
		{
			Role: proxy.SystemRole,
			Content: prompts.BuildSystemMessage(
				systemPrompt,
				useNativeTools,
				payload.ConversationID,
				payload.ContextVersion,
				payload.Timezone,
			),
		},
		{
			Role:    proxy.UserRole,
			Content: payload.Message,
		},
	}, nil
}

// Session Management Handlers

func (h *AssistantMessageHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue("workspace")
	if workspaceID == "" {
		writeJSONError(w, http.StatusBadRequest, "workspace is required")
		return
	}

	sessions, err := h.persistence.ListSessions(workspaceID)
	if err != nil {
		h.logger.Error("failed to list sessions", "workspace", workspaceID, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to list sessions")
		return
	}

	respondJSON(w, sessions)
}

func (h *AssistantMessageHandler) GetSession(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue("workspace")
	sessionID := r.PathValue("session")

	if workspaceID == "" || sessionID == "" {
		writeJSONError(w, http.StatusBadRequest, "workspace and session are required")
		return
	}

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
	workspaceID := r.PathValue("workspace")
	sessionID := r.PathValue("session")

	if workspaceID == "" || sessionID == "" {
		writeJSONError(w, http.StatusBadRequest, "workspace and session are required")
		return
	}

	if err := h.persistence.DeleteSession(workspaceID, sessionID); err != nil {
		h.logger.Error("failed to delete session", "workspace", workspaceID, "session", sessionID, "error", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to delete session")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *AssistantMessageHandler) DeleteAllSessions(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.PathValue("workspace")

	if workspaceID == "" {
		writeJSONError(w, http.StatusBadRequest, "workspace is required")
		return
	}

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
	workspaceID := r.PathValue("workspace")
	sessionID := r.PathValue("session")

	if workspaceID == "" || sessionID == "" {
		writeJSONError(w, http.StatusBadRequest, "workspace and session are required")
		return
	}

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

	if !store.Resolve(req.DecisionID, assistant.GuardrailDecision{
		Allow:   req.Allow,
		Persist: req.Persist,
	}) {
		writeJSONError(w, http.StatusNotFound, "decision not found or already resolved")
		return
	}

	respondJSON(w, map[string]string{"status": "ok"})
}

func (h *AssistantMessageHandler) truncateHistory(history []proxy.Message) []proxy.Message {
	if len(history) <= 1 {
		return history
	}

	totalChars := 0
	for _, m := range history {
		totalChars += len(m.Content)
	}

	if totalChars <= maxHistoryChars {
		return history
	}

	// Sliding window: remove oldest non-system messages
	// Keep the system prompt at index 0 if it exists
	startIdx := 0
	if history[0].Role == proxy.SystemRole {
		startIdx = 1
	}

	for totalChars > maxHistoryChars && startIdx < len(history)-1 {
		// Remove the message at startIdx (the oldest non-system message)
		totalChars -= len(history[startIdx].Content)
		history = append(history[:startIdx], history[startIdx+1:]...)
	}

	return history
}

// computeCancelIndices determines the cancel marker scope for a session
// history at cancel time.  Returns:
//   - canceledIdx: index of the last assistant message in the current turn,
//     or -1 if the cancel happened before any assistant content was produced.
//   - canceledUserIdx: index of the user message whose turn was cancelled.
//     Always set when a cancel occurs; identifies which turn the cancel
//     applies to so filterCancelledTurns can strip it from the LLM context.
//
// The caller persists canceledUserIdx by appending to AssistantSession.
// CancelledIndices (a list that survives reloads) so multiple cancelled
// turns in the same session are all marked and filtered.
//
// This avoids leaking the cancel marker into a prior turn's assistant
// message when the user cancels during the thinking phase.
func computeCancelIndices(history []proxy.Message) (canceledIdx, canceledUserIdx int) {
	lastUserIdx, lastAssistantIdx := -1, -1
	for i := len(history) - 1; i >= 0; i-- {
		switch history[i].Role {
		case proxy.AssistantRole:
			if lastAssistantIdx < 0 {
				lastAssistantIdx = i
			}
		case proxy.UserRole:
			if lastUserIdx < 0 {
				lastUserIdx = i
			}
		}
		if lastUserIdx >= 0 && lastAssistantIdx >= 0 {
			break
		}
	}

	if lastUserIdx < 0 {
		return -1, -1
	}
	if lastAssistantIdx > lastUserIdx {
		return lastAssistantIdx, lastUserIdx
	}
	return -1, lastUserIdx
}

// filterCancelledTurns strips messages from every cancelled turn so they
// don't leak into the LLM context on the next turn.  For each cancelled
// user message index in `cancelledIndices`, the user message and any
// subsequent messages (assistant/tool) up to the next user message are
// removed.  Prior successful turns are kept intact.
//
// Display paths (GetSession) return the unfiltered history so the cancelled
// content is still visible in the UI.
func filterCancelledTurns(history []proxy.Message, cancelledIndices []int) []proxy.Message {
	if len(cancelledIndices) == 0 {
		return history
	}

	skipSet := make(map[int]bool, len(cancelledIndices))
	for _, idx := range cancelledIndices {
		if idx >= 0 && idx < len(history) {
			skipSet[idx] = true
		}
	}
	if len(skipSet) == 0 {
		return history
	}

	result := make([]proxy.Message, 0, len(history))
	skipping := false
	for i, m := range history {
		if skipSet[i] {
			skipping = true
			continue
		}
		if skipping && m.Role == proxy.UserRole {
			skipping = false
		}
		if skipping {
			continue
		}
		result = append(result, m)
	}
	return result
}

// generateRunID returns a short unique identifier for a recording session.
func generateRunID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// resolveSession loads an existing session or creates a new in-memory one.
func (h *AssistantMessageHandler) resolveSession(payload *AssistantMessage, log logging.Logger) (*models.AssistantSession, *handlerError) {
	session, sErr := h.persistence.ReadSession(payload.WorkspaceID, payload.ConversationID)
	if sErr != nil {
		log.Error("failed to load session", "error", sErr)
		return nil, &handlerError{Status: http.StatusInternalServerError, Message: "persistence error"}
	}

	if session == nil {
		if payload.ConversationID == "" {
			payload.ConversationID = "conv_" + time.Now().Format("20060102150405")
		}
		session = &models.AssistantSession{
			ID:             payload.ConversationID,
			WorkspaceID:    payload.WorkspaceID,
			ContextVersion: payload.ContextVersion,
			Timezone:       payload.Timezone,
			History:        []proxy.Message{},
		}
	}
	return session, nil
}

// setupRunInfra creates run directory, event sink, run log, and tee logger
// for recording infrastructure.  Returns zero values when run logging is disabled.
func (h *AssistantMessageHandler) setupRunInfra(client proxy.Client, payload *AssistantMessage, session *models.AssistantSession, modelName string, runID string, procLog logging.Logger) (*automation.RunDir, *automation.EventSink, func(), logging.Logger) {
	var runDir *automation.RunDir
	var eventSink *automation.EventSink
	var runLogCloser func()
	runLog := procLog
	if h.svc.RunLoggingEnabled() && modelName != "" {
		parent := filepath.Join(h.svc.RootDir(), "runs")
		rd, rErr := automation.NewRunDir(parent, payload.WorkspaceID, session.ID, modelName)
		if rErr == nil {
			runDir = rd
			es, esErr := automation.NewEventSink(runDir.EventsPath())
			if esErr == nil {
				eventSink = es
			}
			if rcl, ok := client.(interface{ SetDirForRun(string, string) }); ok {
				rcl.SetDirForRun(runID, runDir.Root)
			} else if rcl, ok := client.(interface{ SetDir(string) }); ok {
				rcl.SetDir(runDir.Root)
			}
			if tl, tlErr := automation.NewTeeLogger(procLog, runDir.LogPath()); tlErr == nil {
				runLog = tl
				runLogCloser = func() {
					if c, ok := tl.(interface{ Close() error }); ok {
						c.Close()
					}
				}
			}
		}
	}
	return runDir, eventSink, runLogCloser, runLog
}

// formatCollectedEvents converts agent events into the wire format for the API response.
func formatCollectedEvents(events []assistant.AgentEvent) []map[string]any {
	out := make([]map[string]any, 0, len(events))
	for _, ev := range events {
		out = append(out, map[string]any{
			"type":    ev.Type,
			"payload": ev.Payload,
		})
	}
	return out
}

// publishSessionLifecycle publishes a lifecycle event to the workspace event bus
// so the frontend can update the conversation sidebar in real time.
func (h *AssistantMessageHandler) publishSessionLifecycle(workspaceID, conversationID, snippet, phase string) {
	if workspaceID == "" || conversationID == "" {
		return
	}
	h.svc.Events().Publish(workspaceID, assistant.AgentEvent{
		ID:        fmt.Sprintf("sse_%d", time.Now().UnixNano()),
		Type:      assistant.EventLifecycle,
		Payload: map[string]any{
			"phase":           phase,
			"conversation_id": conversationID,
			"workspace_id":    workspaceID,
			"snippet":         snippet,
		},
		Timestamp: time.Now(),
	})
}
