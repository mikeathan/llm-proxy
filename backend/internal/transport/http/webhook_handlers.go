package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/automation"
	"llm-proxy/internal/core/tools"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/models"
	"net/http"
	"strings"
	"time"
)

// WebhookHandler receives inbound messages from external platforms (Telegram,
// Slack, etc.) via POST /api/v1/webhooks/{connector_name} and routes them to
// either the agent (for normal messages) or the dispatcher (for /run commands).
// Replies are sent back through the same connector type.
type WebhookHandler struct {
	Registry    func() models.RegistryData
	Persistence *persistence.WorkspaceManager
	Secrets     models.SecretsStore
	Events      *automation.EventBus
	CommTools   *tools.CommunicationTools
	Dispatcher  *automation.Dispatcher
	Assistant   *AssistantMessageHandler
	Logger      logging.Logger
}

func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	connectorName := r.PathValue("connector_name")
	if connectorName == "" {
		writeJSONError(w, http.StatusBadRequest, "connector_name is required")
		return
	}

	reg := h.Registry()
	cfg, ok := reg.Communication.Connectors[connectorName]
	if !ok || !cfg.Enabled {
		h.Logger.Warn("webhook: connector not found or disabled", "connector", connectorName)
		writeJSONError(w, http.StatusNotFound, "connector not found or disabled")
		return
	}
	h.Logger.Info("webhook received", "connector", connectorName, "type", cfg.Type)

	expectedToken := cfg.Settings["webhook_token"]
	if expectedToken != "" {
		actualToken := r.Header.Get("X-Webhook-Token")
		if actualToken == "" {
			actualToken = r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
		}
		if actualToken != expectedToken {
			h.Logger.Warn("webhook: invalid webhook token", "connector", connectorName)
			writeJSONError(w, http.StatusUnauthorized, "invalid webhook token")
			return
		}
	}

	message, chatID, err := parseInboundMessage(cfg.Type, r)
	if err != nil {
		h.Logger.Warn("webhook: parse failed", "connector", connectorName, "type", cfg.Type, "error", err)
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if message == "" {
		h.Logger.Info("webhook: empty message (non-text update ignored)", "connector", connectorName)
		respondJSON(w, map[string]string{"status": "ok"})
		return
	}

	workspaceID := cfg.Settings["workspace_id"]
	if workspaceID == "" {
		h.Logger.Warn("webhook: missing workspace_id in connector settings", "connector", connectorName)
		writeJSONError(w, http.StatusBadRequest, "connector has no workspace_id configured")
		return
	}
	h.Logger.Info("webhook: routing message", "connector", connectorName, "workspace", workspaceID, "chat_id", chatID)

	// Route: /run commands go to dispatcher, everything else goes to the agent
	if strings.HasPrefix(message, "/run ") {
		h.handleAutomation(workspaceID, connectorName, cfg.Type, message)
	} else {
		h.handleAgentMessage(workspaceID, connectorName, cfg.Type, message, chatID)
	}

	h.Logger.Info("webhook: processed successfully", "connector", connectorName)
	respondJSON(w, map[string]string{"status": "ok"})
}

// handleAutomation triggers an automation by name and sends result back.
func (h *WebhookHandler) handleAutomation(workspaceID, connectorName, connectorType, message string) {
	name := strings.TrimSpace(strings.TrimPrefix(message, "/run "))
	if name == "" {
		h.reply(connectorType, "Usage: /run <automation_name>")
		return
	}

	ctx := h.execCtx(workspaceID)
	if err := h.Dispatcher.Trigger(ctx, workspaceID, name, ""); err != nil {
		h.Logger.Error("automation trigger failed", "name", name, "error", err)
		h.reply(connectorType, fmt.Sprintf("Failed to trigger '%s': %v", name, err))
		return
	}

	immediateReply := fmt.Sprintf("Running '%s'...", name)
	h.reply(connectorType, immediateReply)

	// Subscribe for async result with timeout
	resultCh := make(chan string, 1)
	sub, _ := h.Dispatcher.Events().Subscribe(workspaceID)

	go func() {
		defer h.Dispatcher.Events().Unsubscribe(workspaceID, sub)
		select {
		case ev := <-sub:
			if result, ok := extractRunResult(ev); ok {
				resultCh <- result
			}
		case <-time.After(30 * time.Second):
			resultCh <- "Automation started, see Pulse for results."
		case <-ctx.Done():
		}
	}()

	select {
	case result := <-resultCh:
		h.reply(connectorType, result)
	case <-time.After(31 * time.Second):
		h.reply(connectorType, "Automation completed, see Pulse for details.")
	}
}

