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
- `internal/core/automation/` — Scheduled task dispatch and execution, EventBus (SSE broadcasting to frontend)
- `internal/core/automation/broadcast.go` — `EventBus.Subscribe` / `Publish` / `Unsubscribe` per workspace, fans out agent events to SSE-connected clients
- `internal/core/tools/` — Tool implementations (terminal, filesystem, network, search, memory, communication)
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
- `internal/platform/logging/` — Structured logging (global + per-workspace process logs)
- `internal/app/` — Bootstrap, AppContext (central state manager), service wiring
- `internal/transport/http/` — Router, middleware, frontend embed
- `internal/transport/http/handlers/` — HTTP handler types (Admin, System, Process, MCP, Model, Secrets, Dispatcher, Assistant, Proxy, Recordings, Memory, Webhook)

## Critical Contracts (Do Not Break)

### RuntimeManager Interface (`internal/core/llm/manager.go`)

The `RuntimeManager` interface is implemented by `LLMRuntimeManager` (production) and `MockManager` (tests). Any method added to the interface MUST be added to both implementations.

### ToolProvider Interface (`internal/core/assistant/tool_provider.go`)

Defines how tools are listed and provided to the agent. Implemented by `LocalToolRegistry`, `MCPNodeHerder`, and `MultiToolProvider`.

### Guardrail Decision Flow (`internal/core/assistant/agent.go`)

When a tool call is blocked:

1. `GuardrailDecisionCallback` is invoked with a decision ID
2. Callback blocks on a channel (max 60s)
3. Frontend resolves via `POST /admin/api/conversation/guardrail-decision`
4. If `persist: true`, override saved to workspace config

### Model Persistence (Two-Tier)

- Base model info → `registry.json` (handled by `PersistModel`/`PersistReplaceModel`/`PersistDeleteModel`)
- Agent tuning overrides → `settings.yml` under `model_overrides:` (handled by `UpdateSettings`)
- Both are written simultaneously in `handleAddModel` / `handleUpdateModel`

## Test Patterns

See `docs/skills/testing-guide.md` for the full test patterns guide (smoke tests, record-replay, run analysis).

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

Record live LLM interactions by starting the server with `--record-dir`:

```bash
go run main.go --data ./data --record-dir=testdata/recordings
```

This wraps every LLM client in a `RecordingClient` that writes JSONL files to `{record-dir}/{model}/{timestamp}_{session}.jsonl`. Hit different models/prompts through the proxy or agent API to build a fixture library.

Run replay tests offline (no LLM required):

```bash
go test -tags recordreplay ./internal/core/assistant/ -run TestAgent_Execute_AgainstRecordings -v
```

The `recordreplay` build tag ensures these tests are excluded from `go test ./...` — they only run when explicitly invoked. Fixture `.jsonl` files go in `internal/core/assistant/testdata/recordings/` or any path passed to `RunAgainstFixtures`.

## Coding Standards & Architecture

### General Principles

1. **Clean Architecture**: Dependencies point inward. `internal/core/` knows nothing about `internal/transport/`. `internal/platform/` is a dependency-free foundation. Models (`models/`) have zero imports from the rest of the codebase.
2. **SOLID**:
   - **Single Responsibility** per file/function. If a function does two things, split it.
   - **Open/Closed**: Extend via injection (interfaces), not modification.
   - **Liskov Substitution**: Interface consumers should work with any implementation — test mocks must satisfy the same contract.
   - **Interface Segregation**: Keep interfaces small. `RuntimeService` (manager interface) is a good example — methods are focused and cohesive.
   - **Dependency Inversion**: High-level logic (agent loop) depends on abstractions (LLM client interface), not concrete implementations.
3. **DRY but not premature**: Repeat 3 times before extracting. Two similar lines are just similar; three identical patterns means extract.
4. **Idiomatic Go**:
   - Zero-value initialization over constructors for simple types.
   - Accept interfaces, return structs.
   - `fmt.Errorf` with `%w` for error wrapping. Use sentinel errors from `models/llm.go`.
   - No getters/setters for struct fields — export directly.
   - Table-driven tests with `t.Run`.
   - Prefer `range` over index-based loops.
   - `var` zero-init for package-level, `:=` for local.
