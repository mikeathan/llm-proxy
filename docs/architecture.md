# Architecture Reference

This document contains architectural reference material extracted from the agent instructions. It covers directory mappings, critical contracts, test patterns, coding standards, checklists, and common pitfalls.

## Architecture (What Lives Where)

### Type Definitions

`models/` contains ALL shared types. No logic, only structs, constants, and interfaces.

- `models/config.go` — ModelConfig, ProviderConfig, GPUConfig, AgentGuardrailsConfig
- `models/infrastructure.go` — SystemConfig, UserSettings, ModelOverride
- `models/registry.go` — RegistryData, ModelRegistryEntry
- `models/llm.go` — Provider interface, error sentinels
- `models/llm_messages.go` — Message, ToolCall, ChatRequest (includes `MaxTokens`)
- `models/tools.go` — Tool name constants
- `models/workspace.go` — Workspace, AutomationRun, AgentState

### Core Systems

- `internal/core/assistant/` — Agent loop, tool providers, guardrails, prompts, provider tiers, ConversationService
- `internal/core/proxy/` — LLM HTTP client, XML tool call parser, history normalization
- `internal/core/proxy/recorder/` — `RecordingClient` decorator (captures LLM responses to JSONL)
- `internal/core/llm/` — Model lifecycle (start/stop/reap), GGUF scanning, provider registry
- `internal/core/automation/` — Scheduled task dispatch and execution, split by concern: `dispatcher.go` (type/constructor/metrics), `scheduling.go` (cron registration + fsnotify hot-reload), `admission.go` (lane admission, flock wait, cancel-queued), `execution.go` (run + stop), `history.go` (global activity ledger).
- `internal/core/eventbus/` — `Bus.Subscribe` / `Publish` / `Unsubscribe`, keyed by `(workspace, channel)`, fans out agent events to SSE-connected clients (`channel` is `EventChannel` — `assistant` | `automation`, see Pitfall #6); `Sink` writes the same events to a per-run JSONL file.
- `internal/core/runlane/` — Global run scheduler: per-workload-class lanes (`local`/`cloud`), FIFO queueing, interactive preemption (SPEC-007 §V), plus the **model-residency gate**: external `/v1` callers are admitted to the local model (wait/full-refuse) and can never evict a model a run is using (SPEC-007 §V.1).
- `internal/platform/rundir/` — Run output directory layout (`RunDir`) and retention reaper (`RunReaper`).
- `internal/core/egress/` — loopback-only agent egress proxy (host domain allow/deny policy; absolute-form + CONNECT). Sandboxing plan D2/Phase 1.
- Sandboxing enforcement lives in `platform/sandbox` (Provider/Effective) + `platform/process` (spawn) + `platform/sizewatch` (disk accounting). To add a NEW enforcement mechanism (e.g. a future executor): see `docs/PLANS/cross-cutting/agent-os-sandboxing.md` §4.3/§9 — add a build-tagged adapter + capability detection at `sandbox.New` + an `Effective` label + UI badge; the spawn seam and downgrade contract are already wired.
- `internal/core/tools/` — Tool implementations (terminal, filesystem, network, search, memory, communication). Multi-implementation families live in subpackages: `tools/searchproviders/` (search backends) and `tools/notifiers/` (connectors).
- `internal/core/memorycapture/` — Domain logic for saving memory from chats (SPEC-004 §II.8): the explicit-request keyword table (`phrases.go`) and extractor, shared guards, `Capture`, and the per-chat `Reviewer`. No imports of `assistant`, `tools` or HTTP: storage and the model arrive through its `Library` / `Completer` ports (adapters live in `assistant/memory_capture.go` and `handlers/assistant_memory_review_handlers.go`).
- `internal/core/mcp/` — MCP client (SSE transport, tool mirroring)
- `internal/core/orchestrator/` — Token budget management, context length resolution, slot scheduling, stream interleaving, reasoning budget normalization. See SPEC-005.
  - `slot_manager.go` — Manages concurrent inference slots (per-model capacity, queueing, timeout).
  - `budget_manager.go` — Token budget tracking per time window (`Spend`/`Refund`), throttling.
  - `budget_squeezer.go` — Compression fallback when budgets are tight (reduces context).
  - `stream_interceptor.go` — SSE stream parsing, interleaving parallel tool call outputs.
  - `reasoning_normalizer.go` — Normalizes reasoning token formats across providers.
- `internal/core/nodeherder/` — MCP tool provider adapter. Wraps the MCP orchestrator into a `ToolProvider` interface that the agent calls via `ListTools`/`ExecuteTool`. Manages MCP tool registration, mirroring, and credential injection.
  - `provider.go` — `ListTools` (polls MCP server), `ExecuteTool` (forwards to MCP server via `CallTool`), subscription to system prompt updates.
  - `token_manager.go` — Capability token resolution for MCP tool authentication.
- `internal/testing/llmprofiles/` — `FixtureClient` + `RunAgainstFixtures` (replay test framework)

### Infrastructure

- `internal/platform/storage/` — Generic atomic JSON/YAML stores with change callbacks
- `internal/platform/network/` — Shared/guarded HTTP transports + Constitution I.2 infrastructure doer (plus the single-stream audit-invariant test)
- `internal/platform/process/` — Central agent child spawn (R1: Setpgid, kill-group) + rlimit application (Darwin/Linux, measured matrix)
- `internal/platform/sandbox/` — OS confinement Provider seam + `Effective` downgrade reporting; Linux Landlock mechanism + self-exec runner; grant profile model
- `internal/platform/sizewatch/` — TTL-cached workspace disk accounting (`max_storage_gb`, best-effort)
- `internal/platform/units/` — binary size conversions (KiB/MiB/GiB → bytes) shared by storage accounting, sandbox rlimits and fetch limits (single home; no inline `1024`/`<<30` arithmetic)
- `internal/platform/logging/` — Structured logging (global + per-workspace process logs)
- `internal/app/` — Bootstrap, AppContext (central state manager), service wiring
- `internal/transport/http/` — Router, middleware, frontend embed (`handlers/admin_ui.go`: SPA fallback — client routes get `index.html`, `/admin/api/*`, `assets/` and root files 404; SPEC-003 §III.3)
- `internal/transport/http/handlers/` — HTTP handler types (Admin, System, Process, MCP, Model, Secrets, Dispatcher, Assistant, Proxy, Recordings, Memory, Webhook)

### Frontend (`frontend/src/`)

The admin UI contract — destinations, route table, theme tokens, tree, notifications — is SPEC-003 (`docs/SPECS/discovery-panel.md`); its §III.1 is the directory map. Entry points:

- `main.ts` — the one bootstrap: router, `startRunNotifications(router)`, mount
- `router/index.ts` (route table) · `router/routes.ts` (typed builders — the only way to build a location) · `types/routes.ts` (route names)
- `theme/tokenRegistry.ts` (every token name) · `styles/tokens.css` (preset values as `R G B` channels) · `public/theme-boot.js` (pre-paint, contract-tested against `theme/apply.ts`)
- `components/common/<domain>/` shared primitives · `components/layout/` shell · `views/` one lazy chunk per destination
- `/design` (dev builds only) — living gallery of tokens and primitives, with Playwright visual baselines (`e2e/`, `npm run test:visual`)

## Critical Contracts (Do Not Break)

### RuntimeManager Interface (`internal/core/llm/manager.go`)

The `RuntimeManager` interface is implemented by `LLMRuntimeManager` (production) and `MockManager` (tests). Any method added to the interface MUST be added to both implementations.

### ToolProvider Interface (`internal/core/assistant/tool_provider.go`)

Defines how tools are listed and provided to the agent. Implemented by `LocalToolRegistry`, `MCPNodeHerder`, and `MultiToolProvider`.

### Guardrail Decision Flow (`internal/core/assistant/agent.go`)

When a tool call is blocked:

1. `GuardrailDecisionCallback` is invoked with a decision ID
2. Callback blocks on a channel (per-model `guardrail_approval_timeout_seconds`, default 5 min)
3. Frontend resolves via `POST /admin/api/conversation/guardrail-decision`
4. If `persist: true`, override saved to workspace config

### Model Persistence (Two-Tier)

- Base model info → `registry.json` (handled by `PersistModel`/`PersistReplaceModel`/`PersistDeleteModel`)
- Agent tuning overrides → `settings.yml` under `model_overrides:` (handled by `UpdateSettings`)
- Both are written simultaneously in `handleAddModel` / `handleUpdateModel`

## Test Patterns

See `.agents/skills/testing-guide/SKILL.md` for the full test patterns guide (smoke tests, record-replay, run analysis).

- Mock the LLM client, use real tool providers where possible
- Agent tests at `internal/core/assistant/agent_test.go`
- Parser tests at `internal/core/proxy/tool_call_parser_test.go`
- Recorder tests at `internal/core/proxy/recorder/recorder_test.go`
- FixtureClient tests at `internal/testing/llmprofiles/profiles_test.go`
- Record-replay integration tests at `internal/core/assistant/agent_recording_test.go` (build tag: `recordreplay`)
- Use `mocks.NewMockManager()` for manager-related tests
- Test guardrail decisions with `GuardrailDecisionStore`
- Always assert both success AND error paths

### Record-Replay Testing

Record live LLM interactions by starting the server with `--record`:

```bash
go run main.go --record
```

This wraps every LLM client in a `RecordingClient` that writes JSONL transcripts to `data/runs/<model>/<task>/<timestamp>_<session>.jsonl` and enables the replay/fixture store. Each run also gets a per-run folder under `data/runs/` containing `events.jsonl` and `recording.jsonl` whenever run logging is enabled (config `run_logging.enabled`, `--enable-runs`, or `--record`).

Run replay tests offline (no LLM required):

```bash
go test -tags recordreplay ./internal/core/assistant/ -run TestAgent_Execute_AgainstRecordings -v
```

The `recordreplay` build tag ensures these tests are excluded from `go test ./...` — they only run when explicitly invoked. Fixture `.jsonl` files go in `internal/core/assistant/testdata/recordings/` or any path passed to `RunAgainstFixtures`.

## Coding Standards & Architecture

### Principles

Coding principles, Go idioms, error handling, and Vue conventions are owned by the mandatory rule
files and skills — reference them, don't restate them here:

- **Mandatory Go / Vue mechanics:** [`.agents/rules/`](../.agents/rules/).
- **Language-agnostic principles + smells checklist:** [`clean-code`](../.agents/skills/clean-code/SKILL.md).
- **Repo patterns, limits, file checklists:** [`engineering-practices`](../.agents/skills/engineering-practices/SKILL.md).

Repo-specific facts worth keeping here:

- **Dependency direction:** `internal/core/` → `internal/platform/` → `models/`; transport depends on core, never the reverse. `models/` has zero imports from the rest of the codebase.
- **No `init()` functions** in backend code — explicit construction via `New*` / `Initialize*` (the tool-registry connector factories are the documented exception).
- **Production readiness:** graceful shutdown via `signal.NotifyContext` in `main.go`; every long-lived operation takes a `context.Context`; structured `logging.*` with key-value pairs; no secrets in logs.
- **Frontend:** composables are module-level singletons; `npm run build` runs `vue-tsc -b` then `vite build` (TS errors fail the build). The admin UI's contract is SPEC-003 (`docs/SPECS/discovery-panel.md`).
- **Polling through `usePolling`** for new code — pauses when a keep-alive view deactivates, stops on unmount. The older shared pollers own their timers and are ref-counted (`useProcesses`, and `useMetrics`, whose consumers also release on keep-alive deactivate; the header's `HostStats` keeps it running app-wide and it pauses on a hidden tab); `useGlobalRunActivity` runs app-wide and pauses on a hidden tab. Every timer needs a named owner and teardown (Phase 6 leak audit).
- **Admin UI conventions** (tokens only, `utils/format/`, typed route builders, `usePersistedState`, `ConfirmDialog`, `renderMarkdown`) — `.agents/rules/frontend-vue-engineer.md` → Admin UI conventions.

## File Change Checklist

When adding a new model-level field, update these files:

1. `models/config.go` or `models/infrastructure.go` — type definition
2. `internal/transport/http/handlers/registry_handlers.go` — add/update request structs
3. `internal/transport/http/handlers/admin_handlers.go` — view struct
4. `internal/transport/http/handlers/admin_view.go` — view mapping
5. `internal/core/llm/manager.go` — if field affects runtime behavior
6. `internal/testing/mocks/manager.go` — if interface changed
7. Frontend component (if UI field)

When adding a new tool category (e.g. a new category like "communication"):

1. `models/tools.go` — add category constant (e.g. `CategoryCommunication`)
2. `internal/core/tools/manifests/{category}.json` — tool manifest (embedded)
3. `internal/core/tools/{category}.go` — implementation file with category struct + methods
4. `internal/core/assistant/registry.go` — registration wiring:
   - Add field to `LocalToolRegistry` struct
   - Add `init{Category}Tools()` helper function
   - Add `register{Category}Tools()` method on `LocalToolRegistry`
   - Call both from `InitializeAgentStack` and `registerAll()`
5. Frontend: add any category-specific UI (settings, status indicators)

When adding a single tool:

1. `models/tools.go` — constant
2. `internal/core/tools/manifests/{tool}.json` — manifest (embedded)
3. `internal/core/tools/{tool_category}.go` — implementation
4. `internal/core/assistant/registry.go` — registration (add field to `LocalToolRegistry`, add `register{Category}Tools()`, call from `registerAll()`, add `init{Category}Tools()` helper, wire in `InitializeAgentStack`)

When adding a communication connector:

1. `models/config.go` — `ConnectorConfig.Type` is the switch key (no struct change needed — generic map)
2. `internal/core/tools/communication.go` — implement `Connector` interface (Send + Name), use injected `*http.Client` from `NetworkTools.HTTPClient()`
3. `internal/core/tools/notifiers/` — add a self-registering `init()` calling `tools.RegisterConnectorFactory("your_type", factory)`. See `telegram.go` for a reference implementation. Do NOT edit `initCommunicationTools` or `registry.go` — the registry handles it dynamically.
4. Frontend `CommunicationSettings.vue` — add `<option value="your_type">` dropdown entry

When adding an internet-search provider: see the recipe in `.agents/skills/engineering-practices/SKILL.md` ("When adding a search provider"). The contract + `InternetTools` stay in `internal/core/tools/search.go`; providers and the explicit registration table live in `internal/core/tools/searchproviders/`.

When adding a prompt:

1. `internal/core/assistant/prompts/templates.go` — ONLY location for prompt text
2. Logic file (agent.go, tool_call_parser.go) — uses the template, never inlines strings

## Adding a Frontend Settings Tab Checklist

1. Add tab name to `SettingsTab` type in `frontend/src/types/admin.ts`
2. Add the label in `frontend/src/constants/providers.ts` (the category nav shows labels only)
3. If the tab is NOT a cloud provider, add exclusion to `isProviderTab()` in `frontend/src/domain/settings.ts`
4. Register in the appropriate settings group in `getSettingsGroups()` in `frontend/src/domain/settings.ts`
5. Create the settings component in `frontend/src/components/settings/`, built from `Panel` sections and `FormField` controls
6. Import the component in `frontend/src/views/SettingsView.vue` and add a `v-show="activeTab === 'your-tab'"` block (sections stay mounted, so edits survive switching category)
7. If it edits the shared configuration, end the section with `SettingsActions` bound to `useConfig().isDirty` (Discard / Save). A draft of its own (secrets, host settings) emits `dirty-change` so the page's single unsaved-change guard covers it
8. Run `npm run build` — TS errors will catch any missing icon/label entries
9. The tab is a route section (`/settings/<tab>`) automatically — an unknown section renders the in-shell not-found page. Link to it with `toSettings('<tab>')` from `frontend/src/router/routes.ts`, never a hand-built path

## New Backend Endpoint Checklist

1. Handler in `internal/transport/http/{thing}_handlers.go`
2. Route registration (in `router.go` or `main.go` route setup)
3. Types/response structs in the same handler file
4. Frontend: endpoint constant in `api.ts`
5. Frontend: service method in `{thing}Service.ts`
6. Frontend: types in `types/{thing}.ts`
7. Frontend: composable in `composables/use{Thing}.ts`
8. Frontend: view component in `components/{thing}/{Thing}.vue`

## Common Pitfalls

0. **MemoryStore nil-safety** — The `Agent.memoryStore` field is nil when memory is disabled. All code paths (active injection, pre-sieve flush, memory tools) must check `if a.memoryStore == nil` before dereferencing. This is the same pattern as `orch` (orchestrator nil-safety).

1. **Changing an interface without updating mocks** — The `MockManager` in `internal/testing/mocks/` must implement every method of `RuntimeManager` interface. Build fails on missing methods.

2. **Hardcoding prompt strings in logic files** — All prompts live in `templates.go`. Check there first before writing new ones.

3. **Saving model overrides to registry.json** — Agent tuning fields (`max_steps`, `context_budget`, `max_tokens`, `tool_call_format`, `prefill`) go to `settings.yml`, NOT `registry.json`. See Constitution III.5.

   Provider-native reasoning toggles follow the same rule. Keep unset/true/false
   distinct with a nullable field, and preserve explicit `false` during JSON
   serialization.

4. **Modifying history in the normalizer** — `NormalizeHistory()` does role conversion and metadata stripping only. Nag injection and feedback belong in the agent loop (`agent.go`). See Constitution II.8.

5. **Adding new model fields without updating the UI** — New `ModelConfig` fields must be added to:
   - The request structs in `registry_handlers.go` (both add and update)
   - `adminModelView` in `admin_handlers.go`
   - `getModelsView()` mapping in `admin_view.go`
    - Both `runtimeCfg` and `persistCfg` in the handler

   Scoped Vue styles do not cross component boundaries. When a form block moves
   into a child component, move its layout and control styles with it rather than
   relying on the parent component's scoped stylesheet.

6. **Unified agent flow — assistant and automation share the same path** — Both assistant conversations and automation tasks use the same `buildChatRequest`, `processStream`, and `handleNoToolCalls` logic. No context-type branching exists in the agent core. Behavioral differences (memory injection) are expressed via `AgentOptions` fields. Natural completion (no tool calls + substantive visible content) is the canonical completion path — the model writes its answer as plain text when done, without requiring provider-specific tool-call conventions.

   **Event isolation:** although the *agent core* is unified, the *event stream* is NOT. Every `AgentEvent` carries a `Channel` (`ChannelAssistant` for chat, `ChannelAutomation` for scheduled runs) plus a `ConversationID`. The automation `EventBus` keys subscribers and routing by `(workspace, channel)`, and the SSE endpoint `/live` serves a single channel per connection (`?channel=assistant` for the chat pane, defaults to `automation` for the live console). This prevents an automation's `finalReply`/nag output from leaking into the assistant chat. New code that publishes assistant events MUST stamp `Channel: ChannelAssistant` (done by `ConversationService` via `WithChannel`/`WithConversationID`); automation events stamp `ChannelAutomation`. Do not rely on the frontend to filter by conversation — isolation is enforced server-side.

7. **Removing the XML parser** — Even with native tools enabled globally, the XML parser must remain as fallback for non-function-calling responses. Both paths coexist.

8. **`tool_choice` is NOT forced to `"required"`** — Phase 2 (§4.2.8) removed the `tool_choice: "required"` override. The model freely chooses between calling tools and writing text — natural completion requires the model to write its final answer as plain text without tool calls, which is only possible when `tool_choice` is not coerced.

   Reasoning wire params are resolved per provider by `assistant/reasoning_param.go` — a `ReasoningSpec` (typed mode/effort/budget) is applied through a `ReasoningParamResolver` (strategy pattern). Only the provider-appropriate field is serialized: local llama.cpp → `thinking_budget_tokens`; openai/gemini → `reasoning_effort`; openrouter → `reasoning` object; nvidia → `chat_template_kwargs.enable_thinking`. A workload classified `WorkloadLocal` (via the shared `WorkloadClassifier`, the same classifier that drives budget and ICU) always overrides to `thinking_budget_tokens`, so an `openai`-slugged config pointed at a local URL keeps working. See `models/llm_messages.go` `ChatRequest` and `stream.go` `buildChatRequest()`.

   Even when `reasoning_budget` is 0 in the model config, the agent dynamically computes the local think-token budget from `max_tokens` via `DefaultReasoningBudget()` (= `max_tokens / 3`) and syncs it into `ReasoningSpec.Budget`. This is implemented at agent-build time in `resolveReasoningSpec` (`agent.go`), the single source of truth for the resolved spec, and scoped to local/GGUF workloads (ModeThinkTokens). Because `max_tokens` is itself derived from the server's serving context (`ctxLen / 3` in `ApplyMetadataDefaults`, **local-only**), the reasoning budget tracks the context size the user launched the server with — no manual budget config, no model-name matching.     Cloud workloads (openai/gemini/openrouter/nvidia) use `reasoning_effort` / `reasoning` object / `chat_template_kwargs.enable_thinking` instead (see `assistant/reasoning_param.go`), never a numeric budget. An explicit `reasoning_budget` in the model config always overrides the derived value. See `agent.go` `DefaultReasoningBudget()` and `resolveReasoningSpec()`.

    **Never hardcode provider names to decide UI/feature gating.** The `reasoning_enabled`
    toggle and any other provider-varying behaviour must be driven by the declarative
    `ReasoningCapability` table in `assistant/reasoning_param.go`, surfaced to the frontend
    via `provider_defaults[provider].reasoning` in `GET /admin/api/state`. The frontend
    renders from that descriptor (e.g. `reasoning?.toggleable`, `policy.isCloud`) and never
    writes `provider === 'nvidia' || provider === 'openrouter'`. Adding a provider = one
    table row (+ one resolver only if the wire mechanism is new); no UI/backend name checks.
    The table is keyed by *provider type* (wire protocol), not vendor, and is reclassified
    `WorkloadLocal` by the shared `WorkloadClassifier` for local/loopback endpoints.

    **The canonical provider key set has exactly one home:** `models.ProviderIDs()`
    (backend, leaf package — no import cycle) and `PROVIDER_IDS` (frontend,
    `constants/providers.ts`). The numeric tuning table (`models/tuning.go`),
    the two reasoning tables (`assistant/reasoning_param.go`), and the frontend
    `PROVIDER_LABELS` record (typed by `SettingsTab`) all key off it, and a drift test
    (`models/provider_registry_test.go`, `reasoning_param_test.go`) fails CI if any
    table gains or loses a provider without the others. Provider *capabilities* (e.g.
    `supports_base_url`, surfaced via `provider_defaults[id].supports_base_url`) are
    emitted by the backend (`models.SupportsBaseURL`), never re-listed in the UI — the
    old `new Set(['openai','openrouter','nvidia'])` in the pre-redesign `Settings.vue` (now `views/SettingsView.vue`) is gone.

9. **Memory system — see `.agents/skills/memory-system/SKILL.md`** for full architecture: storage, injection, three-tier design, tags, dedup, and known issues.

20. **Agent loop mechanics — see `.agents/skills/agent-loop/SKILL.md`** for: execution flow, sieve, stuck detection, reasoning budget, fallback chain, repetition/spiral detector, and key constants.

22. **Workspace guardrails are a layer, merged additively.** `models.AgentGuardrailsConfig.MergeWith`
    unions lists, lets a workspace only switch things on (`require_review` only off),
    replaces numbers only when > 0, always takes session idle from the layer, and — for a
    saved layer — always takes network access from it. A workspace cannot remove a global
    entry. The UI mirrors this in `frontend/src/domain/guardrailLayers.ts`; change both
    together — `backend/models/config_merge_contract_test.go` and the TS test run the same
    fixture (`frontend/src/__TESTS__/fixtures/guardrailMerge.*.json`) and fail on drift.

21. **Testing — see `.agents/skills/testing-guide/SKILL.md`** for: running smoke tests, analysing run output, record-replay testing, MockClient patterns, common pitfalls.

22. **`maxLength` removed from `filesystem.json` manifest** — The `maxLength` properties were removed because servers enforce it as a grammar constraint, silently truncating content. See `docs/audits/write-file-truncation-cycles.md` for full root-cause analysis.

23. **Unified agent flow — assistant and automation share the same path** — See pitfall #6 above. Same rules apply: natural completion via plain-text answer, no protocol-violation nag in native-tools mode.

24. **No-tool content cap must not amputate a legitimate final answer** — `stream.go` `processStream` terminates a tool-free stream that exceeds `maxTokens` ONLY when there is no prior `ToolRole` in history AND tools are configured (`!priorToolResult && toolsAvailable`). A long plain-text answer after real work (or when no tools exist) is the genuine final report and must run to its natural stop so `checkTaskCompletion` (`session.go`) finalizes it intact. The runaway joke-loop window is still bounded by the `*4` char cap (`exceedsContentCharCap`) and the token-budget `ShouldTerminate`. Do not re-arm the cap unconditionally — that regresses report delivery (see `docs/PLANS/ARCHIVE/assistant-ui/automation-unified-renderer-and-report-truncation.md` §2.1).

25. **Automation UI reuses the assistant renderer — single shared consumer** — Automation runs render through `ChatMessages mode="automation"`, fed by `useLiveConsole` → the shared `useMessageBuilder` (same single event→message consumer as chat; `automationEventsToMessages` is deleted — it caused a cumulative-re-emit cascade). `reasoning`/`tool_stream` events become collapsible reasoning segments via the builder's prefix-replace dedup, not overwrites into the last assistant message (the old `LiveConsole`/`TerminalOutput` overwrite bug). Customize automation UI via the `ChatMessages` `mode` prop + `#run-header` slot, never a bespoke terminal renderer or a forked event mapping.

Block only explicit path operands — leave env-driven resolution (`HOME=.../.sandbox`, `path_extensions` `.sandbox/node_modules/.bin`) and legit dot-dir users (`git`, `.gitignore`) functional.

27. **Upstream transport errors vs model entitlement (NVIDIA etc.)** — A provider catalog (`/v1/models`) is a *public* list; it does not reflect which models your API key can actually call. Non-entitled models can 404, hang for tens of seconds, or get the connection closed mid-POST. A bare Go `"unexpected EOF"` therefore must NEVER be treated as a generic network failure — check the model ID first. Our client now classifies transport failures (`classifyTransportError` → `connection-closed`/`timeout`/`tls`/`http2`/`connection-reset`), logs `model`/`url`/`kind`/`error_class` on every retry, and appends the classification to the surfaced error after retries are exhausted. `addModel` runs a warn-only availability probe (`ProbeChatModel`, `max_tokens:1`, 10s bound) so a wrong/not-entitled model ID is caught at save time instead of as a confusing EOF on the first request. Provider `default_base_url` entries often already include a trailing `/v1` — the proxy client normalizes so it never sends `…/v1/v1/chat/completions` (only the providers `endpointURL` join is allowed; anything appending a raw path risks the doubling regression).

28. **LLM chat traffic is HTTP/1.1-only; provider infra keeps HTTP/2** — `network.LLMChatTransport` (local, 10-min header timeout) and `network.CloudLLMChatTransport` (cloud, 45s header timeout) are the only transports `proxy.LLMClient` may use; both force HTTP/1.1 (`ForceAttemptHTTP2:false` + non-nil empty `TLSNextProto`). NVIDIA's `integrate.api.nvidia.com` has a broken HTTP/2 path for chat completions: after auth passes, curl fails with `Error in the HTTP2 framing layer` and Go's `http.Client` (which negotiates HTTP/2 by default) surfaces `unexpected EOF` on the POST, while HTTP/1.1 is stable — this was the "suddenly failing since today" incident (2026-08-21). The 45s cloud header timeout converts NVIDIA's ~60s server-side connection drop into a clean client-side `timeout` classification so retries fire sooner. `SharedTransport` (provider catalogue, connection tests) intentionally keeps HTTP/2. Never route LLM chat through a transport with HTTP/2 enabled.

29. **Cycle detection must key on (tool, args), never tool name alone** — `checkSequenceRepeat()`'s n-gram window uses full `toolKey` values (name + arguments), and the single-tool spiral requires 12+ consecutive same-name calls that **recycle ≤4 distinct argument values** (varied-args bursts are legitimate batching, Constitution II.1). A run that legitimately dominates on one tool with varying arguments (e.g. a storage-audit automation issuing a series of distinct `find`/`du` commands — the 2026-08-21 workspace-health-test incident, 28 unique `(name,args)` pairs flagged as a "3-tool cycle", then 12 distinct terminal commands flagged as a "spiral") must never abort. True repeating cycles/spirals recur with identical or recycled arguments. When extending any repetition detector, compare arguments, not just names.

30. **Automation runs must never wait for guardrail approval** — `resolveGuardrail` (`tool_exec.go`) denies non-security guardrail violations immediately on the `automation` channel (`ChannelAutomation`) with hard policy guidance, never calling the approval callback. An unattended run has no user to answer the prompt: waiting burned the full 5-minute `GuardrailApprovalTimeout` and then aborted the run with a misleading `agent execution halted: context deadline exceeded` when the run's 10-minute `automationTimeout` expired (the workspace-health-test `xargs` incident, 2026-08-21). Security-boundary violations were already synchronous; the automation channel extends that to every violation. Do not re-introduce approval waits for unattended runs — feed the denial back to the model so it adapts.

31. **Tool execution failures are record-and-continue in every loop strategy** — `executeSingleToolStep` appends the failure to history as a tool result; both `processToolCalls` (react loop) and `executePlan` (plan_execute) then log and continue to the next turn/step. A single failed tool (shell exit code, compile error, missing file, timeout) must never abort the run — the failure becomes a reported outcome in the final report. This is what makes the agent model-agnostic: a weak model's mistakes (e.g. deepseek adding `load` to a TS object literal but not the interface → `tsc` TS2353, the llm-smoke-test incident 2026-08-21) surface in the report instead of killing the run. Only structural errors abort (plan args marshal/validation failure, `MaxPlanSteps`/deadline exceeded). EvaluatorOptimizerStrategy delegates to ReactStrategy, so all three strategies share the convention.

32. **Finalization failure must never discard completed work** — `finalizeReport`'s fallback chain is `bestAvailableAnswer()` → `synthesizeRunSummary()` (per-tool call counts + recorded `{"error": ...}` failures from history) → error only when the run did no tool work. Plan_execute runs have no assistant text (pure tool calls), so without the synthesized summary a provider outage on the report turn killed the whole run with `finalization turn failed` (the llm-smoke-test + NVIDIA timeout incident, 2026-08-21) — all 11 executed steps discarded. A degraded-but-real summary beats a confusing failure every time.

33. **Remote llama.cpp serving `.gguf` must use the LOCAL client transport** — The bootstrap client factory (`bootstrap.go`) must classify via `WorkloadClassifier.ClassifyClient(baseURL, modelID)` (endpoint-local **OR** `.gguf` artifact), never `ClassifyEndpoint` alone. A remote llama-server (e.g. `192.168.50.60:8084`) is not a local interface IP, so host-only classification picks `CloudLLMChatTransport` with its **45s response-header timeout** — but llama-server only sends headers for a non-streaming request after the FULL generation, so any non-streaming fallback (stuck-recovery `computeNextResponseNonStreaming`, finalize) longer than ~45s dies with `net/http: timeout awaiting response headers` (the Ornith repetition-loop incident, 2026-08-29, ~103s generation vs 45s timeout). The manager's workload class already calls this model local (registry name ends `.gguf`); the client must agree. Streaming never hits this (headers arrive immediately), which is why long streaming runs work while the same model's stuck-recovery path fails.
34. **Run-lane release discipline** — Every agent run is admitted through `internal/core/runlane`; an interactive claim's returned ctx derives from the **caller's** ctx (never the lane root) or `/assistant/cancel` stops reaching the run, and a granted slot without a deferred `release()` stalls the lane. See SPEC-007 §V.
35. **Lane state is global — one route, one poller** — the run scheduler's snapshot (`Scheduler.Snapshot()`) takes no arguments, so lane state is exposed **only** by `GET /admin/api/active-runs` (no workspace param; `lane_holders`/`queued`, plus `lanes` — each lane's `limit`/`running`/`waiting`/`holder_keys`, idle lanes included, for the slot bars), polled once by `useGlobalRunActivity` and shared by the header `RunActivityPill` and the workspace view's `laneWaitingLabel` (`views/WorkspacesView.vue` → `MonitorPanel`). `GET /workspaces/{ws}/active-runs` carries **only** workspace-scoped assistant fields (`assistant_running`/`automation_running`/`assistant_conversation_id`/`assistant_queued`) — do not add lane fields back to it: every workspace would fetch identical bytes. Because lane holders are global, any "which run is blocking *this* workspace" question must filter by `holder.workspace_id` (the old unfiltered `laneWaitingLabel` named blockers from other workspaces), and because a failed poll leaves the last lists in place, surfacing the failure ("run state unavailable") is mandatory — silently keeping stale counts reads as live progress.

36. **Model residency is a decided act, never a side effect** — `LLMRuntimeManager.GetInstance` refuses to evict the local model a run or inbound caller is using (`llm.ErrLocalModelBusy`, `llm/residency.go`), so a model is never swapped out from under live work. The single decision point is the `runlane` model gate (`modelgate.go`): the manager refuses, the transport explains (`429` + `X-LLM-Status: busy|queued` + `Retry-After`, checked by a downstream proxy before its transient-retry path), and only the operator's `POST /admin/api/queue/{key}/promote` (or the opt-in `inbound_preempt` host policy, default off) cancels the blocking run — granted only after that run unwinds. While the gate holds an entry the local lane suspends queued starts (`lane.holdStarts`), otherwise a preempted scheduled run re-queues and instantly re-takes the model ahead of the caller. See SPEC-007 §V.1.

37. **`useRunningActivity()` is called by exactly one view** — its state is a module singleton, but the poll belongs to the caller and binds to that caller's workspace; a second caller would re-point or stop the one poll. Anything else reads `runningActivitySnapshot()`.

38. **Edit forms must populate on identity, not object** — `useAutomationForm`'s populate watch is keyed on `editAutomation.value?.id`. The automations view polls every 10 s and replaces the object; a whole-object watch re-populated the form and wiped the operator's edits.

39. **Tailwind reads `shadow-[var(--x)]` as a colour** — an arbitrary shadow built from a CSS variable is parsed as a shadow *colour*, not geometry. The one permitted shadow (the primary button's brand offset) is the `.offset-brand` class in `style.css`.

40. **`v-html` only via `renderMarkdown`** — model and user markdown can carry HTML and `javascript:` links. `utils/markdown/renderMarkdown.ts` escapes raw HTML and allows only `http`/`https`/`mailto`/relative URLs; a new `v-html` fed from anything else is an XSS.

41. **The template store and the workspace copy are separate files** — the UI previews and imports from `<data root>/templates/`, never from `backend/data/templates/`, and a playbook copied into a workspace is a third copy. `TemplateStore.syncTemplates` seeds missing templates and refreshes one only while it still matches the hash recorded in `templates/.shipped.json` (so an operator's edit is never overwritten, and a file with no record is left alone). After changing a shipped template, restart the backend; an already-added workspace copy must be deleted and re-added.

42. **A wide markdown table widens its card unless it scrolls** — `renderMarkdown` wraps every `<table>` in `.md-table-scroll` and `MarkdownViewer` styles it; new markdown surfaces must go through `MarkdownViewer` (or add the same wrapper), or one long URL column pushes the page past the viewport on a phone.

43. **`PUT …/workspaces/{ws}/config` replaces the whole document** — a client that sends only the field it changed erases the rest (automations, guardrails, heartbeat, `assistant_memory`). Read the config fresh, change the one field, write it back (`useAssistantMemory`), or use a dedicated endpoint that mutates one field (`PUT …/heartbeat`).

44. **Streamed `reasoning` / `tool_stream` events are full snapshots, not deltas** — keeping every one filled the replay buffer in minutes (evicting `session_started`, so a reopened chat came back blank) and wrote 11 MB per run to `events.jsonl`. The bus, the sink and the in-memory run log keep only the newest snapshot of a stream (`assistant.SupersedesSnapshot`, only for string payloads with a conversation id); a new snapshot-style event type must be added to `AgentEventType.IsSnapshot`, and a consumer must not assume it sees every intermediate snapshot.

45. **Local-or-cloud is decided from the model's own listing entry, never from a context number or a name** — `Metadata.Serving == "llamacpp"` (set per `/v1/models` entry at discovery, refreshed at startup) makes a model local behind an OpenAI-style URL; a published/serving context alone (cloud catalogs carry one) never does. Budgets, the reasoning spec, the client (`ModelInstance.Local`) and the proxy's own `/v1/models` all follow the one class, so a change to the rule must change them together.
