---
status: partial
date: 2026-11-19
last_reviewed: 2026-11-19
related_specs: [SPEC-008, SPEC-006, SPEC-003]
constitution_references: [I.1, I.2, I.5, II.2, IV.6]
evidence: Built 2026-11-19 on branch stack-next-mcp-and-web-search from origin/main a9974f5a (v0.112.1). Checks run - go build, go vet, go test ./..., check-complexity, each stack GOWORK=off go test -race, npm test, npm run build, Playwright agents-tools and stacks-modules specs. Not run - full npm run test:visual after the last edits, anything on vertex, any real MCP server.
---

# MCP in the Agents stack: handoff

**Status:** partial (2026-11-19). Plan: [`mcp-in-agents-stack.md`](mcp-in-agents-stack.md); decision: ADR-0069 (proposed).

## Branch and state
Branch `stack-next-mcp-and-web-search`, one commit, pushed, PR open. Merge it on its own; it does not depend on another branch.

## Built (phases 1 to 5)
SDK (`DynamicTool`, `ToolSource`, `HTTP` port, `CleanText`); `stacks/agents/mcpclient` (Streamable HTTP, SSE fallback, pool, sealed auth); the Tools layer and module of the Agents stack with six routes; core adapter `toolregistry.StackTools`, the guardrail Ask rule, `stackport.HTTP`, one shared vault, a one-time copy of registry servers; `core/mcp` and `core/nodeherder` deleted; Agents → Tools tab and the redirect from /settings/mcp.

## Left, in order
1. `npm test` and the full `npm run test:visual` (only the two new/changed specs were run).
2. Live check: run against the nodeherder server's POST `/api/mcp` and one third-party server; confirm Ask prompts, Allow, and the module switch on vertex (`scripts/verify-deployment.py`).
3. Phase 6 docs: rewrite SPEC-008 as v2.0 (v1.1 describes files that do not exist); SPEC-003 note; ADR-0069 add the HTTP port, same-host redirects, no environment proxy, and that MCP does not go through the egress proxy; `docs/architecture/map.md`; a learnings line (`learnings/orchestrator.md`).
4. UI polish: shorter trust label ("When agents use its tools"), chevron on the Advanced summary, tighter card height, shorter mobile list.
5. Owner: accept or change ADR-0069; decide on OAuth sign-in and stdio servers (out of scope here); internet search joins the Tools layer in its own change (the system-settings `Search` block moves: ask first).

## Traps found
- A step-up route cannot have a wildcard in its path (the core tells them by exact path): server names travel in the body or the query.
- A stack may not build an `http.Transport` (the architecture test scans the SDK too): that is why the `httpclient` port exists.
- `models.AgentsConfig.MCPServers` must mirror the stack's entry field for field, or a save drops the field; `TestMCPTools_AServerAddedThroughTheRouteReachesTheRunPath...` pins it.
- The guardrail refusal text must not contain "security violation" or "path access denied" (the agent reads those as a hard boundary).
- On a phone the fixed tab bar covers controls scrolled to the bottom: centre them before a tap-area check.
- Unattended runs still see Ask-server tools in their schema and get them refused; hiding them needs the run's channel in the context.

## First file to open
`stacks/agents/mcpclient/conn.go`, then `backend/internal/core/toolregistry/dynamic.go`. First command: `git status -sb && ./scripts/check-agent-harness.sh`.
