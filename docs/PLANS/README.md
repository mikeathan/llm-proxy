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
| [`primary-model-warning-banner.md`](primary-model-warning-banner.md) | Remove model auto-bootstrap; explicit primary/fallback selection + banners | complete | 2026-08-14 | SPEC-003, CONSTITUTION III.4 |
| [`agent-loop/agent-improvements.md`](agent-loop/agent-improvements.md) | Agent Improvements (7-phase; re-scoped 2026-09-05 — Phases 3/6 superseded by SPEC-010, Phase 4/5/7-remainder open) | partial | — | SPEC-001 |
| [`agent-loop/agent-loop-strategies.md`](agent-loop/agent-loop-strategies.md) | Agent Loop Strategies (pluggable loop-strategy engine) | complete | 2026-08-16 | SPEC-001, SPEC-010 |
| [`agent-loop/strategy-agnostic-completion-and-tool-schema.md`](agent-loop/strategy-agnostic-completion-and-tool-schema.md) | Strategy-Agnostic Completion + Tool-Schema/Policy Consistency | complete | 2026-08-18 | SPEC-010, SPEC-006, SPEC-001 |
| [`agent-loop/surface-planning-reasoning.md`](agent-loop/surface-planning-reasoning.md) | Surface Plan-Generation Reasoning (stream the planner) | complete | 2026-08-18 | SPEC-010, SPEC-001, SPEC-003 |
| [`agent-loop/fix-final-report-realignment.md`](agent-loop/fix-final-report-realignment.md) | Fix automation "Final Report" regression | complete | 2026-08-07 | SPEC-001 |
| [`assistant-ui/overhaul-chat-history-layout.md`](assistant-ui/overhaul-chat-history-layout.md) | Assistant UI Overhaul | active | 2026-09-30 | SPEC-003 |
| [`assistant-ui/automation-renderer-unify-consumption.md`](assistant-ui/automation-renderer-unify-consumption.md) | Unify Automation + Assistant Event Consumption | complete | 2026-07-18 | SPEC-003, SPEC-007 |
| [`assistant-ui/automation-edit-form-reactivity.md`](assistant-ui/automation-edit-form-reactivity.md) | Fix Automation Edit Form — reactive populate | complete (absorbed by frontend-redesign-retro, Phase 5) | 2026-08-01 | SPEC-003, SPEC-007 |
| [`assistant-ui/consolidate-app-banner.md`](assistant-ui/consolidate-app-banner.md) | Consolidate banner logic into a single event-driven `AppBanner` | complete | 2026-08-14 | SPEC-003 |
| [`assistant-ui/reasoning-inset-auto-collapse.md`](assistant-ui/reasoning-inset-auto-collapse.md) | Reasoning inset auto-expand while running, collapse on done | complete | 2026-08-14 | SPEC-003 |
| [`cross-cutting/frontend-redesign-retro.md`](cross-cutting/frontend-redesign-retro.md) | Frontend Redesign — Retro Theme System, Shell, Tree & Notifications (absorbs `automation-edit-form-reactivity` at Phase 5) — Phases 0–7 complete; SPEC-003 v2.0 | complete | 2026-09-30 | SPEC-003, SPEC-007 |
| [`cross-cutting/ci-github-actions-and-versioning.md`](cross-cutting/ci-github-actions-and-versioning.md) | CI, GitHub Actions Integration & Versioning Flow (P1 CI + P4 tag implemented; P2/P3 + P6 pending, P5 parked) | partial | 2026-08-30 | — |
| [`cross-cutting/connector-inbound-webhook.md`](cross-cutting/connector-inbound-webhook.md) | Communication Connector Inbound Webhook | complete | 2026-06-28 | SPEC-009 |
| [`cross-cutting/connector-auto-reply.md`](cross-cutting/connector-auto-reply.md) | Communication Connector Auto-Reply & Automation Trigger | active | 2026-06-28 | SPEC-009 |
| [`cross-cutting/webhook-fresh-sessions.md`](cross-cutting/webhook-fresh-sessions.md) | Fresh Webhook Sessions + Source Grouping | complete | 2026-07-09 | SPEC-009 |
| [`cross-cutting/session-source-backend-driven.md`](cross-cutting/session-source-backend-driven.md) | Session `source` derived from backend (single source of truth) | complete | 2026-07-09 | SPEC-009 |
| [`cross-cutting/reasoning-capture-dynamic.md`](cross-cutting/reasoning-capture-dynamic.md) | Dynamic, provider-agnostic reasoning enable + capture + neutral indicator (typed, SOLID) | complete | 2026-07-28 | SPEC-003, SPEC-005 |
| [`cross-cutting/per-model-reasoning-overrides-and-settings-layout.md`](cross-cutting/per-model-reasoning-overrides-and-settings-layout.md) | Per-Model Reasoning Overrides and Settings Layout Repair | complete | 2026-08-02 | SPEC-005 |
| [`cross-cutting/post-implementation-cleanup.md`](cross-cutting/post-implementation-cleanup.md) | Post-Implementation Cleanup: Duplication & Dead Code | active | 2026-08-01 | SPEC-005 |
| [`cross-cutting/agent-os-sandboxing.md`](cross-cutting/agent-os-sandboxing.md) | Agent OS Sandboxing (rev 2 — network-first, uid-first; one action pipeline with OS jail + egress proxy; Phases 0–4 implemented + post-review hardening) | complete (pending Linux-CI) | 2026-09-06 | SPEC-006, SPEC-009 |
| [`cross-cutting/sandbox-runtime-invisibility.md`](cross-cutting/sandbox-runtime-invisibility.md) | Sandbox Runtime Invisibility (`.sandbox` hidden from filesystem listings + terminal output) | complete | 2026-08-25 | SPEC-006, CONSTITUTION II.3 |
| [`cross-cutting/assistant-liveness-heartbeat-package-split.md`](cross-cutting/assistant-liveness-heartbeat-package-split.md) | Assistant Liveness Heartbeat & Package Restructure | partial | 2026-08-18 | SPEC-001, SPEC-010, SPEC-003 |
| [`cross-cutting/agents-md-layering-guardrails.md`](cross-cutting/agents-md-layering-guardrails.md) | AGENTS.md Layering, Override-ability & Write Guardrails | proposed | 2026-08-04 | SPEC-001, CONSTITUTION II.13/II.10 |
| [`cross-cutting/tool-call-grammar-reenable.md`](cross-cutting/tool-call-grammar-reenable.md) | Re-enable Tool-Call Grammar Constraint (opt-in, llama.cpp-safe) | proposed | 2026-09-05 | SPEC-001, SPEC-002 |
| [`cross-cutting/search-tool-calling.md`](cross-cutting/search-tool-calling.md) | Wire up `internet_search` tool calling (pluggable multi-provider, live key, hide-when-unconfigured) | active | 2026-09-12 | SPEC-001, SPEC-006 |
| [`cross-cutting/tool-error-classification.md`](cross-cutting/tool-error-classification.md) | Tool Error Classification & Run-Fatality Policy (terminal tool errors, delivery-vs-essential, failure bound) | active | 2026-09-12 | SPEC-001, SPEC-010, SPEC-006 |
| [`cross-cutting/assistant-conversation-package.md`](cross-cutting/assistant-conversation-package.md) | Assistant Conversation Package (deferred extraction; Step 0 consolidates the LLM/tool test doubles) | proposed | 2026-09-12 | SPEC-001 |
| [`cross-cutting/persist-assistant-run-state-for-reload.md`](cross-cutting/persist-assistant-run-state-for-reload.md) | Persist assistant run state (errors/cancels/running) for reliable reload | complete | 2026-08-20 | SPEC-001, SPEC-003 |
| [`cross-cutting/sqlite-session-storage.md`](cross-cutting/sqlite-session-storage.md) | SQLite session storage (future work, proposed) | proposed | 2026-08-20 | SPEC-001 |
| [`cross-cutting/xdg-config-data-relocation.md`](cross-cutting/xdg-config-data-relocation.md) | XDG Config/Data Relocation + Storage Cleanup + Reset Controls (Phases 0–7, 9–12 complete; Phase 8 removed; reset/clear-runtime-data hardened) | complete | 2026-08-07 | CONSTITUTION III.2/III.4/III.6 |
| [`memory/memory-improvements-implementation-plan.md`](memory/memory-improvements-implementation-plan.md) | Memory Improvements | partial | — | SPEC-004 |
| [`cross-cutting/admin-api-authentication.md`](cross-cutting/admin-api-authentication.md) | Admin / API Authentication and Control-Plane Hardening (no auth today; Host/Origin checks, operator token, `/v1` keys, safer bind) | proposed | 2026-09-30 | SPEC-003, SPEC-006, SPEC-007, SPEC-009 |
| [`cross-cutting/sandbox-and-egress-residuals.md`](cross-cutting/sandbox-and-egress-residuals.md) | Sandbox and Egress Residual Hardening (`0.0.0.0` guard gap, port-kill ownership, shell→loopback, platform residuals) | proposed | 2026-09-30 | SPEC-006, SPEC-008, SPEC-009 |
| [`memory/small-context-memory.md`](memory/small-context-memory.md) | Memory for Small-Context Local Models (per-run injection, window-scaled budget, progress ledger, step-aware hints, operator UX) | complete | 2026-10-01 | SPEC-001, SPEC-004, SPEC-005, SPEC-007 |
| [`orchestrator/local-and-cloud-inference-performance.md`](orchestrator/local-and-cloud-inference-performance.md) | Local and Cloud Inference Performance (measure first; KV-cache/prefix stability, timings capture, cloud TTFT) | proposed | 2026-09-30 | SPEC-001, SPEC-005, SPEC-007 |
| [`cross-cutting/backend-hot-paths-and-leak-hardening.md`](cross-cutting/backend-hot-paths-and-leak-hardening.md) | Backend Hot Paths, Memory Growth and Leak Hardening (gauges, soak test, session checkpoint, list endpoints) | proposed | 2026-09-30 | SPEC-001, SPEC-007 |
| [`assistant-ui/assistant-automation-workbench-layout.md`](assistant-ui/assistant-automation-workbench-layout.md) | Assistant and Automation Workbench Layout (chat+editor split, automation master-detail, one run renderer, context meter) | proposed | 2026-09-30 | SPEC-001, SPEC-003, SPEC-007 |

