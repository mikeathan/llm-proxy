# Documentation Index

This index catalogs every documentation file in the repository. Each entry includes a stable ID,
status, and cross-references to related documents. Use this as the starting point for navigation.

**Status Legend:** `stable` | `draft` | `superseded` | `reference` | `complete` | `partial` | `proposed` | `active`

---

## Constitution (Immutable Law)

| ID | File | Title | Status | Sections |
|----|------|-------|--------|----------|
| LAW | `CONSTITUTION.md` | Architectural Invariants | Law | I–VI (15 subsections) |

## Skills (Reference Guides)

Repo Agent Skills live in `.agents/skills/<name>/SKILL.md` — auto-discovered by Command Code, Pi,
opencode, and dsh, and loaded on demand (only name + description are always in context).

| Skill | Path | Use when |
|-------|------|----------|
| agent-loop | `.agents/skills/agent-loop/SKILL.md` | Sieve, fallback chain, reasoning budget, spiral detector |
| assistant-ui-chat | `.agents/skills/assistant-ui-chat/SKILL.md` | Event handlers, segment system, inactivity timer, SSE timing, scroll behavior, common pitfalls |
| assistant-ui-patterns | `.agents/skills/assistant-ui-patterns/SKILL.md` | AgentIde shell: layout, sidebar/drawer states, mobile breakpoints, shared renderer, UI gotchas |
| automation | `.agents/skills/automation/SKILL.md` | Dispatcher, executor, run lifecycle, templates |
| clean-code | `.agents/skills/clean-code/SKILL.md` | Naming, functions, comments, formatting, boundaries, error handling, tests, SOLID, emergent design, concurrency, smells & heuristics checklist |
| connector-patterns | `.agents/skills/connector-patterns/SKILL.md` | Outbound connector steps, inbound webhook, CONSTITUTION checklist, common errors |
| debugging | `.agents/skills/debugging/SKILL.md` | Root-cause a failing test/build/agent run/tool call: reproduce, locate, isolate, fix, regression-test |
| documentation-stewardship | `.agents/skills/documentation-stewardship/SKILL.md` | Post-change doc pass: SPEC/plan/INDEX/pitfalls/skill/CONSTITUTION mapping + verify |
| engineering-practices | `.agents/skills/engineering-practices/SKILL.md` | Go patterns, code style, frontend icon conventions, file checklists |
| event-streaming-patterns | `.agents/skills/event-streaming-patterns/SKILL.md` | SSE composables, observer chaining, guardrail flow, heartbeat cleanup, dedup |
| lifecycle-events | `.agents/skills/lifecycle-events/SKILL.md` | Session phases, SSE contract, frontend handler, testing |
| llamacpp-setup | `.agents/skills/llamacpp-setup/SKILL.md` | Args, GPU tuning, systemd, performance data |
| memory-system | `.agents/skills/memory-system/SKILL.md` | Injection, three-tier, tags, dedup, gotchas |
| task-planning | `.agents/skills/task-planning/SKILL.md` | Classify, trace the code path, scope, decompose into verifiable steps, plan docs, resume |
| tdd-guide | `.agents/skills/tdd-guide/SKILL.md` | Red/Green/Refactor flow, test grouping, keeping the suite fast |
| testing-guide | `.agents/skills/testing-guide/SKILL.md` | Smoke tests, record-replay, run analysis, templates, MockClient patterns |

## Specifications (Behavioral Contracts)

| ID | File | Title | Status | Constitution Refs |
|----|------|-------|--------|-------------------|
| SPEC-001 | `docs/SPECS/agent-loop.md` | Agent Loop | stable | II.4, II.5, II.6, II.7, II.8, II.10 |
| SPEC-002 | `docs/SPECS/tool-call-parser.md` | Tool Call Parser | stable | II.4 |
| SPEC-003 | `docs/SPECS/discovery-panel.md` | Discovery Panel UI | stable | — |
| SPEC-004 | `docs/SPECS/memory.md` | Memory System | stable | II.12 |
| SPEC-005 | `docs/SPECS/orchestrator.md` | Orchestrator / Budget | stable | VI |
| SPEC-006 | `docs/SPECS/guardrails.md` | Guardrail Engine | stable | II.3 |
| SPEC-007 | `docs/SPECS/automation-dispatcher.md` | Automation Dispatcher | stable | — |
| SPEC-008 | `docs/SPECS/mcp-integration.md` | MCP Integration | stable | — |
| SPEC-009 | `docs/SPECS/communication.md` | Communication Connector System | stable | II.4, II.5, V |
| SPEC-010 | `docs/SPECS/agent-loop-strategies.md` | Agent Loop Strategies | stable | II.4, II.5, II.6, II.7, II.10, II.13 |

