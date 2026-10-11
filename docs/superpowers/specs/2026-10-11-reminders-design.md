# Reminders — design (v1)

**Status:** draft for review · **Date:** 2026-10-11
**Narrows:** `docs/PLANS/autonomy/agenda-reminders-and-followups.md` (Agenda M1) to reminders only. Follow-ups
(`action: run`), `RunReason`, the new run-lane kind and the Agenda UI are explicitly out of scope and stay in that plan.

## Goal

In the assistant (web chat or an inbound connector such as Telegram) the user says "remind me tomorrow morning to
call the bank" or "…every Monday at 9, on Telegram". The reminder is stored and delivered verbatim at that time.
No LLM runs at fire time.

## Behaviour (what the user sees)

| Case | Result |
|---|---|
| No channel named | Delivered to the **default reminder channel** (setting; defaults to the first configured connector type, preferring `telegram`), and always recorded as an in-app notice in the web UI. |
| Channel named ("on Telegram") | Delivered to that connector type only. If that type is not configured, the tool errors **at creation** and the assistant tells the user; nothing is stored. |
| Vague time ("morning", "evening", "tonight") | Mapped by a small fixed table (morning 09:00, afternoon 14:00, evening 18:00, tonight 20:00) in the reminder's timezone. |
| Confirmation | The tool result echoes the resolved absolute time, timezone and channel ("Mon 12 Oct 09:00 Europe/London via telegram"); the assistant must repeat it, so a wrong guess is visible immediately. |
| Recurring | Cron expression plus optional `until`. Uses the one existing cron grammar (`core/automation/cron.go`: 5 fields or descriptor, no seconds). |
| Server was down at fire time | Due < 12 h ago → delivered with a "(late)" prefix. Older → status `missed`, shown as an in-app notice, not sent to the connector. Recurring items skip to the next future occurrence. |
| Delivery failure | Retried on the next wake with backoff (3 attempts); then status `failed` and an in-app notice with the error. Never reported as delivered unless a connector returned nil. |
| Cancel / list | Tools `list_reminders` and `cancel_reminder`, so "what reminders do I have?" and "cancel the bank one" work from chat. |

## Timezone

Resolved in this order when the tool is called: the request's timezone (browser sends it per request; already in the
system prompt) → new optional setting `owner_timezone` (IANA name) → server local zone. The inbound-connector path
currently passes `"UTC"` (`webhook_handlers.go:181`); it falls through to `owner_timezone`/server-local instead of using
that UTC. The resolved zone is stored on the reminder so later DST changes are computed in that zone.

## Architecture

New package `internal/core/reminders` (one purpose: store + scheduler + delivery), wired once from the app bootstrap
layer (AGENTS.md centralization rule).

- **Store** — table `reminders` in the shared SQLite DB (`platform/db`): `id, message, fire_at (UTC), cron, until,
  timezone, connector (""=default), status (scheduled|done|cancelled|missed|failed), attempts, created_by, created_at,
  last_error`. Schema created via the same migration point the memory/ledger stores use (confirm in Phase 1).
- **Scheduler** — a single goroutine tethered to the root context that sleeps until the earliest `fire_at`
  (timer reset on create/cancel via a wake channel), not a polling tick. Loads from the DB at startup, which is also
  where the missed-item policy runs. Injected clock for tests.
- **Delivery** — `Deliverer` interface satisfied by `CommunicationTools.NotifyAll(ctx, msg, connectorType)` plus an
  in-app notice sink. Default-channel resolution lives in the reminders package, not in the tool.
- **Tools** — `schedule_reminder(message, when | cron+until, connector?)`, `list_reminders()`,
  `cancel_reminder(id)`. Manifest in `tools/manifests/reminders.json`, implementation in `tools/reminders.go`, registered
  per the architecture.md "new tool category" checklist (`models/tools.go`, `assistant/registry.go`). `when` is an
  RFC 3339 timestamp the model computes from the current time/timezone already in the system prompt; the tool rejects
  past times and caps the horizon (1 year) and open reminders (50).
- **Config** — `reminders.default_connector` and `owner_timezone`, persisted per `config-persistence` skill (settings,
  API mapping, reload), with a small control in the existing settings UI. No new settings tab.
- **In-app notice** — reuse the existing event bus/notice mechanism if one fits (to verify in Phase 1; otherwise a
  minimal `reminder_due` event the Overview/Activity UI renders). No Agenda view in v1.

## Guardrails and constitution checks

- Reminder text is user content delivered verbatim; it is never executed or interpolated into a prompt.
- Outbound sends go only through configured connectors (existing egress rules); no new network paths.
- The tool's `guardrails.require_review` stays false (no external side effect at creation; delivery goes to the
  user's own connectors).
- Scheduler goroutine exits on root-context cancel; timer stopped on exit (no leak).

## Testing (TDD, per AGENTS.md)

1. `reminders` package with a fake clock: fire on time, two reminders reordered by an earlier insert, cancel before
   fire, cron next-occurrence across a DST change, restart reload, late (<12 h) vs missed (≥12 h), retry then failed.
2. Default-channel resolution: named, unnamed with telegram configured, unnamed with none configured (in-app only),
   named but unconfigured → creation error.
3. Tool tests: vague phrases via explicit RFC 3339 input, past time rejected, echo contains absolute time and zone,
   limits enforced.
4. Agent-loop test with a mock LLM calling `schedule_reminder` → stored row with resolved UTC time.
5. `go test -race ./internal/core/reminders/` (new goroutine), `go run ./tools/check-complexity/`,
   `./scripts/check-agent-harness.sh`.

## Phases

1. **Store + scheduler + delivery** (package only, fake clock, no wiring).
2. **Tools + registry + config + timezone fallback**; webhook path uses the new fallback.
3. **In-app notice + settings control**.
4. **Docs**: new SPEC entry (reminders), SPEC-009 note on default channel, `docs/api-reference.md` if an API is added,
   Agenda plan marked partially delivered, `docs/PLANS/README.md`, tool/manifest docs.

## Out of scope

Self-scheduled follow-ups that run the agent, `RunReason`, new run-lane kind, Agenda/Today UI, snooze, attention-policy
routing (delivery calls `NotifyAll` directly until that plan lands), calendar sync, location reminders.

## Open items to verify in Phase 1 (not assumptions to build on)

- Where SQLite schemas are created/migrated for memory/ledger, and the exact in-app notice mechanism.
- Whether `NotifyAll` to a type with several instances is acceptable (it sends to all matching) — assumed yes.
