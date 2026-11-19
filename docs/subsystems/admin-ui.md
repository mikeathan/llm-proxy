---
subsystem: admin-ui
specs: [SPEC-003]
---

# Admin UI and chat rendering

The Vue admin app: assistant chat view, settings tabs, automation and model screens, themes, rendering performance.

Start here, then open only what the task needs.

| | |
|---|---|
| **Contract (current behavior)** | [SPEC-003](../SPECS/discovery-panel.md) |
| **Code** | `frontend/src/`, `frontend/src/components/AgentIde/`, `frontend/src/composables/`, `frontend/src/components/stacks/` (a stack's Modules tab), `frontend/src/components/dashboard/` (the dashboard: cards, board tabs, grid), `frontend/src/domain/dashboard.ts`, `backend/internal/core/dashboard/`, `frontend/src/views/AgentsView.vue` |
| **Architecture** | [map](../architecture/map.md) · [contracts](../architecture/contracts.md) · [checklists](../architecture/checklists.md) · [pitfalls](../architecture/pitfalls.md) — search pitfalls for: Vue, scoped, SSE, settings tab, theme |
| **Decisions (why)** | [ADR-0068](../decisions/ADR-0068-reminders-are-a-stack-with-its-own-store-a-notify-port-and-a-schedule-port.md) Reminders are a stack: its own store, a notify port for the owner's own words, a schedule port for cron<br>[ADR-0067](../decisions/ADR-0067-a-stack-is-switched-off-by-switching-its-modules-off.md) A stack is switched off by switching all of its modules off; it has no setting of its own<br>[ADR-0066](../decisions/ADR-0066-stack-pages-keep-only-control-and-the-dashboard-holds-everything-to-see-and-decide.md) The stack pages keep only control; the dashboard holds everything to see and to decide about an item, for a stack that is on<br>[ADR-0065](../decisions/ADR-0065-dashboard-card-height-is-fixed-order-is-seen-order-and-stack-cards-follow-the-stack.md) A dashboard card's height is a minimum, its order is the order seen, and a stack's cards are offered while the stack is on<br>[ADR-0064](../decisions/ADR-0064-the-dashboard-shows-and-links-and-the-stack-pages-control.md) The dashboard shows and links; the stack pages control<br>[ADR-0004](../decisions/ADR-0004-explicit-primary-and-fallback-model.md) No model auto-bootstrap; explicit primary and fallback<br>[ADR-0028](../decisions/ADR-0028-stack-module-registry.md) Switchable capabilities are modules in stacks (Stack → Layer → Module), switched on each stack's Modules tab; amends ADR-0022's group name to Stacks<br>[ADR-0022](../decisions/ADR-0022-modules-is-the-navigation-group-for-feature-areas.md) Modules is the navigation group for feature areas, and a page keeps its own settings |
| **Open plans (next)** | [`cross-cutting/frontend-redesign-v2.md`](../PLANS/cross-cutting/frontend-redesign-v2.md)<br>[`assistant-ui/assistant-automation-workbench-layout.md`](../PLANS/assistant-ui/assistant-automation-workbench-layout.md)<br>[`gpu-performance.md`](../PLANS/gpu-performance.md) |
| **Learnings (tried before)** | [`learnings/admin-ui.md`](../learnings/admin-ui.md) — read before proposing an approach |
| **Evidence (audits, evals)** | [`gpu-performance-audit.md`](../audits/gpu-performance-audit.md) |
| **Skills** | [`assistant-ui-chat`](../../.agents/skills/assistant-ui-chat/SKILL.md), [`assistant-ui-patterns`](../../.agents/skills/assistant-ui-patterns/SKILL.md), [`event-streaming-patterns`](../../.agents/skills/event-streaming-patterns/SKILL.md), [`lifecycle-events`](../../.agents/skills/lifecycle-events/SKILL.md) |
| **Guides** | — |
| **Archived plans** | [`ARCHIVE/assistant-ui/`](../PLANS/ARCHIVE/assistant-ui/), [`ARCHIVE/discovery/`](../PLANS/ARCHIVE/discovery/); others by topic in [`ARCHIVE/README.md`](../PLANS/ARCHIVE/README.md) |
