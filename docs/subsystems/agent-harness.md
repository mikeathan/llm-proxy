---
subsystem: agent-harness
specs: []
---

# Docs and the coding-agent harness

How this repo instructs coding agents and keeps its knowledge: AGENTS.md, skills, rules, docs structure, the harness check.

Start here, then open only what the task needs.

| | |
|---|---|
| **Contract (current behavior)** | _no SPEC yet_ — the open plan below is the governing document |
| **Code** | `.agents/skills/`, `.agents/rules/`, `scripts/check-agent-harness.sh` |
| **Architecture** | [map](../architecture/map.md) · [contracts](../architecture/contracts.md) · [checklists](../architecture/checklists.md) · [pitfalls](../architecture/pitfalls.md) — search pitfalls for: docs, skill, AGENTS |
| **Decisions (why)** | [ADR-0001](../decisions/ADR-0001-harness-neutral-agent-instructions.md) Harness-neutral agent instructions<br>[ADR-0002](../decisions/ADR-0002-docs-split-by-lifecycle.md) Docs split by lifecycle, linked by subsystem |
| **Open plans (next)** | _none open_ — follow-ups are listed under "Follow-ups left by archived plans" in [`PLANS/README.md`](../PLANS/README.md) |
| **Learnings (tried before)** | [`learnings/agent-harness.md`](../learnings/agent-harness.md) — read before proposing an approach |
| **Evidence (audits, evals)** | _none yet_ |
| **Skills** | [`documentation-stewardship`](../../.agents/skills/documentation-stewardship/SKILL.md), [`task-planning`](../../.agents/skills/task-planning/SKILL.md), [`pr-commit-message`](../../.agents/skills/pr-commit-message/SKILL.md) |
| **Guides** | — |
| **Archived plans** | [`ARCHIVE/cross-cutting/docs-and-agent-harness-restructure.md`](../PLANS/ARCHIVE/cross-cutting/docs-and-agent-harness-restructure.md) (as built, §0); others by topic in [`ARCHIVE/README.md`](../PLANS/ARCHIVE/README.md) |
