# PLAN: Communication Connector Auto-Reply & Automation Trigger — Phase 3

**Status:** Active  
**SPEC:** `docs/SPECS/communication.md`  
**Depends on:** Phase 1 (connector-system.md), Phase 2 (connector-inbound-webhook.md) — both complete

---

## Goal

Turn the webhook from a passive inbox into an interactive gateway. Two message paths:

```
Telegram sends "list workspace files"
  → AGENT PATH: per-source session → trigger agent → reply via same connector

Telegram sends "/run scan_network"
  → AUTOMATION PATH: dispatcher.Trigger() → reply "Running..." → async result back
```

Both paths record the message in session history for audit trail. Automation results also appear in Pulse dashboard (existing).

---

## Architecture

```
POST /api/v1/webhooks/{connector_name}
  ├── validate token
  ├── parse payload → (message, sourceChatID)
  ├── detect "/run" prefix?
  │     ├── YES → AUTOMATION PATH
  │     └── NO  → AGENT PATH
  │
  ├── AGENT PATH:
  │     ├── findOrCreateSession(workspaceID, connectorName, sourceChatID)
  │     │     └── session key = "wb_{platformType}_{chatID}_{timestamp}"
  │     ├── append message + persist
  │     ├── call h.Assistant.handleAssistant(ctx, payload)
  │     │     └── agent executes, history updated, reply returned
  │     ├── extract reply from result
  │     └── CommunicationTools.SendTo(ctx, connectorType, reply)
  │
  └── AUTOMATION PATH:
        ├── parse automation name from "/run {name}"
        ├── h.Dispatcher.Trigger(ctx, workspaceID, name, "")
        ├── reply immediately: "Running {name}..."
        └── subscribe to Dispatcher events, when run_complete → SendTo result
```

---

## UX: Session Indicators & Notifications

Two small UI additions make the system feel responsive even with the sidebar collapsed:

### 1. Red Dot on Sidebar Toggle Button

When a new `inbound: true` message arrives via SSE or a `/run` automation completes, a red dot appears on the chevron button in the chat header:

```
[ •❮ ] [+] Agent Online    [×]
```

- Reactive state: `inboundCount` ref increments on each `inbound` event
- Resets to 0 when sidebar is opened (`sidebarOpen → true`)
- Only shows when the assistant chat is open (SSE connected)
- ~10 lines CSS + ~5 lines reactive state

### 2. Source Icon in Session List

When the sidebar is expanded, sessions that originated from an external platform show a platform icon next to the title:

```
📞 [Telegram] scan report 2 min ago
💬 [Web] list workspace files 5 min ago
```

- Session ID prefix `wb_` → Telegram icon
- Session ID prefix `conv_` → Web UI icon
- Driven by the session ID format already in use
- ~5 lines in ChatSessionList.vue template

No Pulse changes in this phase. Future refactor will consolidate Pulse, inbound alerts, and automation monitoring into a single dashboard that doesn't duplicate data.

---

## Backend — Files to Modify

### `webhook_handlers.go`

**New fields:**
```go
type WebhookHandler struct {
    Registry    func() models.RegistryData
    Persistence *persistence.WorkspaceManager
    Secrets     models.SecretsStore
    Events      *automation.EventBus
    CommTools   *tools.CommunicationTools     // for sending replies
    Dispatcher  *automation.Dispatcher         // for /run triggers
    Assistant   *AssistantMessageHandler       // for agent execution
    Logger      logging.Logger
}
```

**`ServeHTTP` refactored routing:**

```go
func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    // ... existing: validate token, parse payload, get (message, sourceChatID) ...

    if strings.HasPrefix(message, "/run ") {
        h.handleAutomationTrigger(ctx, workspaceID, connectorName, cfg.Type, message)
    } else {
        h.handleAgentMessage(ctx, workspaceID, connectorName, cfg.Type, message, sourceChatID)
    }

    respondJSON(w, map[string]string{"status": "ok"})
}
```

**`handleAgentMessage`:**
- Per-source session key: `webhookSessionKey(platformType, chatID)` → `"wb_{platformType}_{chatID}_{timestamp}"` (fresh session per message)
- Append message → persist → call `h.Assistant.handleAssistant()` with payload
- Extract `reply` from result map → `h.CommTools.NotifyAll(ctx, reply, connectorType)`

