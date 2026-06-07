---
id: SPEC-008
title: MCP Integration
version: "1.0"
status: stable
last_updated: 2026-05-28
constitution_references: []
related_specs: [SPEC-001, SPEC-006]
supersedes:
---

# SPEC: MCP Integration

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

- `CompositeEngine` tries local tool execution first, falls back to MCP on `ErrToolNotFound`.
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
- MCP tool call timeout: engine returns `ErrToolNotFound` to fall back to local.
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

- Server lifecycle managed via `MCPOrchestrator` (start/stop/reconnect).
- Per-server enable/disable toggles tool availability.
