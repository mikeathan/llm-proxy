# Implementation Plans

Plans document **implementation strategies** — how a feature was (or will be) built.

Each plan has a status field. Use the status to determine relevance:
- **complete** — Implementation is done. Plan kept for historical reference.
- **partial** — Partially implemented. Remaining work is tracked in the plan.
- **proposed** — Not yet started. Design is being evaluated.
- **draft** — Early-stage design, not yet scoped.
- **superseded** — Replaced by a newer design or specification. Kept for historical reference.
- **reference** — Architectural reference document, not an implementation plan.

Organized by subsystem:
- `agent-loop/` — SPEC-001: agent loop, sieves, stuck detection, fallbacks, refactoring
- `memory/` — SPEC-004: memory storage, tags, injection, dedup
- `orchestrator/` — SPEC-005: budget management, token allocation
- `automation/` — SPEC-007: dispatcher, triggers
- `discovery/` — SPEC-003: model catalog, UI panels
- `assistant-ui/` — SPEC-003: chat UI, bubbles, layout
- `cross-cutting/` — Multi-SPEC or standalone: connectors, user input, lifecycle, testing
- `codebase-audit-report.md` — Comprehensive audit: bugs, performance, architecture, docs gaps, testing gaps (July 2026)

## Active Plans

| File | Title | Status | Date | Related Specs |
|------|-------|--------|------|---------------|
| [`codebase-audit-report.md`](codebase-audit-report.md) | Codebase Audit Report (88 findings) | active | 2026-07-03 | — |
| [`2026-07-11-documentation-optimization.md`](2026-07-11-documentation-optimization.md) | Documentation Optimization Plan | active | 2026-07-11 | — |
| [`agent-loop/agent-improvements.md`](agent-loop/agent-improvements.md) | Agent Improvements (7-phase) | partial | — | SPEC-001 |
| [`agent-loop/ephemeral-turn-context.md`](agent-loop/ephemeral-turn-context.md) | Ephemeral Turn Context | — | — | SPEC-001 |
| [`assistant-ui/overhaul-chat-history-layout.md`](assistant-ui/overhaul-chat-history-layout.md) | Assistant UI Overhaul | active | 2026-06-25 | SPEC-003 |
| [`assistant-ui/knight-rider-arc-bubble.md`](assistant-ui/knight-rider-arc-bubble.md) | Knight Rider Arc Bubble | — | — | SPEC-003 |
| [`assistant-ui/cancel-stale-turn-bleed.md`](assistant-ui/cancel-stale-turn-bleed.md) | Cancel Stale Turn Bleed | — | — | SPEC-003 |
| [`cross-cutting/connector-inbound-webhook.md`](cross-cutting/connector-inbound-webhook.md) | Communication Connector Inbound Webhook | complete | 2026-06-28 | SPEC-009 |
| [`cross-cutting/connector-auto-reply.md`](cross-cutting/connector-auto-reply.md) | Communication Connector Auto-Reply & Automation Trigger | active | 2026-06-28 | SPEC-009 |
| [`cross-cutting/webhook-fresh-sessions.md`](cross-cutting/webhook-fresh-sessions.md) | Fresh Webhook Sessions + Source Grouping | complete | 2026-07-09 | SPEC-009 |
| [`cross-cutting/session-source-backend-driven.md`](cross-cutting/session-source-backend-driven.md) | Session `source` derived from backend (single source of truth) | complete | 2026-07-09 | SPEC-009 |
| [`memory/memory-improvements-implementation-plan.md`](memory/memory-improvements-implementation-plan.md) | Memory Improvements | partial | — | SPEC-004 |

Completed, superseded, and not-implemented plans live in [`ARCHIVE/`](ARCHIVE/) — loaded only when their specific topic is relevant.

See [`docs/INDEX.md`](../INDEX.md) for the full catalog with IDs and cross-references.