5. **Production Readiness**:
   - Graceful shutdown on SIGINT/SIGTERM (signal.NotifyContext in main.go).
   - All long-lived operations accept `context.Context` for cancellation.
   - Validate at boundaries, trust internals (Constitution I.1).
   - Structured logging with `logging.Info/Debug/Warn/Error` and key-value pairs.
   - No secrets in logs or error messages.

### Engineering Patterns

1. **Constants over magic values**: Every hardcoded string, int, or float that appears in logic must be a named constant (`const`, not `var`). Group related constants at the top of the file. Exceptions: `0`, `1`, `""`, `nil` in zero-value initialisation or loop counters.
2. **Strategy Pattern for branching to extend**: When a `switch` or `if-else` chain grows with new cases over time, replace it with a strategy map. New cases become registrations, not new branches. Follows Open/Closed — the function is closed for modification, open for extension.
3. **Value Objects for domain primitives**: Use typed constants (`type Scope string`) with a `Validate()` method instead of raw strings. Catches invalid states at compile time and makes the domain vocabulary explicit. Only for values that have a bounded set of valid states — not for freeform strings like names or messages.
4. **Null Object over nil checks**: Prefer returning a no-op object over nil when a function has a valid "do nothing" path. The caller shouldn't need to check for nil before every call. Only applies when the nil case has a meaningful no-op behaviour — not for error paths.
5. **Command Query Separation**: A function either returns data OR mutates state, never both. A save operation returns `error` or `ok`. A query returns data. If a function currently does both, split it into two.
6. **Defensive programming at boundaries**: Validate all inputs at system boundaries (API handlers, tool handlers, store methods). Trust internal callers. If an invalid state is impossible at the call site but the function signature allows it, document the assumption with a comment.
7. **No silent failures**: Every error must be handled — either returned to the caller, logged, or explicitly ignored with a comment explaining why (`_ = doSomething()  // best-effort cleanup`). Never use `_ =` without a comment.
8. **Immutability for function parameters**: Don't modify input slices or maps. Make a copy first (`append([]T{}, input...)` for slices, a fresh `map` for maps). The original caller's data should be unchanged after the call.
9. **Guard clauses over nested ifs**: Return early for error/null/edge cases. The happy path should be flat and left-aligned. Never nest deeper than 3 levels.
10. **Function composition for complex conditions**: Extract multi-condition checks into a named helper function. `if isRetryable(err)` is better than `if errors.Is(err, io.EOF) || errors.Is(err, syscall.ECONNRESET) || ...`. The helper name documents the WHY.

### File Organization

- **One primary type per file**, named after the type (e.g., `agent.go` → `Agent`, `session.go` → `runSession`).
- **Handlers in `internal/transport/http/`** — thin, parse request → call service → write response. No business logic.
- **Services in `internal/platform/`** — reusable infrastructure (storage, logging, network).
- **Implementation in `internal/core/`** — business logic with minimal imports from `internal/platform/`.
- **Package-level state in composables** — module-level `ref()` with `mountCount` pattern for polling.
- **No `init()` functions** — explicit construction via `New*` or `Initialize*`.

### Error Handling

- Validate at system boundaries (user input, external APIs). Trust internal callers.
- Wrap errors with `fmt.Errorf("context: %w", err)` at each layer boundary.
- Use sentinel errors from `models/llm.go` for known LLM conditions (`ErrUnknownModel`, `ErrModelExists`, `ErrModelStarting`).
- Return early on errors — no deep nesting of `if err == nil` (happy path to the left).
- Don't log AND return — one or the other. Return for the caller to handle.

### Frontend (Vue 3 + TypeScript)

- **Composables are singletons** — module-level state shared across components.
- **`ref()` over `reactive()`** for reactive state.
- **Type imports** from `types/` directory (barrel exports).
- **Services are stateless** — API calls only, no local state.
- **Polling with `mountCount`** — ref-counted intervals (starts at first mount, stops when last unmounts).
- **`npm run build`** runs `vue-tsc -b` then `vite build` — TS errors fail the build.

## File Change Checklist

When adding a new model-level field, update these files:

1. `models/config.go` or `models/infrastructure.go` — type definition
2. `internal/transport/http/registry_handlers.go` — add/update request structs
3. `internal/transport/http/admin_handlers.go` — view struct
4. `internal/transport/http/admin_view.go` — view mapping
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

When adding a prompt:

