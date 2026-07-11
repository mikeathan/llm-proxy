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

## Active Implementation Plans

| File | Title | Status | Date | Related Specs |
|------|-------|--------|------|---------------|
| `docs/PLANS/agent-loop/agent-improvements.md` | Agent Improvements (7-phase) | partial | — | SPEC-001 |
| `docs/PLANS/agent-loop/enhanced-agent-flow-and-compatibility.md` | Enhanced Agent Flow & Compatibility | — | — | SPEC-001 |
| `docs/PLANS/agent-loop/ephemeral-turn-context.md` | Ephemeral Turn Context | — | — | SPEC-001 |
| `docs/PLANS/assistant-ui/cancel-stale-turn-bleed.md` | Cancel Stale Turn Bleed | — | — | SPEC-003 |
| `docs/PLANS/assistant-ui/knight-rider-arc-bubble.md` | Knight Rider Arc Bubble | — | — | SPEC-003 |
| `docs/PLANS/assistant-ui/overhaul-chat-history-layout.md` | Assistant UI Overhaul | active | 2026-06-25 | SPEC-003 |
| `docs/PLANS/assistant-ui/simple-three-bubble.md` | Simple Three Bubble | — | — | SPEC-003 |
| `docs/PLANS/cross-cutting/connector-inbound-webhook.md` | Communication Connector Inbound Webhook | complete | 2026-06-28 | SPEC-009 |
| `docs/PLANS/cross-cutting/connector-auto-reply.md` | Communication Connector Auto-Reply & Automation Trigger | active | 2026-06-28 | SPEC-009 |
| `docs/PLANS/cross-cutting/webhook-fresh-sessions.md` | Fresh Webhook Sessions + Source Grouping | complete | 2026-07-09 | SPEC-009 |
| `docs/PLANS/cross-cutting/session-source-backend-driven.md` | Session `source` derived from backend (single source of truth) | complete | 2026-07-09 | SPEC-009 |
| `docs/PLANS/cross-cutting/interactive-user-input.md` | Interactive User Input (ask_user Tool) | proposed | 2026-06-08 | SPEC-001, SPEC-007 |
| `docs/PLANS/cross-cutting/per-run-output-directories.md` | Per-Run Output Directories | draft | 2026-06-05 | — |
| `docs/PLANS/cross-cutting/terminal-ui.md` | Terminal UI (Bubble Tea TUI client) | proposed | 2026-06-27 | SPEC-001, SPEC-003, SPEC-004, SPEC-006, SPEC-007, SPEC-008 |
| `docs/PLANS/memory/mbtcp-implementation.md` | MBTCP: Memory-Backed Tool Call Pre-emption | proposed | 2026-06-03 | SPEC-004 |
| `docs/PLANS/memory/memory-improvements-implementation-plan.md` | Memory Improvements | partial | — | SPEC-004 |

## Archived Plans

Completed, superseded, and not-implemented plans are stored in `docs/PLANS/ARCHIVE/` — loaded only when their specific topic is relevant.

## Audits

| File | Title | Status | Date |
|------|-------|--------|------|
| `docs/audits/agent-stability-report.md` | Agent Stability Audit (13 issues) | complete | 2026-05-28 |
| `docs/audits/ephemeral-turn-context-failed-run.md` | Ephemeral Turn Context — Failed Run Analysis | complete | 2026-06-08 |
| `docs/audits/memory-injection-investigation.md` | Memory Injection + Automation Limitations | reference | 2026-06-03 |
| `docs/audits/remove-memory-rewriter.md` | Remove Memory Rewriter + FTS5 Fix | complete | 2026-06-03 |
| `docs/PLANS/codebase-audit-report.md` | Codebase Audit Report (88 findings) | active | 2026-07-03 |

## Agent Rules (AI Assistant Guidance)

| File | Title | Lines |
|------|-------|-------|
| `.agents/rules/go-staff-engineer.md` | Go Coding Rules for AI | 47 |
| `.agents/rules/frontend-vue-engineer.md` | Vue Coding Rules for AI | 47 |

## Top-Level Guides

| File | Title | Lines | Audience |
|------|-------|-------|----------|
| `README.md` | LLM Proxy — Quick Start & Config | 288 | End users |
| `AGENTS.md` | Instructions for AI Coding Assistants | 78 | AI assistants |
| `docs/architecture.md` | Architecture Reference (mappings, contracts, checklists, pitfalls) | 270 | Developers |
| `CONSTITUTION.md` | Architectural Invariants — The Law | 115 | Everyone |

## Other Documents

| File | Title | Notes |
|------|-------|-------|
| `docs/services/llm-proxy.service` | systemd service unit | Operational |
| `docs/service_setup` | Service installation instructions | Setup |
| `docs/SPEC-change-management.md` | SPEC Lifecycle & Change Management | Reference |
| `docs/SPECS/README.md` | Subdirectory catalog for all SPEC files | Index |
| `docs/audits/README.md` | Subdirectory catalog for audit files | Index |
| `docs/PLANS/ARCHIVE/` | Completed/superseded/not-implemented plans | Archive |
| `docs/skills/` | AI assistant skill files — deep-dive reference guides | Categories |

---

## Directory Navigation

| Directory | Contents |
|-----------|----------|
| `docs/SPECS/` | Behavioral contracts (SPEC-NNN). Read before modifying subsystems. |
| `docs/PLANS/` | Active implementation strategies. Organized by subsystem. |
| `docs/PLANS/ARCHIVE/` | Completed and superseded plans — load on demand. |
| `docs/audits/` | Post-hoc analysis of system behavior against specs. |
| `docs/skills/` | AI assistant reference guides — loaded on demand for deep-dive topics. |
| `.agents/rules/` | Per-language coding rules for AI assistants. |
