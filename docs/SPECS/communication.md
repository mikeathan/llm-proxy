---
id: SPEC-009
title: Communication Connector System
version: "1.0"
status: stable
last_updated: 2026-06-28
constitution_references: [II.4, II.5, V]
related_specs: [SPEC-001, SPEC-006]
supersedes:
---

# SPEC: Communication Connector System

## I. Intent

The agent needs the ability to send notifications, reports, and summaries to external platforms (Telegram, Slack, Discord, email, etc.) as part of its tool-use loop. The communication system must be generic — adding a new platform must require no struct changes, only a new `Connector` interface implementation and a new case in the registry switch.

A separate tool (`notify_user`) is registered in the tool system. When the agent calls it, the tool dispatches to all enabled connectors. This separation keeps the tool layer stable while connectors evolve independently.

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