## Active Implementation Plans

| File | Title | Status | Date | Related Specs |
|------|-------|--------|------|---------------|
| `docs/PLANS/agent-loop/agent-improvements.md` | Agent Improvements (7-phase; re-scoped 2026-09-05 — Phases 3/6 superseded by SPEC-010, Phase 4/5/7-remainder open) | partial | — | SPEC-001 |
| `docs/PLANS/agent-loop/agent-loop-strategies.md` | Agent Loop Strategies (pluggable loop-strategy engine) | complete | 2026-08-16 | SPEC-001, SPEC-010 |
| `docs/PLANS/agent-loop/strategy-agnostic-completion-and-tool-schema.md` | Strategy-Agnostic Completion + Tool-Schema/Policy Consistency (finalizeReport, hardened plan prompt, guardrail-derived schema, non-aborting plan steps) | complete | 2026-08-18 | SPEC-010, SPEC-006, SPEC-001 |
| `docs/PLANS/agent-loop/surface-planning-reasoning.md` | Surface Plan-Generation Reasoning (stream the planner: plan-gen streams + relays `EventReasoning` via the shared primitive) | complete | 2026-08-18 | SPEC-010, SPEC-001, SPEC-003 |
| `docs/PLANS/agent-loop/fix-final-report-realignment.md` | Fix automation "Final Report" regression (canonical empty-finalization-turn fix; GPU P5 depends on it) | complete | 2026-08-07 | SPEC-001 |
| `docs/PLANS/assistant-ui/automation-edit-form-reactivity.md` | Fix Automation Edit Form (reactive populate) | proposed | 2026-08-01 | SPEC-003, SPEC-007 |
| `docs/PLANS/assistant-ui/automation-renderer-unify-consumption.md` | Unify Automation + Assistant Event Consumption | complete | 2026-07-18 | SPEC-003, SPEC-007 |
| `docs/PLANS/ARCHIVE/assistant-ui/cancel-stale-turn-bleed.md` | Cancel Stale Turn Bleed (frontend fixes done; backend SSE bleed homed in overhaul Phase 5) | superseded (merged) | 2026-06-26 | SPEC-003 |
| `docs/PLANS/assistant-ui/consolidate-app-banner.md` | Consolidate banner logic into a single event-driven `AppBanner` | complete | 2026-08-14 | SPEC-003 |
| `docs/PLANS/ARCHIVE/assistant-ui/knight-rider-arc-bubble.md` | Knight Rider Arc Bubble (extracted to `ArcOrbitLoader.vue`; consumed by ChatBubble + ChatInput) | complete | 2026-06-26 | SPEC-003 |
| `docs/PLANS/assistant-ui/overhaul-chat-history-layout.md` | Assistant UI Overhaul | active | 2026-06-25 | SPEC-003 |
| `docs/PLANS/assistant-ui/reasoning-inset-auto-collapse.md` | Reasoning inset auto-expand while running, collapse on done | complete | 2026-08-14 | SPEC-003 |
| `docs/PLANS/ARCHIVE/cross-cutting/cloud-provider-token-budgets.md` | Cloud Provider Token Budgets + Provider Set Reduction (**+ Phase 7 reasoning enable, merged**; M8 probe verified in `context_resolution.go`) | complete | 2026-08-01 | SPEC-005, SPEC-003 |
| `docs/PLANS/cross-cutting/per-model-reasoning-overrides-and-settings-layout.md` | Per-Model Reasoning Overrides and Settings Layout Repair | complete | 2026-08-02 | SPEC-005 |
| `docs/PLANS/cross-cutting/connector-inbound-webhook.md` | Communication Connector Inbound Webhook | complete | 2026-06-28 | SPEC-009 |
| `docs/PLANS/cross-cutting/connector-auto-reply.md` | Communication Connector Auto-Reply & Automation Trigger | active | 2026-06-28 | SPEC-009 |
| `docs/PLANS/cross-cutting/reasoning-capture-dynamic.md` | Dynamic provider-agnostic reasoning + neutral indicator (incl. merged neutral-working-state) | complete | 2026-07-28 | SPEC-003, SPEC-005 |
| `docs/PLANS/cross-cutting/session-source-backend-driven.md` | Session `source` derived from backend (single source of truth) | complete | 2026-07-09 | SPEC-009 |
| `docs/PLANS/ARCHIVE/cross-cutting/universal-agent-completion.md` | Universal Agent Completion Model | complete | 2026-07-21 | SPEC-001, SPEC-002 |
| `docs/PLANS/cross-cutting/webhook-fresh-sessions.md` | Fresh Webhook Sessions + Source Grouping | complete | 2026-07-09 | SPEC-009 |
| `docs/PLANS/cross-cutting/post-implementation-cleanup.md` | Post-Implementation Cleanup: Duplication & Dead Code | active | 2026-08-01 | SPEC-005 |
| `docs/PLANS/cross-cutting/ci-github-actions-and-versioning.md` | CI, GitHub Actions Integration & Versioning Flow (gitleaks, release-please, tag-on-merge; P1 CI + P4 tag implemented, P2/P3 + P6 pending, P5 parked) | partial | 2026-08-30 | — |
| `docs/PLANS/cross-cutting/xdg-config-data-relocation.md` | XDG Config/Data Relocation + Storage Cleanup + Reset Controls (Phases 0–7, 9–12 complete; Phase 8 removed; reset/clear-runtime-data hardened; **2026-08-11: two-root design superseded by single-root consolidation — all files under one root**) | complete | 2026-08-07 | CONSTITUTION III.2/III.4/III.6 |
| `docs/PLANS/cross-cutting/agents-md-layering-guardrails.md` | AGENTS.md Layering, Override-ability & Write Guardrails | proposed | 2026-08-04 | SPEC-001, CONSTITUTION II.13/II.10 |
| `docs/PLANS/cross-cutting/tool-call-grammar-reenable.md` | Re-enable Tool-Call Grammar Constraint (opt-in, llama.cpp-safe) | proposed | 2026-09-05 | SPEC-001, SPEC-002 |
| `docs/PLANS/cross-cutting/search-tool-calling.md` | Wire up `internet_search` tool calling (pluggable multi-provider: provider factory + Search settings tab + live key + hide-when-unconfigured gate) | active | 2026-09-12 | SPEC-001, SPEC-006 |
| `docs/PLANS/cross-cutting/tool-error-classification.md` | Tool Error Classification & Run-Fatality Policy (terminal tool errors, delivery-vs-essential tools, failure bound) | proposed | 2026-09-12 | SPEC-001, SPEC-010, SPEC-006 |
| `docs/PLANS/cross-cutting/assistant-conversation-package.md` | Assistant Conversation Package (deferred extraction; Step 0 consolidates the LLM/tool test doubles) | proposed | 2026-09-12 | SPEC-001 |
| `docs/PLANS/cross-cutting/agent-os-sandboxing.md` | Agent OS Sandboxing (rev 2 — network-first, uid-first; one action pipeline with OS jail + egress proxy as execution backends, dedicated-user deployment, per-run network grants; decisions D1–D8, measured platform facts; Phases 0–4 implemented + post-review hardening pass) | complete — pending Linux-CI runtime confirmation of the Landlock probes + optional macOS Seatbelt on framework-capable hardware | 2026-09-06 | SPEC-006, SPEC-009 |
| `docs/PLANS/cross-cutting/sandbox-runtime-invisibility.md` | Sandbox Runtime Invisibility (`.sandbox` hidden from filesystem listings + terminal output) | complete | 2026-08-25 | SPEC-006, CONSTITUTION II.3 |
| `docs/PLANS/cross-cutting/persist-assistant-run-state-for-reload.md` | Persist assistant run state (errors/cancels/running) for reliable reload | complete | 2026-08-20 | SPEC-001, SPEC-003 |
| `docs/PLANS/cross-cutting/sqlite-session-storage.md` | SQLite session storage (future work, proposed) | proposed | 2026-08-20 | SPEC-001 |
| `docs/PLANS/gpu-performance.md` | GPU Performance (consolidated: completed + next steps; P5 bug note → fix-final-report) | active | 2026-08-06 | — |
| `docs/PLANS/primary-model-warning-banner.md` | Remove model auto-bootstrap; explicit primary/fallback selection + banners | complete | 2026-08-14 | SPEC-003, CONSTITUTION III.4 |
| `docs/PLANS/memory/memory-improvements-implementation-plan.md` | Memory Improvements | partial | — | SPEC-004 |
| `docs/PLANS/unattended-run-safety-hardening.md` | Unattended Run Safety Hardening (13 gaps, 7 leaks, 5 optimizations) | approved | 2026-07-22 | SPEC-001, SPEC-006, SPEC-007 |

