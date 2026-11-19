---
status: active
date: 2026-11-19
last_reviewed: 2026-11-19
related_specs: [SPEC-008, SPEC-006, SPEC-003]
constitution_references: [I.1, I.2, I.5, II.2, IV.6]
evidence: Measured 2026-11-19 on branch stack-next-mcp-and-web-search (from origin/main a9974f5a, v0.112.1). Read - CONSTITUTION I, II.2, IV.6; ADR-0032/0033/0036/0067/0068; the Reminders plan; stacks/sdk (manifest.go, ports.go), stacks/agents, app/stacks.go, app/stackhost.go, app/mcp.go, core/mcp, core/nodeherder, core/toolregistry, core/assistant/guardrails (guardrails.go, tool_exec.go), models/tool_catalog.go, handlers/mcp_handlers.go, the Settings MCP tab, the Agents page; the nodeherder repo's MCP server (docs/runtime-reliability.md). Not run on vertex or in a real browser.
---

# MCP in the Agents stack: a general MCP client (Tools layer)

**Status:** active (2026-11-19). Owner decisions of 2026-11-19: MCP joins the **Agents** stack as a new **Tools** layer (name "Agents" kept); the work is a **rewrite**, not a move of the old code; follow the MCP specification so any server can connect; tools of a server **ask first by default** (Ask or Allow per server); internet search joins the same layer **later, in its own change**.
**Decisions:** [ADR-0069](../../decisions/ADR-0069-mcp-is-a-general-client-in-the-agents-stack-with-dynamic-tools-and-per-server-trust.md) (proposed). Contract: SPEC-008 v2.0 (rewritten in this change; v1.1 described files that do not exist).
**Parent plans:** [next-stack-moves-and-local-models.md](next-stack-moves-and-local-models.md) (section 4), [agents-stack-watch.md](agents-stack-watch.md).

## Why a rewrite

