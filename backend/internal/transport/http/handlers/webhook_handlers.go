package handlers

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/core/automation"
	"llm-proxy/internal/core/chat"
	"llm-proxy/internal/core/commands"
	"llm-proxy/internal/core/proxy"
	"llm-proxy/internal/core/reminders"
	"llm-proxy/internal/core/tools"
	"llm-proxy/internal/platform/logging"
	"llm-proxy/internal/platform/persistence"
	"llm-proxy/internal/platform/ratelimiter"
	"llm-proxy/internal/platform/safe"
	"llm-proxy/models"
	"net/http"
	"strings"
	"time"
)

// webhookRateLimitInterval bounds inbound messages per connector so an
// unauthenticated webhook cannot be used as a resource-exhaustion amplifier.
const webhookRateLimitInterval = time.Second

// AgentTurns runs one chat message as a turn: the part of the chat service a connector uses.
type AgentTurns interface {
	Run(ctx context.Context, req *chat.Request, log logging.Logger) (chat.ExecuteResult, error)
}

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
	Turns       AgentTurns
	Logger      logging.Logger
	Limiter     ratelimiter.Limiter
	// OwnerTimezone returns the owner's configured IANA zone, or "" when unset.
	OwnerTimezone func() string
	// Commands are the slash commands (/remind, ...) answered without the assistant.
	Commands *commands.Registry
}

