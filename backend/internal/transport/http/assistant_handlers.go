package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/ratelimiter"
	"llm-proxy/models"
)

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
	guardrails  *assistant.GuardrailEngine
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
	if err := decodeJSON(r, &payload); err != nil {
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

	// 2. Build or Update History
	if len(session.History) == 0 {
		initial, bErr := h.buildInitialHistory(payload)
		if bErr != nil {
			return nil, &handlerError{Status: http.StatusInternalServerError, Message: "failed to build history"}
		}
		session.History = initial
	} else {
		// Just append user message to existing history
		session.History = append(session.History, proxy.Message{
			Role:    proxy.UserRole,
			Content: payload.Message,
		})
	}

	// 3. Initialize and Execute Agent
	procLog := h.svc.ProcessLogger(payload.WorkspaceID)
	procLog.Info("Assistant request started", "conversation", payload.ConversationID, "message", payload.Message)

	agent := assistant.NewAgent(client, h.provider, h.engine, assistant.AgentOptions{
		Logger:     procLog,
		MaxSteps:   10,
		Guardrails: h.guardrails,
	})

	reply, updatedHistory, agErr := agent.Execute(ctx, session.History)
	if agErr != nil {
		log.Error("agent execution failed", "error", agErr)
		return nil, &handlerError{Status: http.StatusInternalServerError, Message: agErr.Error()}
	}

	// 4. Persistence: Save Updated History
	session.History = updatedHistory
	if pErr := h.persistence.WriteSession(payload.WorkspaceID, session); pErr != nil {
		log.Error("failed to save session", "error", pErr)
		// We continue anyway so the user gets the reply, but it's a warnable offense
	}

	return map[string]any{
		"reply":           reply,
		"conversation_id": session.ID,
		"workspace_id":    session.WorkspaceID,
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

func (h *AssistantMessageHandler) buildInitialHistory(payload *AssistantMessage) ([]proxy.Message, error) {
	systemPrompt, err := h.provider.GetSystemPrompt()
	if err != nil {
		h.logger.Error("failed to get system prompt", "error", err)
		return nil, err
	}

	// Calculate robust relative path from Current Working Directory to Workspaces Dir
	relWs := h.persistence.GetRelativeWorkspacePath()
	jailPrompt := assistant.BuildJailPrompt(relWs, payload.WorkspaceID)

	agentPrompt := ""
	if payload.WorkspaceID != "" && h.persistence != nil {
		agentPromptStr, err := h.persistence.ReadTaskFile(payload.WorkspaceID, "agent.md")
		if err == nil && agentPromptStr != "" {
			agentPrompt = "\n\n# Workspace Agent Directives\n" + agentPromptStr
		}
	}

	return []proxy.Message{
		{
			Role: proxy.SystemRole,
			Content: assistant.BuildSystemMessage(
				systemPrompt+jailPrompt+agentPrompt,
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
