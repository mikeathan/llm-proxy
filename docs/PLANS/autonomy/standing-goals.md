---
status: proposed
date: 2026-10-04
last_reviewed: 2026-10-04
related_specs: [SPEC-001, SPEC-010, SPEC-007, SPEC-006, SPEC-004, SPEC-003]
constitution_references: [II.7, II.10, II.13, III.2, III.3, V.2, V.3]
related_plans: [autonomy-roadmap.md, agenda-reminders-and-followups.md, attention-policy.md, cross-channel-conversations-and-remote-approvals.md, event-driven-triggers.md, ../unattended-run-safety-hardening.md, ../automation/chat-to-automation.md]
---

# Standing Goals — Give It an Outcome, It Decides What and When (milestone M4)

**Status:** proposed — depends on M1–M3 (agenda, attention policy, cross-channel + remote approvals).
Phase 0–1 can start earlier because they are pure.

## Why this plan exists

Automations repeat a **fixed task** on a **fixed schedule** that you wrote. Many real wants are
**outcomes** instead: "find me a flat under €1 200 in district X", "get my reading list summarised every
week until it's empty", "keep an eye on this bug until it's fixed upstream and tell me". For these you
don't know the steps or the right timing in advance — the assistant should work them out, act, check
back, and stop when it's done.

| User benefit | Service benefit |
|---|---|
| State the outcome once; the assistant plans, acts, and decides when to check again. | Reuses every existing piece — agenda for timing, run lane for execution, guardrails for safety, attention policy for messages — no parallel engine. |
| It stops by itself when the goal is reached (and asks you to confirm), or tells you when it is stuck. | Every goal has explicit limits and a readable journal → unattended work is auditable. |
| You can steer it from the phone: "focus on 2-room flats", "pause until Monday". | Goals produce the highest-value proactive messages, which justify the attention policy's budget. |

## Ground truth (verified in code, 2026-10-04)

| Fact | Source |
|---|---|
| Pluggable loop strategies exist, including an evaluator-optimizer strategy that judges and revises output | `backend/internal/core/assistant/loop_strategy.go`; `backend/internal/core/assistant/evaluator_optimizer_strategy.go`; SPEC-010 |
| Run completion rule: substantive final answer + at least one tool result (no keyword matching) | Constitution II.7 |
| A deterministic progress ledger records what a run already did (in-run only; not carried across runs) | `backend/internal/core/assistant/ledger.go:14-31` |
| Stuck/spiral detection exists within a run (`stuck_detected` lifecycle event); unattended spiral detection is planned | SPEC-007 §II.4; `../unattended-run-safety-hardening.md` |
| Automations already carry per-run limits: `allowed_tools`, `network_grant`, `model`, `loop_strategy`, `memory_mode` | `backend/models/workspace.go:103-121` |
| Memory supports workspace and global facts with priorities | SPEC-004; `backend/internal/platform/memory/store.go` |
| Agent file access is jailed to the workspace | Constitution III.3 |

## Design

### Model
```go
type Goal struct {
    ID, Title       string
    Outcome         string     // what "done" looks like, in your words
    SuccessCheck    string     // how to verify it (may be refined by the agent, shown to you)
    Constraints     string     // e.g. "max €1 200, district X, no agencies"
    Workspace       string
    Limits          GoalLimits // max ticks/day, min/max interval, deadline, spend cap, allowed_tools, network_grant, model
    MayInterrupt    bool       // may send `urgent` notices (attention-policy D2)
    Status          string     // active | paused | waiting_for_you | blocked | achieved | abandoned
    NextTickAt      time.Time  // an agenda item owned by "goal:<id>"
}
```
**Journal:** an append-only record per goal (what was done, what was found, what changed, next intention)
stored in SQLite and mirrored to `goals/<id>.md` in the workspace so you — and the agent — can read it.

### A goal tick (one agent run)
1. Started by the goal's agenda item with `RunReason{Kind: goal}`.
2. Context: goal definition, constraints, the **last N journal entries** (bounded, local-model friendly),
   relevant memory.
