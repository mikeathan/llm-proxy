---
subsystem: platform
specs: []
---

# Storage, config, events, logging and backend performance

Data layout and persistence, config precedence and reload, the event bus, logging, metrics, leaks, CI and releases.

Start here, then open only what the task needs.

| | |
|---|---|
| **Contract (current behavior)** | _no SPEC yet_ — the open plan below is the governing document |
| **Code** | `backend/internal/platform/`, `backend/internal/core/eventbus/`, `backend/internal/app/` |
| **Architecture** | [map](../architecture/map.md) · [contracts](../architecture/contracts.md) · [checklists](../architecture/checklists.md) · [pitfalls](../architecture/pitfalls.md) — search pitfalls for: config, settings.yml, eventbus, session, log |
| **Decisions (why)** | _none yet_ — decisions still live in the plans below |
| **Open plans (next)** | [`cross-cutting/backend-hot-paths-and-leak-hardening.md`](../PLANS/cross-cutting/backend-hot-paths-and-leak-hardening.md)<br>[`cross-cutting/sqlite-session-storage.md`](../PLANS/cross-cutting/sqlite-session-storage.md)<br>[`cross-cutting/ci-github-actions-and-versioning.md`](../PLANS/cross-cutting/ci-github-actions-and-versioning.md)<br>[`cross-cutting/post-implementation-cleanup.md`](../PLANS/cross-cutting/post-implementation-cleanup.md)<br>[`cross-cutting/workspace-manager-coupling.md`](../PLANS/cross-cutting/workspace-manager-coupling.md) |
| **Learnings (tried before)** | [`learnings/platform.md`](../learnings/platform.md) — read before proposing an approach |
| **Evidence (audits, evals)** | [`2026-08-28-ops-performance-review.md`](../audits/2026-08-28-ops-performance-review.md)<br>[`2026-09-30-platform-scan.md`](../audits/2026-09-30-platform-scan.md)<br>[`backend-audit-report.md`](../audits/backend-audit-report.md)<br>[`codebase-audit-report.md`](../audits/codebase-audit-report.md) |
| **Skills** | [`config-persistence`](../../.agents/skills/config-persistence/SKILL.md), [`testing-guide`](../../.agents/skills/testing-guide/SKILL.md) |
| **Guides** | [`data-layout.md`](../data-layout.md), [`api-reference.md`](../api-reference.md) |
| **Archived plans** | others by topic in [`ARCHIVE/README.md`](../PLANS/ARCHIVE/README.md) |