> **Consolidation map (2026-08-06):** Three clusters grouped by true overlap.
> - **Cluster A (merged):** `provider-agnostic-reasoning-enable.md` (proposed) → **Phase 7** of `cloud-provider-token-budgets.md`; original archived to `ARCHIVE/cross-cutting/`. Shared surface: `reasoning_param.go`, `models/workload.go`, `tuning.go`, `admin_handlers.go`, `ModelTuningFields.vue`. `WorkloadClass` (budget Phase B+C) is the precondition.
> - **Cluster B (merged):** GPU plan P5 stuck/nudge-loop bug note folded into `fix-final-report-realignment.md` as a sequencing constraint (GPU P1 re-measure blocked until that fix lands). GPU plan keeps only P0–P4 rendering/metrics.
> - **Cluster C (grouped, not merged):** `overhaul-chat-history-layout` + `cancel-stale-turn-bleed` + `knight-rider-arc-bubble` share `ChatBubble.vue`/`ChatInput.vue`/`turnGrouper.ts` but are distinct concerns. ~~`cancel-stale-turn-bleed`'s deferred backend SSE bleed overlaps overhaul **Phase 5** (home for the backend fix).~~ **2026-09-05:** `knight-rider-arc-bubble` (complete) and `cancel-stale-turn-bleed` (frontend fixes done; backend bleed absorbed into overhaul Phase 5) archived.
> - **Archive candidate:** `agent-loop/ephemeral-turn-context.md` (status reverted — historical, no active overlap).

