---
id: ADR-0069
title: MCP is a general client in the Agents stack: tools are dynamic, every server has its own trust, and the old node-herder code is rewritten away
status: proposed
date: 2026-11-19
subsystems: [mcp, platform]
supersedes:
superseded_by:
---

# ADR-0069: MCP is a general client in the Agents stack: tools are dynamic, every server has its own trust, and the old node-herder code is rewritten away

Proposed: it is in force when the owner merges the pull request that carries it. Carries out [ADR-0032](ADR-0032-a-stack-is-a-package-with-a-manifest-kept-apart-by-tests.md)
for MCP, after Reminders ([ADR-0068](ADR-0068-reminders-are-a-stack-with-its-own-store-a-notify-port-and-a-schedule-port.md)), and extends the Agents stack
([ADR-0051](ADR-0051-the-agents-stack-takes-the-watch-and-the-core-keeps-the-walk-the-words-and-the-gate.md)) from watching agents to also giving them tools.

## Context
- `core/mcp` and `core/nodeherder` (1,411 lines) were written for one trusted LAN server over the deprecated SSE transport, with no authentication, a first-match
  tool lookup across servers, a blocking tool listing and a dead token manager for that one server's login. Measured in the plan
  [`mcp-in-agents-stack.md`](../PLANS/cross-cutting/mcp-in-agents-stack.md).
- The owner wants general MCP support (any server, the specification), not support for one server.
- MCP tools are listed by the server at run time. The manifest pattern of ADR-0068 (tools declared as data) cannot carry them.
- A tool outside the guardrail catalogue has no row; its side effects and its reach are unknown to the service.

## Decision
1. **MCP is the Tools layer of the Agents stack**, one module (MCP servers), in `stacks/agents`. Its consumers are only agents; the stack already watches their calls.
2. **A general client over Streamable HTTP**, with the specification's backward-compatible fallback to the old HTTP+SSE transport, bearer-token or one-header
   authentication (the value sealed with the vault port and never returned), the guarded dial of the `guard` port for every connection, and tools only. Out of scope:
   stdio (it starts a program; ADR-0033, Constitution II.3), OAuth sign-in, and resources, prompts, sampling, elicitation and roots.
3. **Dynamic tools through the SDK.** A stack that sets `Manifest.DynamicTools` implements `stacksdk.ToolSource` (list, call), as plain data. The core validates names
   (`mcp__<server>__<tool>`, at most 64 characters, no collision with a built-in or a declared tool), cuts and cleans server text, bounds schemas and counts, and answers
   a listing from a cache so the run path never waits on a server.
4. **Trust is per server: Ask or Allow, Ask by default.** Ask makes the guardrail engine refuse the call with an approvable refusal (the existing approval flow, exact
   call, this workspace); unattended runs, which cannot ask, therefore use only servers set to Allow. A server's own read-only annotation is not trusted. Lock down still
   refuses every tool outside the catalogue.
5. **A rewrite, with nothing kept for the old shape:** `core/mcp`, `core/nodeherder`, the old routes and Settings tab, `MCPServerConfig` and `RegistryData.MCPServers`
   are deleted; the old server list is copied once into the stack's settings and then emptied. "nodeherder" stays only as the name of the owner's separate app, which is
   now an ordinary server entry.
6. Internet search joins the same layer in a later, separate change.

## Consequences
- Easier: any server that speaks the specification works; a server that is down cannot hold a run; one switch (the module, or the stack) stops all outside tool servers;
  the nodeherder app needs no code of its own.
- Harder: the SDK gains one capability (`ToolSource`) that lets a stack add tools the core did not list at start; the core must keep every call behind the guardrail check.
- Behaviour changed on purpose: tool names are prefixed (`mcp__server__tool`); a server's tools ask first until the owner sets Allow; the Settings → MCP tab moves to
  Agents → Tools; a server name must be lower-case letters, digits and hyphens.
- Cost: one more goroutine set per enabled server (a reconnect loop and a ping), all under the stack's lifecycle context.
- Open risk: not yet run against a real third-party server; the plan's live check does that.

## Alternatives considered
- **A separate Integrations stack** — nothing besides agents uses MCP, and the Agents stack already owns the agent tool-call feed.
- **Keep the old code and add Streamable HTTP and headers to it** — the defects are structural (first-match routing, blocking listing, untethered lifecycle, the settings
  home); patching each would leave an interface named for one server.
- **A catalogue row per MCP tool, or trusting the server's annotations** — the service cannot know a server's tools in advance, and the annotations are the server's claim.
- **Declare MCP tools in a manifest** — they change at run time.
- **Server list stays in `registry.json`** — the stack reads only its own settings document (IV.6); a second owner of the list would be needed.

## Links
- Plan [`mcp-in-agents-stack.md`](../PLANS/cross-cutting/mcp-in-agents-stack.md); SPEC-008; `stacks/agents/`, `stacks/sdk/`, `backend/internal/core/toolregistry/`, `backend/internal/core/assistant/guardrails/`.