3. The agent works within the goal's limits (guardrails apply; blocked calls go to remote approval).
4. It must end with a structured **tick report** (validated, one retry on bad format):
   `{progress: none|some|done_candidate, summary, findings[], question_for_user?, next_check: <duration or time>, why}`.
5. The service — not the model — applies it: appends the journal; clamps `next_check` to the goal's
   min/max interval and schedules it; sends a notice via the attention policy only when there is a
   finding, a question, or a status change (no "nothing new" messages).

### Finishing and getting stuck
- `done_candidate` → a verification tick (evaluator-optimizer strategy against `SuccessCheck`) → if it
  passes, status `waiting_for_you` with "I think this is done — confirm?" You confirm → `achieved`.
- **Stuck:** K consecutive ticks with `progress: none` → `blocked`, one notice explaining why, ticks stop
  until you respond.
- **Deadline** passed or limits exhausted → `paused` with a summary.

### Steering
- From chat or Telegram (cross-channel continuity): "pause goal flat-hunt", "focus on 2-room flats" →
  an agent tool `update_goal(id, change)` that appends a **user steering** entry to the journal (the
  constraints you set are never silently rewritten by the agent).
- From the UI: goal detail screen (`autonomy-ux-design.md`).

### Event-driven waits
Where a goal is "tell me when X changes", the agent may attach a `watch` event source
(`event-driven-triggers.md`) to the goal instead of polling with ticks.

## Decisions for the user

| # | Decision | Recommendation |
|---|---|---|
| D1 | Default limits for a new goal | ≤ 4 ticks/day, interval 1 h–7 days, 30-day deadline, tools: same as the workspace's unattended profile |
| D2 | Stuck threshold K | 3 ticks without progress |
| D3 | Can the agent create goals itself (from a chat)? | It may *propose* a goal; you confirm in one click. Never created silently |
| D4 | Goal ticks per workspace in parallel | One at a time per workspace (workspace lock already serialises runs) |

## Phases

### Phase 0 — Tick report contract (pure)
- Schema, validation, clamping rules; prompt templates for tick and verification (`prompts/templates.go`).
- **Acceptance:** table tests for parsing/validation/clamping; `go run ./tools/check-complexity/`.

### Phase 1 — Goal store and journal
- SQLite tables, journal append, workspace markdown mirror (atomic writes, III.2; jailed path, III.3).
- **Acceptance:** store tests; mirror file matches journal after restart.

### Phase 2 — Tick execution (needs the agenda plan Phase 3 and D1)
- Goal-owned agenda items, `RunReason{goal}`, context assembly, tick report application, notices via the
  attention policy.
- **Acceptance:** record-replay test of a 3-tick goal with a mock LLM: journal grows, next tick scheduled
  within limits, a finding produces exactly one notice, "nothing new" produces none.

### Phase 3 — Finish, stuck, deadline (needs D2)
- Verification tick, `waiting_for_you` confirmation (also via Telegram buttons), stuck and deadline rules.
- **Acceptance:** tests for each transition; confirmation from a linked Telegram chat resolves the goal.

### Phase 4 — Steering + proposals (needs D3)
- `update_goal` and `propose_goal` tools; journal steering entries.
- **Acceptance:** agent-loop tests; constraints unchanged unless the user's steering says so.

### Phase 5 — UI (after `autonomy-ux-design.md` Phase 2)
- Goal list, new-goal form, goal detail with journal timeline, Today "Active goals"; "make this a goal"
  from `../automation/chat-to-automation.md`.
- **Acceptance:** `npm test && npm run build && npm run test:visual`.

### Phase 6 — Docs
- SPEC-011 (goals), SPEC-010 note (tick strategy use), `docs/api-reference.md`.
- **Acceptance:** `./scripts/check-agent-harness.sh` green.

## Risks and non-goals
- **Runaway work or spend** — per-goal limits, the attention policy's autonomy spend cap, stuck detection,
  and the unattended-run hardening plan.
- **Small local models producing poor tick reports** — strict schema + one retry; a goal can pin a stronger
  model for ticks.
- **Non-goal:** multi-goal planning/prioritisation across goals, sub-goal trees (later, if needed).

## Remaining Work
All phases (0–6). Phases 2–5 blocked on M1–M3 and the UX design.
