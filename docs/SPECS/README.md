# Specifications

Specifications define **behavioral contracts** — what a subsystem must do, not how to implement it.
This is the canonical SPEC catalog (there is no duplicate table in `docs/INDEX.md`).

Each spec has a stable ID (`SPEC-NNN`), version number, and status. Read the relevant spec before
modifying or extending the subsystem it describes. Lifecycle and change rules:
[`docs/SPEC-change-management.md`](../SPEC-change-management.md).

| ID | File | Title | Status | Constitution Refs |
|----|------|-------|--------|-------------------|
| SPEC-001 | `agent-loop.md` | Agent Loop | stable | II.4, II.5, II.6, II.7, II.8, II.10 |
| SPEC-002 | `tool-call-parser.md` | Tool Call Parser | stable | II.4 |
| SPEC-003 | `discovery-panel.md` | Admin UI (v2.5; formerly Discovery Panel) | stable | — |
| SPEC-004 | `memory.md` | Memory System | stable | II.12 |
| SPEC-005 | `orchestrator.md` | Orchestrator / Budget | stable | VI |
| SPEC-006 | `guardrails.md` | Guardrail Engine | stable | II.3 |
| SPEC-007 | `automation-dispatcher.md` | Automation Dispatcher | stable | — |
| SPEC-008 | `mcp-integration.md` | MCP Integration | stable | — |
| SPEC-009 | `communication.md` | Communication Connector System | stable | II.4, II.5, V |
| SPEC-010 | `agent-loop-strategies.md` | Agent Loop Strategies | stable | II.4, II.5, II.6, II.7, II.10, II.13 |

**Adding a SPEC:** take the next unused ID, write the sections (Intent, Functional Requirements, Data
Model, Behavior, Error Handling), add a row above, and follow `docs/SPEC-change-management.md`.
