---
id: SPEC-009
title: Communication Connector System
version: "1.4"
status: stable
last_updated: 2026-10-09
constitution_references: [II.4, II.5, V]
related_specs: [SPEC-001, SPEC-006]
supersedes:
---

# SPEC: Communication Connector System

## Changelog

- **1.4 (2026-10-09)** — Connector changes apply without a restart. The one shared `CommunicationTools` (agent
  `notify_user`, automation report delivery, inbound webhook) is rebuilt from config by `Reload` whenever the registry
  or the secrets change (`app.watchConnectorConfig`); previously a connector added or edited in Settings was only used
  after a restart, and deliveries failed with "connector … is not configured or enabled". Webhook re-registration
  remains a startup step.

- **1.3 (2026-10-03)** — Telegram hardening: part size is counted in UTF-16 units (Telegram's own
  measure, so emoji count twice); a 429 is retried once when `retry_after` is at most 10 s (a
  longer wait is returned as the error); transport errors no longer include the bot token in the
  request URL.

- **1.2 (2026-10-03)** — Telegram `Send` splits text over 4000 characters on line boundaries and
  retries a part as plain text when Telegram answers 400 "can't parse entities" (model-written
  text with stray `_`/`*` previously lost the whole message). Automation result delivery
  (`notify`, SPEC-007 §II.6) uses `Connector.Send` from the dispatcher; it is system-side, like
  the inbound reply path in 1.1, not agent egress.

- **1.1 (2026-09-06)** — Connector SEND gating under host network policy
  (SPEC-006 §II.7). Agent-initiated sends — the `notify_user` tool (including
  auto-reply and triggered messages issued *through the tool loop*) — are agent
  egress: when the effective network scope is none (host `sandboxing.network`
  off, or an automation grant of none), `notify_user` is hidden from the schema
  and denied synchronously, never routed to the approval flow. Inbound webhook
  receipt is server-side and unaffected (Constitution I.4); replies delivered to
  the *initiating* chat by the webhook handler (auto-reply to the inbound
  message, `/run` result replies) ride the guarded connector client as part of
  the inbound conversation flow and are exempt — the destination is the operator
  who initiated the message. A per-run `lan`/`internet` grant re-enables
  `notify_user` within the host ceiling.

## I. Intent

The agent needs the ability to send notifications, reports, and summaries to external platforms (Telegram, Slack, Discord, email, etc.) as part of its tool-use loop. The communication system must be generic — adding a new platform must require no struct changes, only a new `Connector` interface implementation and a new case in the registry switch.

A separate tool (`notify_user`) is registered in the tool system. When the agent calls it, the tool dispatches to all enabled connectors. This separation keeps the tool layer stable while connectors evolve independently.

The enabled connectors are built from the registry and secrets (`buildConnectors`) into one shared `CommunicationTools`, and rebuilt on every registry or secrets change, so a connector added, disabled or re-keyed in Settings is used by the next send. Connectors must therefore be cheap to construct and own no goroutines or long-lived resources: a replaced instance is dropped, not closed.

## II. Functional Requirements

### 1. Connector Interface

```go
type Connector interface {
    Name() string
    Send(ctx context.Context, message string) error
}
```

- `Name()` returns a human-readable platform name (e.g. `"Telegram"`, `"Slack"`). Used for error attribution in logs.
- `Send()` transmits the message to the external platform. Must be context-aware for cancellation and timeouts.

### 2. Config Model

`CommunicationConfig` uses a map of named connectors:

```go
type CommunicationConfig struct {
    Connectors map[string]ConnectorConfig `json:"connectors"`
}

type ConnectorConfig struct {
    Type      string            `json:"type"`      // e.g. "telegram"
    Enabled   bool              `json:"enabled"`
    Settings  map[string]string `json:"settings"`  // type-specific key-value pairs
    SecretRef string            `json:"secret_ref,omitempty"` // key in SecretsStore
}
```

- The map key is a user-assigned name (e.g. `"my-telegram"`, `"alerts-channel"`).
- `Settings` is a generic key-value bag. Each connector type documents its expected keys (e.g. `chat_id` for Telegram).
- `SecretRef` references a secret in the `SecretsStore` at path `("connector", name)`.

### 3. Connector Type Constants

Connector type strings are typed constants in `models/config.go`:

```go
const (
    ConnectorTypeTelegram = "telegram"
)
```

New connector types are added as new constants.

### 4. Tool Registration

The existing `notify_user` tool (manifests/communication.json) dispatches to all connectors:

```
Tool: notify_user
Arguments: { "message": string }
Category: communication
Behavior: iterates all connectors in CommunicationTools.connectors, calls Send() on each.
Return: "Notification sent successfully" on success; combined error on partial/full failure.
```

