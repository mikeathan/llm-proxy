# Archived plans

Finished, superseded and not-implemented plans, kept as evidence. **Not loaded by default** — open one only
when its topic is in play. Current behavior lives in the SPECs and `docs/architecture/`; decisions in
`docs/decisions/`; lessons in `docs/learnings/`. If an archived plan disagrees with those, they win.

Archiving: `git mv` the plan here under the same folder name, add a row below, and remove its rows from
[`../README.md`](../README.md). Rows marked with a date and specs were archived from the live tracker on
2026-10-26; the rest predate this index.

| File | Title | Status | Date | Related Specs |
|------|-------|--------|------|---------------|
| [`agent-loop/agent-loop-strategies.md`](agent-loop/agent-loop-strategies.md) | Agent Loop Strategies (pluggable loop-strategy engine) | complete | 2026-08-16 | SPEC-001, SPEC-010 |
| [`agent-loop/agents-editor.md`](agent-loop/agents-editor.md) | Agents editor — a Settings section to create, duplicate, edit and delete profiles and helpers (same `profiles/*.md` files; `GET/PUT/DELETE /admin/api/profiles/{id}`, schema endpoint) | complete | 2026-10-23 | SPEC-012, SPEC-001, SPEC-003 |
| [`agent-loop/agnostic-agent-loop.md`](agent-loop/agnostic-agent-loop.md) | ARCHITECTURAL BLUEPRINT: Universal LLM-Agnostic Agent Loop | complete | — | — |
| [`agent-loop/auto-context-budget.md`](agent-loop/auto-context-budget.md) | Auto-Compute context_budget (and prefill) for Local LLMs | complete | — | — |
| [`agent-loop/autodetect-native-format.md`](agent-loop/autodetect-native-format.md) | Plan: Auto-Detect Native Tool Format & Jinja Template from GGUF | not_implemented | — | — |
| [`agent-loop/enhanced-agent-flow-and-compatibility.md`](agent-loop/enhanced-agent-flow-and-compatibility.md) | Plan - Enhanced Agent Flow & Model Compatibility | superseded | — | — |
| [`agent-loop/ephemeral-turn-context.md`](agent-loop/ephemeral-turn-context.md) | Ephemeral Turn Context — Reduce Recap Overhead | reverted | — | — |
| [`agent-loop/fallback-native-tools-fix.md`](agent-loop/fallback-native-tools-fix.md) | Fix: Reasoning-Stuck Fallback Actually Retries Without Native Tools | complete | — | — |
| [`agent-loop/fix-final-report-realignment.md`](agent-loop/fix-final-report-realignment.md) | Fix automation "Final Report" regression | complete | 2026-08-07 | SPEC-001 |
| [`agent-loop/refactor-assistant-clean-code.md`](agent-loop/refactor-assistant-clean-code.md) | Refactoring Assistant Package — Clean Code & Architecture | complete | — | — |
| [`agent-loop/request-size-estimator.md`](agent-loop/request-size-estimator.md) | Request size estimator — one request measure scaled by the server's own token counts, typed overflow error, one-step recovery (vertex 16k overflow, 2026-10-17) | complete | 2026-10-18 | SPEC-001, SPEC-005, SPEC-004 |
| [`agent-loop/run-pipeline-and-agent-commands.md`](agent-loop/run-pipeline-and-agent-commands.md) | Run pipeline cleanup and agent commands — Part C of delegation: dead code, one chat service (lifecycle out of the handler, ≤4 parameters), `/research` and `/agent` turn commands for every channel, merged duplicates, package moves | complete | 2026-10-24 | SPEC-001, SPEC-007, SPEC-009, SPEC-012, SPEC-003 |
| [`agent-loop/strategy-agnostic-completion-and-tool-schema.md`](agent-loop/strategy-agnostic-completion-and-tool-schema.md) | Strategy-Agnostic Completion + Tool-Schema/Policy Consistency | complete | 2026-08-18 | SPEC-010, SPEC-006, SPEC-001 |
| [`agent-loop/surface-planning-reasoning.md`](agent-loop/surface-planning-reasoning.md) | Surface Plan-Generation Reasoning (stream the planner) | complete | 2026-08-18 | SPEC-010, SPEC-001, SPEC-003 |
| [`agent-loop/tool-choice-temperature-implementation.md`](agent-loop/tool-choice-temperature-implementation.md) | Tool Choice & Temperature — Implementation Plan | complete | — | — |
| [`agent-loop/tool-failure-skip.md`](agent-loop/tool-failure-skip.md) | Plan: Graceful Tool Failure Recovery | superseded | — | — |
| [`agent-loop/unify-assistant-automation.md`](agent-loop/unify-assistant-automation.md) | Unify Assistant and Automation Agent Paths | superseded | — | — |
| [`agent-loop/use-helpers-control.md`](agent-loop/use-helpers-control.md) | Use Helpers — a composer switch that makes a research chat delegate (a local model never does on its own: 0/5 unprompted, 5/5 when asked); `use_helpers` note on the message, `GET /admin/api/helpers`, third row in Run settings | complete | 2026-10-22 | SPEC-001, SPEC-003, SPEC-012 |
| [`assistant-ui/automation-edit-form-reactivity.md`](assistant-ui/automation-edit-form-reactivity.md) | Fix Automation Edit Form — reactive populate | complete | 2026-08-01 | SPEC-003, SPEC-007 |
| [`assistant-ui/automation-renderer-unify-consumption.md`](assistant-ui/automation-renderer-unify-consumption.md) | Unify Automation + Assistant Event Consumption | complete | 2026-07-18 | SPEC-003, SPEC-007 |
| [`assistant-ui/automation-unified-renderer-and-report-truncation.md`](assistant-ui/automation-unified-renderer-and-report-truncation.md) | Unify Automation UI on Assistant Renderer + Fix Report Truncation | complete | — | — |
| [`assistant-ui/cancel-stale-turn-bleed.md`](assistant-ui/cancel-stale-turn-bleed.md) | Fix: stale turn bleed on cancel + new message | superseded | — | — |
| [`assistant-ui/composer-queue-and-reminder-alerts.md`](assistant-ui/composer-queue-and-reminder-alerts.md) | Composer message queue while a run is live, and live web UI alerts when a reminder fires (owner phone testing, 2026-10-18) | complete | 2026-10-18 | SPEC-003, SPEC-011, SPEC-001 |
| [`assistant-ui/consolidate-app-banner.md`](assistant-ui/consolidate-app-banner.md) | Consolidate banner logic into a single event-driven `AppBanner` | complete | 2026-08-14 | SPEC-003 |
| [`assistant-ui/consolidated-streaming-bubbles.md`](assistant-ui/consolidated-streaming-bubbles.md) | Consolidated Streaming Bubbles | superseded | — | — |
| [`assistant-ui/knight-rider-arc-bubble.md`](assistant-ui/knight-rider-arc-bubble.md) | Knight rider arc on assistant bubble | complete | — | — |
| [`assistant-ui/overhaul-chat-history-layout.md`](assistant-ui/overhaul-chat-history-layout.md) | Assistant UI Overhaul | complete | 2026-09-30 | SPEC-003 |
| [`assistant-ui/reasoning-inset-auto-collapse.md`](assistant-ui/reasoning-inset-auto-collapse.md) | Reasoning inset auto-expand while running, collapse on done | complete | 2026-08-14 | SPEC-003 |
| [`assistant-ui/running-indicator-webhook.md`](assistant-ui/running-indicator-webhook.md) | Running-indicator for webhook-triggered assistant sessions | superseded | — | — |
| [`assistant-ui/simple-three-bubble.md`](assistant-ui/simple-three-bubble.md) | Simple Three-Bubble UI | superseded | — | — |
| [`automation/automation-authoring-ux.md`](automation/automation-authoring-ux.md) | Automation Authoring UX (schedule picker so no cron knowledge is needed, server schedule preview + timezone, write/pick the task in the form, next run + test run, create-from-playbook) | complete | 2026-10-11 | SPEC-007, SPEC-003 |
| [`automation/automation-dispatcher-blueprint.md`](automation/automation-dispatcher-blueprint.md) | Unified Automation Dispatcher & Agent IDE - Architectural Blueprint | superseded | — | — |
| [`automation/memory-and-heartbeat-simplification.md`](automation/memory-and-heartbeat-simplification.md) | Memory defaults + overrides (assistant/automation) and a simple per-workspace Heartbeat section | complete | 2026-10-04 | SPEC-004, SPEC-007, SPEC-003 |
| [`autonomy/reminders-design.md`](autonomy/reminders-design.md) | Reminders — design (v1) | draft | 2026-10-11 | SPEC-011 |
| [`autonomy/reminders-implementation.md`](autonomy/reminders-implementation.md) | Reminders Implementation Plan (built; SPEC-011 is the contract) | complete | 2026-10-11 | SPEC-011 |
| [`cross-cutting/agent-os-sandboxing.md`](cross-cutting/agent-os-sandboxing.md) | Agent OS Sandboxing (rev 2 — network-first, uid-first; one action pipeline with OS jail + egress proxy; Phases 0–4 implemented + post-review hardening) | complete | 2026-09-06 | SPEC-006, SPEC-009 |
| [`cross-cutting/assistant-automation-parity.md`](cross-cutting/assistant-automation-parity.md) | Plan: Assistant-Automation Parity + Frontend UX Improvements | complete | — | — |
| [`cross-cutting/assistant-cancel-endpoint.md`](cross-cutting/assistant-cancel-endpoint.md) | Add explicit /assistant/cancel endpoint + in-process agent registry | superseded | — | — |
| [`cross-cutting/assistant-conversation-package.md`](cross-cutting/assistant-conversation-package.md) | Assistant Conversation Package (deferred extraction; done instead as C5 of the run-pipeline plan, into `core/chat`) | superseded | 2026-10-23 | SPEC-001 |
| [`cross-cutting/cloud-provider-token-budgets.md`](cross-cutting/cloud-provider-token-budgets.md) | Cloud Provider Token Budgets + Provider Set Reduction | complete | — | — |
| [`cross-cutting/connector-inbound-webhook.md`](cross-cutting/connector-inbound-webhook.md) | Communication Connector Inbound Webhook | complete | 2026-06-28 | SPEC-009 |
| [`cross-cutting/connector-system.md`](cross-cutting/connector-system.md) | PLAN: Generic Outbound Communication Connector System — Phase 1 | superseded | — | — |
| [`cross-cutting/docs-and-agent-harness-restructure.md`](cross-cutting/docs-and-agent-harness-restructure.md) | Docs and Agent Harness Restructure (lifecycle-split docs, subsystem hubs, slim skills, vendor-neutral harness) | complete | 2026-10-26 | — |
| [`cross-cutting/frontend-redesign-retro.md`](cross-cutting/frontend-redesign-retro.md) | Frontend Redesign — Retro Theme System, Shell, Tree & Notifications (absorbs `automation-edit-form-reactivity` at Phase 5) — Phases 0–7 complete; SPEC-003 v2.0 | complete | 2026-09-30 | SPEC-003, SPEC-007 |
| [`cross-cutting/interactive-user-input.md`](cross-cutting/interactive-user-input.md) | Plan: Interactive User Input (ask_user Tool) | superseded | — | — |
| [`cross-cutting/llamacpp-grammar-constraint.md`](cross-cutting/llamacpp-grammar-constraint.md) | Plan: Provider-Agnostic Output Constraints for Tool Calls | complete | — | — |
| [`cross-cutting/per-model-reasoning-overrides-and-settings-layout.md`](cross-cutting/per-model-reasoning-overrides-and-settings-layout.md) | Per-Model Reasoning Overrides and Settings Layout Repair | complete | 2026-08-02 | SPEC-005 |
| [`cross-cutting/per-run-output-directories.md`](cross-cutting/per-run-output-directories.md) | Plan: Per-Run Output Directories | superseded | — | — |
| [`cross-cutting/persist-assistant-run-state-for-reload.md`](cross-cutting/persist-assistant-run-state-for-reload.md) | Persist assistant run state (errors/cancels/running) for reliable reload | complete | 2026-08-20 | SPEC-001, SPEC-003 |
| [`cross-cutting/plan-recordings.md`](cross-cutting/plan-recordings.md) | Plan: Recording Playback for Automations | not_implemented | — | — |
| [`cross-cutting/primary-model-warning-banner.md`](cross-cutting/primary-model-warning-banner.md) | Remove model auto-bootstrap; explicit primary/fallback selection + banners | complete | 2026-08-14 | SPEC-003, CONSTITUTION III.4 |
| [`cross-cutting/process-lifecycle-management.md`](cross-cutting/process-lifecycle-management.md) | Process Lifecycle Management — Orphan Cleanup & Admin Process View | complete | — | — |
| [`cross-cutting/provider-agnostic-reasoning-enable.md`](cross-cutting/provider-agnostic-reasoning-enable.md) | Provider-Agnostic Reasoning Enable | proposed | — | — |
| [`cross-cutting/reasoning-capture-dynamic.md`](cross-cutting/reasoning-capture-dynamic.md) | Dynamic, provider-agnostic reasoning enable + capture + neutral indicator (typed, SOLID) | complete | 2026-07-28 | SPEC-003, SPEC-005 |
| [`cross-cutting/record-replay-test-framework.md`](cross-cutting/record-replay-test-framework.md) | Plan: Record-and-Replay LLM Testing Framework | complete | — | — |
| [`cross-cutting/run-recordings.md`](cross-cutting/run-recordings.md) | Make run recordings readable and safe (run logging already records every request; masked on write, listed without `--record`, content endpoint) | complete | 2026-10-20 | SPEC-001, SPEC-007 |
| [`cross-cutting/sandbox-runtime-invisibility.md`](cross-cutting/sandbox-runtime-invisibility.md) | Sandbox Runtime Invisibility (`.sandbox` hidden from filesystem listings + terminal output) | complete | 2026-08-25 | SPEC-006, CONSTITUTION II.3 |
| [`cross-cutting/session-source-backend-driven.md`](cross-cutting/session-source-backend-driven.md) | Session `source` derived from backend (single source of truth) | complete | 2026-07-09 | SPEC-009 |
| [`cross-cutting/system-blueprint.md`](cross-cutting/system-blueprint.md) | System Blueprint: LLM-Proxy | reference | — | — |
| [`cross-cutting/terminal-ui.md`](cross-cutting/terminal-ui.md) | Plan: Terminal UI (TUI) for LLM-Proxy | superseded | — | — |
| [`cross-cutting/tool-error-classification.md`](cross-cutting/tool-error-classification.md) | Tool Error Classification & Run-Fatality Policy (terminal tool errors, delivery-vs-essential, failure bound) | complete | 2026-09-12 | SPEC-001, SPEC-010, SPEC-006 |
| [`cross-cutting/unattended-run-safety-hardening.md`](cross-cutting/unattended-run-safety-hardening.md) | Unattended Run Safety Hardening (13 gaps, 7 leaks, 5 optimizations) | complete | 2026-07-22 | SPEC-001, SPEC-006, SPEC-007 |
| [`cross-cutting/unified-provider-management.md`](cross-cutting/unified-provider-management.md) | Unified Provider & Model Management | not_implemented | — | — |
| [`cross-cutting/universal-agent-completion.md`](cross-cutting/universal-agent-completion.md) | Universal Agent Completion Model | complete | — | — |
| [`cross-cutting/webhook-fresh-sessions.md`](cross-cutting/webhook-fresh-sessions.md) | Fresh Webhook Sessions + Source Grouping | complete | 2026-07-09 | SPEC-009 |
| [`cross-cutting/xdg-config-data-relocation.md`](cross-cutting/xdg-config-data-relocation.md) | XDG Config/Data Relocation + Storage Cleanup + Reset Controls (Phases 0–7, 9–12 complete; Phase 8 removed; reset/clear-runtime-data hardened) | complete | 2026-08-07 | CONSTITUTION III.2/III.4/III.6 |
| [`discovery/discovery-panel-implementation.md`](discovery/discovery-panel-implementation.md) | PLAN-001: Discovery Panel Implementation | not_implemented | — | — |
| [`memory/agent-memory-system.md`](memory/agent-memory-system.md) | Agent Memory System — Implementation Plan | complete | — | — |
| [`memory/mbtcp-implementation.md`](memory/mbtcp-implementation.md) | MBTCP: Memory-Backed Tool Call Pre-emption | superseded | — | — |
| [`memory/memory-dedup-search-fix.md`](memory/memory-dedup-search-fix.md) | Memory: Relevance Search + Jaccard Dedup | complete | — | — |
| [`memory/memory-tags-system.md`](memory/memory-tags-system.md) | Plan: Memory Tags System | superseded | — | — |
| [`memory/memory-three-tier-redesign.md`](memory/memory-three-tier-redesign.md) | Plan: Three-Tier Memory Architecture | superseded | — | — |
| [`memory/small-context-memory.md`](memory/small-context-memory.md) | Memory for Small-Context Local Models (per-run injection, window-scaled budget, progress ledger, step-aware hints, operator UX) | complete | 2026-10-01 | SPEC-001, SPEC-004, SPEC-005, SPEC-007 |
| [`orchestrator/consistent-token-budget.md`](orchestrator/consistent-token-budget.md) | Consistent Token Budget Computation | complete | — | — |
| [`orchestrator/resource-aware-orchestration.md`](orchestrator/resource-aware-orchestration.md) | Resource-Aware Orchestration Layer: Detailed Implementation Plan | complete | — | — |

## Consolidation notes (history)

> **2026-09-05 hygiene pass:** archived as complete — `knight-rider-arc-bubble.md` (extraction to `ArcOrbitLoader` verified in code), `cloud-provider-token-budgets.md` (all phases incl. merged Phase 7 + M8 probe verified). Archived as merged — `cancel-stale-turn-bleed.md` (backend SSE bleed now homed in overhaul Phase 5). `xdg-config-data-relocation.md` removed from this table (already complete, row was stale).
>
> **2026-09-25 consolidation:** the completed `cross-cutting/global-run-lane-scheduler.md` and `cross-cutting/inbound-request-admission.md` were folded into **SPEC-007 §V/§V.1** (run scheduler + model-residency contract) plus `docs/architecture.md` (package layout, pitfalls #34–#36) and the `automation` skill, then removed from `docs/PLANS/` — their design rationale remains in git history.
