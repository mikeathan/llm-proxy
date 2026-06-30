# Communication Connector Implementation Guide

**Source docs:** SPEC-009, `docs/PLANS/cross-cutting/connector-system.md`, `docs/PLANS/cross-cutting/connector-inbound-webhook.md`

---

## Outbound Connector

Every communication platform you add must implement the `Connector` interface and follow the exact 6-step registration pattern below. No struct changes in `config.go` are needed — the map-based config handles any new platform generically.

### Step 1 — Constant

Add a typed constant in `backend/models/config.go`:

```go
const (
    ConnectorTypeTelegram = "telegram"
    ConnectorTypeSlack    = "slack"   // future
    ConnectorTypeDiscord  = "discord" // future
)
```

### Step 2 — Interface Implementation

Add a new struct in `backend/internal/core/tools/communication.go`:

```go
type SlackNotifier struct {
    WebhookURL string
    client     *http.Client
}

func NewSlackNotifier(webhookURL string, client *http.Client) *SlackNotifier {
    return &SlackNotifier{WebhookURL: webhookURL, client: client}
}

func (s *SlackNotifier) Name() string { return "Slack" }

func (s *SlackNotifier) Send(ctx context.Context, message string) error {
    // POST to WebhookURL using s.client
    // Handle non-200 responses
}
```

**Critical rules:**
- Constructor must accept `*http.Client` — never use `http.DefaultClient` (CONSTITUTION §5).
- `Name()` returns a human-readable platform name for error attribution.
- `Send()` must be context-aware (use `http.NewRequestWithContext`).
- Read response body on error for diagnostic detail (io.LimitReader to 1KB).
- Call `defer resp.Body.Close()` immediately after the response check.

### Step 3 — Registry Switch

Add a case in `initCommunicationTools` at `backend/internal/core/assistant/registry.go`:

```go
case models.ConnectorTypeSlack:
    webhookURL := cfg.Settings["webhook_url"]
    if webhookURL == "" {
        continue
    }
    comm.AddConnector(name, tools.NewSlackNotifier(webhookURL, network.HTTPClient()))
```

**Pattern:**
- Read each setting from `cfg.Settings["key"]`.
- Read the secret/token from `appCtx.Secrets().GetSecret("connector", name)`.
- Call `network.HTTPClient()` — never construct a client directly.
- If required settings are missing, `continue` (skip connector, log warning already handled by default case).

### Step 4 — Frontend Settings Option

Add a new `<option>` to `CommunicationSettings.vue`:

```vue
<select v-model="form.type" class="form-input">
  <option value="telegram">Telegram</option>
  <option value="slack">Slack</option>
</select>
```

Add the platform-specific settings fields to the `add-form` section. For each new field, add it to the `ConnectorForm` interface and the `settings` object in `addConnector()`.

### Step 5 — Tests

Add tests in `backend/internal/core/tools/communication_test.go`:

```go
func TestSlackNotifier_Send_Success(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Assert method, headers, body
        w.WriteHeader(http.StatusOK)
    }))
    defer srv.Close()

    n := NewSlackNotifier(srv.URL, srv.Client())
    // Rewrite the test URL: n.client.Transport rewrites to srv.URL
    err := n.Send(context.Background(), "test")
    // Assert no error
}
```

Use the `roundTripperFunc` adapter from `communication_test.go` when the API URL is hardcoded (like Telegram's).

### Step 6 — Documentation

- Update `docs/SPECS/communication.md` with the new platform's details.
- Add a plan entry to `docs/PLANS/cross-cutting/` if the work is large.

---

## Inbound Webhook

Inbound messages are handled by `WebhookHandler` (`POST /api/v1/webhooks/{connector_name}`), not through the `Connector` interface. Each platform needs:

1. A payload parser function (e.g. `parseTelegramWebhook`) in `webhook_handlers.go`
2. A case in the `parseInboundMessage` switch

**Security:**
- Validate the `X-Webhook-Token` header against `cfg.Settings["webhook_token"]`.
- For Telegram: set a secret token via BotFather, it's sent as `X-Telegram-Bot-Api-Secret-Token`.
- Health-check payloads (empty messages from Telegram) return 200 with no processing.

**Routing:**
- The connector must have `workspace_id` in `cfg.Settings`.
- Messages are appended to the latest session for that workspace.
- An `inbound_message` event is published to the EventBus.

---

## CONSTITUTION Compliance Checklist

| Rule | Check |
|------|-------|
| §5 — No `http.DefaultClient` | Constructor accepts injected `*http.Client` |
| §5 — Context propagation | `context.Context` as first arg, `NewRequestWithContext` |
| §4 — No prompt changes | Tool manifest unchanged (globally named `notify_user`) |
| §3 — No telemetry | Connector data is user-configured, no auto-reporting |

---

## Common Errors

| Symptom | Cause | Fix |
|---------|-------|-----|
| Connector silently skipped at init | Token not saved to secrets store | Call `PUT /admin/api/secrets/tools?category=connector&provider=<name>` from frontend |
| "some notifications failed" in agent response | One or more connectors returned an error | Check individual errors in the combined message |
| 401 from Telegram API | Token invalid or revoked | Update token via Communication Settings UI |
| Webhook returns 404 | Connector name doesn't match config map | Check the connector_name in the URL matches the config key |
| HTTP call never reaches target | Client injected but nil | Verify `network.HTTPClient()` returned non-nil |
