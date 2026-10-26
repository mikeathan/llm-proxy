# Subsystems

One hub page per subsystem. **Classify the task, open its hub, then load only what you need** — the hub links
the contract (SPEC), the code, the architecture notes, the decisions (ADRs), the open plans, what was tried
before (learnings), the evidence, and the skills.

Hubs are indexes, not content: keep facts in the SPEC, ADR, plan or ledger they belong to and link them here.
When you add a SPEC, plan, ADR, ledger line or audit for a subsystem, check its hub still points at it
(the harness checks every link resolves and every open plan is listed in a hub).

| Subsystem | SPECs | Open plans |
|-----------|-------|-----------:|
| [Agent loop, tool calling and the run pipeline](agent-loop.md) | SPEC-001, SPEC-002, SPEC-010 | 6 |
| [Admin UI and chat rendering](admin-ui.md) | SPEC-003 | 3 |
| [Memory](memory.md) | SPEC-004 | 4 |
| [Inference, budgets, providers and performance](orchestrator.md) | SPEC-005 | 4 |
| [Guardrails, sandbox and egress](guardrails.md) | SPEC-006 | 4 |
| [Automation and heartbeat](automation.md) | SPEC-007 | 4 |
| [MCP integration](mcp.md) | SPEC-008 | 0 |
| [Communication connectors](communication.md) | SPEC-009 | 2 |
| [Autonomy and reminders](autonomy.md) | SPEC-011 | 7 |
| [Prompt profiles and prompt text](prompt-profiles.md) | SPEC-012 | 2 |
| [Network audit](network-audit.md) | — | 1 |
| [Tool catalog and registry](tool-registry.md) | — | 1 |
| [Storage, config, events, logging and backend performance](platform.md) | — | 4 |
| [Docs and the coding-agent harness](agent-harness.md) | — | 0 |
