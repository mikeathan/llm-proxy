# LLM Proxy — Project Guide

## Quick Reference

### Build & Test
```bash
cd backend
go build ./...          # build all packages
go test ./...           # run all tests
go test ./internal/core/assistant/... -v   # agent-loop tests
go test ./internal/core/proxy/... -v       # parser + history tests
go run main.go          # start the server (default :4001)
```

Go module root is `backend/`. All go commands run from there.

### Frontend
```bash
cd frontend
npm install             # install dependencies
npm run dev             # dev server (proxies API to :4001)
npm run build           # production build → ../backend/internal/transport/http/frontend_dist/
```

The frontend is embedded in the Go binary via `//go:embed all:frontend_dist`. After `npm run build`, rebuild the backend and restart to see UI changes.

### Start a Production Server
```bash
cd backend
go run main.go --data ./data
# Listens on http://0.0.0.0:4001
# Admin UI at http://localhost:4001/admin/
# OpenAI-compatible API at http://localhost:4001/v1/chat/completions
```

---

## Architecture: Three-Tier Configuration

```
Tier 1: config.json         — System infrastructure (host, ports, timeouts, workspace dir)
Tier 2: settings.yml        — User preferences (local paths, guardrails, per-model agent tuning)
Tier 3: registry.json       — Dynamic state (models, providers, MCP servers, primary/fallback)

+ Workspace overrides: ~/.config/llm-proxy/{workspace}/config.yaml — per-workspace guardrails + automations
```

**Persistence locations:**
| Data | File | Location |
|------|------|----------|
| System config | `data/config.json` | Relative to `--data` flag |
| User settings | `settings.yml` | `~/.config/llm-proxy/` (XDG) or `data/.internal/` (fallback) |
| Model registry | `data/registry.json` | Relative to `--data` flag |
| Encrypted secrets | `data/secrets.json` | Relative to `--data` flag |
| Workspace state | `data/workspaces/{id}/` | Run files, heartbeat, process logs |
| Workspace config | `{metadata}/{id}/config.yaml` | Guardrails, automations per workspace |
| Templates | `data/templates/` | Task playbook templates |

**Model overrides** — Agent tuning fields (max_steps, context_budget, max_tokens, tool_call_format, prefill) are saved to `settings.yml` under `model_overrides:`, NOT to `registry.json`. At startup, overrides are merged onto the base model catalogue from the registry. See invariant #11.

---

## Directory Map

