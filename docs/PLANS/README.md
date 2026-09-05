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
- `cross-cutting/` — Multi-SPEC or standalone: connectors, user input, lifecycle, testing, agent instructions

> The **Codebase Audit Report** (88 findings) was an audit, not a plan — it now lives at
> [`docs/audits/codebase-audit-report.md`](../audits/codebase-audit-report.md).

## Active Plans

> Status legend: `active` (in flight) · `approved` (accepted, in progress) · `partial` (some phases done)
> · `proposed` (design pending) · `complete` (done, kept for reference) · `reverted`/`superseded` (archived).

| File | Title | Status | Date | Related Specs |
|------|-------|--------|------|---------------|
| [`unattended-run-safety-hardening.md`](unattended-run-safety-hardening.md) | Unattended Run Safety Hardening (13 gaps, 7 leaks, 5 optimizations) | approved | 2026-07-22 | SPEC-001, SPEC-006, SPEC-007 |
| [`gpu-performance.md`](gpu-performance.md) | GPU Performance (consolidated: completed + next steps) | active | 2026-08-06 | — |
| [`agent-loop/agent-improvements.md`](agent-loop/agent-improvements.md) | Agent Improvements (7-phase) | partial | — | SPEC-001 |
| [`agent-loop/agent-loop-strategies.md`](agent-loop/agent-loop-strategies.md) | Agent Loop Strategies (pluggable loop-strategy engine) | complete | 2026-08-16 | SPEC-001, SPEC-010 |
| [`agent-loop/strategy-agnostic-completion-and-tool-schema.md`](agent-loop/strategy-agnostic-completion-and-tool-schema.md) | Strategy-Agnostic Completion + Tool-Schema/Policy Consistency | complete | 2026-08-18 | SPEC-010, SPEC-006, SPEC-001 |
| [`agent-loop/surface-planning-reasoning.md`](agent-loop/surface-planning-reasoning.md) | Surface Plan-Generation Reasoning (stream the planner) | complete | 2026-08-18 | SPEC-010, SPEC-001, SPEC-003 |
| [`agent-loop/fix-final-report-realignment.md`](agent-loop/fix-final-report-realignment.md) | Fix automation "Final Report" regression | complete | 2026-08-07 | SPEC-001 |
| [`assistant-ui/overhaul-chat-history-layout.md`](assistant-ui/overhaul-chat-history-layout.md) | Assistant UI Overhaul | active | 2026-06-25 | SPEC-003 |
| [`assistant-ui/automation-renderer-unify-consumption.md`](assistant-ui/automation-renderer-unify-consumption.md) | Unify Automation + Assistant Event Consumption | complete | 2026-07-18 | SPEC-003, SPEC-007 |
| [`assistant-ui/automation-edit-form-reactivity.md`](assistant-ui/automation-edit-form-reactivity.md) | Fix Automation Edit Form — reactive populate | proposed | 2026-08-01 | SPEC-003, SPEC-007 |
| [`assistant-ui/knight-rider-arc-bubble.md`](assistant-ui/knight-rider-arc-bubble.md) | Knight Rider Arc Bubble | active | 2026-06-26 | SPEC-003 |
| [`assistant-ui/cancel-stale-turn-bleed.md`](assistant-ui/cancel-stale-turn-bleed.md) | Cancel Stale Turn Bleed | active | 2026-06-26 | SPEC-003 |
| [`cross-cutting/cloud-provider-token-budgets.md`](cross-cutting/cloud-provider-token-budgets.md) | Cloud Provider Token Budgets + Provider Set Reduction | active | 2026-08-01 | SPEC-005, SPEC-003 |
| [`cross-cutting/connector-inbound-webhook.md`](cross-cutting/connector-inbound-webhook.md) | Communication Connector Inbound Webhook | complete | 2026-06-28 | SPEC-009 |
| [`cross-cutting/connector-auto-reply.md`](cross-cutting/connector-auto-reply.md) | Communication Connector Auto-Reply & Automation Trigger | active | 2026-06-28 | SPEC-009 |
| [`cross-cutting/webhook-fresh-sessions.md`](cross-cutting/webhook-fresh-sessions.md) | Fresh Webhook Sessions + Source Grouping | complete | 2026-07-09 | SPEC-009 |
| [`cross-cutting/session-source-backend-driven.md`](cross-cutting/session-source-backend-driven.md) | Session `source` derived from backend (single source of truth) | complete | 2026-07-09 | SPEC-009 |
| [`cross-cutting/reasoning-capture-dynamic.md`](cross-cutting/reasoning-capture-dynamic.md) | Dynamic, provider-agnostic reasoning enable + capture + neutral indicator (typed, SOLID) | complete | 2026-07-28 | SPEC-003, SPEC-005 |
| [`cross-cutting/per-model-reasoning-overrides-and-settings-layout.md`](cross-cutting/per-model-reasoning-overrides-and-settings-layout.md) | Per-Model Reasoning Overrides and Settings Layout Repair | complete | 2026-08-02 | SPEC-005 |
| [`cross-cutting/post-implementation-cleanup.md`](cross-cutting/post-implementation-cleanup.md) | Post-Implementation Cleanup: Duplication & Dead Code | active | 2026-08-01 | SPEC-005 |
| [`cross-cutting/assistant-liveness-heartbeat-package-split.md`](cross-cutting/assistant-liveness-heartbeat-package-split.md) | Assistant Liveness Heartbeat & Package Restructure | proposed | 2026-08-18 | SPEC-001, SPEC-010, SPEC-003 |
| [`cross-cutting/agents-md-layering-guardrails.md`](cross-cutting/agents-md-layering-guardrails.md) | AGENTS.md Layering, Override-ability & Write Guardrails | proposed | 2026-08-04 | SPEC-001, CONSTITUTION II.13/II.10 |
| [`cross-cutting/tool-call-grammar-reenable.md`](cross-cutting/tool-call-grammar-reenable.md) | Re-enable Tool-Call Grammar Constraint (opt-in, llama.cpp-safe) | proposed | 2026-09-05 | SPEC-001, SPEC-002 |
| [`cross-cutting/persist-assistant-run-state-for-reload.md`](cross-cutting/persist-assistant-run-state-for-reload.md) | Persist assistant run state (errors/cancels/running) for reliable reload | complete | 2026-08-20 | SPEC-001, SPEC-003 |
| [`cross-cutting/sqlite-session-storage.md`](cross-cutting/sqlite-session-storage.md) | SQLite session storage (future work, proposed) | proposed | 2026-08-20 | SPEC-001 |
| [`cross-cutting/xdg-config-data-relocation.md`](cross-cutting/xdg-config-data-relocation.md) | XDG Config/Data Relocation + Storage Cleanup + Reset Controls (Phases 0–7, 9–12 complete; Phase 8 removed; reset/clear-runtime-data hardened) | complete | 2026-08-07 | CONSTITUTION III.2/III.4/III.6 |
| [`memory/memory-improvements-implementation-plan.md`](memory/memory-improvements-implementation-plan.md) | Memory Improvements | partial | — | SPEC-004 |

