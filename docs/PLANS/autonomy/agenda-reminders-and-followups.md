---
status: proposed
date: 2026-10-04
last_reviewed: 2026-10-04
related_specs: [SPEC-007, SPEC-001, SPEC-009, SPEC-003]
constitution_references: [II.2, II.10, II.13, II.14, III.2, III.3, IV.1, IV.2, V.2, V.3]
related_plans: [autonomy-roadmap.md, attention-policy.md, standing-goals.md, event-driven-triggers.md, autonomy-ux-design.md]
---

# Agenda — Reminders and Self-Scheduled Follow-ups (milestone M1)

**Status:** proposed — needs D1–D3 before Phase 2. Phase 0–1 change no user-facing behaviour.

## Why this plan exists

The assistant has no sense of *future time*. It can't remind you of anything, and it can't decide "I'll
check this again tomorrow". Automations are the only scheduled thing; they are fixed tasks the
operator writes by hand and are the wrong shape for "remind me Friday at 9" or a one-off follow-up.

| User benefit | Service benefit |
|---|---|
| **Reminders** — "remind me Friday at 9 to call the bank", from the web chat or Telegram; delivered on time. No LLM involved, so they cost nothing and can't be garbled. | One scheduling primitive that goals, follow-ups and digests all build on — instead of each inventing its own timer. |
| **The assistant comes back by itself** — "the parcel isn't shipped yet; I'll check again tomorrow 10:00" — and it actually does. | Follow-ups are ordinary runs through the run lane: same admission, residency, guardrails, recording, run history. |
| One place to see and cancel everything that is scheduled (Today/Agenda views). | `RunReason` gives every proactive run a recorded cause — debuggable and explainable. |

## Ground truth (verified in code, 2026-10-04)

| Fact | Source |
|---|---|
| Trigger types: `cron`, `interval`, `manual` only; all are persisted per workspace in `config.yaml` as part of an automation | `backend/models/workspace.go:5-10`, `:62-66`, `:103-121` |
| The dispatcher schedules with `robfig/cron/v3`; every fire goes through `admitRun` → run lane (dedupes per key) | `backend/internal/core/automation/dispatcher.go:16`, `:92`; `backend/internal/core/automation/admission.go:44-77` |
| Run lane job kinds: `automation`, `interactive`, `inbound` | `backend/internal/core/runlane/types.go:19-25` |
| `ExecuteRequest` has no "why did this run start" field | `backend/internal/core/automation/execution.go:397-414` |
| Outbound messages: `CommunicationTools.NotifyAll(ctx, message, connectorType)` sends text to every (or one type of) connector | `backend/internal/core/tools/communication.go:92-114` |
| Agent-facing notify tool is `notify_user` (manifest) | `backend/internal/core/tools/manifests/communication.json` |
| Adding a tool / tool category has an ordered checklist | `docs/architecture.md` → File Change Checklist |
| The webhook path passes `Timezone: "UTC"` to the agent regardless of the user | `backend/internal/transport/http/handlers/webhook_handlers.go:181` |
| Prompt strings live only in `core/assistant/prompts/templates.go` | Constitution II.13 |

## Design

### Data model (SQLite, roadmap R3)
```go
type AgendaItem struct {
    ID         string
    Owner      string        // "user" | "agent" | "goal:<id>"
    Workspace  string        // required for action=run; optional for message
    When       Schedule      // {At time.Time} one-off, or {Cron string, Until *time.Time}
    Timezone   string        // owner tz at creation (roadmap R2)
    Action     string        // "message" | "run"
    Message    string        // action=message: text delivered verbatim
    Task       string        // action=run: instruction for the agent
    Context    string        // why it was created (session ID, goal ID, user text) — bounded
    Limits     RunLimits     // model, allowed_tools, network_grant, max_steps (action=run)
    Urgency    string        // "low" | "normal" | "urgent" (attention policy input)
    Status     string        // scheduled | done | cancelled | failed | missed
    NextFireAt time.Time
    CreatedBy  string        // "ui" | "chat:<session>" | "telegram" | "agent:<run>"
}
```

### `RunReason` (shared, introduced here)
`RunReason{Kind, Summary, SourceID, Items[]}` is passed through `admitRun` → `ExecuteRequest` and
rendered into the run's first user message by **one** template in `prompts/templates.go`. Event triggers
(`event-driven-triggers.md`) and goal ticks (`standing-goals.md`) reuse it. Stored in the run history so
the UI's "why" panel can show it.

