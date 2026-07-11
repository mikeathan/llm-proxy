# Implementation Plans

Plans document **implementation strategies** — how a feature was (or will be) built.

Each plan has a status field. Use the status to determine relevance:
- **complete** — Implementation is done. Plan kept for historical reference.
- **partial** — Partially implemented. Remaining work is tracked in the plan.
- **proposed** — Not yet started. Design is being evaluated.
- **not_implemented** — Design completed but implementation deferred.
- **superseded** — Replaced by a newer design or specification. Kept for historical reference.
- **reference** — Architectural reference document, not an implementation plan.

Organized by subsystem:
- `agent-loop/` — SPEC-001: agent loop, sieves, stuck detection, fallbacks, refactoring
- `memory/` — SPEC-004: memory storage, tags, injection, dedup
- `orchestrator/` — SPEC-005: budget management, token allocation
- `automation/` — SPEC-007: dispatcher, triggers
- `discovery/` — SPEC-003: model catalog, UI panels
- `cross-cutting/` — Multi-SPEC or standalone: grammar, user input, lifecycle, testing

- `cross-cutting/` — Multi-SPEC or standalone: grammar, user input, lifecycle, testing
- `codebase-audit-report.md` — Comprehensive audit: bugs, performance, architecture, docs gaps, testing gaps (July 2026)

See [`docs/INDEX.md`](../INDEX.md) for the full catalog with IDs and cross-references.