Completed, superseded, and not-implemented plans live in [`ARCHIVE/`](ARCHIVE/) — loaded only when their specific topic is relevant.

## Remaining Work (non-complete plans)

Filtered view of everything not `complete`. Use this as the live "what's left" tracker — **no separate todo folder**, status stays on the plan itself.

| Status | Plan | Open scope |
|--------|-------|-----------|
| approved | Unattended Run Safety Hardening | Steps 6+ (post-refactor safety fixes, optimizations) |
| active | GPU Performance | P0–P4 rendering/metrics; P5 blocked on fix-final-report-realignment |
| active | Assistant UI Overhaul | Phases 4–5 (refresh resilience, deferred backend SSE bleed) |
| active | Knight Rider Arc Bubble | extraction to `ArcOrbitLoader` (input/header) |
| active | Cancel Stale Turn Bleed | deferred backend SSE bleed follow-up |
| active | Cloud Provider Token Budgets | Phase 7 reasoning enable (merged); remaining budget phases |
| active | Connector Auto-Reply | Phase 3 interactive gateway paths |
| active | Post-Implementation Cleanup | execute findings register (dead code/dup sweep) |
| partial | Agent Improvements | remaining of 7 phases |
| partial | Memory Improvements | remaining phases (nudge, meter, dedup, search, tagging) |
| proposed | Automation Edit Form Reactivity | implement derive-don't-sync refactor |
| proposed | AGENTS.md Layering & Guardrails | design acceptance + implementation |
| proposed | Agent OS Sandboxing | Phases 1–6 (rlimits → FS jail → network switch → OS network deny → egress proxy → deployment hardening) |
| proposed | Assistant Liveness Heartbeat & Package Restructure | heartbeat component + still_thinking + loading fix + package extractions (4.1/4.2) |
| partial | XDG Config/Data Relocation | Phases 0–7, 9–11 done (paths pkg, storage tests, resolver unification, race/perf/security fixes, AppConfig settings.yml merge, relocation, reset controls, permission hardening); Phase 8 removed; Phase 7 templates embed + Phase 12 docs/.gitignore/untrack remain/finalized |

See [`docs/INDEX.md`](../INDEX.md) for the full catalog with IDs and cross-references.