// handleAgentMessage appends the message, triggers the agent, and replies.
func (h *WebhookHandler) handleAgentMessage(workspaceID, connectorName, connectorType, message string, chatID string) {
	session, err := h.findOrCreateSourceSession(workspaceID, connectorName, chatID)
	if err != nil {
		h.Logger.Error("session error for webhook", "error", err)
		return
	}

	session.History = append(session.History, models.Message{
		Role:    models.UserRole,
		Content: message,
	})

	if err := h.Persistence.WriteSession(workspaceID, session); err != nil {
		h.Logger.Error("failed to persist session", "error", err)
		return
	}

	payload := &AssistantMessage{
		WorkspaceID:    workspaceID,
		ConversationID: session.ID,
		Message:        message,
		Timezone:       "UTC",
	}

	h.Logger.Info("webhook agent request", "workspace", workspaceID, "connector", connectorName)
	result, handlerErr := h.Assistant.handleAssistant(context.Background(), payload, h.Logger)
	if handlerErr != nil {
		h.Logger.Error("webhook agent execution failed", "error", handlerErr.Message)
		h.reply(connectorType, "Agent processing failed. Check the assistant UI for details.")
		return
	}

	if resultMap, ok := result.(map[string]any); ok {
		if reply, ok := resultMap["reply"].(string); ok && reply != "" {
			h.reply(connectorType, reply)
		}
	}
}

// reply sends a message back through the connector the user came from.
func (h *WebhookHandler) reply(connectorType, message string) {
	if h.CommTools == nil {
		return
	}
	if err := h.CommTools.NotifyAll(context.Background(), message, connectorType); err != nil {
		h.Logger.Error("webhook reply failed", "connector_type", connectorType, "error", err)
	}
}

// execCtx returns a background context tied to the workspace for cancellations.
func (h *WebhookHandler) execCtx(workspaceID string) context.Context {
	return context.Background()
}

// findOrCreateSourceSession creates a per-source session keyed by connector+chatID.
func (h *WebhookHandler) findOrCreateSourceSession(workspaceID, connectorName, chatID string) (*models.AssistantSession, error) {
	sessionKey := fmt.Sprintf("wb_%s_%s", connectorName, chatID)

	session, err := h.Persistence.ReadSession(workspaceID, sessionKey)
	if err == nil && session != nil {
		return session, nil
	}

	return &models.AssistantSession{
		ID:          sessionKey,
		WorkspaceID: workspaceID,
		History:     []models.Message{},
		UpdatedAt:   time.Now(),
	}, nil
}

// ---- Platform-specific parsing ----

type webhookPayload struct {
	Message string
	ChatID  string
}

func parseInboundMessage(connectorType string, r *http.Request) (message, chatID string, err error) {
	switch connectorType {
	case models.ConnectorTypeTelegram:
		return parseTelegramWebhook(r)
	default:
		return "", "", fmt.Errorf("unsupported connector type for inbound: %s", connectorType)
	}
}

func parseTelegramWebhook(r *http.Request) (message, chatID string, err error) {
	body, readErr := io.ReadAll(io.LimitReader(r.Body, 65536))
	if readErr != nil {
		return "", "", fmt.Errorf("failed to read request body: %w", readErr)
	}

	var payload struct {
		Message struct {
			Text string `json:"text"`
			Chat struct {
				ID int64 `json:"id"`
			} `json:"chat"`
		} `json:"message"`
	}
	if parseErr := json.Unmarshal(body, &payload); parseErr != nil {
		return "", "", fmt.Errorf("invalid telegram webhook payload: %w", parseErr)
	}

	text := strings.TrimSpace(payload.Message.Text)
	chatIDStr := fmt.Sprintf("%d", payload.Message.Chat.ID)
	return text, chatIDStr, nil
}

// extractRunResult reads a run result from an event channel.
func extractRunResult(ev assistant.AgentEvent) (string, bool) {
	if ev.Type != "run_complete" && ev.Type != "tool_result" {
		return "", false
	}
	if m, ok := ev.Payload.(map[string]any); ok {
		if result, ok := m["result"].(string); ok && result != "" {
			return result, true
		}
	}
	return "", false
}