## Archived Plans

Completed, superseded, and not-implemented plans are stored in `docs/PLANS/ARCHIVE/` — loaded only when their specific topic is relevant. On 2026-07-11, 7 stale plans were archived: `agent-loop/enhanced-agent-flow-and-compatibility.md`, `assistant-ui/simple-three-bubble.md`, `cross-cutting/interactive-user-input.md`, `cross-cutting/per-run-output-directories.md`, `cross-cutting/terminal-ui.md`, `assistant-ui/running-indicator-webhook.md`, `memory/mbtcp-implementation.md`. On 2026-08-01, the superseded `assistant-ui/automation-unified-renderer-and-report-truncation.md` was archived (replaced by `automation-renderer-unify-consumption.md`); the root `DUPLICATION_AUDIT.md` and `cross-cutting/reasoning-neutral-working-state.md` were merged into `docs/audits/codebase-audit-report.md` (appendix) and `docs/PLANS/cross-cutting/reasoning-capture-dynamic.md` respectively. On 2026-08-06, `cross-cutting/provider-agnostic-reasoning-enable.md` (proposed) was merged into `cross-cutting/cloud-provider-token-budgets.md` as **Phase 7** and archived to `ARCHIVE/cross-cutting/` (consolidation — shared `WorkloadClass`/`reasoning_param.go` surface). On 2026-08-07: the **Codebase Audit Report** was reclassified from plan to audit and moved to `docs/audits/codebase-audit-report.md`; `agent-loop/ephemeral-turn-context.md` (reverted) was archived to `ARCHIVE/agent-loop/`; `fix-final-report-realignment.md` moved to `agent-loop/`; `agents-md/` merged into `cross-cutting/`. The "Remaining Work" view in `PLANS/README.md` now tracks non-complete plans by status (no separate todo folder). On 2026-09-05 (plan-hygiene review): `assistant-ui/knight-rider-arc-bubble.md` archived as complete (`ArcOrbitLoader` extraction verified in code); `assistant-ui/cancel-stale-turn-bleed.md` archived as merged (backend SSE bleed homed in overhaul Phase 5); `cross-cutting/cloud-provider-token-budgets.md` archived as complete (all phases incl. merged Phase 7 + M8 probe verified in `orchestrator/context_resolution.go`); CI plan status corrected `proposed`→`partial`; `agent-improvements.md` re-scoped; memory plan duplicate-Phase-2 numbering fixed. On 2026-09-25, the completed `cross-cutting/global-run-lane-scheduler.md` and `cross-cutting/inbound-request-admission.md` plans were **consolidated into SPEC-007 §V/§V.1** (run scheduler + model-residency contract), `docs/architecture.md` (package layout, pitfalls #34–#36) and the `automation` skill, and removed from `docs/PLANS/` (git history retains the design rationale).

## Audits

| File | Title | Status | Date |
|------|-------|--------|------|
| `docs/audits/2026-06-26-stale-turn-bleed.md` | Stale Turn Bleed on Cancel + New Message | complete | 2026-06-26 |
| `docs/audits/2026-07-06-assistant-debug-cycle.md` | Full Debug Cycle (tool calls, history leak, GBNF) | complete | 2026-07-06 |
| `docs/audits/agent-stability-report.md` | Agent Stability Audit (13 issues) | complete | 2026-05-28 |
| `docs/audits/gpu-performance-audit.md` | GPU Performance — consolidated audit (all knowledge + fixes + lessons) | reference | 2026-08-06 |
| `docs/audits/known-performance-findings.md` | Known Performance Findings — provider TTFT vs local logic, SSE reader fix | reference | 2026-08-18 |
| `docs/audits/2026-08-28-ops-performance-review.md` | Ops & Backend Performance Review — findings + fixes (log rotation, tail reads, host-metrics cache, EventBus byte budget, compact session marshal) | reference | 2026-08-28 |
| `docs/audits/2026-08-30-llm-smoke-test-incomplete-run.md` | llm-smoke-test Incomplete Run — terminal newline collapse, premature finalization on truncated ReAct scaffold, local native-tools auto-detection | complete | 2026-08-30 |
| `docs/audits/ephemeral-turn-context-failed-run.md` | Ephemeral Turn Context — Failed Run Analysis | complete | 2026-06-08 |
| `docs/audits/memory-injection-investigation.md` | Memory Injection + Automation Limitations | reference | 2026-06-03 |
| `docs/audits/remove-memory-rewriter.md` | Remove Memory Rewriter + FTS5 Fix | complete | 2026-06-03 |
| `docs/audits/backend-audit-report.md` | Backend Audit Report (bugs, leaks, bottlenecks) | reference | 2026-07-03 |
| `docs/audits/hermes-write-file-guardrail.md` | Hermes Agent write_file Guardrail | reference | — |
| `docs/audits/write-file-truncation-cycles.md` | write_file Truncation Cycles + Block Editing | reference | — |
| `docs/audits/degenerate-stream-repetition-guard.md` | Degenerate stream repetition loop — content guard & per-stream duration cap | complete | 2026-08-20 |
| `docs/audits/codebase-audit-report.md` | Codebase Audit Report (88 findings + resolved duplication appendix) | active | 2026-07-03 |

## Agent Rules (AI Assistant Guidance)

| File | Title |
|------|-------|
| `.agents/rules/go-staff-engineer.md` | Go Coding Rules for AI |
| `.agents/rules/frontend-vue-engineer.md` | Vue Coding Rules for AI |

## Top-Level Guides

| File | Title | Audience |
|------|-------|----------|
| `README.md` | LLM Proxy — Quick Start & Config | End users |
| `AGENTS.md` | Instructions for AI Coding Assistants | AI assistants |
| `docs/architecture.md` | Architecture Reference (mappings, contracts, checklists, pitfalls) | Developers |
| `CONSTITUTION.md` | Architectural Invariants — The Law | Everyone |
| `docs/guides/loop-strategy.md` | Loop Strategy — Operator Guide (which archetype to pick) | Operators |

## Other Documents

| File | Title | Notes |
|------|-------|-------|
| `docs/services/llm-proxy.service` | systemd service unit (hardened: dedicated user, single root `/var/lib/llm-proxy`, strict lockdown) | Operational |
| `docs/service_setup.md` | Service installation instructions | Setup |
| `docs/data-layout.md` | Data Layout — root resolution, meta/ vs runs/ split, cleanup surfaces | Reference |
| `docs/SPEC-change-management.md` | SPEC Lifecycle & Change Management | Reference |
| `docs/SPECS/README.md` | Subdirectory catalog for all SPEC files | Index |
| `docs/audits/README.md` | Subdirectory catalog for audit files | Index |
| `docs/PLANS/ARCHIVE/` | Completed/superseded/not-implemented plans | Archive |
| `.agents/skills/` | Repo Agent Skills — auto-discovered by AI agents, loaded on demand | Skills |

---

## Directory Navigation

| Directory | Contents |
|-----------|----------|
| `docs/SPECS/` | Behavioral contracts (SPEC-NNN). Read before modifying subsystems. |
| `docs/guides/` | Operator-facing how-to guides (non-normative; SPECs stay authoritative). |
| `docs/PLANS/` | Active implementation strategies. Organized by subsystem. |
| `docs/PLANS/ARCHIVE/` | Completed and superseded plans — load on demand. |
| `docs/audits/` | Post-hoc analysis of system behavior against specs. |
| `.agents/skills/` | Repo Agent Skills — auto-discovered by AI agents, loaded on demand for deep-dive topics. |
| `.agents/rules/` | Per-language coding rules for AI assistants. |