```
backend/
├── main.go                          — Entry point: flags, env, logging, data init, server start
├── models/                          — All shared type definitions (no logic)
│   ├── config.go                    — Config, ModelConfig, AgentGuardrailsConfig, ProviderConfig, GPUConfig
│   ├── infrastructure.go            — SystemConfig, UserSettings, LocalSettings, ModelOverride
│   ├── registry.go                  — RegistryData, ModelRegistryEntry, ProviderRegistryEntry
│   ├── llm.go                       — Provider interface, error sentinels
│   ├── llm_messages.go              — Message, ChatRequest, ToolCall, FunctionCall, ChatRole
│   ├── secrets.go                   — SecretsStore interface, SecretData
│   ├── tools.go                     — All tool name constants
│   ├── workspace.go                 — Workspace, AutomationRun, AgentState, WorkspaceConfig
│   ├── templates.go                 — TemplateMetadata, Template
│   └── provider_manifest.go         — ProviderManifest
├── utils/                           — Clock, URL/net helpers, encoding
└── internal/
    ├── app/                         — Application lifecycle
    │   ├── app.go                   — App struct, New(), InitializeData(), startup/shutdown
    │   ├── app_context.go           — AppContext: central state manager, persistence operations
    │   └── bootstrap.go             — bootstrap(): wires all services, builds HTTP router
    ├── buildinfo/                   — Version, Commit, BuildDate
    ├── core/
│   ├── assistant/               — Agent loop and tool orchestration
│   │   ├── agent.go             — Agent struct, Execute(), processToolCalls(), computeNextResponse()
│   │   ├── agent_events.go      — AgentEvent type, Observer interface, event constants
│   │   ├── engine.go            — Engine interface (ExecuteTool)
│   │   ├── registry.go          — LocalToolRegistry, InitializeAgentStack(), tool registration
│   │   ├── tiers.go             — ProviderTuningDefaults, ProviderTiers() — per-provider agent tuning defaults
│   │   ├── tool_provider.go     — ToolProvider interface, MultiToolProvider, CompositeEngine
    │   │   ├── guardrail_decision.go — GuardrailDecisionStore, NewGuardrailDecisionCallback
    │   │   ├── content_filter.go    — FilterStreamingMarkup (hides <tool_call> from UI during streaming)
    │   │   ├── guardrails/
    │   │   │   └── guardrails.go    — GuardrailEngine, ValidateToolCall(), PersistOverride()
    │   │   └── prompts/
    │   │       ├── templates.go     — ALL prompt strings (nags, feedback, system, rules) — SINGLE SOURCE OF TRUTH
    │   │       └── system_prompt.go — BuildSystemMessage (assembles tool manual + rules)
    │   ├── automation/               — Scheduled/manual task execution
    │   │   ├── dispatcher.go        — Dispatcher: cron-based scheduler, workspace management
    │   │   ├── executor.go          — TaskExecutor interface, LLMTaskExecutor
    │   │   ├── broadcast.go         — EventBus (per-workspace pub/sub for SSE streaming)
    │   │   ├── registry.go          — AutomationRegistry
    │   │   ├── strategies.go        — IsolatedStrategy (clean each run), PersistentStrategy (state continuity)
    │   │   └── trigger.go           — CronTrigger, IntervalTrigger, ManualTrigger
    │   ├── llm/                     — Model lifecycle management
    │   │   ├── manager.go           — RuntimeManager interface, LLMRuntimeManager, NewManagerFromRegistry()
    │   │   ├── lifecycle.go         — Start, stop, idle-reap model processes
    │   │   ├── enricher.go          — normalizeModelConfig, environment injection
    │   │   ├── metadata/
    │   │   │   └── gguf_scanner.go  — GGUF header scanning (fast, header-only, not full file read)
    │   │   └── providers/
    │   │       ├── registrar.go     — ProviderRegistrar (local + cloud provider management)
    │   │       ├── registry.go      — ProviderRegistry singleton (loads definitions/*.json)
    │   │       ├── local_provider.go — LocalProvider: llama.cpp process lifecycle
    │   │       ├── provider_openai_compatible.go — OpenAI-compatible provider
    │   │       ├── provider_gemini.go — Gemini provider
    │   │       └── provider_vertex.go — Vertex AI provider
    │   ├── mcp/                     — Model Context Protocol integration
    │   │   ├── orchestrator.go      — Manages multiple MCP client connections
    │   │   ├── client.go            — SSE-based MCP client (mark3labs/mcp-go)
    │   │   ├── nodeherder_adapter.go — Bridge: MCP → ToolProvider + MCPService
    │   │   └── resource_mirror.go   — Cached system prompt from MCP server
    │   ├── nodeherder/
    │   │   └── provider.go          — MCPService interface
    │   ├── proxy/                   — LLM HTTP client + text processing
    │   │   ├── client.go            — Client interface, LLMClient (Chat + Stream/SSE)
    │   │   ├── provider.go          — LLMClientProvider, RuntimeClientProvider (failover logic)
    │   │   ├── tool_call_parser.go  — ParseContentToolCalls(), sanitizeJSON(), ParseError
    │   │   ├── history.go           — NormalizeHistory(), NormalizeContextSieve(), SanitizeHistory()
    │   │   ├── message.go           — Type aliases for LLM wire protocol
    │   │   └── utils.go             — TruncateResult(), TruncateLines()
    │   └── tools/                   — Agent tool implementations
    │       ├── terminal.go          — TerminalTools: command execution with jailed shell sessions
    │       ├── filesystem.go        — FileSystemTools: list, read, write, append (path validation + extension whitelist)
    │       ├── network.go           — NetworkTools: fetch_url, scan_local_network, get_network_info
    │       ├── search.go            — InternetTools: web search via Tavily API
    │       ├── communication.go     — CommunicationTools: notify_user (Telegram, etc.)
    │       ├── security.go          — IsSecurePath(), path jail prevention
    │       └── manifests.go         — ToolManifest loader, GetDefaultGuardrails(), embed.FS for manifests/
    ├── platform/                    — Cross-cutting infrastructure
    │   ├── logging/                 — Logger interface + FileLogger, BufferLogger, PulseLogger, NopLogger
    │   ├── storage/                 — Generic atomic Store[T], DataManager, crypto, secrets
    │   │   ├── store.go             — Generic JSON/YAML atomic file store with OnChange callbacks
    │   │   ├── manager.go           — DataManager: orchestrates all stores + fsnotify watcher
    │   │   ├── secrets_store.go     — AES-256-GCM encrypted secret storage
    │   │   └── resolver.go          — PathResolver for metadata/workspace directories
    │   ├── persistence/
    │   │   └── workspace.go         — WorkspaceManager: flock-locked atomic state/config I/O
    │   ├── network/                 — IP/address utilities (DNS rebinding protection)
    │   ├── metrics/                 — GPU/CPU/token metrics collection
    │   └── ratelimiter/             — Rate limiter
    ├── shell/                       — Persistent shell session management
    │   ├── shell.go                 — Terminal + ShellProvider interfaces
    │   └── terminal.go              — HostShellManager, persistentShell, ShellSession, prepareShellEnv()
    ├── testing/
    │   ├── mocks/                   — MockManager, MockAdminService, mock LLM providers, etc.
    │   └── utils/                   — Test utilities
    └── transport/http/              — HTTP API + embedded frontend
        ├── router.go                — Custom HTTP router with method dispatch
        ├── services.go              — RuntimeService, AdminService, AssistantService interfaces
        ├── admin_handlers.go        — AdminHandlers: state, start/stop, config CRUD
        ├── admin_view.go            — Response view construction (getModelsView, getProvidersView)
        ├── registry_handlers.go     — Model add/update/delete handlers
        ├── assistant_handlers.go    — Chat API + GuardrailDecisionHandler
        ├── proxy_handlers.go        — Reverse proxy to LLM (OpenAI-compatible /v1/chat/completions)
        ├── dispatcher_handlers.go   — Automation/workspace management + SSE streaming
        ├── system_handlers.go       — System config, restart, host settings
        ├── secrets_handlers.go      — API key / tool secret CRUD
        ├── admin_template_handlers.go — Template CRUD
        ├── process_handlers.go      — Process log endpoints
        └── http_internal.go         — JSON decode/respond helpers
```