Completed, superseded, and not-implemented plans live in [`ARCHIVE/`](ARCHIVE/) — loaded only when their specific topic is relevant. The detailed plan archive/consolidation changelog (previously in `docs/INDEX.md`) is retained in that file's git history.

## Remaining Work (non-complete plans)

Filtered view of everything not `complete`. Use this as the live "what's left" tracker — **no separate todo folder**, status stays on the plan itself.

| Status | Plan | Open scope |
|--------|-------|-----------|
| approved | Unattended Run Safety Hardening | Steps 6–9 (context-aware I/O hardening, unattended tool restriction & spiral detection, perf optimizations, docs sync) |
| active | GPU Performance | P0–P4 rendering/metrics (P5 unblocked — fix-final-report-realignment landed) |
| active | Assistant UI Overhaul | Phase 5 backend SSE-bleed fix only (Phase 4 and the chat chrome delivered by the retro redesign, 2026-09-30) |
| active | Connector Auto-Reply | Phase 3 interactive gateway paths |
| active | Post-Implementation Cleanup | execute findings register (dead code/dup sweep) |
| partial | CI & Versioning | P2/P3 hygiene work + P6 (verify tag flow on a real merge); P5 build-release deliberately parked |
| partial | Agent Improvements | re-scoped 2026-09-05: only Phase 4 (tool dedup), Phase 5 (UsageTracker), Phase 7 remainder (sub-agent wrap) still valid — see re-scope note in plan |
| partial | Memory Improvements | Session Search tool (FTS5 `session_search`) + Skill System (procedural memory) — everything else done |
| partial | Assistant Liveness Heartbeat & Package Restructure | §4.3/§4.4 package extractions deferred (§2, §3, §4.1 done; §4.2 rejected) |
| proposed | AGENTS.md Layering & Guardrails | design acceptance + implementation |
| proposed | Agent OS Sandboxing | Phases 1–6 (rlimits → FS jail → network switch → OS network deny → egress proxy → deployment hardening) |
| proposed | Tool-Call Grammar Re-enable | envelope-aware GBNF + opt-in per-model toggle, XML path only |
| proposed | Search Tool Calling | provider factory (Tavily/Brave/SerpAPI) + Search settings tab + live key + hide-when-unconfigured gate |
| active | Tool Error Classification & Run-Fatality Policy | implemented (`ErrToolUnavailable`, `toolpolicy`, delivery warnings) with automated gates green; manual end-to-end verification pending |
| proposed | Assistant Conversation Package | Step 0 consolidate LLM/tool test doubles, then extract `conversation` (loop core/strategies stay — need a session facade) |
| proposed | SQLite Session Storage | JSON → SQLite migration (deliberately deferred, future work) |
| proposed | Admin / API Authentication | Phase 0 (route-classification + exploit-shaped tests, prove/kill key-exfil via base URL) → Phase 1 zero-config hardening (Host/Origin, drop `ACAO: *`, PID-stop ownership, mandatory webhook secret) → Phases 2–4 need user decisions A1–A7 |
| proposed | Sandbox and Egress Residuals | Phase 1 guard fix (`0.0.0.0`/`::`/multicast, fail-closed pre-check, `freePort` ownership) → Phase 2 shell→loopback decision → Phase 3 optional platform items |
| proposed | Local/Cloud Inference Performance | Phase 0 measurement (llama.cpp `timings`, cloud TTFT, prefix-stability test) gates every optimisation |
| proposed | Backend Hot Paths and Leaks | Phase 0 gauges + build-tagged soak + benchmarks → session-checkpoint decision (A/B/C) → list endpoints |
| proposed | Assistant/Automation Workbench Layout | Phase 0 wireframes + SPEC-003 amendment draft need sign-off → `context_usage` event + SSE-bleed backend fix → automation master-detail/one renderer → workspace workbench |

> **2026-09-05 hygiene pass:** archived as complete — `knight-rider-arc-bubble.md` (extraction to `ArcOrbitLoader` verified in code), `cloud-provider-token-budgets.md` (all phases incl. merged Phase 7 + M8 probe verified). Archived as merged — `cancel-stale-turn-bleed.md` (backend SSE bleed now homed in overhaul Phase 5). `xdg-config-data-relocation.md` removed from this table (already complete, row was stale).
>
> **2026-09-25 consolidation:** the completed `cross-cutting/global-run-lane-scheduler.md` and `cross-cutting/inbound-request-admission.md` were folded into **SPEC-007 §V/§V.1** (run scheduler + model-residency contract) plus `docs/architecture.md` (package layout, pitfalls #34–#36) and the `automation` skill, then removed from `docs/PLANS/` — their design rationale remains in git history.

See [`docs/INDEX.md`](../INDEX.md) for the catalog router (SPECs → `docs/SPECS/README.md`, audits → `docs/audits/README.md`, skills → `.agents/skills/`).
