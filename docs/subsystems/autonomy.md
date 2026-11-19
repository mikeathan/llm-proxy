---
subsystem: autonomy
specs: [SPEC-011]
---

# Autonomy and reminders

The always-on assistant program: reminders, agenda and follow-ups, attention policy, standing goals, event triggers.

Start here, then open only what the task needs.

| | |
|---|---|
| **Contract (current behavior)** | [SPEC-011](../SPECS/reminders.md) |
| **Code** | `stacks/reminders/` (the Reminders stack), `backend/internal/core/commands/`, `backend/internal/core/tools/reminders.go` |
| **Architecture** | [map](../architecture/map.md) · [contracts](../architecture/contracts.md) · [checklists](../architecture/checklists.md) · [pitfalls](../architecture/pitfalls.md) — search pitfalls for: reminder, agenda, RunReason |
| **Decisions (why)** | [ADR-0068](../decisions/ADR-0068-reminders-are-a-stack-with-its-own-store-a-notify-port-and-a-schedule-port.md) Reminders are a stack: its own store, a notify port for the owner's own words, a schedule port for cron |
| **Open plans (next)** | [`cross-cutting/reminders-stack.md`](../PLANS/cross-cutting/reminders-stack.md)<br>[`autonomy/autonomy-roadmap.md`](../PLANS/autonomy/autonomy-roadmap.md)<br>[`autonomy/autonomy-ux-design.md`](../PLANS/autonomy/autonomy-ux-design.md)<br>[`autonomy/agenda-reminders-and-followups.md`](../PLANS/autonomy/agenda-reminders-and-followups.md)<br>[`autonomy/attention-policy.md`](../PLANS/autonomy/attention-policy.md)<br>[`autonomy/cross-channel-conversations-and-remote-approvals.md`](../PLANS/autonomy/cross-channel-conversations-and-remote-approvals.md)<br>[`autonomy/standing-goals.md`](../PLANS/autonomy/standing-goals.md)<br>[`autonomy/event-driven-triggers.md`](../PLANS/autonomy/event-driven-triggers.md) |
| **Learnings (tried before)** | [`learnings/autonomy.md`](../learnings/autonomy.md) — read before proposing an approach |
| **Evidence (audits, evals)** | _none yet_ |
| **Skills** | [`automation`](../../.agents/skills/automation/SKILL.md) |
| **Guides** | — |
| **Archived plans** | others by topic in [`ARCHIVE/README.md`](../PLANS/ARCHIVE/README.md) |
