# Documentation Index

This index catalogs every documentation file in the repository. Each entry includes a stable ID,
status, and cross-references to related documents. Use this as the starting point for navigation.

**Status Legend:** `stable` | `draft` | `superseded` | `reference` | `complete` | `partial` | `not_implemented`

---

## Constitution (Immutable Law)

| ID | File | Title | Status | Sections |
|----|------|-------|--------|----------|
| LAW | `CONSTITUTION.md` | Architectural Invariants | Law | I–VI (15 subsections) |

## Skills (Reference Guides)

| | File | Title | Topics |
|-|------|-------|--------|
| | `docs/skills/memory-system.md` | Memory Architecture & Tags | Injection, three-tier, tags, dedup, gotchas |
| | `docs/skills/agent-loop.md` | Agent Loop & Stuck Detection | Sieve, fallback chain, reasoning budget, spiral detector |
| | `docs/skills/testing-guide.md` | Testing Guide | Smoke tests, record-replay, run analysis, templates |
| | `docs/skills/engineering-practices.md` | Go Engineering Practices | Patterns, code style, architecture rules |
| | `docs/skills/llamacpp-setup.md` | llama.cpp Server Setup | Args, GPU tuning, systemd, performance data |
| | `docs/skills/automation.md` | Automation System | Dispatcher, executor, run lifecycle, templates |

## Specifications (Behavioral Contracts)

| ID | File | Title | Status | Constitution Refs |

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

## Implementation Plans

| File | Title | Status | Date | Related Specs |
|------|-------|--------|------|---------------|
| `docs/PLANS/agent-loop/agnostic-agent-loop.md` | Universal LLM-Agnostic Agent Loop | complete | 2026-05-09 | SPEC-001 |
| `docs/PLANS/memory/agent-memory-system.md` | Agent Memory System | complete | — | SPEC-004 |
| `docs/PLANS/agent-loop/agent-improvements.md` | Agent Improvements (7-phase) | partial | — | SPEC-001 |
| `docs/PLANS/cross-cutting/interactive-user-input.md` | Interactive User Input (ask_user Tool) | proposed | 2026-06-08 | SPEC-001, SPEC-007 |
| `docs/PLANS/cross-cutting/llamacpp-grammar-constraint.md` | GBNF Grammar Constraints for Tool Calls | proposed | 2026-06-09 | SPEC-001, SPEC-003 |
| `docs/PLANS/memory/memory-three-tier-redesign.md` | Three-Tier Memory Architecture (scope, mode, keep) | proposed | 2026-06-09 | SPEC-004 |
| `docs/PLANS/memory/memory-tags-system.md` | Memory Tags System | complete | 2026-06-08 | SPEC-004 |
| `backend/data/templates/memory-cascade-test.md` | Memory Cascade — Persona Recall & Cross-Ref | reference | 2026-06-09 | SPEC-004 |
| `backend/data/templates/memory-tags-test.md` | Memory Tags & Type Isolation Test | reference | 2026-06-09 | SPEC-004 |
| `docs/PLANS/orchestrator/resource-aware-orchestration.md` | Resource-Aware Orchestration | complete | — | SPEC-005 |
| `docs/PLANS/orchestrator/consistent-token-budget.md` | Consistent Token Budget | complete | — | SPEC-005 |
| `docs/PLANS/agent-loop/auto-context-budget.md` | Auto-Compute Context Budget | complete | — | SPEC-001 |
| `docs/PLANS/agent-loop/tool-choice-temperature-implementation.md` | Tool Choice & Temperature | complete | — | SPEC-001 |
| `docs/PLANS/cross-cutting/process-lifecycle-management.md` | Process Lifecycle Management | complete | — | — |
| `docs/PLANS/agent-loop/refactor-assistant-clean-code.md` | Refactor Assistant Package | complete | — | SPEC-001 |
| `docs/PLANS/cross-cutting/record-replay-test-framework.md` | Record-and-Replay LLM Testing | complete | 2026-05-24 | — |
| `docs/PLANS/agent-loop/fallback-native-tools-fix.md` | Reasoning-Stuck Fallback Fix | complete | — | SPEC-001 |
| `docs/PLANS/memory/memory-dedup-search-fix.md` | Relevance Search + Jaccard Dedup | complete | — | SPEC-004 |
| `docs/PLANS/memory/memory-improvements-implementation-plan.md` | Memory Improvements | partial | — | SPEC-004 |
| `docs/PLANS/memory/mbtcp-implementation.md` | MBTCP: Memory-Backed Tool Call Pre-emption | proposed | 2026-06-03 | SPEC-004 |
| `docs/PLANS/cross-cutting/unified-provider-management.md` | Unified Provider & Model Management | not_implemented | — | — |
| `docs/PLANS/discovery/discovery-panel-implementation.md` | Discovery Panel Implementation | not_implemented | — | SPEC-003 |
| `docs/PLANS/agent-loop/autodetect-native-format.md` | Auto-Detect Native Tool Format | not_implemented | — | SPEC-001 |
| `docs/PLANS/agent-loop/tool-failure-skip.md` | Graceful Tool Failure Recovery | complete | 2026-05-31 | SPEC-001 |
| `docs/PLANS/cross-cutting/plan-recordings.md` | Recording Playback for Automations | not_implemented | — | — |
| `docs/PLANS/cross-cutting/per-run-output-directories.md` | Per-Run Output Directories | draft | 2026-06-05 | — |
| `docs/PLANS/cross-cutting/system-blueprint.md` | System Blueprint: LLM-Proxy | reference | — | all |