---

## Critical Invariants (DO NOT VIOLATE)

These are the architectural rules. Every change must comply. If a task requires violating an invariant, halt and request a formal override.

### 1. XML Tool Call Format (Parser)
Parser accepts only: `<tool_call>{"tool":"name","args":{...}}</tool_call>`
- No naked JSON, no markdown fences, no greedy fallbacks
- Rejected formats get specific `ParseError.Feedback()` (defined in `templates.go`)
- `sanitizeJSON()` in `tool_call_parser.go` handles: Python booleans/None, markdown fences, trailing commentary, invalid escapes

### 2. Tool Role Conversion (History)
- When `useNativeTools=false`: `tool` role → `user` role with `tool_call_id` in content (`Tool result [call_N]: <json>`)
- When `useNativeTools=true`: tool roles pass through as-is
- `SanitizeHistory()` preserves: `role`, `content`, `tool_calls`, `tool_call_id`
- This avoids Jinja template errors in llama.cpp while preserving call/result association

### 3. No Auto-Nags in Normalization
Nag injection belongs in the agent loop (`agent.go`), NOT in `NormalizeHistory()`. Nags are: prompt corrections when the model produces malformed tool calls, duplicate detection, feedback injection.

### 4. Per-Model Config Flow
`ModelConfig.MaxSteps`, `MaxTokens`, `ContextBudget`, `ToolCallFormat`, `Prefill` are:
- Read by the executor from `Runtime.ListModels()`
- Passed to `AgentOptions` when creating the Agent
- The agent uses them directly (no global defaults override per-model settings)
- Provider-tier defaults defined in `assistant/tiers.go` (`ProviderTiers()`) set baseline values for each provider type (local, gemini, openai, etc.) and are exposed via the admin API for the frontend to prefill model forms

