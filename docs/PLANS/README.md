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
| [`codebase-audit-report.md`](codebase-audit-report.md) | Codebase Audit Report (88 findings + resolved duplication appendix) | active | 2026-07-03 | — |
| [`unattended-run-safety-hardening.md`](unattended-run-safety-hardening.md) | Unattended Run Safety Hardening (13 gaps, 7 leaks, 5 optimizations) | approved | 2026-07-22 | SPEC-001, SPEC-006, SPEC-007 |
| [`gpu-metrics-background-sampler.md`](gpu-metrics-background-sampler.md) | GPU Metrics Background Sampler | complete | 2026-07-23 | — |
| [`fix-final-report-realignment.md`](fix-final-report-realignment.md) | Fix automation "Final Report" regression | — | — | SPEC-001 |
| [`agent-loop/agent-improvements.md`](agent-loop/agent-improvements.md) | Agent Improvements (7-phase) | partial | — | SPEC-001 |
| [`agent-loop/ephemeral-turn-context.md`](agent-loop/ephemeral-turn-context.md) | Ephemeral Turn Context | reverted | — | SPEC-001 |
| [`assistant-ui/overhaul-chat-history-layout.md`](assistant-ui/overhaul-chat-history-layout.md) | Assistant UI Overhaul | active | 2026-06-25 | SPEC-003 |
| [`assistant-ui/automation-renderer-unify-consumption.md`](assistant-ui/automation-renderer-unify-consumption.md) | Unify Automation + Assistant Event Consumption | complete | 2026-07-18 | SPEC-003, SPEC-007 |
| [`assistant-ui/knight-rider-arc-bubble.md`](assistant-ui/knight-rider-arc-bubble.md) | Knight Rider Arc Bubble | active | 2026-06-26 | SPEC-003 |
| [`assistant-ui/cancel-stale-turn-bleed.md`](assistant-ui/cancel-stale-turn-bleed.md) | Cancel Stale Turn Bleed | active | 2026-06-26 | SPEC-003 |
| [`cross-cutting/cloud-provider-token-budgets.md`](cross-cutting/cloud-provider-token-budgets.md) | Cloud Provider Token Budgets + Provider Set Reduction | proposed | 2026-08-01 | SPEC-005, SPEC-003 |
| [`cross-cutting/connector-inbound-webhook.md`](cross-cutting/connector-inbound-webhook.md) | Communication Connector Inbound Webhook | complete | 2026-06-28 | SPEC-009 |
| [`cross-cutting/connector-auto-reply.md`](cross-cutting/connector-auto-reply.md) | Communication Connector Auto-Reply & Automation Trigger | active | 2026-06-28 | SPEC-009 |
| [`cross-cutting/webhook-fresh-sessions.md`](cross-cutting/webhook-fresh-sessions.md) | Fresh Webhook Sessions + Source Grouping | complete | 2026-07-09 | SPEC-009 |
| [`cross-cutting/session-source-backend-driven.md`](cross-cutting/session-source-backend-driven.md) | Session `source` derived from backend (single source of truth) | complete | 2026-07-09 | SPEC-009 |
| [`cross-cutting/reasoning-capture-dynamic.md`](cross-cutting/reasoning-capture-dynamic.md) | Dynamic, provider-agnostic reasoning enable + capture + neutral indicator (typed, SOLID) | complete | 2026-07-28 | SPEC-003, SPEC-005 |
| [`cross-cutting/universal-agent-completion.md`](cross-cutting/universal-agent-completion.md) | Universal Agent Completion Model | complete | 2026-07-21 | SPEC-001, SPEC-002 |
| [`memory/memory-improvements-implementation-plan.md`](memory/memory-improvements-implementation-plan.md) | Memory Improvements | partial | — | SPEC-004 |

Completed, superseded, and not-implemented plans live in [`ARCHIVE/`](ARCHIVE/) — loaded only when their specific topic is relevant.

See [`docs/INDEX.md`](../INDEX.md) for the full catalog with IDs and cross-references.