1. `internal/core/assistant/prompts/templates.go` — ONLY location for prompt text
2. Logic file (agent.go, tool_call_parser.go) — uses the template, never inlines strings

## Adding a Frontend Settings Tab Checklist

1. Add tab name to `SettingsTab` type in `frontend/src/types/admin.ts`
2. Add icon + label in `frontend/src/constants/providers.ts`
3. If the tab is NOT a cloud provider, add exclusion to `isProviderTab()` in `frontend/src/domain/settings.ts`
4. Register in the appropriate settings group in `getSettingsGroups()` in `frontend/src/domain/settings.ts`
5. Create the settings component in `frontend/src/components/settings/`
6. Import the component in `frontend/src/components/settings/Settings.vue`
7. Add `v-show="activeTab === 'your-tab'"` div in the Settings.vue template
8. Run `npm run build` — TS errors will catch any missing icon/label entries

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

4. **Modifying history in the normalizer** — `NormalizeHistory()` does role conversion and metadata stripping only. Nag injection and feedback belong in the agent loop (`agent.go`). See Constitution II.8.

5. **Adding new model fields without updating the UI** — New `ModelConfig` fields must be added to:
   - The request structs in `registry_handlers.go` (both add and update)
   - `adminModelView` in `admin_handlers.go`
   - `getModelsView()` mapping in `admin_view.go`
   - Both `runtimeCfg` and `persistCfg` in the handler

6. **Unified agent flow — assistant and automation share the same path** — Both assistant conversations and automation tasks use the same `buildChatRequest`, `processStream`, and `handleNoToolCalls` logic. No context-type branching exists in the agent core. Behavioral differences (memory injection) are expressed via `AgentOptions` fields. The `handleNoToolCalls` handler uses a single unified flow: exit on readable text despite parse error (non-native mode), nag on empty or when native-tools protocol is violated. When `useNativeTools` is true and tools are available, the model must produce a tool call — freeform text is treated as a protocol violation and triggers a nag/retry.

7. **Removing the XML parser** — Even with native tools enabled globally, the XML parser must remain as fallback for non-function-calling responses. Both paths coexist.

8. **`tool_choice: "required"` for native tools applies to both paths** — When `useNativeTools` is true and tools are available, `buildChatRequest` sets `tool_choice: "required"`, `temperature` (from model config), and `reasoning_budget` (when configured) on every request, regardless of context type. This prevents thinking-only EOS responses by forcing the LLM to always call a tool.

   Both `reasoning_budget` and `thinking_budget_tokens` are sent in the request for broad provider compatibility. OpenAI ignores the unknown `thinking_budget_tokens` field; llama.cpp reads `thinking_budget_tokens` (server-side enforcement) and ignores the unknown `reasoning_budget` field. See `models/llm_messages.go` `ChatRequest` and `stream.go` `buildChatRequest()`.

   Even when `reasoning_budget` is 0 in the model config, the agent dynamically computes `max_tokens / streamReasoningBudgetDivisor` (3) via `DefaultReasoningBudget()` and syncs it to `a.reasoningBudget`. See `agent.go` `DefaultReasoningBudget()`.

9. **Memory system — see `docs/skills/memory-system.md`** for full architecture: storage, injection, three-tier design, tags, dedup, and known issues.

20. **Agent loop mechanics — see `docs/skills/agent-loop.md`** for: execution flow, sieve, stuck detection, reasoning budget, fallback chain, repetition/spiral detector, and key constants.

21. **Testing — see `docs/skills/testing-guide.md`** for: running smoke tests, analysing run output, record-replay testing, MockClient patterns, common pitfalls.

22. **`maxLength` removed from `filesystem.json` manifest** — The `maxLength` properties were removed because servers enforce it as a grammar constraint, silently truncating content. See `docs/audits/write-file-truncation-cycles.md` for full root-cause analysis.

23. **Unified agent flow — assistant and automation share the same path** — Both assistant conversations and automation tasks use the same `buildChatRequest`, `processStream`, and `handleNoToolCalls` logic. No context-type branching exists in the agent core. Behavioral differences (memory injection) are expressed via `AgentOptions` fields. The `handleNoToolCalls` handler uses a single unified flow: exit on readable text despite parse error (non-native mode), nag on empty or when native-tools protocol is violated. When `useNativeTools` is true and tools are available, the model must produce a tool call — freeform text is treated as a protocol violation and triggers a nag/retry.