### 5. Repetition Detection Survives Context Sieve
- `recentCalls` is never reset on context pruning
- `duplicateStreak` detector catches infinite loops across sieve boundaries
- After 3 consecutive identical (tool + args) calls, inject a duplicate nag
- After 5+, abort with "infinite loop detected"

### 6. Exit Is Explicit
- **Automation mode**: `submit_final_answer` is the ONLY canonical exit
- **Chat mode**: exits on tool-result→assistant-reply cycle, 2 consecutive assistant messages with no tool calls, or premature termination (empty/repeated output)
- NO heuristic keyword matching ("task complete", "summary" — these are NOT valid exit signals)

### 7. Native Tools
- Cloud models (OpenAI, Gemini): native tool schemas when `UseNativeTools()` returns true
- Local models: default to text/XML; can opt into native via `ModelConfig.ToolCallFormat = "native"`
- The HTTP client does NOT strip tools — the agent controls this decision
- When native tools are enabled, XML parser still runs as fallback for non-function-calling responses
- Automation mode with native tools sends `tool_choice: "required"`, `temperature: 0.1`, and
  `reasoning_budget: max_tokens/3` on the `ChatRequest` to force the model to always call a tool
  (preventing thinking-only EOS responses) and cap wasted thinking tokens so the model has budget
  left for the actual tool call. These fields are omitted for XML mode or non-automation contexts
  via `omitempty`.

### 8. Prompts Centralized (SINGLE SOURCE OF TRUTH)
`internal/core/assistant/prompts/templates.go` is the ONLY location for ALL prompt strings:
- System messages, nag prompts, parse-error feedback, JSON error translations
- Escalation prefixes, protocol instructions
- No hardcoded prompt strings anywhere else — not in `agent.go`, not in `tool_call_parser.go`, not in any other file

### 9. Network Mediated
All network interaction passes through `NetworkTools`. Raw `http.Client` or `net.Dial` is prohibited (Constitution I). This ensures DNS rebinding protection and boundary checks.

### 10. GGUF Parsing
Metadata extraction uses the authorized GGUF parsing library. No filename-based regex extraction. The scanner reads only the GGUF header (KB, not GB) using SkipLargeMetadata+MMap.

### 11. Model Persistence (Two-Tier)
- **Base model info** (name, provider, filename, port, args, credential_id) → `registry.json`
- **Agent tuning overrides** (max_steps, context_budget, max_tokens, tool_call_format, prefill) → `settings.yml` under `model_overrides:`
- On save: both updated simultaneously
- On load: registry.json base + settings.yml overrides merged at startup in `NewManagerFromRegistry()`
- On settings change: `ApplyModelOverrides()` re-applies overrides to runtime models
- Never put model overrides directly in registry.json

### 12. Guardrail Decision Flow
When a tool call is blocked:
1. `ValidateToolCall()` fails → creates `GuardrailBlockedPayload` with decision_id
2. `onGuardrail` callback registers a channel in `GuardrailDecisionStore` + publishes SSE event
3. Agent blocks on channel (60s timeout)
4. User approves/denies via `POST /admin/api/conversation/guardrail-decision`
5. If approved with `persist: true` → `PersistOverride()` writes to workspace `config.yaml`
6. Agent continues or fails based on decision

---

## Agent Loop: Step-by-Step

See [`docs/SPECS/agent-loop.md`](docs/SPECS/agent-loop.md) (SPEC-001) for the authoritative specification and `docs/skills/agent-loop.md` for the execution flow, sieve algorithm, stuck detection, and reasoning budget details.

The agent loop runs in `internal/core/assistant/agent.go`, method `Execute()` (line ~97).

