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

| File | Title | Topics |
|------|-------|--------|
| `docs/skills/agent-loop.md` | Agent Loop & Stuck Detection | Sieve, fallback chain, reasoning budget, spiral detector |
| `docs/skills/assistant-ui-chat.md` | Assistant UI Chat Architecture | Event handlers, segment system, inactivity timer, SSE timing, scroll behavior, common pitfalls |
| `docs/skills/assistant-ui-patterns.md` | Assistant UI Patterns | Sidebar states, SSE event flow, tool rendering, mobile breakpoints, common gotchas |
| `docs/skills/automation.md` | Automation System | Dispatcher, executor, run lifecycle, templates |
| `docs/skills/connector-patterns.md` | Connector Implementation Guide | Outbound connector steps, inbound webhook, CONSTITUTION checklist, common errors |
| `docs/skills/documentation-stewardship.md` | Documentation Stewardship | Post-completion checklist for doc updates |
| `docs/skills/engineering-practices.md` | Engineering Practices | Go patterns, code style, frontend icon conventions, file checklists |
| `docs/skills/event-streaming-patterns.md` | Event Streaming Patterns | SSE composables, observer chaining, guardrail flow, heartbeat cleanup, dedup |
| `docs/skills/lifecycle-events.md` | Session Lifecycle Events | Session phases, SSE contract, frontend handler, testing |
| `docs/skills/llamacpp-setup.md` | llama.cpp Server Setup | Args, GPU tuning, systemd, performance data |
| `docs/skills/memory-system.md` | Memory Architecture & Tags | Injection, three-tier, tags, dedup, gotchas |
| `docs/skills/testing-guide.md` | Testing Guide | Smoke tests, record-replay, run analysis, templates, MockClient patterns |
| `docs/skills/tool-failure-investigation.md` | Tool Failure Investigation | Error tracing, handler scoping, recovery prompt verification |

## Specifications (Behavioral Contracts)

| ID | File | Title | Status | Constitution Refs |
|----|------|-------|--------|-------------------|
| SPEC-001 | `docs/SPECS/agent-loop.md` | Agent Loop | stable | II.4, II.5, II.6, II.7, II.8, II.10 |
| SPEC-002 | `docs/SPECS/tool-call-parser.md` | Tool Call Parser | stable | II.4 |
| SPEC-003 | `docs/SPECS/discovery-panel.md` | Discovery Panel UI | stable | — |
| SPEC-004 | `docs/SPECS/memory.md` | Memory System | stable | II.12 |
| SPEC-005 | `docs/SPECS/orchestrator.md` | Orchestrator / Budget | stable | VI |
| SPEC-006 | `docs/SPECS/guardrails.md` | Guardrail Engine | stable | I.5 |
| SPEC-007 | `docs/SPECS/automation-dispatcher.md` | Automation Dispatcher | stable | — |
| SPEC-008 | `docs/SPECS/mcp-integration.md` | MCP Integration | stable | — |
| SPEC-009 | `docs/SPECS/communication.md` | Communication Connector System | stable | II.4, II.5, V |
| SPEC-010 | `docs/SPECS/agent-loop-strategies.md` | Agent Loop Strategies | stable | II.4, II.5, II.6, II.7, II.10, II.13 |

## Active Implementation Plans

