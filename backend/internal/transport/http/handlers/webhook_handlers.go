package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/automation"
	"llm-proxy/internal/core/proxy"
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
	Events      assistant.EventPublisher
	CommTools   *tools.CommunicationTools
	Dispatcher  *automation.Dispatcher
	Assistant   *AssistantMessageHandler
	Logger      logging.Logger
}

func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	vals, ok := requirePathParams(w, r, "connector_name")
	if !ok {
		return
	}
	connectorName := vals[0]

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
		h.replyToChat(connectorName, "Usage: /run <automation_name>")
		return
	}

	ctx := h.execCtx(workspaceID)
	if err := h.Dispatcher.Trigger(ctx, workspaceID, name, ""); err != nil {
		h.Logger.Error("automation trigger failed", "name", name, "error", err)
		h.replyToChat(connectorName, fmt.Sprintf("Failed to trigger '%s': %v", name, err))
		return
	}

	immediateReply := fmt.Sprintf("Running '%s'...", name)
	h.replyToChat(connectorName, immediateReply)

	resultCh := make(chan string, 1)
	sub, _ := h.Dispatcher.Events().Subscribe(workspaceID, assistant.ChannelAutomation)

	go func() {
		defer h.Dispatcher.Events().Unsubscribe(workspaceID, assistant.ChannelAutomation, sub)
		for {
			select {
			case ev := <-sub:
				if ev.Type == assistant.EventMessage {
					if msg, ok := ev.Payload.(proxy.Message); ok && msg.Content == "✔ Execution complete." {
						state, err := h.Persistence.ReadState(workspaceID)
						if err == nil {
							if run, ok := state.LastRuns[name]; ok && run.Output != "" {
								resultCh <- run.Output
								return
							}
						}
						resultCh <- "Automation completed, see Pulse for details."
						return
					}
				}
			case <-time.After(30 * time.Second):
				resultCh <- "Automation started, see Pulse for results."
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	select {
	case result := <-resultCh:
		h.replyToChat(connectorName, result)
	case <-time.After(31 * time.Second):
		h.replyToChat(connectorName, "Automation completed, see Pulse for details.")
	}
}

// handleAgentMessage runs the agent for an inbound message in a goroutine.
// Each message gets a fresh conversation via webhookSessionKey so the agent
// never inherits prior run history.  The webhook HTTP response is sent
// immediately so Telegram does not timeout; the reply is delivered via the
// connector's sendMessage API when execution completes.
func (h *WebhookHandler) handleAgentMessage(workspaceID, connectorName, connectorType, message string, chatID string) {
	h.Logger.Info("webhook agent request", "workspace", workspaceID, "connector", connectorName)

	payload := &AssistantMessage{
		WorkspaceID:    workspaceID,
		ConversationID: webhookSessionKey(connectorType, chatID),
		Message:        message,
		Timezone:       "UTC",
		// No hardcoded ExcludeTools here — the exposed tool schema is derived
		// from guardrail policy (DisabledToolNames) at the agent narrow waist,
		// so a statically-disabled tool (e.g. notify_user with Communication
		// disabled) is never advertised regardless of channel.
	}

	// Run agent in a goroutine — the webhook response to Telegram is sent
	// immediately to avoid timeouts.  The agent's reply is delivered via
	// the connector's sendMessage API when execution completes.
	go h.runAgentReply(workspaceID, connectorName, connectorType, chatID, payload)
}

// runAgentReply runs handleAssistant and sends the result back via the connector.
func (h *WebhookHandler) runAgentReply(workspaceID, connectorName, connectorType, chatID string, payload *AssistantMessage) {
	result, handlerErr := h.Assistant.RunWithCancel(context.Background(), workspaceID, payload, h.Logger)
	if handlerErr != nil {
		h.Logger.Error("webhook agent execution failed", "error", handlerErr.Message)
		return
	}
	if resultMap, ok := result.(map[string]any); ok {
		if reply, ok := resultMap["reply"].(string); ok && reply != "" {
			h.replyToChat(connectorName, reply)
		}
	}
}

// replyToChat sends a message back through the originating connector.
// The connector owns all platform-specific logic via the Connector interface.
func (h *WebhookHandler) replyToChat(connectorName, message string) {
	conn, ok := h.CommTools.GetByName(connectorName)
	if !ok {
		h.Logger.Warn("no connector registered", "connector", connectorName)
		return
	}
	if err := conn.Send(context.Background(), message); err != nil {
		h.Logger.Error("webhook reply failed", "connector", connectorName, "error", err)
	}
}

// execCtx returns a background context tied to the workspace for cancellations.
func (h *WebhookHandler) execCtx(workspaceID string) context.Context {
	return context.Background()
}

// webhookSessionKey builds a unique conversation ID per inbound webhook
// message.  The timestamp suffix guarantees a fresh session every time so the
// agent never inherits prior run history (which confused models such as Qwen
// into re-emitting previous answers).  Prior runs persist in storage and stay
// visible in the assistant history panel.  The "wb_{platformType}_" prefix is
// detected by the frontend to group sessions by source.
func webhookSessionKey(platformType, chatID string) string {
	return fmt.Sprintf("wb_%s_%s_%s", platformType, chatID, time.Now().UTC().Format("20060102T150405Z"))
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