**`handleAutomationTrigger`:**
- Extract `automationName := strings.TrimPrefix(message, "/run ")`
- `h.Dispatcher.Trigger(ctx, workspaceID, automationName, "")`
- Immediate reply: `"Running {automationName}..."` via `NotifyAll`
- Async: subscribe to `h.Dispatcher.Events()`, on `run_complete` → send result back
- Timeout after 30s: send "Automation started, see Pulse for results"

**Telegram payload parser updated** — extract `chat.id` field for session key:

```go
type telegramPayload struct {
    Message struct {
        Text string `json:"text"`
        Chat struct {
            ID int64 `json:"id"`
        } `json:"chat"`
    } `json:"message"`
}
```

### `bootstrap.go`

Wire new dependencies into `WebhookHandler`:

```go
webhookHandler := &api.WebhookHandler{
    Registry:    s.AppCtx.GetRegistry,
    Persistence: s.Persistence(),
    Secrets:     s.AppCtx.Secrets(),
    Events:      s.Events(),
    CommTools:   s.ToolProvider().(*LocalToolRegistry).Communication, // or expose via interface
    Dispatcher:  disp,
    Assistant:   assistant,
    Logger:      s.Logger(),
}
```

Note: `CommTools` access may need a getter added to `ToolProvider` or exposed from the registry as a public field.

---

## Frontend — Files to Modify

### `ChatSessionList.vue`

Show a platform icon next to each session based on its ID prefix:

```vue
<span class="session-source-icon">
  <Icon v-if="sourceIcon(session.source ?? sessionSource(session.id))" :name="sourceIcon(session.source ?? sessionSource(session.id))!" />
</span>
```
> Source comes from the backend (`SessionBrief.source` / SSE `source`); the frontend only maps it via `utils/assistant/source.ts`. It no longer parses the session ID prefix.

### `AssistantChat.vue`

Add `inboundCount` ref:

```typescript
const inboundCount = ref(0)

// In the SSE handler or watcher:
watch(() => displayEvents.value, () => {
  const last = displayEvents.value[displayEvents.value.length - 1]
  if (last?.payload?.inbound) {
    inboundCount.value++
  }
})

watch(sidebarOpen, (open) => {
  if (open) inboundCount.value = 0
})
```

Template — red dot on toggle button:

```vue
<button @click="toggleSidebar" class="btn-header-action relative">
  <Icon :name="sidebarOpen ? 'chevron-left' : 'chevron-right'" size="sm" />
  <span v-if="inboundCount > 0 && !sidebarOpen" class="badge-dot" />
</button>
```

CSS:

```css
.badge-dot {
  @apply absolute top-0 right-0 w-2 h-2 bg-red-500 rounded-full border-2 border-gray-800;
}
```

---

## Files Not Changed

| File | Reason |
|------|--------|
| `communication.go` | `NotifyAll` already filters by type |
| `assistant_handlers.go` | `handleAssistant` is reused, not modified |
| `models/config.go` | No config changes |
| `manifests/communication.json` | Tool unchanged |
| Pulse dashboard | Future refactor |

---

## CONSTITUTION Compliance

| Section | Check |
|---------|-------|
| §5 — Network/Terminal | Outbound replies go through `CommunicationTools` → `NetworkTools`, as already implemented |
| §4 — Model Architecture | Agent execution reused from existing `handleAssistant` |
| §3 — No Telemetry | User-configured webhook |

---

## Manual Testing Steps

1. Start server
2. Connect Telegram with workspace_id set
3. Register webhook URL with Telegram Bot API
4. Send "list workspace files" to Telegram bot
   → Agent receives it, processes, reply sent back to Telegram
5. Send "/run daily_report" to Telegram bot
   → Immediate "Running daily_report..." reply
   → Automation executes, result sent back
6. Open assistant chat → verify session exists with Telegram message
7. Collapse sidebar → new Telegram message → red dot appears on chevron

## Future Work

- Pulse dashboard refactor: merge automation runs, inbound alerts, system metrics
- Multiple chat IDs per connector (group chats, multiple users per workspace)
- Voice memo transcription via Telegram voice messages
