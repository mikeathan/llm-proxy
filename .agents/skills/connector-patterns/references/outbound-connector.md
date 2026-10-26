# Communication Connector Implementation Guide — Outbound Connector — step by step

Reference for the [`connector-patterns`](../SKILL.md) skill, loaded on demand. You are adding or changing an outbound connector.

Contents: Outbound Connector

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

### Step 3 — Factory Registration

Register a connector factory in the notifier package's `init()` so the registry can build it dynamically by type string. Do NOT edit `initCommunicationTools` or `registry.go`:

```go
package notifiers

import (
    "llm-proxy/internal/core/tools"
    "llm-proxy/models"
)

func init() {
    tools.RegisterConnectorFactory("slack", func(
        name string,
        cfg models.ConnectorConfig,
        secrets models.SecretsStore,
        network *tools.NetworkTools,
    ) (tools.Connector, bool) {
        webhookURL := cfg.Settings["webhook_url"]
        if webhookURL == "" {
            return nil, false
        }
        return NewSlackNotifier(webhookURL, network.HTTPClient()), true
    })
}
```

**Pattern:**
- Read each setting from `cfg.Settings["key"]`.
- Read the secret/token from `secrets.GetSecret("connector", name)`.
- Call `network.HTTPClient()` — never construct a client directly.
- If required settings are missing, return `(nil, false)` to skip the connector (warning logged automatically by `buildConnector`).
- Return `(tools.Connector, true)` on success.
- See `backend/internal/core/tools/notifiers/telegram.go` `init()` for a reference implementation.

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
