---
status: complete
last_reviewed: 2026-07-11
---

# PLAN: Communication Connector Inbound Webhook — Phase 2

**Status:** Stable — updated for async agent execution and live visibility  
**SPEC:** `docs/SPECS/communication.md`  
**Depends on:** Phase 1 (connector-system.md — Complete)

---

## Goal

Add an inbound webhook endpoint so external platforms (Telegram, Slack, etc.) can deliver messages to the agent. When a message arrives, the system creates or appends to a session, persists it, publishes an event to the frontend, and optionally triggers the agent to respond.

---

## Architecture

```
Telegram user → message → Telegram Bot → POST /api/v1/webhooks/{connector_name}
                                                      │
                                                      ▼
                                              WebhookHandler
                                                      │
                                                      ├── Lookup connector config from registry
                                                      ├── Validate auth (X-Webhook-Token header)
                                                      ├── Read/parse payload (platform-specific)
                                                      │
                                                      ▼
                                              MessageRouter
                                                      │
                                                      ├── Find or create session for workspace
                                                      ├── Append user message to session.History
                                                      ├── Persist session
                                                      ├── Publish inbound_message event via EventBus
                                                      │
                                                      ▼
                                              Frontend receives SSE event
                                              Shows inbound message in chat UI
                                              (User sees the message and can respond)

                                              Optional: trigger agent execution
                                              (Phase 2.5 — see Future Work)
```

### Connector Config Extension

Each connector entry in `registry.json` gains two new optional settings:

```json
{
  "connectors": {
    "my-telegram": {
      "type": "telegram",
      "enabled": true,
      "settings": {
        "chat_id": "12345",
        "workspace_id": "my-workspace",
        "webhook_token": "secret-from-botfather"
      },
      "secret_ref": "my-telegram"
    }
  }
}
```

| Setting | Purpose |
|---------|---------|
| `workspace_id` | Workspace to route inbound messages to. Required for inbound to work. |
| `webhook_token` | Secret token sent in `X-Webhook-Token` header. Telegram calls this the "secret token" set via BotFather. Used to authenticate incoming requests. |

---

## Backend — Files to Create

### `internal/transport/http/webhook_handlers.go` (NEW)

**`WebhookHandler`** struct:

```go
type WebhookHandler struct {
    registry    func() models.RegistryData
    persistence *persistence.WorkspaceManager
    builder     *AgentBuilder
    secrets     models.SecretsStore
    events      *automation.EventBus
}
```

**`POST /api/v1/webhooks/{connector_name}`** handler:

```go
func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    connectorName := r.PathValue("connector_name")
    
    // 1. Look up connector config
    reg := h.registry()
    cfg, ok := reg.Communication.Connectors[connectorName]
    if !ok || !cfg.Enabled {
        writeJSONError(w, http.StatusNotFound, "connector not found or disabled")
        return
    }
    
    // 2. Validate webhook token
    expectedToken := cfg.Settings["webhook_token"]
    if expectedToken != "" {
        actualToken := r.Header.Get("X-Webhook-Token")
        if actualToken != expectedToken {
            writeJSONError(w, http.StatusUnauthorized, "invalid webhook token")
            return
        }
    }
    
    // 3. Route to platform-specific parser
    workspaceID := cfg.Settings["workspace_id"]
    if workspaceID == "" {
        writeJSONError(w, http.StatusBadRequest, "connector has no workspace_id configured")
        return
    }
    
    message, err := parseInboundMessage(cfg.Type, r)
    if err != nil {
        writeJSONError(w, http.StatusBadRequest, err.Error())
        return
    }
    if message == "" {
        w.WriteHeader(http.StatusOK) // Telegram sends periodic health checks
        return
    }
    
    // 4. Find or create session
    sessions, _ := h.persistence.ListSessions(workspaceID, 1)
    var session *models.AssistantSession
    if len(sessions) > 0 {
        session, _ = h.persistence.ReadSession(workspaceID, sessions[0].ID)
    }
    if session == nil {
        session = &models.AssistantSession{
            ID:          uuid.New().String(),
            WorkspaceID: workspaceID,
            History:     []proxy.Message{},
        }
    }
    
    // 5. Append inbound message as user message
    session.History = append(session.History, proxy.Message{
        Role:    proxy.UserRole,
        Content: message,
    })
    
    // 6. Persist
    h.persistence.WriteSession(workspaceID, session)
    
    // 7. Publish event so the frontend receives it via SSE
    h.events.Publish(workspaceID, assistant.AgentEvent{
        Type: "inbound_message",
        Payload: map[string]any{
            "session_id": session.ID,
            "connector":  connectorName,
            "text":       message,
        },
    })
    
    // 8. Optionally trigger agent execution (see future work)
    
    respondJSON(w, map[string]string{"status": "ok"})
}
```

