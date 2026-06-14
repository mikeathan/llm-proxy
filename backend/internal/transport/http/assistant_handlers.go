package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
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

	result, err := h.handleAssistant(r.Context(), payload, log)
	if err != nil {
		writeJSONError(w, err.Status, err.Message)
		return
	}

	respondJSON(w, result)
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

	// 1. Load or Create Session
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

	// 3. Load model config and resolve useNativeTools before building history
	// so the system prompt selects the correct tool format instructions.
	procLog := h.svc.ProcessLogger(payload.WorkspaceID)
	procLog.Info("Assistant request started", "conversation", payload.ConversationID, "message", payload.Message)

	modelName, _ := h.svc.SelectModels()
	useNativeTools := false
	var modelCfg models.ModelConfig
	if cfg, ok := h.svc.ModelConfig(modelName); ok {
		modelCfg = cfg
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

	// Generate run ID early so it's available for recording setup.
	runID := generateRunID()
	execCtx := models.WithTaskName(ctx, session.ID)
	execCtx = models.WithRunID(execCtx, runID)
	execCtx = assistant.WithUsageTracker(execCtx)

	// Set up run directory and recording infrastructure (same pattern as automation's setupRunDir).
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

	// Build agent options matching automation's buildAgentOptions pattern.
	var collectedEvents []assistant.AgentEvent
	publishObs := func(ev assistant.AgentEvent) {
		collectedEvents = append(collectedEvents, ev)
		h.svc.Events().Publish(payload.WorkspaceID, ev)
		if eventSink != nil {
			eventSink.Write(ev)
		}
	}

	opts := assistant.AgentOptions{
		Logger:      runLog,
		MaxSteps:    assistant.DefaultMaxSteps,
		Guardrails:  h.guardrails,
		WorkspaceID: payload.WorkspaceID,
		ModelName:   modelName,
		Observer:    publishObs,
		GuardrailDecisionHandler: assistant.NewGuardrailDecisionCallback(
			h.svc.GuardrailDecisionStore(),
			publishObs,
		),
	}

	// Apply model config overrides using the shared helper.
	if opts.ApplyModelConfig(modelCfg) {
		tools, listErr := h.provider.ListTools(ctx)
		if listErr == nil && len(tools) > 0 {
			opts.PlanStrategy = assistant.NewExecutionPlanStrategy(client, tools, procLog)
		}
	}
	opts.Orchestrator = h.svc.Orchestrator()

	agent := assistant.NewAgent(client, h.provider, h.engine, opts)

	reply, updatedHistory, agErr := agent.Execute(execCtx, session.History)

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
		log.Error("agent execution failed", "error", agErr)
		return nil, &handlerError{Status: http.StatusInternalServerError, Message: agErr.Error()}
	}

	// 4. Persistence: Save Updated History
	session.History = updatedHistory
	if pErr := h.persistence.WriteSession(payload.WorkspaceID, session); pErr != nil {
		log.Error("failed to save session", "error", pErr)
	}

	// Write run metadata and final report to the run directory.
	if runDir != nil {
		if reply != "" {
			runDir.WriteFinalReport(reply)
		}
		meta := automation.RunMeta{
			Model:      modelName,
			Task:       session.ID,
			DurationMs: 0,
		}
		if t := assistant.GetUsageTracker(execCtx); t != nil {
			meta.LLMCalls = t.LLMCalls
			meta.ToolCalls = t.ToolCalls
		}
		runDir.WriteMeta(meta)
	}

	eventsJSON := make([]map[string]any, 0, len(collectedEvents))
	for _, ev := range collectedEvents {
		eventsJSON = append(eventsJSON, map[string]any{
			"type":    ev.Type,
			"payload": ev.Payload,
		})
	}

	return map[string]any{
		"reply":           reply,
		"conversation_id": session.ID,
		"workspace_id":    session.WorkspaceID,
		"events":          eventsJSON,
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

// generateRunID returns a short unique identifier for a recording session.
func generateRunID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