| File | Title | Status | Date | Related Specs |
|------|-------|--------|------|---------------|
| `docs/PLANS/agent-loop/agent-improvements.md` | Agent Improvements (7-phase) | partial | — | SPEC-001 |
| `docs/PLANS/agent-loop/agent-loop-strategies.md` | Agent Loop Strategies (pluggable loop-strategy engine) | complete | 2026-08-16 | SPEC-001, SPEC-010 |
| `docs/PLANS/agent-loop/strategy-agnostic-completion-and-tool-schema.md` | Strategy-Agnostic Completion + Tool-Schema/Policy Consistency (finalizeReport, hardened plan prompt, guardrail-derived schema, non-aborting plan steps) | complete | 2026-08-18 | SPEC-010, SPEC-006, SPEC-001 |
| `docs/PLANS/agent-loop/surface-planning-reasoning.md` | Surface Plan-Generation Reasoning (stream the planner: plan-gen streams + relays `EventReasoning` via the shared primitive) | complete | 2026-08-18 | SPEC-010, SPEC-001, SPEC-003 |
| `docs/PLANS/agent-loop/fix-final-report-realignment.md` | Fix automation "Final Report" regression (canonical empty-finalization-turn fix; GPU P5 depends on it) | complete | 2026-08-07 | SPEC-001 |
| `docs/PLANS/assistant-ui/automation-edit-form-reactivity.md` | Fix Automation Edit Form (reactive populate) | proposed | 2026-08-01 | SPEC-003, SPEC-007 |
| `docs/PLANS/assistant-ui/automation-renderer-unify-consumption.md` | Unify Automation + Assistant Event Consumption | complete | 2026-07-18 | SPEC-003, SPEC-007 |
| `docs/PLANS/assistant-ui/cancel-stale-turn-bleed.md` | Cancel Stale Turn Bleed | active | 2026-06-26 | SPEC-003 |
| `docs/PLANS/assistant-ui/consolidate-app-banner.md` | Consolidate banner logic into a single event-driven `AppBanner` | complete | 2026-08-14 | SPEC-003 |
| `docs/PLANS/assistant-ui/knight-rider-arc-bubble.md` | Knight Rider Arc Bubble | active | 2026-06-26 | SPEC-003 |
| `docs/PLANS/assistant-ui/overhaul-chat-history-layout.md` | Assistant UI Overhaul | active | 2026-06-25 | SPEC-003 |
| `docs/PLANS/assistant-ui/reasoning-inset-auto-collapse.md` | Reasoning inset auto-expand while running, collapse on done | complete | 2026-08-14 | SPEC-003 |
| `docs/PLANS/cross-cutting/cloud-provider-token-budgets.md` | Cloud Provider Token Budgets + Provider Set Reduction (**+ Phase 7 reasoning enable, merged**) | active | 2026-08-01 | SPEC-005, SPEC-003 |
| `docs/PLANS/cross-cutting/per-model-reasoning-overrides-and-settings-layout.md` | Per-Model Reasoning Overrides and Settings Layout Repair | complete | 2026-08-02 | SPEC-005 |
| `docs/PLANS/cross-cutting/connector-inbound-webhook.md` | Communication Connector Inbound Webhook | complete | 2026-06-28 | SPEC-009 |
| `docs/PLANS/cross-cutting/connector-auto-reply.md` | Communication Connector Auto-Reply & Automation Trigger | active | 2026-06-28 | SPEC-009 |
| `docs/PLANS/cross-cutting/reasoning-capture-dynamic.md` | Dynamic provider-agnostic reasoning + neutral indicator (incl. merged neutral-working-state) | complete | 2026-07-28 | SPEC-003, SPEC-005 |
| `docs/PLANS/cross-cutting/session-source-backend-driven.md` | Session `source` derived from backend (single source of truth) | complete | 2026-07-09 | SPEC-009 |
| `docs/PLANS/ARCHIVE/cross-cutting/universal-agent-completion.md` | Universal Agent Completion Model | complete | 2026-07-21 | SPEC-001, SPEC-002 |
| `docs/PLANS/cross-cutting/webhook-fresh-sessions.md` | Fresh Webhook Sessions + Source Grouping | complete | 2026-07-09 | SPEC-009 |
| `docs/PLANS/cross-cutting/post-implementation-cleanup.md` | Post-Implementation Cleanup: Duplication & Dead Code | active | 2026-08-01 | SPEC-005 |
| `docs/PLANS/cross-cutting/ci-github-actions-and-versioning.md` | CI, GitHub Actions Integration & Versioning Flow (gitleaks, release-please, tag-on-merge; free/no external services) | proposed | 2026-08-30 | — |
| `docs/PLANS/cross-cutting/xdg-config-data-relocation.md` | XDG Config/Data Relocation + Storage Cleanup + Reset Controls (Phases 0–7, 9–12 complete; Phase 8 removed; reset/clear-runtime-data hardened; **2026-08-11: two-root design superseded by single-root consolidation — all files under one root**) | complete | 2026-08-07 | CONSTITUTION III.2/III.4/III.6 |
| `docs/PLANS/cross-cutting/agents-md-layering-guardrails.md` | AGENTS.md Layering, Override-ability & Write Guardrails | proposed | 2026-08-04 | SPEC-001, CONSTITUTION II.13/II.10 |
| `docs/PLANS/cross-cutting/tool-call-grammar-reenable.md` | Re-enable Tool-Call Grammar Constraint (opt-in, llama.cpp-safe) | proposed | 2026-09-05 | SPEC-001, SPEC-002 |
| `docs/PLANS/cross-cutting/agent-os-sandboxing.md` | Agent OS Sandboxing (Landlock/Seatbelt FS+network jail, rlimits, network switch, 6 phases; threat model + Docker/profiles rejected-decisions record) | proposed | 2026-08-31 | SPEC-006 |
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
> - **Cluster C (grouped, not merged):** `overhaul-chat-history-layout` + `cancel-stale-turn-bleed` + `knight-rider-arc-bubble` share `ChatBubble.vue`/`ChatInput.vue`/`turnGrouper.ts` but are distinct concerns. `cancel-stale-turn-bleed`'s deferred backend SSE bleed overlaps overhaul **Phase 5** (home for the backend fix).
> - **Archive candidate:** `agent-loop/ephemeral-turn-context.md` (status reverted — historical, no active overlap).

