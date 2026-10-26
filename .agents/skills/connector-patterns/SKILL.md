---
name: connector-patterns
description: "Communication connector implementation guide: outbound connector steps, inbound webhook, CONSTITUTION checklist, and common errors. Use when adding or changing a connector."
metadata:
  last_reviewed: "2026-10-26"
---

# Communication Connector Implementation Guide

**Source docs:** SPEC-009, `docs/PLANS/ARCHIVE/cross-cutting/connector-system.md`, `docs/PLANS/ARCHIVE/cross-cutting/connector-inbound-webhook.md`

---

## Reference files

Open only the one your task needs:

| File | Open it when |
|------|--------------|
| [`references/outbound-connector.md`](references/outbound-connector.md) | You are adding or changing an outbound connector. |

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
- The message triggers an agent run (`RunWithCancel` → `runAgentReply`); the run's `session_*` lifecycle events are published to the workspace EventBus on the assistant channel. There is no `inbound_message` event type.

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

- **Connectors are rebuilt live.** `CommunicationTools.Reload` rebuilds every connector whenever the registry or
  secrets change (SPEC-009 1.4), so a factory must be cheap and side-effect free: no goroutines, timers or network
  calls at construction. Startup-only work (webhook re-registration) belongs in `initCommunicationTools`, not the
  factory.

| Symptom | Cause | Fix |
|---------|-------|-----|
| Connector silently skipped at init | Token not saved to secrets store | Call `PUT /admin/api/secrets/tools?category=connector&provider=<name>` from frontend |
| "some notifications failed" in agent response | One or more connectors returned an error | Check individual errors in the combined message |
| 401 from Telegram API | Token invalid or revoked | Update token via Communication Settings UI |
| Webhook returns 404 | Connector name doesn't match config map | Check the connector_name in the URL matches the config key |
| HTTP call never reaches target | Client injected but nil | Verify `network.HTTPClient()` returned non-nil |
