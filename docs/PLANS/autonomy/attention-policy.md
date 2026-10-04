---
status: proposed
date: 2026-10-04
last_reviewed: 2026-10-04
related_specs: [SPEC-009, SPEC-001, SPEC-005, SPEC-003]
constitution_references: [II.2, II.13, II.14, III.2, IV.1, IV.2, V.2, V.3]
related_plans: [autonomy-roadmap.md, agenda-reminders-and-followups.md, cross-channel-conversations-and-remote-approvals.md, standing-goals.md, ../orchestrator/cost-and-savings-ledger.md]
---

# Attention Policy — Quiet Hours, Interruption Budget, Digest, Autonomy Spend Cap (milestone M2)

**Status:** proposed — needs D1–D4 before Phase 2. Phase 0–1 change no behaviour.

## Why this plan exists

An always-on assistant fails in one of two ways: it is **silent** (you never learn what it did) or it is
**annoying** (it pings you at 23:40 about something trivial). Today every message the agent sends with
`notify_user` goes out immediately to every connector — there is no notion of time of day, importance,
or "you have had enough messages today". Before the agenda and goals start messaging on their own, a
single policy must decide *whether, when and where* a message reaches you.

| User benefit | Service benefit |
|---|---|
| **Quiet hours** — nothing non-urgent between, say, 22:00 and 07:30. | One gate for every proactive message: a single place to test, log and explain delivery decisions. |
| **Interruption budget** — at most N pings a day; everything else waits for the digest. | Messages are coalesced and deduplicated, so a looping automation cannot flood Telegram. |
| **Digest** — one morning (or evening) summary of what was held. Nothing is lost. | Every notice records its decision → the "why" panel can answer "why didn't I get this?". |
| **Autonomy spend cap** — autonomous work (agenda runs, goal ticks) stops for the day at your limit and tells you. | Bounds the worst case of unattended runs on paid keys. |

## Ground truth (verified in code, 2026-10-04)

| Fact | Source |
|---|---|
| All outbound connector messages go through `CommunicationTools.NotifyAll`, which sends immediately to all matching connectors | `backend/internal/core/tools/communication.go:92-114` |
| The agent's `notify_user` tool has only `connector` and `message` parameters — no urgency | `backend/internal/core/tools/manifests/communication.json` |
| Direct replies to an inbound Telegram message go through `replyToChat` → `conn.Send` (a *response*, not proactive) | `backend/internal/transport/http/handlers/webhook_handlers.go:210-220` |
| Connector `Send` is text-only (`Send(ctx, message string)`) | `backend/internal/core/tools/communication.go:15-18` |
| The web UI already has toasts, a notification dot and run notifications | `frontend/src/components/ui/Toast.vue`; `frontend/src/composables/assistant/useRunNotifications.ts` |
| Token usage is persisted for chat turns only; automations and `/v1` record none yet (the spend cap depends on the ledger plan) | `../orchestrator/cost-and-savings-ledger.md` → Ground truth |

## Design

### One entry point
```go
type Notice struct {
    Source   string   // "agenda:<id>" | "goal:<id>" | "automation:<ws>/<name>" | "agent:<run>" | "system"
    Urgency  string   // "low" | "normal" | "urgent"
    Topic    string   // coalescing key, e.g. "goal:flat-hunt"
    Title    string
    Body     string
    Actions  []Action // optional buttons (approve/deny/snooze) — rendered by interactive connectors
    Reason   RunReason
}
func (p *Policy) Submit(ctx context.Context, n Notice) Decision // deliver_now | hold_for_digest | drop_duplicate
```
- `notify_user` gains an optional `urgency` parameter and calls `Submit` instead of `NotifyAll`.
- Agenda reminders, goal updates, consolidation reports and automation failure notices call `Submit`.
- **Exempt:** direct replies to a message you just sent (you are clearly present) and the approval
  request flow's *timing* (approvals are always `urgent`, but still respect a hard "do not disturb").