### Content Filtering During Streaming
The `FilterStreamingMarkup()` function in `content_filter.go` hides `<tool_call>` patterns from UI display during streaming. This prevents the user from seeing partial, un-executed tool calls. Complete tool calls are only shown after execution.

---

## Tool System

### Available Tools (defined in `models/tools.go`)
| Constant | Name | Category | Description |
|----------|------|----------|-------------|
| `ToolTerminalExecute` | `execute_terminal_command` | terminal | Run shell commands in jailed session |
| `ToolListDirectory` | `list_directory` | filesystem | List directory contents |
| `ToolFileRead` | `read_file` | filesystem | Read file contents |
| `ToolFileWrite` | `write_file` | filesystem | Write/create files. No server-side length enforcement — model is self-limited by `max_tokens`. For large content, use `append_file` to add more. |
| `ToolFileAppend` | `append_file` | filesystem | Append content to existing file. No server-side length enforcement. |
| `ToolFileEditBlock` | `edit_file_block` | filesystem | Replace exact block of text in existing file. Trailing whitespace normalized for matching. Error on multiple matches — include more context to disambiguate. |
| `ToolNetworkFetch` | `fetch_url` | network | HTTP GET request |
| `ToolNetworkScan` | `scan_local_network` | network | Scan LAN for devices |
| `ToolNetworkInfo` | `get_network_info` | network | Get local IP/subnet |
| `ToolSearchInternet` | `internet_search` | search | Web search via Tavily |
| `ToolNotifyUser` | `notify_user` | communication | Send notification |
| `ToolSubmitFinalAnswer` | `submit_final_answer` | system | Mark task complete, exit agent |
| `ToolSystemError` | `system_error` | system | System-level error feedback |

### Tool Registration Flow
1. Tool instances created in `InitializeAgentStack()` (`registry.go`)
2. Each tool loads its manifest from `internal/core/tools/manifests/*.json` (embedded)
3. Manifests define: description, parameters schema, default guardrails
4. Tools are registered in `LocalToolRegistry.registerAll()`
5. Local tools + MCP tools are aggregated via `MultiToolProvider`
6. Engine chain: `CompositeEngine(localRegistry, mcpEngine)` — tries local first, falls back to MCP

### Tool Call Format (XML)
```xml
<tool_call>{"tool":"tool_name","args":{"key":"value"}}</tool_call>
```
- JSON object with `tool` (string) and `args` (object) fields
- Multiple tool calls can appear in a single message
- Each is independently parsed and executed
- After extraction, the XML tags are removed from message content

---

## Guardrail System

### Validation Hierarchy
1. **Global**: Secret pattern detection (API keys), user-defined blocked patterns (regex)
2. **Terminal**: Command whitelist, blocked patterns, path jail prevention, timeout enforcement, external path access (workspace-level only)
3. **Filesystem**: Path validation, extension whitelist, filename blocking, read-only enforcement, path jail
4. **Network**: LAN/Internet boundary, domain blocking, IP blocking
5. **Search**: Query length limits, site blocking
6. **Communication**: Review requirement, message limits

### Override Stack (highest priority last)
1. Provider Manifests (embedded defaults from `manifests/*.json`)
2. `settings.yml` → `guardrails:` (user-level overrides)
3. `{workspace}/config.yaml` → `guardrails:` (workspace-level overrides)

Merging is done via `AgentGuardrailsConfig.MergeWith()` which ORs booleans, overrides non-zero ints, and merges slices with dedup.

### Guardrail Decision (Approval) Flow
When a tool call is blocked, the agent does NOT silently fail:
1. Decision ID generated, stored in `GuardrailDecisionStore`
2. `EventGuardrailBlocked` published to SSE → frontend shows approval banner
3. Agent blocks up to 60s waiting for user
4. User clicks: Allow & Remember / Allow Once / Deny
5. Decision resolved → agent continues or fails
6. If "Allow & Remember": override persisted to workspace `config.yaml`

