---
id: SPEC-008
title: MCP Integration
version: "1.1"
status: stable
last_updated: 2026-09-29
constitution_references: []
related_specs: [SPEC-001, SPEC-006]
supersedes:
---

# SPEC: MCP Integration

## Changelog

- **1.1 (2026-09-29)** — Reference correction (no behavior change): the CompositeEngine fallback
  sentinel is `ErrToolNotInternal` (not `ErrToolNotFound`); the server-lifecycle type is
  `mcp.Orchestrator` (formerly `ClientPool`).

## I. Intent

The MCP (Model Context Protocol) integration allows the agent to consume tools provided by
external MCP servers via SSE transport. MCP servers are managed alongside local tools through
the `MultiToolProvider`, making them transparent to the agent loop.

## II. Functional Requirements

### 1. MCP Client

- SSE-based transport using `mark3labs/mcp-go`.
- Connects to MCP servers configured in `registry.json` under `mcp_servers`.
- Each server provides a list of tools via the MCP `tools/list` endpoint.
- Tool calls to MCP servers are forwarded via the MCP `tools/call` endpoint.

### 2. Tool Mirroring

- MCP tools are exposed through `NodeHerder` → `MCPService` interface.
- `MCPNodeHerder` implements `ToolProvider`, wrapping MCP tools in the standard `proxy.Tool` format.
- `MultiToolProvider` aggregates local tools (`LocalToolRegistry`) and MCP tools (`MCPNodeHerder`).

### 3. Composite Engine

- `CompositeEngine` tries local tool execution first, falls back to MCP when the local engine returns `ErrToolNotInternal`.
- This ensures local tools take priority over MCP tools with the same name.
- MCP-specific errors (connection, timeout) are mapped to standard tool errors.

### 4. Resource Mirroring

- `resource_mirror.go` caches system prompts from MCP servers.
- Cached prompts are injected into the agent's system message on each turn.
- Prompts are refreshed on reconnection.

### 5. Guardrails on MCP Tools

- MCP tool calls go through the same `ValidateToolCall()` flow as local tools.
- Guardrail rules in `settings.yml` and workspace `config.yaml` apply.
- MCP tool manifests use default guardrails (no embedded manifest override).

## III. Error Handling

- MCP connection failure: MCP tools disappear from the tool list (graceful degradation).
- MCP tool call timeout: the MCP engine returns the timeout error to the caller — there is no
  MCP→local fallback; the CompositeEngine only falls back local→MCP when the local engine returns
  `ErrToolNotInternal`.
- MCP SSE reconnection: automatic with exponential backoff (max 30s).

## IV. Configuration

MCP servers are configured in `registry.json`:

```json
{
  "mcp_servers": [
    {
      "name": "my-mcp-server",
      "url": "http://localhost:8080/sse",
      "enabled": true
    }
  ]
}
```

- Server lifecycle managed via `mcp.Orchestrator` (formerly `ClientPool`; start/stop/reconnect).
- Per-server enable/disable toggles tool availability.
