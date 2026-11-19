---
status: proposed
date: 2026-11-19
last_reviewed: 2026-11-19
related_specs: [SPEC-005, SPEC-006]
constitution_references: [IV.6]
evidence: Measured 2026-11-19 on branch stack-next-mcp-and-web-search, cut from origin/main a9974f5a (v0.112.1) with rg over backend/ (non-test files). Read - ADR-0032, ADR-0039, ADR-0067, ADR-0068, the Reminders plan, docs/subsystems/orchestrator.md, core/llm/manager.go, core/llm/providers/registrar.go, stacks/sdk/ports.go. Not read in full - the SPECs, the learnings ledgers of the other subsystems, Constitution IV.6 text, core/netaudit internals. Nothing built, no code changed.
---

# Where the next stack move can come from (local models first)

**Status:** proposed (2026-11-19). A measurement and a ranking, to be read by the owner before any move. No code, branch or ADR.
**Test applied** (the one ADR-0068 used): a candidate qualifies only if the run path never depends on it (the Reminders plan deferred `memorycapture` and `delegation` for this reason) and a stack can hold it through ports that already exist or one small new one.

## 1. Local models: not a stack yet

Local llama.cpp is not a separable feature in this code. Evidence (importers of each `LLMRuntimeManager` method outside `core/llm`, 2026-11-19):

| Group | Methods and who calls them |
|---|---|
| **Run path** | `GetInstance`, `RecordActivity` (`core/proxy/provider.go`); `EffectiveToolCallFormat` (`core/chat`, `core/automation/executor.go`, `core/delegation/runner.go`, `app/bootstrap.go`); `SelectModels` (chat, automation, proxy, inbound gate, netaudit); `ClassifyModel` and `EnsureModel` (`proxy_handlers.go`); `SetResidencyGuard` (`app/bootstrap.go`); `platform/procwatch` is imported by `core/llm/providers` |
| **Admin only** | model CRUD (`AddModel`, `UpdateModel`, `RemoveModel`), `TestProviderConnection`, `ListProviderModels`, `ProbeModelAvailability`, `ActiveLogs`, `ClearLogs`, `StopActive`, `core/llm/metadata` (GGUF scanner; imported by `registry_handlers.go`, `admin_view.go`) |

Why a whole move fails:
- **It is the run path.** Every run asks the manager for an instance; a stack that is "off" (ADR-0067) would mean no inference. Reminders could be off because nothing waits on it.
- **Local and cloud share one registrar.** `providers/registrar.go` has `RegisterLocal` and `RegisterCloud` together; the registry (`models.RegistryData`, `ModelConfig`) is read by both. Cutting local out splits the registrar and the model config.
- **The admin slice is thin and tied to the same data.** The CRUD routes edit the same registry the run path reads. Moving only them makes two owners of one record (the reason ADR-0067 rejected a stored stack flag) and needs adapters back into the core: the "bolt-on" the owner has refused.
- **Spawning is core.** Launching the llama.cpp process goes through the sandbox and process control (ADR-0033); a stack gets no way around them.
- **A stack's data must be plain data.** The SDK requires nothing, so `ModelConfig` and the provider types would be redeclared in the SDK: a copy of `internal/`, the risk ADR-0032 names.

**What would make it possible** (not proposed): a small *model* port in the SDK that lets a stack **use** a model (ask for a completion by role, no process control). That helps `memorycapture`, `delegation` and any future agent stack, not the hosting of models. Hosting local models stays core.

Possible, but small: the **Models page's server-log and status cards** could be shown on the dashboard as cards (ADR-0064); that is UI, not a stack.

## 2. Ranked sweep of the rest

Importers are non-test files by package; "run path" means the run, chat or proxy path calls it.