### External Path Access (Terminal)
`TerminalGuardrailsConfig.AllowedExternalPaths` lets a workspace-level override grant the agent access to absolute paths outside the workspace jail (e.g. `/home/user/projects/other`). Constraints:
- **Workspace-level only** — cannot be set in global `settings.yml` guardrails (the field exists but global config isn't auto-applied to workspace jails)
- Agents in workspaces with non-empty `allowed_external_paths` see absolute-path and `..` traversal checks relaxed to permit the listed roots (validated via `IsSecurePath`)
- The frontend shows amber hazard indicators: a warning dot on the workspace explorer, a hazard border on the guardrail form textarea, and a banner in workspace settings
- `HasExternalAccess()` on `TerminalGuardrailsConfig` returns true when any external paths are configured — used by callers that need a quick safety check
- Merged via `MergeWith()` alongside other slice fields (deduplicated union)

---

## HTTP API Reference

### Admin Endpoints (all under `/admin/api/`)
| Method | Path | Purpose |
|--------|------|---------|
| GET | `/state` | Full admin state (models, config, guardrails, providers) |
| POST | `/start` | Start active model |
| POST | `/stop` | Stop active model |
| POST | `/models` | Add model |
| PUT | `/models` | Update model |
| DELETE | `/models?name=X` | Remove model |
| GET | `/config` | Get global config |
| PUT | `/config` | Update global config |
| POST | `/system/restart` | Restart system |
| GET/PUT | `/host` | Host machine settings |
| POST | `/host/terminal/reset` | Reset terminal session |
| GET | `/logs` | Process logs |
| GET | `/app-logs/tail` | Application log tail |
| GET/PUT | `/log-level` | Log level management |
| GET | `/metrics` | System metrics (CPU, GPU, tokens/s) |
| GET/POST/PUT/DELETE | `/mcp` | MCP server CRUD |
| GET | `/providers/models` | List provider models |
| GET | `/providers/manifests` | Provider manifest list |
| GET | `/providers/test` | Test provider connection |
| GET/PUT/DELETE | `/secrets/keys` | Provider API key management |
| GET/PUT | `/secrets/tools` | Tool secrets (Tavily, Telegram) |
| GET | `/templates` | List playbook templates |
| GET | `/templates/:id` | Get template content |

### Conversation Endpoints
| Method | Path | Purpose |
|--------|------|---------|
| POST | `/conversation/message` | Send chat message to agent |
| POST | `/conversation/guardrail-decision` | Submit guardrail approve/deny |
| GET | `/conversation/sessions/:ws` | List chat sessions |
| GET/DELETE | `/conversation/sessions/:ws/:id` | Get/delete session |

### Dispatcher Endpoints
| Method | Path | Purpose |
|--------|------|---------|
| GET | `/dispatcher/automations` | List all automations |
| POST | `/dispatcher/trigger/:ws/:name` | Trigger automation |
| POST | `/dispatcher/stop/:ws` | Stop running automation |
| GET | `/dispatcher/workspaces` | List workspaces |
| POST | `/dispatcher/workspaces` | Create workspace |
| DELETE | `/dispatcher/workspaces/:ws` | Delete workspace |
| POST | `/dispatcher/workspaces/:ws/automations` | Create automation |
| PUT/DELETE | `/dispatcher/workspaces/:ws/automations/:name` | Update/delete automation |
| GET/PUT/DELETE | `/dispatcher/workspaces/:ws/files/:file` | File CRUD |
| GET | `/dispatcher/workspaces/:ws/state` | Workspace state |
| GET/PUT | `/dispatcher/workspaces/:ws/config` | Workspace config |
| GET | `/dispatcher/workspaces/:ws/live` | SSE stream of live events |

### Proxy Endpoint
| Method | Path | Purpose |
|--------|------|---------|
| ANY | `/v1/chat/completions` | OpenAI-compatible reverse proxy to LLM |

---

## Coding Rules

See [`AGENTS.md`](AGENTS.md) for the canonical coding rules. Key highlights:

### General
- **Constitution first** — check invariants before writing code
- **Docs stay in sync** — update specs and constitution when architecture changes
- **Build + test before reporting done** — never claim success without verification
- **Targeted reads** — use grep and targeted file reads, avoid large directory listings