No agent loop changes — the tool is registered and called via the standard `execute_tool` flow (SPEC-001 §3).

### 5. Connector Initialization

In `registry.go`, `initCommunicationTools()` reads the config map and creates connectors:

```go
for name, cfg := range reg.Communication.Connectors {
    if !cfg.Enabled { continue }
    switch cfg.Type {
    case models.ConnectorTypeTelegram:
        token := secrets.GetSecret("connector", name)
        chatID := cfg.Settings["chat_id"]
        if token == "" || chatID == "" { continue }
        comm.AddConnector(name, tools.NewTelegramNotifier(token, chatID, network.HTTPClient()))
    default:
        log.Warn("unknown connector type", "name", name, "type", cfg.Type)
    }
}
```

- Each connector receives `network.HTTPClient()` for outbound HTTP (CONSTITUTION §5 compliance).
- Secrets are read per-connector at path `("connector", name)`, not a global `"communication"` path.

### 6. Telegram Connector

`TelegramNotifier` sends messages via the Telegram Bot API:

| Field | Source |
|-------|--------|
| Token | `SecretsStore.GetSecret("connector", name)` — injected via constructor |
| ChatID | `cfg.Settings["chat_id"]` — from config map entry |
| HTTP Client | `network.HTTPClient()` — from `NetworkTools` |

Implementation details:
- POST to `https://api.telegram.org/bot{token}/sendMessage`
- Form-encoded body with `chat_id`, `text`, `parse_mode=Markdown`
- Parts over 4000 UTF-16 units are split on line boundaries (a single longer line is hard-split);
  a 400 "can't parse entities" reply is retried once without `parse_mode`; a 429 with
  `retry_after` <= 10 s is retried once after that wait
- Response body read (up to 1KB) on error for diagnostic detail
- Uses injected `*http.Client`, never `http.DefaultClient`

### 7. Error Handling

- `NotifyAll()` collects errors from all connectors into a combined error.
- Each error is attributed to the connector name + platform name (e.g. `"my-telegram (Telegram): ..."`).
- Partial failures do not block other connectors — all connectors are attempted.
- A connector with missing credentials (empty token or chat_id) is silently skipped at init time.

## III. Architecture

```
registry.json "communication" block
        │
        ▼
initCommunicationTools()          (registry.go)
        │
        ├── for each Connectors[name]:
        │   ├── check Enabled
        │   ├── switch on Type → instantiate connector
        │   │   └── TelegramNotifier (token + chatID + httpClient)
        │   └── AddConnector(name, connector) → CommunicationTools.connectors map
        │
        ▼
LocalToolRegistry.Communication    (registry.go)
        │
        ▼
registerCommunicationTools()       (registry.go)
        │
        ├── tool: notify_user
        └── handler: r.Communication.NotifyAll(ctx, args.Message)
                │
                ▼
        CommunicationTools.NotifyAll()
                │
                ├── range over connectors map
                └── conn.Send(ctx, message)
```

### Data Flow

```
Agent calls notify_user({"message": "..."})
    → tool execution (tool_exec.go)
    → handler in registry.go
    → CommunicationTools.NotifyAll(ctx, message)
    → TelegramNotifier.Send(ctx, message)
    → POST to Telegram API via NetworkTools HTTP client
    → return success/error back through tool call chain
```

### Config Persistence

- Stored in `registry.json` under `communication.connectors`.
- Admin API exposes via `AdminSystemHandler` (read) and `SystemUpdatePayload` (write).
- Frontend Communication Settings panel reads/writes through the admin API.

## IV. CONSTITUTION References

| Section | Requirement | Compliance |
|---------|-------------|------------|
| II.4 | Tool call format (XML) | Unchanged. `notify_user` uses existing tool schema. |
| II.5 | System prompt stability | Unchanged. Tool descriptions auto-generated. |
| V | Network I/O via NetworkTools | Telegram uses `network.HTTPClient()`, never `http.DefaultClient`. |

## V. Related SPECs

| SPEC | Relationship |
|------|-------------|
| SPEC-001 | Agent loop orchestrates tool calls including `notify_user`. |
| SPEC-006 | Communication guardrails (review caps, require_review) apply to `notify_user`. |

## VI. Future Work (Phase 2+)

- **Inbound webhook**: Receive messages from external platforms (e.g. Telegram Bot webhook -> inject into active session).
- **`check_incoming_messages` tool**: Poll-based inbound for agents to check for user updates.
- **Additional connector types**: Slack, Discord, email, etc. — each needs a `Connector` implementation and a switch case.
