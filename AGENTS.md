# AGENTS.md — Instructions for Coding Agents

This file is written for AI coding assistants (Claude Code, Cursor, Copilot, etc.) working on this codebase. Read it before making any changes.

## Quick Start

Go module root is `backend/`. All `go` commands run from that directory.

```bash
cd backend
go build ./...          # must pass
go vet ./...            # no official linter; this is the sanity check
go test ./...           # must pass
go test ./internal/core/assistant/... -v   # agent loop tests
go test ./internal/core/proxy/... -v       # parser + history tests
go test -tags recordreplay ./internal/core/assistant/... -v  # recording replay tests
go run main.go --data ./data               # start server on :4001
go run main.go --data ./data --record-dir=testdata/recordings  # record mode
```

Go 1.26.2. No golangci-lint, no Makefile, no pre-commit hooks — verification is manual.

## Before You Write Any Code

1. Read `CONSTITUTION.md` — it defines 12 immutable invariants. Your change must comply with all of them.
2. If modifying the agent loop or tool parser, read `docs/SPECS/agent-loop.md` and `docs/SPECS/tool-call-parser.md`.
3. `.agents/rules/` has deeper Go and Vue guidance — check there if a task needs architecture-level patterns.
4. Run `go build ./... && go test ./...` to establish a clean baseline.

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
- `internal/core/assistant/` — Agent loop, tool providers, guardrails, prompts, provider tiers
- `internal/core/proxy/` — LLM HTTP client, XML tool call parser, history normalization
- `internal/core/proxy/recorder/` — `RecordingClient` decorator (captures LLM responses to JSONL)
- `internal/core/llm/` — Model lifecycle (start/stop/reap), GGUF scanning, provider registry
- `internal/core/automation/` — Scheduled task dispatch and execution
- `internal/core/tools/` — Tool implementations (terminal, filesystem, network, search)
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

### Guardrail Decision Flow (`internal/core/assistant/guardrail_decision.go`)
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

8. **Forgetting `tool_choice: "required"` for native tools in automation** — When `useNativeTools` is true and the request is an automation task, the agent sets `tool_choice: "required"`, `temperature: 0.1`, and `reasoning_budget: max_tokens/4` on the `ChatRequest`. This forces the LLM to always call a tool (preventing thinking-only EOS responses) and caps wasted thinking tokens so the model has budget left for the actual tool call. The `omitempty` tags ensure these fields are omitted for XML mode or non-automation contexts.

9. **Reasoning-stuck detection in `processStream`** — When a model generates more than `DefaultReasoningStuckThreshold` (2000) chars of reasoning content without any text output or native tool call deltas, the stream is aborted early. This prevents infinite thinking loops that occur when the server-side `reasoning_budget` is not enforced as a hard cap. The aborted stream triggers the empty-response fallback, which retries non-streaming — a fundamentally different code path that breaks the reasoning loop. See `agent.go` `processStream()`.

10. **Progressive sieve recovery on consecutive stuck events** — On the 1st reasoning-stuck event, the reactive sieve (first 2 + last 6 messages) is applied and a nag prompt ("Stop analyzing, call a tool") is added to the history. On the 2nd consecutive stuck event, an aggressive sieve (first 2 + last 3 messages) is applied with a stronger nag prompt. On the 3rd consecutive stuck event, the agent fails with a clear error ("model stuck in reasoning loop"). This prevents infinite spinning while giving verbose models (Gemma 4) multiple chances to recover.

11. **Context resolution order in `resolveContextLength`** — The priority is `Metadata.Nctx` (serving context from llama.cpp `/slots`) → `Metadata.ContextLength` (training context from GGUF) → `knownCtx` (model name fragment) → `providerCtxDefaults` (per-provider). Forgetting to check `Nctx` first causes the proxy to ignore the detected server context size and fall through to the provider default (128K), resulting in `max_tokens` and `reasoning_budget` that exceed the actual server capacity. See `internal/core/orchestrator/budget_squeezer.go`.

12. **`ApplyMetadataDefaults` sets `ToolCallFormat` to `"native"` when empty** — This matches the UI's "Default" value for cloud provider tiers. Models that need XML text mode must explicitly set `tool_call_format: xml` in the settings.yml override. The stuck-detector fix (Gemini) ensures that models that struggle with native tools gracefully fall back to non-streaming rather than triggering the death spiral. See `internal/core/orchestrator/budget_squeezer.go`.

13. **Settings override `tool_call_format: ""` blocks the default** — When the UI saves "Default" for an existing model, the save handler writes `tool_call_format: ""` to settings.yml. This empty string override takes highest priority and blocks `ApplyMetadataDefaults` from filling in `"native"`. The model silently switches to XML text mode. If you see a model unexpectedly failing with XML parse errors, check settings.yml for `tool_call_format: ""` and either remove it or set it to `"native"`.

## File Change Checklist

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
4. `internal/core/assistant/registry.go` — registration

When adding a prompt:
1. `internal/core/assistant/prompts/templates.go` — ONLY location for prompt text
2. Logic file (agent.go, tool_call_parser.go) — uses the template, never inlines strings

## Test Patterns

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
