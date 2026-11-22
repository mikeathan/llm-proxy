---
subsystem: agent-harness
last_updated: 2026-10-26
---

# Learnings — Docs and the coding-agent harness

What was tried and what happened. Read before proposing an approach in this area; add a line when a run,
eval, audit or merged phase teaches something (newest last within each section). Format and rules:
[`README.md`](README.md).

## Dead ends — do not retry without new evidence

_None recorded yet._

## Root causes and fixes that held

- **Tried:** Adding a vendor-specific instruction file beside `AGENTS.md` → **that harness stopped loading `AGENTS.md` (verified 2026-10-09)**. **Lesson:** the vendor file must import `AGENTS.md`; keep it a pointer. Evidence: [ADR-0001-harness-neutral-agent-instructions.md](../decisions/ADR-0001-harness-neutral-agent-instructions.md).
- **Tried:** Listing finished plans in the live tracker → **30 KB tracker, 31 finished plans shown as active**. **Lesson:** archive on completion; one row per open plan. Evidence: [ADR-0002-docs-split-by-lifecycle.md](../decisions/ADR-0002-docs-split-by-lifecycle.md).
- **Tried:** Nested `AGENTS.md` for path-scoped rules → **loads only when the session starts inside that folder**. **Lesson:** keep the root "Before coding" step as the primary way rules load. Evidence: [ADR-0001-harness-neutral-agent-instructions.md](../decisions/ADR-0001-harness-neutral-agent-instructions.md).
- **Tried:** Repo bookkeeping keys at the top level of skill frontmatter → **outside the Agent Skills spec; some harnesses ignore or reject them**. **Lesson:** put them under `metadata`. Evidence: [ADR-0001-harness-neutral-agent-instructions.md](../decisions/ADR-0001-harness-neutral-agent-instructions.md).
- **Tried:** Reading the repo's agent instructions against a five-layer checklist (prompt shape, shared context, skills, autonomy, owned files) → **17 of 19 skills had no "Skip" line saying when they do not apply; the gate commands were written out in three places (`AGENTS.md`, `tdd-guide`, `task-planning`)**. **Lesson:** every skill states its boundary (the harness now fails a skill without a `Skip:` line) and the gate commands live in `AGENTS.md` → Workflow only. Not done: no skill has been run with and without to show it changes an outcome; that comparison is still open.
