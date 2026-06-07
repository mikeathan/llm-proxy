# Documentation Index

This index catalogs every documentation file in the repository. Each entry includes a stable ID,
status, and cross-references to related documents. Use this as the starting point for navigation.

**Status Legend:** `stable` | `draft` | `superseded` | `reference` | `complete` | `partial` | `not_implemented`

---

## Constitution (Immutable Law)

| ID | File | Title | Status | Sections |
|----|------|-------|--------|----------|
| LAW | `CONSTITUTION.md` | Architectural Invariants | Law | I–VI (15 subsections) |

## Specifications (Behavioral Contracts)

| ID | File | Title | Status | Constitution Refs |
|----|------|-------|--------|-------------------|
| SPEC-001 | `docs/SPECS/agent-loop.md` | Agent Loop | stable | II.4, II.5, II.6, II.7, II.8, II.10 |
| SPEC-002 | `docs/SPECS/tool-call-parser.md` | Tool Call Parser | stable | II.4 |
| SPEC-003 | `docs/SPECS/discovery-panel.md` | Discovery Panel UI | stable | — |
| SPEC-004 | — | Memory System *(not yet written)* | — | II.12 |
| SPEC-005 | — | Orchestrator / Budget *(not yet written)* | — | VI |
| SPEC-006 | — | Guardrail Engine *(not yet written)* | — | I.5 |
| SPEC-007 | — | Automation Dispatcher *(not yet written)* | — | — |
| SPEC-008 | — | MCP Integration *(not yet written)* | — | — |

## Implementation Plans

| File | Title | Status | Date | Related Specs |
|------|-------|--------|------|---------------|
| `docs/PLANS/agnostic-agent-loop.md` | Universal LLM-Agnostic Agent Loop | complete | 2026-05-09 | SPEC-001 |
| `docs/PLANS/agent-memory-system.md` | Agent Memory System | complete | — | SPEC-004 |
| `docs/PLANS/agent-improvements.md` | Agent Improvements (7-phase) | partial | — | SPEC-001 |
| `docs/PLANS/resource-aware-orchestration.md` | Resource-Aware Orchestration | complete | — | SPEC-005 |
| `docs/PLANS/consistent-token-budget.md` | Consistent Token Budget | complete | — | SPEC-005 |
| `docs/PLANS/auto-context-budget.md` | Auto-Compute Context Budget | complete | — | SPEC-001 |
| `docs/PLANS/tool-choice-temperature-implementation.md` | Tool Choice & Temperature | complete | — | SPEC-001 |
| `docs/PLANS/process-lifecycle-management.md` | Process Lifecycle Management | complete | — | — |
| `docs/PLANS/refactor-assistant-clean-code.md` | Refactor Assistant Package | complete | — | SPEC-001 |
| `docs/PLANS/record-replay-test-framework.md` | Record-and-Replay LLM Testing | complete | 2026-05-24 | — |
| `docs/PLANS/fallback-native-tools-fix.md` | Reasoning-Stuck Fallback Fix | complete | — | SPEC-001 |
| `docs/PLANS/memory-dedup-search-fix.md` | Relevance Search + Jaccard Dedup | complete | — | SPEC-004 |
| `docs/PLANS/memory-improvements-implementation-plan.md` | Memory Improvements | partial | — | SPEC-004 |
| `docs/PLANS/mbtcp-implementation.md` | MBTCP: Memory-Backed Tool Call Pre-emption | proposed | 2026-06-03 | SPEC-004 |
| `docs/PLANS/unified-provider-management.md` | Unified Provider & Model Management | not_implemented | — | — |
| `docs/PLANS/discovery-panel-implementation.md` | Discovery Panel Implementation | not_implemented | — | SPEC-003 |
| `docs/PLANS/autodetect-native-format.md` | Auto-Detect Native Tool Format | not_implemented | — | SPEC-001 |
| `docs/PLANS/plan-recordings.md` | Recording Playback for Automations | not_implemented | — | — |
| `docs/PLANS/enhanced-agent-flow-and-compatibility.md` | Enhanced Agent Flow & Model Compatibility | partial | — | SPEC-001 |
| `docs/PLANS/automation-dispatcher-blueprint.md` | Unified Automation Dispatcher — Blueprint | superseded | 2026-04-01 | SPEC-007 |
| `docs/PLANS/system-blueprint.md` | System Blueprint: LLM-Proxy | reference | — | all |

## Audits

| File | Title | Status | Date |
|------|-------|--------|------|
| `docs/audits/agent-stability-report.md` | Agent Stability Audit (13 issues) | complete | 2026-05-28 |
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
| `docs/samples/` | JSON samples (device_context, llm_response, metrics_query) | Examples |

---

## Directory Navigation

| Directory | Contents |
|-----------|----------|
| `docs/SPECS/` | Behavioral contracts (SPEC-NNN). Read before modifying subsystems. |
| `docs/PLANS/` | Implementation strategies. Status reflects completeness. |
| `docs/audits/` | Post-hoc analysis of system behavior against specs. |
| `docs/samples/` | Example JSON payloads for API responses and tool calls. |
| `.agents/rules/` | Per-language coding rules for AI assistants. |
