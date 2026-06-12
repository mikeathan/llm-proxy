# AGENTS.md — Instructions for Coding Agents

This file is written for AI coding assistants (Claude Code, Cursor, Copilot, etc.) working on this codebase. Read it before making any changes.

## Git Policy

Never run any git operation (stash, commit, add, unstage, reset, push, pull, branch, etc.) without asking first. The user has full control over git. If you need git for any reason (e.g., checking baseline test behavior), ask before proceeding.

## Quick Start

See [`CLAUDE.md`](CLAUDE.md) for build commands and [`docs/INDEX.md`](docs/INDEX.md) for documentation navigation.

Go module root is `backend/`. All `go` commands run from that directory.
Go 1.26.2. No golangci-lint, no Makefile, no pre-commit hooks — verification is manual.

## Before You Write Any Code

1. Read `CONSTITUTION.md` — it defines 6 architectural sections covering validation boundaries, system prompt format, model architecture, terminal/network safety, no telemetry, and abstraction/invariants. Your change must comply with all of them.
2. Read the relevant SPEC file(s) for the subsystem you are modifying. See [`docs/INDEX.md`](docs/INDEX.md) for the mapping of subsystems to SPEC IDs (SPEC-001 through SPEC-008).
3. `.agents/rules/` has deeper Go and Vue guidance — check there if a task needs architecture-level patterns.
4. See [`docs/INDEX.md`](docs/INDEX.md) for the full documentation catalog.
5. Run `go build ./... && go test ./...` to establish a clean baseline.

**Documentation stewardship**: After any change (new feature, refactor, behavior fix, or revert), update all affected docs:
- **SPEC files** (`docs/SPECS/`) — update behavioral contracts if the change alters system behaviour
- **Plan files** (`docs/PLANS/`) — add a new entry documenting what changed and why, organized by subsystem
- **Skill files** (`docs/skills/`) — add new gotchas, patterns, or architecture decisions discovered during the work
- **INDEX** (`docs/INDEX.md`) — add entries for any new files created; update statuses for changed plans
- **Audits** (`docs/audits/`) — create a new audit for any regression or post-mortem analysis
- **AGENTS.md** — update Common Pitfalls if a new pattern emerges that future agents should know

## Architecture (What Lives Where)

See [`CLAUDE.md`](CLAUDE.md) for the full architecture map.

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

- `internal/core/assistant/` — Agent loop, tool providers, guardrails, prompts, provider tiers
- `internal/core/proxy/` — LLM HTTP client, XML tool call parser, history normalization
- `internal/core/proxy/recorder/` — `RecordingClient` decorator (captures LLM responses to JSONL)
- `internal/core/llm/` — Model lifecycle (start/stop/reap), GGUF scanning, provider registry
- `internal/core/automation/` — Scheduled task dispatch and execution
- `internal/core/tools/` — Tool implementations (terminal, filesystem, network, search, memory)
- `internal/core/mcp/` — MCP client (SSE transport, tool mirroring)
- `internal/testing/llmprofiles/` — `FixtureClient` + `RunAgainstFixtures` (replay test framework)

### Infrastructure

- `internal/platform/storage/` — Generic atomic JSON/YAML stores with change callbacks
- `internal/platform/logging/` — Structured logging (global + per-workspace process logs)
- `internal/app/` — Bootstrap, AppContext (central state manager), service wiring
- `internal/transport/http/` — All HTTP handlers + embedded frontend

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

## Coding Rules (Go)

### Comments

- **No comments unless the WHY is non-obvious.** Well-named identifiers document the WHAT.
- **Single-line only.** No multi-line docstrings or comment blocks.
- **Never remove existing comments unless they are stale** (referencing removed code, outdated behavior, or incorrect logic). If a comment is still accurate, keep it.
- If removing the comment wouldn't confuse a reader, remove it.

### Error Handling

- Validate at system boundaries (user input, external APIs) — trust internal code.
- Use `fmt.Errorf` with `%w` to wrap errors and maintain the chain.
- Use sentinel errors from `models/llm.go` for known conditions (`ErrUnknownModel`, `ErrModelExists`, `ErrModelStarting`).