| # | Candidate | Target | Lines / importers | Run path | Ports needed | Conflicting ADR | Verdict |
|---|---|---|---|---|---|---|---|
| 1 | **MCP client** (`core/mcp`, `core/nodeherder`, `handlers/mcp_handlers.go`) | new **Integrations** stack, or Agents | mcp 1,150 / app only; nodeherder 254 / app, `core/mcp`, `core/toolregistry`, `handlers` | **Partly**: tools reach the run through `toolregistry` and `nodeherder.MCPService` (`ListTools`, `CallTool`) | **a dynamic tools port** (section 4): MCP tools are listed by the server at run time, so the Reminders manifest pattern does not fit; the dial is the SDK's existing guarded dial | none found; SPEC-008, hub says "decisions still live in the plans" | **Best next candidate. Measure first**: `toolregistry` and `handlers/services.go` hold the `MCPService` interface, so an adapter like Reminders' is needed |
| 2 | **Web search tools** (`core/tools/search.go`, `searchproviders/`) | new **Web** stack | not measured | yes, an assistant tool | tools in manifest; egress through the guarded dial | not read | Candidate. Measure importers and the egress rules (Constitution I) |
| 3 | **Prompt profile catalogue** (`core/assistant/profile`, ADR-0009) | Agents | 1,262 / 20+ (`core/chat` x3, `core/assistant`, `core/run`, `core/automation`, `core/delegation`, `core/commands`, `handlers` x3) | **Yes** | a run port | none | **The Reminders plan calls this "the next without a run port"; the importers above say otherwise.** Do not start without re-reading why |
| 4 | `core/memorycapture` | Agents or Memory | 1,031 / `core/chat` x2, `handlers` | yes | model port + run port | none | Needs a port first (as the Reminders plan said) |
| 5 | `core/delegation` | Agents | 595 / app | yes | run port | none | Needs a port first |
| 6 | `core/agentwatch` rest | Agents | 553 / app x2 | no | none new | ADR-0051 (proposed): core keeps the walk, the words and the gate | Already designed; waits on the owner (not run on vertex) |
| 7 | `core/radiodigest`, `core/radiowatch`, `core/stackalerts` | Radio | 367, 65, 88 / app, `agentwatch` | no | none | ADR-0036: the core words alerts; `radiodigest` says so in its package doc | **Stay core on purpose** |
| 8 | `core/devicejoin` | Network + Radio | 465 / app x2, `handlers` | no | none | ADR-0061: joined by tag **in the core** | Stay core |
| 9 | `platform/routerssh`, `routeprobe`, `mcast`, `neighbors`, `hostwatch` | Network | 435 / 352 / 312 / 719 / 3,705 | no | privileged or guarded ports | ADR-0034 (helper is core), ADR-0039 (listeners and `portprobe` stay core); `TestOnlyTheServiceCodeReachesTheRouter` | Stay core |
| 10 | `core/netaudit` (29k lines, 15 `app` importers, 11 `handlers`) | Network | large | no | many | ADR-0038/0039: sensors moved, listeners and the words stay | The remaining bulk is words, findings and listeners that the ADRs keep in core; not re-measured file by file |

## 3. Recommendation

1. **Do not move local models.** Say so in the hub for `orchestrator` once the owner agrees (a learnings line: "local llama.cpp shares the registrar and the registry with the cloud path and is the run path; a stack would need a model port first").
2. **MCP and web search are measured in section 4**: web search alone is not worth a stack; with MCP it makes one (outside tool sources). The next move is that stack, once the owner answers the section 4 decisions.
3. **Build the model port only when the owner wants `memorycapture` or `delegation` moved**; it unlocks #3 to #5 together.

## 4. Measured 2026-11-19 (branch stack-next-mcp-and-web-search from origin/main a9974f5a): MCP and web search

The owner asked (2026-11-19) whether a web tool on its own makes sense. **Answer: no.** Web search is one tool, one provider list and one key. What a stack needs is several modules the owner would switch as a group. What fits is a group the owner would switch together. The group that exists in the code is **outside tool sources**: MCP servers plus internet search. `fetch_url` and `get_network_info` do not join it (below).

| Piece | Size and callers | Coupling found | Moves? |
|---|---|---|---|
| **MCP client** `core/mcp` | 1,261 lines; one importer of the package (`app/mcp.go`); `handlers/mcp_handlers.go` 77 lines, four routes `/admin/api/mcp` | Run path sees it only through `nodeherder.MCPService` (`ListTools`, `CallTool`, `UseNativeTools`), held by `toolregistry` (`NewEngine`, `InitializeAgentStack`), `handlers/services.go`, `app/bootstrap.go`. Its dial is the guarded `DialContext` handed in at `app/mcp.go:19`. Servers live in `registry.json` (`models.MCPServerRegistryEntry`) and `models.MCPServerConfig`. **Its tools are dynamic**: listed from the server at run time, not declared in a manifest; every MCP tool is `CategoryGeneral` outside the guardrail catalogue (`models/tool_catalog.go`), so the core's guardrail gate must keep judging them by name. | **Yes, with one new port.** The Reminders pattern (tools declared as data, called by name) does not fit a dynamic list. Needs a **dynamic tools port**: `ListTools` and `CallTool` as plain data, the core adding the guardrail check around every call. Also the guarded dial (port `guard` exists) and the registry entries as the stack's own settings (a settings move like Reminders' `reminders:` block) |
| **Internet search** `tools/search.go`, `tools/searchproviders` | about 15 KB in 5 files without tests; tool in `manifests/search.json`; `registerSearchTools` in `toolregistry` | Static manifest tool: fits the Reminders pattern. But: its HTTP client is `NetworkTools.HTTPClient()` (the guarded egress); the provider choice lives in `RegistryData.Search` and the key in the secrets store under `CategorySearch`, both edited through the shared system-settings payload (`app_context_system.go`, `SystemUpdatePayload.Search`, `system_handlers.go`); the guardrail engine hides the tool through `searchAvailable` and the tool catalogue row (`GateSearch`) is core data | **Yes, small, after MCP.** Ports: `fetch` or the guarded client (exists: `stackport/fetch.go`, but it is a GET-text port, search needs headers and a POST for some providers: **check**), `vault` for the key (exists). Untouched risk: the settings payload still carries `Search`, so a clean move changes the system-settings contract (SPEC-level, ask) |
| **`fetch_url`, `get_network_info`** `tools/network.go` | the **guard itself**: `NetworkTools` is what `stackport/fetch.go`, the Network stack's sensors, `netaudit` (about 12 files) and `mcp` dial through | It enforces the network guardrails (Constitution I.1 to I.3) and is the single egress object | **Stays core.** A stack gets egress only through it (ADR-0033's rule applied to egress) |
| Connectors (`tools/communication.go`, `notifiers/telegram.go`) | reached by `stackport.Notify` | already the backend of the notify port | stays core |