**Platform-specific parsers:**

```go
func parseInboundMessage(connectorType string, r *http.Request) (string, error) {
    switch connectorType {
    case models.ConnectorTypeTelegram:
        return parseTelegramWebhook(r)
    default:
        return "", fmt.Errorf("unsupported connector type for inbound: %s", connectorType)
    }
}

func parseTelegramWebhook(r *http.Request) (string, error) {
    var payload struct {
        Message struct {
            Text string `json:"text"`
            Chat struct {
                ID int64 `json:"id"`
            } `json:"chat"`
        } `json:"message"`
    }
    if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
        return "", fmt.Errorf("invalid telegram webhook payload: %w", err)
    }
    // Telegram sends periodic empty updates — treat as health check
    if payload.Message.Text == "" {
        return "", nil
    }
    return payload.Message.Text, nil
}
```

---

## Backend — Files to Modify

### `internal/app/bootstrap.go`

Register the webhook route (non-admin, publicly accessible):

```go
webhookHandler := &api.WebhookHandler{
    registry:    appCtx.GetRegistry,
    persistence: appCtx.Persistence(),
    builder:     builder,
    secrets:     appCtx.Secrets(),
    events:      appCtx.Events(),
}
mux.Handle("POST /api/v1/webhooks/{connector_name}", webhookHandler)
```

Note: This route is OUTSIDE the `/admin/api/` prefix — it must be publicly accessible since Telegram (and other platforms) need to POST to it without authentication.

### `internal/core/tools/communication.go`

No changes needed. The `Connector` interface stays as-is for outbound. Inbound is handled by the webhook handler directly.

### `models/config.go`

No changes needed. `settings` is already a generic `map[string]string` — `workspace_id` and `webhook_token` are just entries in the map.

### `automation/broadcast.go` — `EventBus`

No changes needed. `InboundMessage` can be published as an `AgentEvent.Type` string.

---

## Frontend — Files to Modify

### `frontend/src/components/settings/CommunicationSettings.vue`

- Add `workspace_id` input field to the add-form
- Add `webhook_token` (or reuse the bot token) input field
- Generate and display the webhook URL for each connector: `https://<host>/api/v1/webhooks/<connector_name>`
- The webhook URL should be copyable (add a CopyButton)
- Add a "Connect to Telegram" section that explains the setup steps

### `frontend/src/constants/api.ts`

No new endpoints needed — the webhook endpoint is for external platforms, not the frontend.

---

## Security

| Risk | Mitigation |
|------|-----------|
| Unauthenticated POST to webhook | `X-Webhook-Token` header validated against `settings.webhook_token`. Telegram sends this header if configured in BotFather. |
| Message spoofing | The token is a shared secret known only to the platform and the server. |
| Session race condition | `WriteSession` uses atomic rename — concurrent writes from webhook + user chat could interleave. Mitigation: simple per-workspace mutex in the webhook handler. |
| No rate limiting | Add a simple rate limiter per connector if needed. |

---

## Manual Testing Steps

1. Start server: `cd backend && go run main.go --data ./data`
2. Open Settings → Communication
3. Add a Telegram connector with:
   - Name: `my-telegram`
   - Chat ID: `12345`
   - Workspace ID: `<your-workspace-id>`
   - Bot token + webhook token set
4. Note the webhook URL displayed: `https://your-host/api/v1/webhooks/my-telegram`
5. Configure Telegram Bot via BotFather: set webhook to the above URL, set secret token to match `webhook_token`
6. Send a message to your bot in Telegram
7. Verify the message appears in the assistant UI
8. Verify the session is persisted (reload page, check session list)

---

## CONSTITUTION Compliance