### Rules (evaluated in order, all deterministic)
1. **Duplicate/coalesce:** same `Topic` within a window → merge into one pending notice ("3 updates on
   goal X").
2. **Do-not-disturb** (manual, e.g. `/mute until 8am`) → hold everything, including urgent.
3. **Quiet hours** (per weekday, owner timezone) → hold `low`/`normal`; `urgent` passes, capped at K per
   night.
4. **Budget:** delivered-today count ≥ N → hold `low`/`normal`; `urgent` passes.
5. **Low urgency** → always digest.
6. Otherwise deliver now, on the **preferred channel**: the channel you used most recently within X
   minutes (presence), else the configured default; the web UI always shows it too.

### Digest
Agenda-scheduled (uses `agenda-reminders-and-followups.md`), at configured times; groups held notices by
source; each entry links to the web UI. An empty digest is not sent.

### Autonomy spend cap
Before an agenda run or goal tick is admitted, check today's autonomous spend (from the ledger) against
the cap. Over cap → the run is not started, its item is re-scheduled for tomorrow, and one `normal`
notice is submitted. Interactive chat is never blocked by this cap.

### Persistence
Notices and decisions in SQLite (roadmap R3), retention-bounded; settings in the owner config section
(roadmap R1).

## Decisions for the user

| # | Decision | Recommendation |
|---|---|---|
| D1 | Defaults | Quiet hours 22:00–07:30; budget 6/day; digest 08:00; urgent cap 2/night |
| D2 | Who may mark something `urgent`? | System and approvals always; the agent only for goals/follow-ups you flagged "may interrupt" |
| D3 | Spend cap default | Off until the ledger plan's Phase 4 lands; then $1/day suggested on first enable |
| D4 | Should automation *failures* notify by default? | Yes, `normal`, coalesced per automation |

## Phases

### Phase 0 — Characterise (no behaviour change)
- Tests pinning `NotifyAll` behaviour and the `notify_user` tool contract (error = not delivered).
- **Acceptance:** `go test ./internal/core/tools/ -count=1` green.

### Phase 1 — Policy engine (pure)
- `Notice`, rules, decision record; fake clock and fake store.
- **Acceptance:** table tests for every rule incl. DST and midnight wrap of quiet hours; complexity ≤12.

### Phase 2 — Wire in (needs D1, D2, D4)
- `notify_user` → `Submit` (+ `urgency` param); agenda message delivery → `Submit`; automation failure
  notices; persistence; digest via the agenda.
- **Acceptance:** agent-loop test — `notify_user` during quiet hours is held and reported to the agent as
  "queued for digest" (not as an error, not as "delivered"); digest test delivers the held notice once.

### Phase 3 — Spend cap (needs D3 and the ledger plan's Phase 2)
- **Acceptance:** fake ledger over cap → agenda run not admitted, rescheduled, one notice submitted.

### Phase 4 — UI + Telegram controls (after `autonomy-ux-design.md` Phase 2)
- Attention settings screen, "held for digest" on Today, the "why" panel; `/mute`, `/snooze` commands
  (with the cross-channel plan).
- **Acceptance:** `npm test && npm run build && npm run test:visual`.

### Phase 5 — Docs
- SPEC-011 (attention policy), SPEC-009 (notify path), `docs/api-reference.md`.
- **Acceptance:** `./scripts/check-agent-harness.sh` green.

## Risks and non-goals
- **Holding something important** — urgency rules plus the digest guarantee nothing is lost; the Today
  view always shows held items.
- **Agent misuse of `urgent`** — D2 restricts it.
- **Non-goal:** learned/ML relevance scoring. Deterministic rules first; a relevance score can be added
  later as one more rule.

## Remaining Work
All phases (0–5). Phase 2 blocked on D1/D2/D4; Phase 3 on D3 and the ledger plan.