// agentTimezone is the zone a connector-originated run reports to the agent: the
// owner's configured zone when it is set and valid, else UTC (a connector message
// carries no browser zone).
func (h *WebhookHandler) agentTimezone() string {
	owner := ""
	if h.OwnerTimezone != nil {
		owner = h.OwnerTimezone()
	}
	return reminders.ChatZone(owner, "")
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

	// Per-connector rate limit: the webhook endpoint is public, so without a
	// bound a flood of messages would spawn unbounded agent runs.
	if h.Limiter != nil && !h.Limiter.Allow(connectorName, webhookRateLimitInterval) {
		h.Logger.Warn("webhook: rate limited", "connector", connectorName)
		writeJSONError(w, http.StatusTooManyRequests, "rate limited")
		return
	}
	h.Logger.Info("webhook received", "connector", connectorName, "type", cfg.Type)

	expectedToken := cfg.Settings["webhook_token"]
	if expectedToken != "" {
		actualToken := r.Header.Get("X-Webhook-Token")
		if actualToken == "" {
			actualToken = r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
		}
		// Constant-time compare so the token cannot be recovered via timing.
		if subtle.ConstantTimeCompare([]byte(actualToken), []byte(expectedToken)) != 1 {
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

	// Route: slash commands are answered in code (a turn command starts its turn),
	// /run goes to the dispatcher, everything else goes to the agent.
	in := inbound{workspaceID: workspaceID, connectorName: connectorName, connectorType: cfg.Type, chatID: chatID}
	switch {
	case h.handleCommand(in, message):
	case strings.HasPrefix(message, "/run "):
		h.handleAutomation(workspaceID, connectorName, cfg.Type, message)
	default:
		h.handleAgentMessage(in, message)
	}

	h.Logger.Info("webhook: processed successfully", "connector", connectorName)
	respondJSON(w, map[string]string{"status": "ok"})
}

// inbound is where a connector message came from: the workspace its connector serves, the connector, and the chat.
type inbound struct {
	workspaceID, connectorName, connectorType, chatID string
}

// handleCommand answers a registered slash command in the chat it came from and
// reports whether the message was one. A turn command's turn then runs the way
// any message does, its answer sent back to the chat.
func (h *WebhookHandler) handleCommand(in inbound, message string) bool {
	reply, handled := h.Commands.Execute(agentRunContext(in.connectorType), message, commands.Env{Zone: h.agentTimezone()})
	if !handled {
		return false
	}
	if reply.Text != "" {
		h.replyPlainToChat(in.connectorName, reply.Text)
	}
	if reply.Turn != nil && !reply.Failed {
		h.startAgentTurn(in, *reply.Turn)
	}
	return true
}

// handleAutomation triggers an automation by name and sends result back.
func (h *WebhookHandler) handleAutomation(workspaceID, connectorName, connectorType, message string) {
	name := strings.TrimSpace(strings.TrimPrefix(message, "/run "))
	if name == "" {
		h.replyToChat(connectorName, "Usage: /run <automation_name>")
		return
	}

	ctx := h.execCtx(workspaceID)

	// Trigger is a cheap lane enqueue (the run timeout starts at dequeue), so
	// it runs inline; the run's progress is observed by the monitor goroutine
	// below.
	if _, err := h.Dispatcher.Trigger(workspaceID, name, ""); err != nil {
		h.Logger.Error("automation trigger failed", "name", name, "error", err)
	}

	immediateReply := fmt.Sprintf("Running '%s'...", name)
	h.replyToChat(connectorName, immediateReply)

	resultCh := make(chan string, 1)
	// Only this automation's own run: another automation finishing in the workspace
	// must not be taken for it.
	stream := assistant.AutomationStream(name)
	sub, _ := h.Dispatcher.Events().Subscribe(workspaceID, stream)

	safe.Go("webhook automation monitor", func() {
		defer h.Dispatcher.Events().Unsubscribe(workspaceID, stream, sub)
		for {
			select {
			case ev := <-sub:
				if ev.Type == assistant.EventMessage {
					if msg, ok := ev.Payload.(proxy.Message); ok && msg.Content == assistant.MsgExecutionComplete {
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
	})

	select {
	case result := <-resultCh:
		h.replyToChat(connectorName, result)
	case <-time.After(31 * time.Second):
		h.replyToChat(connectorName, "Automation completed, see Pulse for details.")
	}
}

// handleAgentMessage runs the agent for an inbound message.
func (h *WebhookHandler) handleAgentMessage(in inbound, message string) {
	h.startAgentTurn(in, commands.TurnSpec{Message: message})
}

// startAgentTurn runs a turn for an inbound message in a goroutine. Each message
// gets a fresh conversation via webhookSessionKey so the agent never inherits
// prior run history. The webhook HTTP response is sent immediately so Telegram
// does not timeout; the reply is delivered via the connector's sendMessage API
// when execution completes.
func (h *WebhookHandler) startAgentTurn(in inbound, turn commands.TurnSpec) {
	h.Logger.Info("webhook agent request", "workspace", in.workspaceID, "connector", in.connectorName, "agent", turn.Agent)
	payload := h.agentRequest(in, turn)
	safe.Go("webhook agent reply", func() { h.runAgentReply(in.connectorName, in.connectorType, payload) })
}

// agentRequest is the chat turn of an inbound message: the turn's message, agent and helpers, in a fresh conversation.
func (h *WebhookHandler) agentRequest(in inbound, turn commands.TurnSpec) *chat.Request {
	return &chat.Request{
		WorkspaceID:    in.workspaceID,
		ConversationID: webhookSessionKey(in.connectorType, in.chatID),
		Message:        turn.Message,
		Timezone:       h.agentTimezone(),
		Agent:          turn.Agent,
		UseHelpers:     turn.UseHelpers,
		// No hardcoded ExcludeTools here — the exposed tool schema is derived
		// from guardrail policy (DisabledToolNames) at the agent narrow waist,
		// so a statically-disabled tool (e.g. notify_user with Communication
		// disabled) is never advertised regardless of channel.
	}
}

// agentRunContext is the context of a connector-originated run: it records which
// connector the message came from so a reminder asked for there is answered there.
func agentRunContext(connectorType string) context.Context {
	return models.WithOriginConnector(context.Background(), connectorType)
}

// runAgentReply runs the message as a chat turn and sends the answer back via the connector.
func (h *WebhookHandler) runAgentReply(connectorName, connectorType string, payload *chat.Request) {
	result, err := h.Turns.Run(agentRunContext(connectorType), payload, h.Logger)
	if err != nil {
		h.Logger.Error("webhook agent execution failed", "error", err)
		return
	}
	if result.Reply != "" {
		h.replyToChat(connectorName, result.Reply)
	}
}

// replyToChat sends a message back through the originating connector.
// The connector owns all platform-specific logic via the Connector interface.
func (h *WebhookHandler) replyToChat(connectorName, message string) {
	h.reply(connectorName, message, false)
}

// replyPlainToChat is replyToChat for text that holds the user's own words (a
// reminder's message): a connector that parses markup sends it as typed.
func (h *WebhookHandler) replyPlainToChat(connectorName, message string) {
	h.reply(connectorName, message, true)
}

func (h *WebhookHandler) reply(connectorName, message string, plain bool) {
	conn, ok := h.CommTools.GetByName(connectorName)
	if !ok {
		h.Logger.Warn("no connector registered", "connector", connectorName)
		return
	}
	send := conn.Send
	if p, isPlain := conn.(tools.PlainSender); plain && isPlain {
		send = p.SendPlain
	}
	if err := send(context.Background(), message); err != nil {
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