### Abstraction

- Don't DRY until the pattern repeats 3+ times. Three similar lines > premature abstraction.
- Don't add features, refactor, or introduce abstractions beyond what the task requires.
- No feature flags, backward-compat shims, or `// TODO` stubs.

### Prompts

- ALL prompt strings go in `internal/core/assistant/prompts/templates.go`. Nowhere else.
- This includes system messages, nag prompts, parse-error feedback, JSON translations.

### Network & Terminal

- All network I/O via `NetworkTools` (never raw `http.Client` or `net.Dial`).
- All terminal execution via `ShellProvider` (never raw `os/exec`).
- All file paths validated with `IsSecurePath` for workspace jailing.

### Cyclomatic Complexity & Readability

- Keep functions short and focused: limit any function to a maximum of 80 lines. If a function grows larger, extract sub-logic into small, well-named helper functions.
- Keep cyclomatic complexity under 10 per function. Avoid nested conditionals deeper than 3 levels; instead, structure flow with early returns and guard clauses ("happy path to the left").
- Encapsulate transient loop or session state in temporary structs (e.g. `agentRunner`) instead of passing multiple pointers to simple type counters (like `*int`, `*bool`) between functions.
- Decouple orchestration logic from data parsing/formatting. Put parsing and validation logic in dedicated helper types/functions.

## Coding Rules (TypeScript/Vue — Frontend)

- `frontend/` is a Vue 3 + Vite + TypeScript SPA
- Composables are singletons — state is module-level, shared across components
- Use `ref()` for reactive state, not `reactive()`
- Type imports from `types/` directory (barrel exports)
- Services are stateless — API calls only, no local state caching
- Polling uses `mountCount` pattern: ref counts subscribers, stops when zero
- Dev server at `localhost:5173` proxies `/admin/api` to `:4001` — start backend first
- Build output goes to `../backend/internal/transport/http/frontend_dist/` (Go embed)
- `npm run build` runs `vue-tsc -b` (type-check) then `vite build` — TS errors fail the build
- Model form defaults and derived names live in `src/utils/modelUtils.ts` — reusable across components

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

6. **Forcing native tools on local models without opt-in** — Local models default to XML text mode. A model can opt into native via `ToolCallFormat: "native"`, but don't change the default. The XML parser is the fallback.

7. **Removing the XML parser** — Even with native tools enabled globally, the XML parser must remain as fallback for non-function-calling responses. Both paths coexist.

8. **Forgetting `tool_choice: "required"` for native tools in automation** — When `useNativeTools` is true and the request is an automation task, the agent sets `tool_choice: "required"`, `temperature: 0.1`, and `reasoning_budget: max_tokens/3` on the `ChatRequest`. This forces the LLM to always call a tool (preventing thinking-only EOS responses) and caps wasted thinking tokens so the model has budget left for the actual tool call. The `omitempty` tags ensure these fields are omitted for XML mode or non-automation contexts.

   Both `reasoning_budget` and `thinking_budget_tokens` are sent in the request for broad provider compatibility. OpenAI ignores the unknown `thinking_budget_tokens` field; llama.cpp reads `thinking_budget_tokens` (server-side enforcement) and ignores the unknown `reasoning_budget` field. See `models/llm_messages.go` `ChatRequest` and `stream.go` `prepareChatRequest()`.

   Even when `reasoning_budget` is 0 in the model config, the agent **dynamically computes** `max_tokens/3` in `prepareChatRequest()` and syncs it to `a.reasoningBudget`. See `stream.go` `prepareChatRequest()`.

