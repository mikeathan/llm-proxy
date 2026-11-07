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
| **Decisions (why)** | [ADR-0032](../decisions/ADR-0032-a-stack-is-a-package-with-a-manifest-kept-apart-by-tests.md) A stack is a package with a manifest, kept apart by tests; Network is the first moved out of the main source (proposed)<br>[ADR-0033](../decisions/ADR-0033-the-sandbox-is-core-and-stacks-gain-no-way-around-it.md) The sandbox is core and cannot be swapped or configured off, and a stack gains no way around it (proposed)<br>[ADR-0034](../decisions/ADR-0034-the-privileged-helper-is-core-not-a-stack-and-decides-who-is-calling.md) The privileged helper is core, not a stack, and it decides who is calling (proposed) |
| **Open plans (next)** | [`cross-cutting/backend-hot-paths-and-leak-hardening.md`](../PLANS/cross-cutting/backend-hot-paths-and-leak-hardening.md)<br>[`cross-cutting/sqlite-session-storage.md`](../PLANS/cross-cutting/sqlite-session-storage.md)<br>[`cross-cutting/ci-github-actions-and-versioning.md`](../PLANS/cross-cutting/ci-github-actions-and-versioning.md)<br>[`cross-cutting/post-implementation-cleanup.md`](../PLANS/cross-cutting/post-implementation-cleanup.md)<br>[`cross-cutting/workspace-manager-coupling.md`](../PLANS/cross-cutting/workspace-manager-coupling.md)<br>[`cross-cutting/stacks-as-packages-and-network-stack.md`](../PLANS/cross-cutting/stacks-as-packages-and-network-stack.md) (proposed 2026-11-06: a stack as a package with a manifest, kept apart by Constitution IV.6 tests, Network first, the isolation library extracted; running a stack as its own process is out of scope) |
| **Learnings (tried before)** | [`learnings/platform.md`](../learnings/platform.md) — read before proposing an approach |
| **Evidence (audits, evals)** | [`2026-08-28-ops-performance-review.md`](../audits/2026-08-28-ops-performance-review.md)<br>[`2026-09-30-platform-scan.md`](../audits/2026-09-30-platform-scan.md)<br>[`backend-audit-report.md`](../audits/backend-audit-report.md)<br>[`codebase-audit-report.md`](../audits/codebase-audit-report.md) |
| **Skills** | [`config-persistence`](../../.agents/skills/config-persistence/SKILL.md), [`testing-guide`](../../.agents/skills/testing-guide/SKILL.md) |
| **Guides** | [`data-layout.md`](../data-layout.md), [`api-reference.md`](../api-reference.md) |
| **Archived plans** | others by topic in [`ARCHIVE/README.md`](../PLANS/ARCHIVE/README.md) |
