---
status: proposed
date: 2026-10-04
last_reviewed: 2026-10-04
related_specs: [SPEC-001, SPEC-003, SPEC-004, SPEC-006, SPEC-007, SPEC-009]
constitution_references: [I.4, II.2, II.10, II.12, II.13, II.14, V.1, V.2, V.3]
related_plans: [autonomy-ux-design.md, agenda-reminders-and-followups.md, attention-policy.md, cross-channel-conversations-and-remote-approvals.md, standing-goals.md, event-driven-triggers.md, ../memory/memory-consolidation.md]
---

# Autonomous Assistant — Roadmap (umbrella)

**Status:** proposed. This is the **umbrella** for the autonomy program. It owns the vision, the shared
vocabulary, the cross-plan decisions and the order of work. It contains no implementation phases of its
own — each capability has its own plan, listed below, with its own phases, acceptance criteria and
Remaining Work.

## Vision

Today the service is a strong *tool*: it answers when asked and runs automations on a clock. The goal of
this program is an **always-on personal assistant** that:

- **remembers time** — reminds you, and comes back to things by itself ("check tomorrow whether the
  package shipped");
- **pursues goals** — you give it an outcome; it decides what to do and when, over days;
- **reaches you wherever you are** — one conversation across the web UI and Telegram, including
  approving or denying blocked actions from your phone;
- **knows when not to talk** — quiet hours and a daily interruption budget, with everything else held for
  a digest;
- **gets better every day** — a nightly consolidation of what it learned, which you review.

## Principles (apply to every plan in this program)

1. **You stay in control.** Everything the assistant schedules or pursues is visible, editable, pausable
   and cancellable in one place (the Today view — see `autonomy-ux-design.md`).
2. **Nothing silent, nothing spammy.** Every proactive message passes the attention policy; anything
   held back appears in a digest, never disappears.
3. **Bounded autonomy.** Every self-scheduled run, goal and channel has explicit limits (frequency, run
   count, spend, tools, network). Guardrails (SPEC-006) are never bypassed — remote approvals only add a
   new place to *answer* them.
4. **Explainable.** Every proactive action records *why* it happened (`RunReason`, see below) and shows it.
5. **Local-first.** Plain reminders are delivered without an LLM call; consolidation and goal checks
   prefer the local model.
6. **Single operator.** SPEC-003 §I: one person owns this server. "You" is one owner identity with linked
   channels — no users or roles.

## Shared building blocks

| Block | Owned by | Used by |
|---|---|---|
| **Agenda** — scheduled items, one-off or recurring, created by you or by the agent. An item either *delivers a message* (reminder, no LLM) or *starts an agent run* (follow-up, goal check). | `agenda-reminders-and-followups.md` | reminders, follow-ups, standing goals, digest delivery |
| **RunReason** — "why did this run start?" (agenda item, event, goal tick, upstream run), passed into the run and rendered into its first message by one prompt template | `agenda-reminders-and-followups.md` (Phase 1) | event triggers, goals, chat-to-automation dry runs |
| **Attention policy** — the single gate for every proactive message: quiet hours, interruption budget, urgency, digest, channel choice; plus the optional spend cap for autonomous work | `attention-policy.md` | reminders, goals, consolidation report, `notify_user`, automation notices |
| **Owner identity + channel links** — "you" across web UI and Telegram; conversation continuity; remote approvals | `cross-channel-conversations-and-remote-approvals.md` | replying to reminders, steering goals from the phone, approving blocked actions |
| **Event sources** — `file`, `watch`, `after` triggers | `event-driven-triggers.md` | automations, and goals that wait for "when X changes" |

## Glossary

| Term | Meaning |
|---|---|
| Agenda item | One scheduled thing: `when` (one-off time or recurrence), `action` (`message` or `run`), `owner` (`user` or `agent`), optional workspace |
| Reminder | Agenda item with `action: message` — delivered verbatim, no LLM |
| Follow-up | Agenda item with `action: run`, usually created by the agent via the `schedule_followup` tool |
| Standing goal | A long-lived outcome with success criteria, a journal and limits; it advances through goal ticks |
| Goal tick | One agent run for a goal, started by an agenda item the goal owns |
| Notice | Any proactive outbound message, before the attention policy decides deliver-now / hold-for-digest / drop |
| Digest | A scheduled bundle of held notices |
| Owner channel | A linked, verified channel (e.g. one Telegram chat ID) that is "you" |

## Order of work (milestones)

```
M0  UX design (no code)                      autonomy-ux-design.md
M1  Safe inbound + reminders                 cross-channel P0 (sender allow-list, ctx, tz)
                                             agenda P0–P3 (store, RunReason, reminders, follow-ups)
M2  Proactive but polite                     attention-policy P0–P3
M3  One assistant, everywhere                cross-channel P1–P4 (continuity, remote approvals)
M4  Goals                                    standing-goals (needs M1–M3)
M5  Gets better every day                    memory-consolidation (can start after M2)
--  In parallel, independent                 event-driven-triggers; chat-to-automation
```

Why this order: reminders (M1) are the smallest thing you can use daily; the agent must not message you
on its own before the attention policy exists (M2); goals need scheduling, polite delivery and remote
approvals to be useful unattended (M1–M3). **M1 starts with a security fix** — inbound Telegram messages
are not checked against the owner's chat today (see the cross-channel plan, Phase 0).

## Cross-plan decisions (decide once, here)

| # | Decision | Recommendation |
|---|---|---|
| R1 | Where do user-level settings (timezone, quiet hours, owner channels) live? | One `owner` section in the system config (CONFIG root, atomic writes per III.2) — not per workspace |
| R2 | Timezone source | Explicit owner setting, defaulting to the server's local zone; the browser's zone is offered as a one-click fix. (The webhook path hard-codes `"UTC"` today — `backend/internal/transport/http/handlers/webhook_handlers.go:181`) |
| R3 | Storage for agenda, goals, notices | The shared SQLite DB (`backend/internal/platform/db/db.go`) next to memory and the ledger; goal journals also mirrored as readable markdown in the workspace |
| R4 | New SPEC | **SPEC-011 Autonomous Assistant** (agenda, RunReason, attention policy, goals, owner identity) — drafted at the end of M0, once the UX settles the contracts; amend SPEC-007 (agenda runs go through the run lane), SPEC-009 (interactive connectors), SPEC-004 (consolidation), SPEC-006 (remote decisions), SPEC-003 (Today view) |
| R5 | Model for autonomous work | Default: the configured primary; per goal override. Agenda message delivery uses no model |

## How existing plans fit

| Plan | Relationship |
|---|---|
| `event-driven-triggers.md` (moved here from `automation/` on 2026-10-04) | Part of this program: the *event* half of "when does the assistant act". Reuses `RunReason` from the agenda plan |
| `../automation/chat-to-automation.md` | Independent. Its draft can later also offer "make this a goal" (standing-goals Phase 5) |
| `../orchestrator/cost-and-savings-ledger.md` | Supplies the spend data for the attention policy's autonomous spend cap |
| `../cross-cutting/connector-auto-reply.md` | Amended: its fresh-session-per-message design is replaced by conversation continuity (cross-channel plan) |
| `../cross-cutting/admin-api-authentication.md` | Complementary: it protects the admin/API surface; the cross-channel plan protects the *Telegram* surface (sender allow-list) |
| `../unattended-run-safety-hardening.md` | Prerequisite mindset for goals: unattended tool restriction and spiral detection apply to goal ticks |
| `../memory/memory-improvements-implementation-plan.md` | Consolidation complements (does not replace) session search and the skill system |
| `../assistant-ui/assistant-automation-workbench-layout.md` | Coordinated in `autonomy-ux-design.md` so the Today view fits the workbench shell |

## Remaining Work
M0–M5 as tracked in each child plan. This umbrella is `complete` when every child plan is `complete`
or explicitly descoped here.
