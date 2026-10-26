---
subsystem: memory
specs: [SPEC-004]
---

# Memory

Facts the assistant keeps about the user and workspaces: storage tiers, hot-memory injection, search, capture and review.

Start here, then open only what the task needs.

| | |
|---|---|
| **Contract (current behavior)** | [SPEC-004](../SPECS/memory.md) |
| **Code** | `backend/internal/platform/memory/`, `backend/internal/core/memorycapture/`, `backend/internal/core/tools/memory_tools.go`, `backend/internal/core/assistant/hot_memory.go` |
| **Architecture** | [map](../architecture/map.md) · [contracts](../architecture/contracts.md) · [checklists](../architecture/checklists.md) · [pitfalls](../architecture/pitfalls.md) — search pitfalls for: memory, MemoryStore, FTS5, inject |
| **Decisions (why)** | [ADR-0005](../decisions/ADR-0005-no-llm-rewrite-on-the-run-path.md) No extra LLM call on the run path to apply memory |
| **Open plans (next)** | [`memory/memory-gaps-plan.md`](../PLANS/memory/memory-gaps-plan.md)<br>[`memory/assistant-memory-capture.md`](../PLANS/memory/assistant-memory-capture.md)<br>[`memory/memory-consolidation.md`](../PLANS/memory/memory-consolidation.md)<br>[`memory/memory-improvements-implementation-plan.md`](../PLANS/memory/memory-improvements-implementation-plan.md) |
| **Learnings (tried before)** | [`learnings/memory.md`](../learnings/memory.md) — read before proposing an approach |
| **Evidence (audits, evals)** | [`2026-10-15-next-work-analysis.md`](../audits/2026-10-15-next-work-analysis.md)<br>[`memory-injection-investigation.md`](../audits/memory-injection-investigation.md)<br>[`remove-memory-rewriter.md`](../audits/remove-memory-rewriter.md) |
| **Skills** | [`memory-system`](../../.agents/skills/memory-system/SKILL.md) |
| **Guides** | [`guides/memory-testing.md`](../guides/memory-testing.md) |
| **Archived plans** | [`ARCHIVE/memory/`](../PLANS/ARCHIVE/memory/); others by topic in [`ARCHIVE/README.md`](../PLANS/ARCHIVE/README.md) |