`core/nodeherder` and `core/mcp` (1,411 lines) were written in April 2026 for one trusted server on the LAN (the owner's separate nodeherder app) and have had no real use for months. Measured defects, not opinions:

| Finding | Evidence |
|---|---|
| Nothing in the service is specific to nodeherder except one dead file and a name | `core/nodeherder/token_manager.go` (a client-credentials call to nodeherder's `/api/auth/token`, no caller outside mocks, "for future use"); `MCPNodeHerder` is a generic adapter with a specific name; `MCPService` is generic |
| Old transport only | `client.NewSSEMCPClient` (`core/mcp/client.go:219`). Streamable HTTP (spec 2025-03-26 and later) is the current transport; SSE is the deprecated one |
| No authentication at all | `MCPServerConfig` is name, URL, enabled, CA path; no header, no token |
| A server's own tool names collide | `Orchestrator.CallTool` lists every server's tools on every call and takes the first name match; `deduplicateTools` keeps the first of two equal names |
| Blocking | `ListTools` calls each server in turn with the run's context; a hanging server holds the run |
| A sent `Origin` that is the service's own bind address | `client.go:169`, probably one server's workaround; no spec basis for a non-browser client |
| Add and update drop the CA path | `app_context_mcp.go`: `AddMCPServer` and `UpdateMCPServer` write only name, URL, enabled |
| Unattached lifecycle | `app/mcp.go` reloads with `context.Background()`; Constitution II.2 names the MCP client as a thing that may not run untethered |
| Resource prompts promised, never built | SPEC-008 §4 names `resource_mirror.go`; it does not exist; `ReadResource` has no caller |

## What is built

**Scope: the MCP client for tools, over Streamable HTTP, as the specification describes it.** Out of scope, said plainly: the stdio transport (it starts a program; Constitution II.3 forbids a raw `os/exec` for agent work and a stack gets no way around the sandbox, ADR-0033), OAuth 2.1 sign-in (an interactive browser flow; servers that take a bearer token or an API-key header work), and MCP resources, prompts, sampling, elicitation and roots (the agent loop uses tools only; the client declares no capability for the rest, so a server does not ask for them).

### Protocol (the client)
- **Streamable HTTP** with `mark3labs/mcp-go` (already a dependency): one endpoint, POST for every message, `Mcp-Session-Id` kept for the session, `MCP-Protocol-Version` sent, the server's JSON or SSE answer to a POST accepted.
- **Backward compatibility the specification asks for:** a server that answers the first POST with 400, 404 or 405 is tried as the old HTTP+SSE transport (once, remembered per server for the session). No setting.
- **Initialize** with the latest protocol version the library knows, accept the version the server answers if the library supports it, otherwise refuse with a plain sentence. Client capabilities: none. Server `instructions` are shown in the UI (cut and escaped) and are never put in a prompt.
- **Tools:** `tools/list` with pagination (cursor loop, bounded), refreshed on `notifications/tools/list_changed` and on reconnect; a cache that answers `ListTools` at once; `tools/call` with the run's context, a per-call deadline (60 s, a named constant) and cancellation on the run's end.
- **Session loss:** a 404 for a known session means it ended; the client re-initialises and retries the call once (specification: "the client MUST start a new session").
- **Results:** `content` blocks are read in order. Text is kept; `structuredContent` is preferred when the server sent it; an image or audio block becomes one fixed sentence naming its type and size (the model cannot use the bytes); an embedded resource's text is kept; a resource link becomes its name and URI; `isError` is a tool error, not a transport error. The whole answer is cut at 256 KiB before it crosses the port, with a marker.
- **Connection health:** a reconnect loop with capped exponential backoff and jitter, `ping` while idle, a status per server (connecting, ready, stopped with a fixed reason). No goroutine outlives the stack: all work is derived from the stack's start context and `Stop` waits for it.
- **Network:** every dial goes through the `guard` port (the core's `DialContext`: DNS-rebinding protection and the LAN/internet boundary, Constitution I.1 to I.3). Redirects are followed only to the same host. A custom CA file per server stays.

### Servers and secrets
- A server: `name` (`^[a-z][a-z0-9-]{0,23}$`, used in tool names), `url` (http or https), `enabled`, `trust` (`ask` or `allow`), `auth` (none, bearer token, or one named header), `ca_cert` (path), `disabled_tools` (names the owner switched off).
- **The token or header value is sealed** with the vault port (purpose `mcp`), stored in the stack's settings document as a sealed string, **never returned by any route** (the list says `has_secret`), never logged, never in a prompt. A factory reset replaces the key, so a sealed value cannot open afterwards.
- Servers live in the stack's settings document (the `agents` settings key gains `mcp_servers`); the old `registry.json` `mcp_servers` list is **copied once** into it by the core at start and then emptied (a migration, not a shim: nothing reads the old field afterwards, and the field is deleted from the model).

### Tools reach the run through a new SDK capability
- **Dynamic tools.** A stack that sets `Manifest.DynamicTools` implements `stacksdk.ToolSource`: `ListTools(ctx) ([]DynamicTool, error)` and `CallDynamic(ctx, name, args)`. Plain data (name, description, JSON schema, the server's name, whether it asks first). It is a method set the core calls, not a Host port, like `Observer` and `AirReporter`.
- **Names:** `mcp__<server>__<tool>`, at most 64 characters (a long tool name is cut and gets a six-hex hash). The stack keeps the name to (server, tool) map; two servers can never collide, and a server's tool can never take a built-in's name. The core refuses a name that is not `^[A-Za-z0-9_-]{1,64}$`, that a built-in or a declared tool already has, or that appears twice.
- **Server text is untrusted:** the core cuts a description at 1,024 characters, removes control characters, bounds a schema at 16 KiB and the tools at 64 per server and 128 in all, and the model-facing schema is ordered required-first (`schemaRequiredFirst` is kept, it fixed a real model problem, `learnings/orchestrator.md`).
- **Never waited on (Reminders' rule):** `ListTools` answers from the cache within a short bound and returns an empty list while the module is off, the stack is not running or a server is down; a hung server cannot hold a run.
- **The core wraps every call** in the guardrail engine. Lock down already refuses every tool outside the built-in catalogue; that stays.

### Ask or Allow (the guardrail rule)
- A tool outside the catalogue has no row (`EffectOf` says mutating, reach none). Instead of a row each, **a server has one trust setting**: **Ask** (default) makes the engine return an *approvable* refusal for the call, which the existing approval flow shows ("Allow once", exact call, this workspace, never a standing allow for the tool); **Allow** lets the call through. The setting is the owner's, saved with step-up. A server's own "read-only" annotation is not trusted.
- Unattended runs (automations, heartbeats) have no one to ask: the engine already denies an approvable refusal there at once, so an automation uses a server's tools only when the owner set the server to Allow.
- The result of a call is data, never instructions: it passes through the same untrusted-output fence as every tool result.

### Module, stack, routes and screen
- **Agents** stack, new layer **Tools** (bottom; Watch stays above it), module **MCP servers** (`mcp_servers`, an "off" flag in `AgentsConfig`, so an install that has no servers sees no change; switching it needs the passphrase, Constitution I.5). Off: nothing dialled, the tools are not listed, a change is refused with a plain sentence.
- Routes (declared in the manifest, the core mounts them, writes are step-up): `GET /admin/api/mcp/servers` (the list with status, tool count and the tools), `POST /admin/api/mcp/servers`, `PUT /admin/api/mcp/servers/{name}`, `DELETE /admin/api/mcp/servers/{name}`, `POST /admin/api/mcp/servers/{name}/test` (connect now and report). The old `/admin/api/mcp` routes, handlers, the `stepUpPrefixes` entry and the Settings → MCP tab are removed (no second path to one job).
- **Screen:** the **Tools** tab of the Agents page (`/agents/tools`): the server list as cards (name, address, status line in words, the tool count, Ask/Allow), an add/edit form (name, address, how to sign in, trust), a test button that says what happened and what to do, and a per-server list of tools with a switch each. States: loading, empty (with the one thing to do), error (cause and next step), switched off (links to Modules). Phone layout and keyboard use are checked.

### Removed from the core
`core/mcp`, `core/nodeherder` (incl. `token_manager.go` and `MCPService`), `app/mcp.go`, `app/app_context_mcp.go`, the MCP handlers and routes, the `AdminService` methods, `models.MCPServerConfig`, `models.MCPServerRegistryEntry` and `RegistryData.MCPServers`, the Settings tab and its service, composable and types, the `NodeHerder` field of the app services, the mocks. The name "nodeherder" remains only in docs that talk about the owner's separate app.

## Phases

1. **SDK.** `DynamicTool`, `ToolSource`, `Manifest.DynamicTools`, the validation of names and bounds as shared helpers, tests. *Accept:* `GOWORK=off go test -race` in `stacks/sdk`, the boundary tests.
2. **Client in `stacks/agents`.** Transport with fallback, pool, cache, mapping, backoff, sealed auth, against an in-process `mcp-go` server (Streamable HTTP and SSE) in tests: initialize, pagination, list-changed, session loss, auth header, an error result, a slow server, a big result, a refusing server. *Accept:* `GOWORK=off go test -race` in `stacks/agents`.
3. **Stack surface.** Manifest (layer, module, routes, `DynamicTools`), settings, routes, status, off behaviour. *Accept:* route tests in the stack.
4. **Core.** Adapter to `proxy.Tool`, `toolregistry` over the new interface, the guardrail Ask rule, the module switch, the one-time migration, removal of the old code, the route and step-up tests, the lock down test. *Accept:* `go build ./... && go test ./...`, `go run ./tools/check-complexity/`, the architecture tests.
5. **Frontend.** Tools tab, components, composable, types, constants, tests, e2e mock, visual test, the built `index.html`. *Accept:* `npm test`, `npm run build`, `npm run test:visual`.
6. **Docs and live check.** ADR-0069, SPEC-008 v2.0, SPEC-003 note, hubs, map, learnings; a live run against the nodeherder server (and an `mcp-go` test server on this machine); the harness check.

## Built so far (2026-11-19, branch stack-next-mcp-and-web-search, uncommitted, not pushed, not run on vertex)

Phases 1 to 5 are built and green: SDK (`DynamicTool`, `ToolSource`, `HTTP` port, `CleanText`); `stacks/agents/mcpclient` (Streamable HTTP with SSE fallback, pool, sealed auth, tests against in-process servers, `-race`); stack surface (Tools layer, module, 6 routes, step-up writes); core (`toolregistry.StackTools`, guardrail Ask rule, `stackport.HTTP`, shared vault, migration of registry servers, old `core/mcp` and `core/nodeherder` deleted); frontend (Agents → Tools tab, composable, card, form, redirect from /settings/mcp, "server · tool" labels). Checks run: `go build/vet/test ./...`, complexity (no new), stacks `GOWORK=off go test -race`, `npm test` (one view test then fixed), `npm run build`, Playwright agents-tools and stacks-modules specs (desktop and mobile).
**Not run:** the full `npm run test:visual` and `npm test` after the last edits; any live check against a real MCP server or vertex.

## Remaining Work

0. **First file to open:** this note, then `stacks/agents/agents.go` and `backend/internal/core/toolregistry/tool_provider.go`. First command: `git status -sb && ./scripts/check-agent-harness.sh`.
1. **Phase 6:** ADR-0069 stays proposed (add: PortHTTP, same-host redirects, no env proxy, MCP not through the egress proxy); rewrite SPEC-008 as v2.0 (v1.1 is stale); SPEC-003 note; map.md, orchestrator hub (drop nodeherder), learnings line; run `./scripts/check-agent-harness.sh`; a live run against the nodeherder server.
2. **UI polish left:** shorten the trust label ("When agents use its tools"), add a chevron to the Advanced summary, tighten card height; then rerun both test suites and the full visual suite.
3. Rewrite this section after each step. OAuth sign-in and stdio stay out of scope (owner call).
2. **Later, its own change:** internet search into the Tools layer (needs a check of how the providers use the guarded dial, and the system-settings payload's `Search` block moves: ask first).
3. **Not planned, owner's call:** OAuth sign-in for MCP servers; stdio servers (needs a sandboxed program runner).

## Traps (read before building)
- The tool schema's property **order** reached the model and mattered once (`learnings/orchestrator.md`): keep `schemaRequiredFirst`.
- A stack that is off or not running must answer at once; the run path never waits on it (ADR-0068 rule).
- Do not log a header value, a token, a tool argument or a tool result; log the server name, the tool name and a count.