9. **Reasoning-stuck detection in `processStream`** — When a model generates reasoning content without any text output or native tool call deltas exceeding the derived threshold, the stream is aborted early. The threshold is `maxTokens * 2` chars, floored at `MinReasoningStuckThreshold` (2000 chars). This makes the stuck check a **safety net** for servers that don't enforce reasoning budgets — models with enforced budgets (via `reasoning_budget = maxTokens/3`) will exhaust their allocated thinking tokens before the stuck threshold fires. For models with `reasoningBudget == 0` (no server-side enforcement), an additional earlier check fires at `maxTokens / stuckNonReasoningDivisor` chars. Current divisor=1 gives threshold at `maxTokens` — catches stuck 2x faster than the baseline while avoiding false positives on models (e.g. Gemma 4) that produce legitimate reasoning content without an explicit budget. See `stream.go` `stuckNonReasoningDivisor` and `checkStreamStuck()`, and `docs/audits/write-file-truncation-cycles.md` for the tuning history. A `lifecycle` event with phase `stuck_detected` is emitted to the UI. See `stream.go` `stuckThreshold()`.

    **Do NOT change `streamReasoningBudgetDivisor` from 3.** Divisor 4 was tried in production and caused recompilation loops at turn 18+: 682 tokens was too few for the model to plan its next step in a 40+ message history. Divisor 3 (910 tokens) was the minimum that eliminated the loops. Higher divisors waste the output budget; lower divisors cause mid-reasoning cutoff. The smoke test covers this — run it before and after any change. See `docs/audits/memory-injection-investigation.md`.

   Some models (e.g. Qwen 3.5) emit `<tool_call>` blocks inside `<think>` reasoning content. Before declaring stuck, `processStream` scans the accumulated reasoning content for embedded `<tool_call>` blocks. If found, `<think>` tags are stripped and the cleaned content is promoted to `fullMsg.Content` so the XML tool call parser can process it downstream. This prevents a false stuck detection when the tool call is invisible to the native-tool parser. See `agent.go` `toolCallInContent` regex and `cleanReasoningContent()`.

10. **Progressive sieve recovery on consecutive stuck events** — On the 1st reasoning-stuck event, the reactive sieve (first 2 + last 6 messages) is applied and a nag prompt ("Stop analyzing, call a tool") is added to the history. On the 2nd consecutive stuck event, an aggressive sieve (first 2 + last 3 messages) is applied with a stronger nag prompt. On the 3rd consecutive stuck event, the agent fails with a clear error ("model stuck in reasoning loop"). This prevents infinite spinning while giving verbose models (Gemma 4) multiple chances to recover. Note: the stuck detection triggers a retry, not a fail — progressive sieve catches repeated failures.

11. **Context compression in `applyPhysicalSieve`** — Before dropping old messages when context budget is exceeded, the sieve first compresses long `Content` (>4000 chars) and `ReasoningContent` (>2000 chars) in older messages by truncating to head+tail with a `...[Truncated]...` marker. Only if compression isn't enough are messages dropped. This preserves message structure while reducing token consumption. See `agent.go` `truncateLongContent()`.

