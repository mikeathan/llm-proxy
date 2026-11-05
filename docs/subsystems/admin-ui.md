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
| **Code** | `frontend/src/`, `frontend/src/components/AgentIde/`, `frontend/src/composables/` |
| **Architecture** | [map](../architecture/map.md) · [contracts](../architecture/contracts.md) · [checklists](../architecture/checklists.md) · [pitfalls](../architecture/pitfalls.md) — search pitfalls for: Vue, scoped, SSE, settings tab, theme |
| **Decisions (why)** | [ADR-0004](../decisions/ADR-0004-explicit-primary-and-fallback-model.md) No model auto-bootstrap; explicit primary and fallback<br>[ADR-0028](../decisions/ADR-0028-stack-module-registry.md) Switchable capabilities are modules in stacks (Stack → Layer → Module), switched on each stack's Modules tab; amends ADR-0022's group name to Stacks<br>[ADR-0022](../decisions/ADR-0022-modules-is-the-navigation-group-for-feature-areas.md) Modules is the navigation group for feature areas, and a page keeps its own settings |
| **Open plans (next)** | [`cross-cutting/frontend-redesign-v2.md`](../PLANS/cross-cutting/frontend-redesign-v2.md)<br>[`assistant-ui/assistant-automation-workbench-layout.md`](../PLANS/assistant-ui/assistant-automation-workbench-layout.md)<br>[`gpu-performance.md`](../PLANS/gpu-performance.md) |
| **Learnings (tried before)** | [`learnings/admin-ui.md`](../learnings/admin-ui.md) — read before proposing an approach |
| **Evidence (audits, evals)** | [`gpu-performance-audit.md`](../audits/gpu-performance-audit.md) |
| **Skills** | [`assistant-ui-chat`](../../.agents/skills/assistant-ui-chat/SKILL.md), [`assistant-ui-patterns`](../../.agents/skills/assistant-ui-patterns/SKILL.md), [`event-streaming-patterns`](../../.agents/skills/event-streaming-patterns/SKILL.md), [`lifecycle-events`](../../.agents/skills/lifecycle-events/SKILL.md) |
| **Guides** | — |
| **Archived plans** | [`ARCHIVE/assistant-ui/`](../PLANS/ARCHIVE/assistant-ui/), [`ARCHIVE/discovery/`](../PLANS/ARCHIVE/discovery/); others by topic in [`ARCHIVE/README.md`](../PLANS/ARCHIVE/README.md) |
