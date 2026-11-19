---
subsystem: mcp
specs: [SPEC-008]
---

# MCP integration

Model Context Protocol servers as tool sources.

Start here, then open only what the task needs.

| | |
|---|---|
| **Contract (current behavior)** | [SPEC-008](../SPECS/mcp-integration.md) |
| **Code** | `stacks/agents/mcpclient/`, `backend/internal/core/toolregistry/dynamic.go` |
| **Architecture** | [map](../architecture/map.md) · [contracts](../architecture/contracts.md) · [checklists](../architecture/checklists.md) · [pitfalls](../architecture/pitfalls.md) — search pitfalls for: MCP |
| **Decisions (why)** | [ADR-0069](../decisions/ADR-0069-mcp-is-a-general-client-in-the-agents-stack-with-dynamic-tools-and-per-server-trust.md) MCP is a general client in the Agents stack (proposed) |
| **Open plans (next)** | [`cross-cutting/mcp-in-agents-stack-handoff-2026-11-19.md`](../PLANS/cross-cutting/mcp-in-agents-stack-handoff-2026-11-19.md) (partial: start here)<br>[`cross-cutting/mcp-in-agents-stack.md`](../PLANS/cross-cutting/mcp-in-agents-stack.md) (active 2026-11-19: the rewrite into `stacks/agents`) |
| **Learnings (tried before)** | [`learnings/mcp.md`](../learnings/mcp.md) — read before proposing an approach |
| **Evidence (audits, evals)** | _none yet_ |
| **Skills** | — |
| **Guides** | — |
| **Archived plans** | others by topic in [`ARCHIVE/README.md`](../PLANS/ARCHIVE/README.md) |