12. **Native tools empty-stream fallback depends on model format** — When streaming with native tools returns empty (no content, no tool calls, only reasoning), the fallback depends on `usePrefill`:
    - **Native-only models** (`usePrefill=false`, e.g. OpenAI-compatible models): skips the XML retry entirely, goes directly to non-streaming Chat + nag prompt. The XML retry would waste ~60s with stuck detection disabled (the model can't produce `<tool_call>` blocks).
    - **XML-text models** (`usePrefill=true`, e.g. local models): retries via XML streaming with `useNativeTools` temporarily disabled, `tool_choice` suppressed, `reasoningBudget` suppressed, and stuck detection skipped. If the XML stream also returns empty, falls back to non-streaming Chat. See `agent.go` `computeNextResponseStreamXML()`.

    When normalizing history for XML mode, `NormalizeHistory()` converts native tool calls to `<tool_call>` XML blocks matching the system prompt format (not "Called tool: X" text). This ensures the model sees consistent format examples in its conversation history. See `history.go` `NormalizeHistory()`.

13. **Lifecycle events for UI progress** — The agent emits `lifecycle` events with a `phase` field for structured progress reporting: `stuck_detected` (with `reasoning_chars`), `fallback_started` (with `reason` and `mode`), `fallback_waiting` (with `elapsed` time), and `fallback_completed`. These are appended as system messages in the frontend, never overwriting assistant streaming content. The non-streaming heartbeat (`computeNextResponseNonStreaming`) uses `fallback_waiting` with elapsed time instead of the old `tool_stream` overwrite. See `agent_events.go` `notifyLifecycle()`.

14. **Goroutine leak fix in `processStream` heartbeat** — The 30-second heartbeat goroutine now selects on a `streamDone` channel (closed via `defer`) in addition to `ctx.Done()`. This ensures the goroutine exits when `processStream` returns for ANY reason (stuck detection, stream EOF, errors), not just context cancellation. Prevents misleading "stream still generating" log lines after the stream has ended. See `agent.go` `processStream()` and `docs/skills/agent-loop.md`.

15. **Context resolution order in `resolveContextLength`** — The priority is `Metadata.Nctx` (serving context from llama.cpp `/slots`) → `Metadata.ContextLength` (training context from GGUF) → `knownCtx` (model name fragment) → `providerCtxDefaults` (per-provider). Forgetting to check `Nctx` first causes the proxy to ignore the detected server context size and fall through to the provider default (128K), resulting in `max_tokens` and `reasoning_budget` that exceed the actual server capacity. See `internal/core/orchestrator/budget_squeezer.go`.

16. **`ApplyMetadataDefaults` sets `ToolCallFormat` to `"native"` when empty** — See `docs/skills/agent-loop.md` (hook system) and `internal/core/orchestrator/budget_squeezer.go`.

17. **Settings override `tool_call_format: ""` blocks the default** — When the UI saves "Default" for an existing model, the save handler writes `tool_call_format: ""` to settings.yml. This empty string override takes highest priority and blocks `ApplyMetadataDefaults` from filling in `"native"`. The model silently switches to XML text mode. If you see a model unexpectedly failing with XML parse errors, check settings.yml for `tool_call_format: ""` and either remove it or set it to `"native"`.

18. **Memory system — see `docs/skills/memory-system.md`** for full architecture: storage, injection, three-tier design, tags, dedup, and known issues.

19. **Agent loop mechanics — see `docs/skills/agent-loop.md`** for: execution flow, sieve, stuck detection, reasoning budget, fallback chain, repetition/spiral detector, and key constants.

20. **Testing — see `docs/skills/testing-guide.md`** for: running smoke tests, analysing run output, record-replay testing, common pitfalls.

21. **`maxLength` removed from `filesystem.json` manifest** — The `maxLength` properties were removed because servers enforce it as a grammar constraint, silently truncating content. See `docs/audits/write-file-truncation-cycles.md` for full root-cause analysis.

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

### File Change Checklist

When adding a new model-level field, update these files:

1. `models/config.go` or `models/infrastructure.go` — type definition
2. `internal/transport/http/registry_handlers.go` — add/update request structs
3. `internal/transport/http/admin_handlers.go` — view struct
4. `internal/transport/http/admin_view.go` — view mapping
5. `internal/core/llm/manager.go` — if field affects runtime behavior
6. `internal/testing/mocks/manager.go` — if interface changed
7. Frontend component (if UI field)

When adding a tool:

1. `models/tools.go` — constant
2. `internal/core/tools/manifests/{tool}.json` — manifest (embedded)
3. `internal/core/tools/{tool_category}.go` — implementation
4. `internal/core/assistant/registry.go` — registration (add field to `LocalToolRegistry`, add `register{Category}Tools()`, call from `registerAll()`, add `init{Category}Tools()` helper, wire in `InitializeAgentStack`)

When adding a prompt:

1. `internal/core/assistant/prompts/templates.go` — ONLY location for prompt text
2. Logic file (agent.go, tool_call_parser.go) — uses the template, never inlines strings

### New Backend Endpoint Checklist

1. Handler in `internal/transport/http/{thing}_handlers.go`
2. Route registration (in `router.go` or `main.go` route setup)
3. Types/response structs in the same handler file
4. Frontend: endpoint constant in `api.ts`
5. Frontend: service method in `{thing}Service.ts`
6. Frontend: types in `types/{thing}.ts`
7. Frontend: composable in `composables/use{Thing}.ts`
8. Frontend: view component in `components/{thing}/{Thing}.vue`
