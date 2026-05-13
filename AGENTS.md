# AGENTS.md — Instructions for Coding Agents

This file is written for AI coding assistants (Claude Code, Cursor, Copilot, etc.) working on this codebase. Read it before making any changes.

## Quick Start

```bash
cd backend
go build ./...          # must pass
go test ./...           # must pass
go test ./internal/core/assistant/... -v   # agent loop tests
go test ./internal/core/proxy/... -v       # parser + history tests
go run main.go --data ./data               # start server
```

Go module root is `backend/`. All go commands run from that directory.

## Before You Write Any Code

1. Read `CONSTITUTION.md` — it defines 12+ immutable invariants. Your change must comply with all of them.
2. If modifying the agent loop or tool parser, read `docs/SPECS/agent-loop.md` and `docs/SPECS/tool-call-parser.md`.
3. Run `go build ./... && go test ./...` to establish a clean baseline.

## Architecture (What Lives Where)

### Type Definitions
`models/` contains ALL shared types. No logic, only structs, constants, and interfaces.
- `models/config.go` — ModelConfig, ProviderConfig, GPUConfig, AgentGuardrailsConfig
- `models/infrastructure.go` — SystemConfig, UserSettings, ModelOverride
- `models/registry.go` — RegistryData, ModelRegistryEntry
- `models/llm.go` — Provider interface, error sentinels
- `models/llm_messages.go` — Message, ToolCall, ChatRequest
- `models/tools.go` — Tool name constants
- `models/workspace.go` — Workspace, AutomationRun, AgentState

### Core Systems
- `internal/core/assistant/` — Agent loop, tool providers, guardrails, prompts
- `internal/core/proxy/` — LLM HTTP client, XML tool call parser, history normalization
- `internal/core/llm/` — Model lifecycle (start/stop/reap), GGUF scanning, provider registry
- `internal/core/automation/` — Scheduled task dispatch and execution
- `internal/core/tools/` — Tool implementations (terminal, filesystem, network, search)
- `internal/core/mcp/` — MCP client (SSE transport, tool mirroring)

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

## Coding Rules (TypeScript/Vue — Frontend)

- `frontend/` is a Vue 3 + Vite + TypeScript SPA
- Composables are singletons — state is module-level, shared across components
- Use `ref()` for reactive state, not `reactive()`
- Type imports from `types/` directory (barrel exports)
- Services are stateless — API calls only, no local state caching
- Polling uses `mountCount` pattern: ref counts subscribers, stops when zero
- Build output goes to `../backend/internal/transport/http/frontend_dist/` (Go embed)

## Common Pitfalls

1. **Changing an interface without updating mocks** — The `MockManager` in `internal/testing/mocks/` must implement every method of `RuntimeManager`. Build fails on missing methods.

2. **Hardcoding prompt strings in logic files** — All prompts live in `templates.go`. Check there first before writing new ones.

3. **Saving model overrides to registry.json** — Agent tuning fields (`max_steps`, `context_budget`, `tool_call_format`, `prefill`) go to `settings.yml`, NOT `registry.json`. See Constitution III.5.

4. **Modifying history in the normalizer** — `NormalizeHistory()` does role conversion and metadata stripping only. Nag injection and feedback belong in the agent loop (`agent.go`). See Constitution II.8.

5. **Adding new model fields without updating the UI** — New `ModelConfig` fields must be added to:
   - The request structs in `registry_handlers.go` (both add and update)
   - `adminModelView` in `admin_handlers.go`
   - `getModelsView()` mapping in `admin_view.go`
   - Both `runtimeCfg` and `persistCfg` in the handler

6. **Forcing native tools on local models without opt-in** — Local models default to XML text mode. A model can opt into native via `ToolCallFormat: "native"`, but don't change the default. The XML parser is the fallback.

7. **Removing the XML parser** — Even with native tools enabled globally, the XML parser must remain as fallback for non-function-calling responses. Both paths coexist.

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
- Use `mocks.NewMockManager()` for manager-related tests
- Test guardrail decisions with `GuardrailDecisionStore`
- Always assert both success AND error paths