## Archived Plans

Completed, superseded, and not-implemented plans are stored in `docs/PLANS/ARCHIVE/` — loaded only when their specific topic is relevant. On 2026-07-11, 7 stale plans were archived: `agent-loop/enhanced-agent-flow-and-compatibility.md`, `assistant-ui/simple-three-bubble.md`, `cross-cutting/interactive-user-input.md`, `cross-cutting/per-run-output-directories.md`, `cross-cutting/terminal-ui.md`, `assistant-ui/running-indicator-webhook.md`, `memory/mbtcp-implementation.md`. On 2026-08-01, the superseded `assistant-ui/automation-unified-renderer-and-report-truncation.md` was archived (replaced by `automation-renderer-unify-consumption.md`); the root `DUPLICATION_AUDIT.md` and `cross-cutting/reasoning-neutral-working-state.md` were merged into `docs/PLANS/codebase-audit-report.md` (appendix) and `docs/PLANS/cross-cutting/reasoning-capture-dynamic.md` respectively. On 2026-08-06, `cross-cutting/provider-agnostic-reasoning-enable.md` (proposed) was merged into `cross-cutting/cloud-provider-token-budgets.md` as **Phase 7** and archived to `ARCHIVE/cross-cutting/` (consolidation — shared `WorkloadClass`/`reasoning_param.go` surface). On 2026-08-07: the **Codebase Audit Report** was reclassified from plan to audit and moved to `docs/audits/codebase-audit-report.md`; `agent-loop/ephemeral-turn-context.md` (reverted) was archived to `ARCHIVE/agent-loop/`; `fix-final-report-realignment.md` moved to `agent-loop/`; `agents-md/` merged into `cross-cutting/`. The "Remaining Work" view in `PLANS/README.md` now tracks non-complete plans by status (no separate todo folder).

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

| File | Title | Lines |
|------|-------|-------|
| `.agents/rules/go-staff-engineer.md` | Go Coding Rules for AI | 48 |
| `.agents/rules/frontend-vue-engineer.md` | Vue Coding Rules for AI | 53 |

## Top-Level Guides

| File | Title | Lines | Audience |
|------|-------|-------|----------|
| `README.md` | LLM Proxy — Quick Start & Config | 63 | End users |
| `AGENTS.md` | Instructions for AI Coding Assistants | 53 | AI assistants |
| `docs/architecture.md` | Architecture Reference (mappings, contracts, checklists, pitfalls) | 272 | Developers |
| `CONSTITUTION.md` | Architectural Invariants — The Law | 116 | Everyone |
| `docs/guides/loop-strategy.md` | Loop Strategy — Operator Guide (which archetype to pick) | — | Operators |

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
| `docs/skills/` | AI assistant skill files — deep-dive reference guides | Categories |
| `docs/skills/README.md` | Quick-reference "when to load which skill" map | Navigation |

---

## Directory Navigation

| Directory | Contents |
|-----------|----------|
| `docs/SPECS/` | Behavioral contracts (SPEC-NNN). Read before modifying subsystems. |
| `docs/guides/` | Operator-facing how-to guides (non-normative; SPECs stay authoritative). |
| `docs/PLANS/` | Active implementation strategies. Organized by subsystem. |
| `docs/PLANS/ARCHIVE/` | Completed and superseded plans — load on demand. |
| `docs/audits/` | Post-hoc analysis of system behavior against specs. |
| `docs/skills/` | AI assistant reference guides — loaded on demand for deep-dive topics. |
| `.agents/rules/` | Per-language coding rules for AI assistants. |