### Scheduler
- An `agenda.Service` owns a single timer loop (tethered to the root context — II.2/II.14) that wakes at
  the earliest `NextFireAt`, not a polling tick. Loaded from the DB at startup.
- **Missed items** (server was off): reminders due < N hours ago are delivered with "(late)"; older ones
  become `missed` and appear on Today. Runs are never replayed in bulk after downtime (D3).
- `action: message` → attention policy → connector/UI. No LLM, no run lane.
- `action: run` → `RunReason{Kind: agenda}` → run lane as a new job kind `agenda` (or `automation` with a
  synthetic name — decided in Phase 1 against SPEC-007 §V) in the item's workspace, with its limits.

### Creating items
| From | How |
|---|---|
| Web UI | Agenda view form (`autonomy-ux-design.md`) and a REST API |
| Chat (web or Telegram) | Agent tools `schedule_reminder(when, message)` and `schedule_followup(when, task)`; the agent resolves "Friday at 9" with the owner timezone (roadmap R2). The tool result echoes the absolute time so mistakes are visible immediately |
| Goals | `standing-goals.md` creates its own `goal:<id>` items |

**Limits on the agent** (agent-created items only): max open follow-ups per workspace, minimum gap
between follow-ups, maximum horizon (e.g. 30 days), and a follow-up chain depth limit (a follow-up that
schedules a follow-up …). Exceeding a limit returns a tool error the agent must report.

### API
`GET/POST /admin/api/agenda`, `PATCH/DELETE /admin/api/agenda/{id}`, `POST /admin/api/agenda/{id}/snooze`.

## Decisions for the user

| # | Decision | Recommendation |
|---|---|---|
| D1 | May the agent create follow-ups without asking? | Yes within limits (max 10 open per workspace, min 15 min apart, ≤ 30 days ahead); each is shown on Today and announced in the reply |
| D2 | Recurring reminders in v1? | Yes (cron + "until"); recurring *runs* stay automations |
| D3 | Missed reminders after downtime | Deliver if < 12 h late, else mark `missed` and show on Today |
| D4 | New run-lane kind `agenda`? | Yes — keeps lane metrics and the active-runs UI honest |

## Phases

### Phase 0 — Characterise (no behaviour change)
- Tests pinning `admitRun` dedupe and `ExecuteRequest` construction (the seams this plan touches).
- **Acceptance:** `go test ./internal/core/automation/ -count=1` green.

### Phase 1 — `RunReason` plumbing
- Type, propagation through `admitRun`/`ExecuteRequest`, prompt template, persisted on `AutomationRun`.
- **Acceptance:** a manual trigger with a reason → the first user message contains the rendered reason;
  history entry carries it; complexity check passes (`go run ./tools/check-complexity/`).

### Phase 2 — Store + scheduler + reminders (needs D2, D3)
- SQLite table, `agenda.Service` timer loop, missed-item policy, `action: message` delivery
  (directly via `NotifyAll` until `attention-policy.md` lands, then through it), REST API.
- **Acceptance:** fake-clock tests — fire at time, DST change, restart reload, missed handling; `go test
  -race ./internal/core/agenda/` (new package, new goroutine).

### Phase 3 — Agent tools + follow-ups (needs D1, D4)
- `schedule_reminder`, `schedule_followup` (manifest + tool + registry per the File Change Checklist),
  agent limits, `action: run` through the run lane with `RunReason{agenda}`.
- **Acceptance:** agent-loop test with a mock LLM calling the tool → item stored with resolved absolute
  time; limit exceeded → tool error; fired follow-up runs in the right workspace with its limits.

### Phase 4 — UI (after `autonomy-ux-design.md` Phase 2)
- Agenda view, Today "Next up" section, create/edit/snooze/cancel.
- **Acceptance:** `npm test && npm run build && npm run test:visual`.

### Phase 5 — Docs
- SPEC-011 (agenda + RunReason), SPEC-007 (new lane kind), `docs/api-reference.md`, tool manifest docs.
- **Acceptance:** `./scripts/check-agent-harness.sh` green.

## Risks and non-goals
- **Wrong times** (timezone, "next Friday" ambiguity) — absolute time always echoed back; the owner
  timezone is explicit (R2); tests across DST.
- **Agent scheduling spam** — limits above, plus the attention policy on delivery.
- **Non-goal:** calendar sync (a future MCP preset), location-based reminders.

## Remaining Work
All phases (0–5). Phase 2 blocked on D2/D3, Phase 3 on D1/D4, Phase 4 on the UX design.
