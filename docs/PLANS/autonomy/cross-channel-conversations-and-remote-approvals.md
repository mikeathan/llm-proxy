---
status: proposed
date: 2026-10-04
last_reviewed: 2026-10-04
related_specs: [SPEC-009, SPEC-006, SPEC-001, SPEC-003]
constitution_references: [I.4, II.2, II.10, II.14, III.2, IV.1, IV.2, V.1, V.2, V.3]
related_plans: [autonomy-roadmap.md, attention-policy.md, ../cross-cutting/connector-auto-reply.md, ../cross-cutting/webhook-fresh-sessions.md, ../cross-cutting/admin-api-authentication.md]
---

# Cross-Channel Conversations and Remote Approvals (milestones M1 + M3)

**Status:** proposed. **Phase 0 is a security fix with no open decision and should ship first, ahead of
the rest of the autonomy program.** Later phases need D1–D4.

## Why this plan exists

Three problems, one surface (Telegram today, more connectors later):

1. **Anyone can drive the agent through the bot.** The inbound webhook verifies that a request came from
   *Telegram* (secret header), but never checks *who* sent the message. Anyone who finds the bot's
   username can start agent runs in the configured workspace, with whatever tools that workspace allows.
2. **No continuity.** Every inbound message starts a brand-new session; the assistant forgets the message
   you sent a minute ago, and a conversation started in the web UI cannot be continued on the phone.
3. **Blocked actions wait for a browser.** Guardrail approvals can only be answered in the web UI. An
   unattended run that hits a blocked call waits (default 5 minutes) and is then denied — autonomy stops
   the moment you are away from the desk.

| User benefit | Service benefit |
|---|---|
| Only *you* can talk to your assistant through Telegram. | Closes an unauthenticated path to agent runs (terminal, filesystem, network tools). |
| One conversation: start on the phone, continue at the desk, and back. Replies to reminders and goal questions land in context. | One owner identity used by the attention policy (where to deliver) and goals (who to ask). |
| Approve / Deny / Always allow from your phone, with buttons. | Unattended runs and goals keep moving safely; guardrails are unchanged — only *where* you can answer grows. |

## Ground truth (verified in code, 2026-10-04)

| Fact | Source |
|---|---|
| Webhook auth: optional `webhook_token` compared against `X-Telegram-Bot-Api-Secret-Token`; **the sender's chat ID is never compared with the configured `chat_id`** | `backend/internal/transport/http/handlers/webhook_handlers.go:66-99` |
| Inbound parsing reads only `message.text` and `message.chat.id`; button presses (`callback_query`) produce an empty message and are ignored | `webhook_handlers.go:252-275`, `:84-88` |
| Every inbound message gets a fresh session `wb_{type}_{chat}_{timestamp}` — deliberately, because some models (Qwen) re-emitted previous answers when they inherited history | `webhook_handlers.go:225-235`; `../cross-cutting/webhook-fresh-sessions.md` |
| Agent replies run in a goroutine with `context.Background()` (not tethered to the app root context) | `webhook_handlers.go:191`, `:196`, `:216` — Constitution II.2/II.14 |
| Inbound runs pass `Timezone: "UTC"` regardless of the user | `webhook_handlers.go:181` |
| Connector interface is text-only: `Send(ctx, message)`; optional capability interfaces already exist as a pattern (`WebhookAware`) | `backend/internal/core/tools/communication.go:15-28` |
| Guardrail approval: `GuardrailDecisionStore` keeps pending decision channels keyed by `DecisionID`; the agent waits on the channel or ctx; a timed-out decision keeps a tombstone so a late "allow & remember" still persists an override | `backend/internal/core/assistant/agent.go:330-470` |
| Decision shape: `{Allow, Persist}` | `backend/internal/core/assistant/agent_events.go:123-126` |
| Default approval wait: 5 minutes, configurable (`guardrail_approval_timeout_seconds`); the comment at `agent.go:449-451` still says "60s" (stale) | `agent.go:62`; `backend/internal/transport/http/handlers/admin_handlers.go:167`, `:404` |
| Web UI answers via `POST /admin/api/conversation/guardrail-decision` | `backend/internal/app/routes.go:263` |
| The approval wait happens inside the run, which holds its run-lane slot (for local models, the only slot) | `backend/internal/core/automation/admission.go:44-60` (the lane job runs `executeAutomation`) |
| The active auto-reply plan's Future Work already lists "multiple chat IDs per connector" | `../cross-cutting/connector-auto-reply.md` → Future Work |

## Design

### Owner identity and channel links
- Owner config section (roadmap R1) holds **linked channels**: `{connector, chat_id, verified_at}`.
- **Linking flow:** Settings → Channels → "Link Telegram" shows a one-time code; you send `/link <code>`
  to the bot; the chat is linked. Existing configs migrate their configured `chat_id` as already linked.
- **Inbound rule:** a message from an unlinked chat gets one fixed reply ("This assistant is private.")
  and never reaches the agent; rate-limited and logged.

### Conversation continuity
- Inbound messages from a linked channel continue the owner's **active thread**: the session you used
  most recently on any channel if it was active within T hours, else a new thread. `/new` forces a new
  thread; the web UI shows the channel of every message.
