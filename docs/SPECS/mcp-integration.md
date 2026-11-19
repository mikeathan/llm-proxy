---
id: SPEC-008
title: MCP Integration
version: "2.0"
status: stable
last_updated: 2026-11-19
constitution_references: []
related_specs: [SPEC-001, SPEC-006]
supersedes:
---

# SPEC: MCP Integration

## Changelog

- **2.0 (2026-11-19)** — Rewritten for ADR-0069: MCP is a general client in the Agents stack (Tools layer). v1.1 described `resource_mirror.go`, SSE only, `registry.json` servers and `NodeHerder`; none of those exist now.

## I. Intent

Agents may use the tools of MCP servers the owner lists, over the MCP specification's Streamable HTTP transport, with nothing contacted until a server is added and switched on.

## II. Functional Requirements

1. **Transport.** Streamable HTTP; a server that answers the first POST with 400, 404 or 405 is tried once as the old HTTP+SSE transport. Not supported: stdio, OAuth sign-in, resources, prompts, sampling, elicitation, roots (client declares no capability for them).
2. **Sign-in.** None, a bearer token, or one named header. The value is sealed by the vault, never returned, logged or put in a prompt; a stored value is never sent to a different address (the edit is refused until it is typed again).
3. **Network.** Every connection goes through the guard (`httpclient` port): no environment proxy, same-host redirects only, optional per-server CA file.
4. **Tools.** Listed with pagination, cached, refreshed on `tools/list_changed` and every five minutes; named `mcp__<server>__<tool>` (at most 64 characters). The core cuts descriptions (1,024 bytes) and schemas (16 KiB), allows 128 tools, and orders arguments required-first. Listing never waits on a server and is empty while the module is off.
5. **Calls.** 60 s deadline; a session the server ended is started again and the call retried once; results are text (images and audio as a sentence with their size), cut at 256 KiB; a tool error is the tool's own words.
6. **Trust.** Each server is Ask (default: every call is an approvable refusal, exact call, this workspace; unattended runs cannot use it) or Allow. A name no server offers any more but that looks like one asks. Lock down refuses every such tool.
7. **Module.** Agents stack, Tools layer, module `mcp_servers` (off flag `mcp_off`, step-up). Routes: `GET/POST/PUT/DELETE /admin/api/mcp/servers`, `POST /admin/api/mcp/test`; writes need the passphrase.
8. **Status.** Per server: connecting, ready, stopped (a problem id with fixed words), off.

## III. Where

`stacks/agents/mcpclient/`, `stacks/agents/mcp.go`, `backend/internal/core/toolregistry/dynamic.go`, `backend/internal/core/assistant/guardrails/` (`ErrAsksFirst`). Decisions: ADR-0069. Settings: the `agents` document (`mcp_servers`, `mcp_off`).