| Section | Check |
|---------|-------|
| §5 — Network/Terminal | No outbound network calls in the webhook handler. Telegram sends the webhook to US. |
| §3 — No Telemetry | Webhook is user-configured, not telemetry. |
| §4 — Model Architecture | Agent session flow is reused from existing `handleAssistant` pattern. |

---

## No Changes Required (verified)

| Area | Reason |
|------|--------|
| Connector interface | Outbound only. Inbound is handled by the webhook handler. |
| Tool manifest / `notify_user` | Not changed. |
| Agent loop (agent.go, session.go) | Webhook appends to history but doesn't trigger agent execution in this phase. |
| Guardrails | Inbound messages are user text — no guardrail validation needed. |
| System prompt | Unchanged. |
| SSE event types | `inbound_message` is a new event type but follows existing pattern. |
| Frontend SSE composable | `useAssistantSSE` already handles unknown event types gracefully — new `inbound_message` type would be ignored unless we add a handler. |

---

## Future Work (Post-Phase 2)

### Phase 2.5 — Auto-Respond to Inbound Messages

After appending the inbound message, trigger the agent loop automatically:

```go
// After step 6 (persist session), trigger agent execution:
llmHistory := session.History  // or build from last N messages
builder := AgentBuilder{svc: h.svc, persistence: h.persistence, ...}
agent := builder.Build(workspaceID, ...)
events, updatedHistory, err := agent.Execute(ctx, llmHistory)
session.History = updatedHistory
h.persistence.WriteSession(workspaceID, session)
```

Challenges:
- Need to cancel any in-flight agent execution for this workspace (use the `running` sync.Map pattern from `assistant_handlers.go`)
- The agent's response events need to be published to the EventBus for the frontend to receive
- The agent's response also needs to be sent back through the connector (e.g. Telegram reply via `notify_user` or direct API call)

### Async Agent Execution

Since this plan was written, the webhook handler was changed to run the agent asynchronously:

**Double-append fix:** `handleAgentMessage` previously appended the user message to `session.History`, then `handleAssistant` appended it again. The webhook handler now delegates message appending entirely to `handleAssistant`, which is the single append point for both HTTP chat and webhook flows.

- `handleAgentMessage` persists the user message and returns immediately (200 OK to Telegram)
- The agent runs in a background goroutine via `RunWithCancel` (shared helper from `assistant_handlers.go`)
- `RunWithCancel` registers the workspace in the `running` map, making the agent cancellable via the cancel endpoint
- When the agent finishes, `replyToChat` sends the reply via the connector's `Send()` method (the canonical reply path)
- The frontend connects SSE on `initWorkspace`, so lifecycle events (`session_started`, `session_progress`, `session_completed`) arrive immediately

**Tool exclusion for webhook context:** The webhook handler sets `ExcludeTools: []string{models.ToolNotifyUser}` in the `AssistantMessage` payload. The agent builder wraps the `ToolProvider` with a `filteredToolProvider` (see `internal/core/assistant/filtered_provider.go`) that removes `notify_user` from `ListTools()`. The model never sees the `notify_user` tool in webhook context — replies are handled exclusively by `replyToChat` via the connector's `Send()` method.

This follows the OpenClaw pattern of a single output path (ReplyDispatcher): the agent produces output, and the handler routes it to the source connector. No ambiguity, no double-send possible. `notify_user` remains available for HTTP chat, automation, and cron where explicit broadcast is needed.

See `docs/skills/lifecycle-events.md` for the full webhook flow.

### Phase 2.6 — Automatic Reply via Connector

After the agent completes, send the response summary back through the same connector. This requires the webhook handler to know the agent's final answer and call `TelegramNotifier.Send(ctx, summary)` to reply.

---

## File Change Summary

| File | Action | Lines +/- |
|------|--------|-----------|
| `backend/internal/transport/http/webhook_handlers.go` | **Create** | ~160 |
| `backend/internal/app/bootstrap.go` | Add route registration | +5 |
| `frontend/src/components/settings/CommunicationSettings.vue` | Add workspace_id field, webhook URL display | +30 |
| `docs/SPECS/communication.md` | Add inbound webhook section | +40 |
| `docs/PLANS/cross-cutting/connector-system.md` | Mark Phase 1 reference | — |

Total: ~235 new lines