- **Addressing the earlier reason for fresh sessions:** the history is not replayed raw. Continuity uses
  the existing context machinery — sieve, progress ledger, hot memory — and a regression test with the
  recorded Qwen case from `../cross-cutting/webhook-fresh-sessions.md` must pass before continuity is on by default.
  Continuity can be disabled per model (D2).
- Replies are sent to the channel the message came from; *proactive* messages go where the attention
  policy says.

### Interactive connectors
A new optional interface, following the `WebhookAware` pattern so `Connector` stays unchanged:
```go
type InteractiveSender interface {
    SendWithActions(ctx context.Context, message string, actions []Action) (messageRef string, err error)
    UpdateMessage(ctx context.Context, messageRef, message string) error // "Approved ✓"
}
```
Telegram implements it with inline keyboards; inbound `callback_query` is parsed, acknowledged
(`answerCallbackQuery`) and dispatched. Callback data carries a short opaque token (Telegram limits it
to 64 bytes) that maps to the `DecisionID` server-side.

### Remote approvals
- On `guardrail_blocked`, besides the existing UI event, the approval is submitted to the attention policy
  as an `urgent` notice with actions **Approve once / Deny / Always allow** (= `{Allow, Persist}`).
- A button press from a **linked** chat resolves the same `GuardrailDecisionStore` entry as the web UI;
  whichever answers first wins, and the other surface updates ("approved from Telegram"). Presses from
  any other chat are rejected.
- **Longer waits without blocking the GPU** (D3): for unattended runs the approval wait can be longer
  than 5 minutes, but a run waiting for a human must not hold the only local slot all night. v1: a
  separate, longer timeout for unattended runs plus a lane-release rule is investigated in Phase 3;
  if releasing the slot mid-run is not feasible, the run is cancelled at timeout and its goal/agenda
  item is rescheduled after the decision arrives (the decision is persisted via the existing tombstone).

## Decisions for the user

| # | Decision | Recommendation |
|---|---|---|
| D1 | Continuity window T | 6 hours |
| D2 | Continuity default for models that misbehaved before | On by default; off per model in model settings; the regression test gates the default |
| D3 | Unattended approval wait | 30 minutes, then cancel and reschedule (Phase 3 decides whether the lane slot can be released while waiting) |
| D4 | "Always allow" from Telegram | Allowed, but the confirmation message names exactly what was allowed and links to undo it in Settings |

## Phases

### Phase 0 — Inbound security and hygiene (no decision needed; ship first)
- Reject messages whose chat ID is not the connector's configured `chat_id` (later: linked channels);
  fixed reply + log + rate limit.
- Tether `runAgentReply`/`replyToChat` to the application root context (II.2/II.14).
- Pass the owner timezone instead of `"UTC"` (falls back to server local until roadmap R2 lands).
- Fix the stale "60s" comment in `agent.go`.
- **Acceptance:** handler tests — message from another chat ID → no agent run, fixed reply; configured
  chat → unchanged behaviour; `go test ./internal/transport/http/handlers/ -run Webhook -count=1`;
  `go test -race` on the handler package (goroutine lifecycle changed).

### Phase 1 — Owner identity and linking
- Owner config section, linking flow, migration of existing `chat_id`.
- **Acceptance:** tests for link with valid/expired/reused code; restart keeps links (atomic write, III.2).

### Phase 2 — Conversation continuity (needs D1, D2)
- Active-thread resolution, `/new`, channel tag on messages, Qwen regression test from recordings.
- **Acceptance:** record-replay test — two Telegram messages 5 min apart share a session and the second
  answer uses the first; the recorded re-emission case does not reproduce.

### Phase 3 — Interactive connectors and remote approvals (needs D3, D4)
- `InteractiveSender`, Telegram inline keyboards, `callback_query` parsing, decision routing, cross-surface
  update, unattended timeout/reschedule (and the lane-release investigation).
- **Acceptance:** test — blocked tool → Telegram receives buttons; press from linked chat resolves the
  decision and the web UI prompt is invalidated; press from another chat rejected; timeout → run cancelled
  and item rescheduled; `go test -race ./internal/core/assistant/ ./internal/transport/http/handlers/`.

### Phase 4 — UI (after `autonomy-ux-design.md` Phase 2)
- Channels screen (link/unlink/mute), channel badges in the conversation, "approved from Telegram" state.
- **Acceptance:** `npm test && npm run build && npm run test:visual`.

### Phase 5 — Docs
- SPEC-009 (sender verification, interactive connectors, continuity — replaces the fresh-session rule),
  SPEC-006 (remote decisions), SPEC-011 (owner identity), mark `../cross-cutting/connector-auto-reply.md`
  as amended by this plan, `docs/api-reference.md`.
- **Acceptance:** `./scripts/check-agent-harness.sh` green.

## Risks and non-goals
- **Lockout** after Phase 0 if `chat_id` is misconfigured — the fixed reply tells the sender their chat
  ID so the owner can fix the setting; the web UI is unaffected.
- **Context pollution** from continuity — bounded by the sieve and window; `/new` is always available.
- **Non-goal:** multiple people per assistant, group chats, other connector types (each new connector
  implements the same interfaces later).

## Remaining Work
All phases (0–5). Phase 0 has no dependencies and should land first.
