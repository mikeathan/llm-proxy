---
subsystem: autonomy
specs: [SPEC-011, SPEC-013]
---

# Autonomy and reminders

The always-on assistant program: reminders, agenda and follow-ups, attention policy, standing goals, event triggers.

Start here, then open only what the task needs.

| | |
|---|---|
| **Contract (current behavior)** | [SPEC-011](../SPECS/reminders.md) · [SPEC-013](../SPECS/newsroom.md) (Newsroom) |
| **Code** | `stacks/reminders/` (the Reminders stack), `stacks/newsroom/` (the Newsroom stack), `backend/internal/core/commands/`, `backend/internal/core/tools/reminders.go` |
| **Architecture** | [map](../architecture/map.md) · [contracts](../architecture/contracts.md) · [checklists](../architecture/checklists.md) · [pitfalls](../architecture/pitfalls.md) — search pitfalls for: reminder, agenda, RunReason |
| **Decisions (why)** | [ADR-0072](../decisions/ADR-0072-a-stack-hands-the-core-a-structured-notice-and-the-core-words-and-sends-it.md) A stack hands the core a structured notice and the core words and sends it (proposed)<br>[ADR-0071](../decisions/ADR-0071-a-stack-may-ask-the-core-for-a-model-answer-through-a-narrow-model-port-with-no-tools-no-memory-and-no-choice-of-model.md) A stack may ask the core for a model answer through a narrow model port: no tools, no memory, no choice of model (proposed)<br>[ADR-0068](../decisions/ADR-0068-reminders-are-a-stack-with-its-own-store-a-notify-port-and-a-schedule-port.md) Reminders are a stack: its own store, a notify port for the owner's own words, a schedule port for cron |
| **Open plans (next)** | [`autonomy/newsroom-handoff-2026-11-20.md`](../PLANS/autonomy/newsroom-handoff-2026-11-20.md) (partial 2026-11-20: Newsroom stack, sources to feed journal built; start here)<br>[`cross-cutting/reminders-stack.md`](../PLANS/cross-cutting/reminders-stack.md)<br>[`autonomy/autonomy-roadmap.md`](../PLANS/autonomy/autonomy-roadmap.md)<br>[`autonomy/autonomy-ux-design.md`](../PLANS/autonomy/autonomy-ux-design.md)<br>[`autonomy/agenda-reminders-and-followups.md`](../PLANS/autonomy/agenda-reminders-and-followups.md)<br>[`autonomy/attention-policy.md`](../PLANS/autonomy/attention-policy.md)<br>[`autonomy/cross-channel-conversations-and-remote-approvals.md`](../PLANS/autonomy/cross-channel-conversations-and-remote-approvals.md)<br>[`autonomy/standing-goals.md`](../PLANS/autonomy/standing-goals.md)<br>[`autonomy/event-driven-triggers.md`](../PLANS/autonomy/event-driven-triggers.md)<br>[`autonomy/topic-watch.md`](../PLANS/autonomy/topic-watch.md)<br>[`autonomy/morning-paper.md`](../PLANS/autonomy/morning-paper.md) |
| **Learnings (tried before)** | [`learnings/autonomy.md`](../learnings/autonomy.md) — read before proposing an approach |
| **Evidence (audits, evals)** | _none yet_ |
| **Skills** | [`automation`](../../.agents/skills/automation/SKILL.md) |
| **Guides** | — |
| **Archived plans** | others by topic in [`ARCHIVE/README.md`](../PLANS/ARCHIVE/README.md) |