## Audits

| File | Title | Status | Date |
|------|-------|--------|------|
| `docs/audits/agent-stability-report.md` | Agent Stability Audit (13 issues) | complete | 2026-05-28 |
| `docs/audits/ephemeral-turn-context-failed-run.md` | Ephemeral Turn Context — Failed Run Analysis | complete | 2026-06-08 |
| `docs/audits/memory-injection-investigation.md` | Memory Injection + Automation Limitations | reference | 2026-06-03 |
| `docs/audits/remove-memory-rewriter.md` | Remove Memory Rewriter + FTS5 Fix | complete | 2026-06-03 |

## Agent Rules (AI Assistant Guidance)

| File | Title | Lines |
|------|-------|-------|
| `.agents/rules/go-staff-engineer.md` | Go Coding Rules for AI | 47 |
| `.agents/rules/frontend-vue-engineer.md` | Vue Coding Rules for AI | 47 |

## Top-Level Guides

| File | Title | Lines | Audience |
|------|-------|-------|----------|
| `README.md` | LLM Proxy — Quick Start & Config | 288 | End users |
| `CLAUDE.md` | Project Guide — Architecture, Invariants, API | 465 | Developers |
| `AGENTS.md` | Instructions for AI Coding Assistants | 321 | AI assistants |
| `CONSTITUTION.md` | Architectural Invariants — The Law | 143 | Everyone |

## Other Documents

| File | Title | Notes |
|------|-------|-------|
| `docs/services/llm-proxy.service` | systemd service unit | Operational |
| `docs/service_setup` | Service installation instructions | Setup |
| `docs/SPECS/README.md` | Subdirectory catalog for all SPEC files | Index |
| `docs/audits/README.md` | Subdirectory catalog for audit files | Index |
| `docs/skills/` | AI assistant skill files — deep-dive reference guides | Categories |

---

## Directory Navigation

| Directory | Contents |
|-----------|----------|
| `docs/SPECS/` | Behavioral contracts (SPEC-NNN). Read before modifying subsystems. |
| `docs/PLANS/` | Implementation strategies. Organized by subsystem: `agent-loop/`, `memory/`, `orchestrator/`, `automation/`, `discovery/`, `cross-cutting/`. |
| `docs/audits/` | Post-hoc analysis of system behavior against specs. |
| `docs/skills/` | AI assistant reference guides — loaded on demand for deep-dive topics. |
| `.agents/rules/` | Per-language coding rules for AI assistants. |