**Facts checked for this section.** (a) A stack's `go.mod` may require third-party libraries, never the service or another stack (`archtest/boundary_test.go` `moduleViolations`), so `mark3labs/mcp-go` (already in `backend/go.mod`, v0.43.2) may move with the code; it is not a new dependency. (b) `stacksdk.Guard` has `DialContext(ctx, network, addr)`, the same signature `mcp.Orchestrator.DialContext` takes today. (c) The SDK `Fetch` port is a guarded **GET only** (no method, header or client), so search providers that need a header or a POST cannot use it; they would dial through `Guard` and build their own client, and that must be compared with what `NetworkTools.HTTPClient()` adds (the egress proxy, the policy per run): **unverified**. (d) **SPEC-008 §4 is stale**: `resource_mirror.go` does not exist; `core/mcp/client.go` has `ReadResource` and the interface in `types.go` lists it, but `rg` finds no caller, so no server text reaches the system prompt today. SPEC-008 should be corrected in whichever change moves MCP; if resource mirroring is ever built, outside text in the system prompt must pass ADR-0036 and the untrusted-data fence.

**Switch-off behaviour (ADR-0067, as for Reminders):** with the stack off, no MCP server is dialled, the MCP tools and `internet_search` are not listed, a change to a server is refused with a plain sentence, and the answer to a run is unchanged otherwise. This is a real gain for a security-minded owner: one switch for "nothing here talks to outside tool servers".

**Verdict.** One stack, **Integrations** (name open): module 1 *MCP servers* (dials the owner's listed servers; traffic: LAN or internet by URL), module 2 *Internet search* (needs a provider key; traffic: internet). Build order: the dynamic tools port and MCP first (the larger and cleaner piece, one importer), search second (it only needs ports that exist, plus the settings-contract decision). Not worth doing: search alone.

**The dynamic tools port must not be waited on:** `ListTools` has a short timeout and returns an empty list when the stack is off, not running, unreachable or slow, and a server that hangs never blocks a run (the section 1 test: the run path never depends on it). Every `CallTool` is wrapped by the core's guardrail check; the stack gets no way around it.

**Open owner decisions before a plan is written:**
1. Where it lives: a **new Integrations stack** (MCP servers + internet search), or **join the Agents stack** (MCP servers are agent tool sources, and the Agents feed already reads `fetch_url` hosts). Scope: both pieces, or MCP only for now.
2. The system-settings payload carries `Search`; may the stack take that block (a persisted-key-compatible move, like `reminders:`)?
3. A dynamic tools port is a new SDK port that lets a stack add tools the core did not list at start. Accept it with the core wrapping every call in the guardrail check (the stack gains no way around it).
4. Should MCP's `tls_ca_cert` and server list stay in `registry.json` or move to the stack's own file?

**Not measured:** the exact line counts of the MCP and search code (about, from `wc`), the handler tests that would move, what `registry_handlers_test.go` needs, whether any provider needs more than a GET through the `fetch` port, the frontend (the MCP admin screen's route and the search settings tab).

## Remaining Work

0. **First file to open:** this note; then `backend/internal/core/toolregistry/registry.go` and `backend/internal/app/mcp.go`. First command: `rg -n "mcp\\.|MCPService|NodeHerder" backend --glob '*.go' -g '!*_test.go'`.
1. **Owner:** answer the four decisions in section 4 (Integrations stack with MCP and search, search settings block, dynamic tools port, MCP settings home); accept or change the "local models stay core" verdict.
2. Measure the chosen candidate and write its own plan (inspect, ports, owner decisions, switch-off behaviour). Nothing is built before that.
3. Row 10 (netaudit) is not re-measured; row 2 is now section 4.
