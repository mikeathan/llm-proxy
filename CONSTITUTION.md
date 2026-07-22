---
id: LAW
status: law
last_reviewed: 2026-07-11
---

# Constitution (The Law)

This document defines the immutable architectural and security laws of the project. All code must adhere to these rules without exception.

## I. Network Security (The Guarded Hull)

1.  **No Raw Sockets**: No code is permitted to instantiate a raw `http.Client`, `http.DefaultClient`, `net.Dial`, or `net.DialContext` for external or LAN communication.
2.  **Mandatory Guardrails**: All network interactions must pass through the `NetworkTools` abstraction.
    *   **Agent Tools**: Must use `network.FetchURL` or `network.HTTPClient()`.
    *   **Infrastructure**: Must use `network.DialContext()` to ensure DNS rebinding protection and boundary checks (LAN vs Internet) are applied at the socket level.
3.  **Boundary Enforcement**: The system must strictly distinguish between LAN access and Internet access. Tools must verify destination safety before the first byte is sent.
4.  **Inbound Webhook Exemption**: Inbound HTTP handlers that receive messages from external platforms (webhooks, callbacks, bot API requests) are exempt from §I.1–I.3. They use standard `net/http` handlers and require no `NetworkTools` wrapper. The exemption applies only to receiving incoming connections — any outbound network call triggered by the handler (e.g. replying to a Telegram message) must still pass through `NetworkTools`.

## II. Resource Management (The Bounded Deck)

1.  **Parallel Tool Execution**: To improve efficiency and stay within operational turn limits, the agent loop supports executing multiple independent tool calls in a single turn.
    *   The agent should prioritize batching related filesystem and shell operations into a single turn (The "Rule of Batching") to conserve its execution budget. Each action must still follow the Thought -> Action pattern, but multiple Action blocks are permitted.
2.  **Lifecycle Tethering**: No background process (Dispatcher, MCP Client, Watcher) shall be started with `context.Background()`. All long-running processes must be tethered to a cancellable context derived from the application's root lifecycle.
3.  **Sandbox Isolation**: All execution of untrusted code (scripts, binaries, WASM) must occur within the verified `Sandboxing` subsystem. No raw `os/exec` calls are allowed for agent-triggered work. Terminal execution must utilize a persistent shell session to maintain state (CWD, environment) throughout the agent's task lifecycle. **Environment variables passed from the host to the sandbox must be strictly filtered using an explicit Allowlist (defined via `terminal.json` overrides) to prevent secret leakage.**
4.  **XML Tool Call Format**: The agent loop must use strict XML tags (`<tool_call>{"tool":"name","args":{...}}</tool_call>`) to delimit actions.
    *   Standard Markdown JSON blocks are prohibited for tool execution to prevent desynchronization with conversational text.
    *   The parser must rely on regex-based XML extraction rather than brace-counting or manual state tracking.
    *   No naked JSON, no markdown fences, no greedy fallbacks.
    *   Rejected formats get specific `ParseError.Feedback()` (defined in `prompts/templates.go`) instead of generic nags.
    *   `sanitizeJSON()` handles Python booleans/None, markdown fences, trailing commentary, and invalid escapes before unmarshaling.
5.  **Dual-Path Tool Interface**: Tool definitions are injected into the system prompt as text (the "Tool Manual").
    *   **Native path**: When `UseNativeTools()` returns true (cloud models, local models with `ToolCallFormat: "native"`), the OpenAI `tools` array is sent in the API request. llama.cpp's Jinja template handles formatting; tool calls are returned server-side.
    *   **Text/XML path**: When `UseNativeTools()` returns false (default for local models), tools are sent ONLY as text instructions. The model must output `<tool_call>` XML manually, parsed client-side by regex.
    *   The HTTP client does NOT strip tools — the agent controls this decision.
    *   Both paths coexist: native tool_calls are accumulated from streaming deltas; the XML parser runs as fallback on text content for non-function-calling responses.
    *   When `ToolCallFormat: "xml"` is set on a config, XML mode is forced even if the provider supports native tools.
6.  **Token Budgeting & Structural Sieve**: To prevent context size overflow and 400-series errors, the system enforces a configurable character-based history budget (default 8,000 characters, overridable per-model via `ModelConfig.ContextBudget`).
    *   **Locked Head**: The System Prompt and the User's Initial Task are immutable and never pruned.
    *   **Structural Sieve**: When the budget is exceeded, the history is physically pruned by keeping the Locked Head and the last 10 messages (Priority Tail), while omitting all intermediate turns. This preserves the original goal and the immediate state while ensuring the context window is never overwhelmed.
    *   **Reactive Sieve**: When the LLM returns a context-size overflow error (e.g. `request exceeds the available context size`), the agent applies an aggressive sieve (system + task + last 3 turns) and retries.  This catches cases where the character-budget sieve didn't fire because the model's actual token context is smaller than expected (e.g. llama.cpp with `--ctx-size 8192` but metadata reporting 262K training context).
    *   **Repetition Detection**: The repetition detector (`recentCalls`, `duplicateStreak`) survives sieve boundaries to prevent loops from spanning across prune events. After 3 consecutive identical (tool + args) calls, inject a duplicate nag. After 5+, abort with "infinite loop detected."
    *   **Resource-Aware Orchestration**: A higher-level token and cost budgeting system (see Section VI) may further constrain model usage. The Structural Sieve reduces context pressure; the Orchestrator enforces hard ICU caps and applies adaptive squeezing when approaching those caps. Both systems coexist — the Sieve prunes history, the Orchestrator limits per-request and per-window token consumption.
7.  **Natural Task Completion**: The agent completes a task when it produces a final assistant message with substantive visible content (≥20 characters after stripping inline reasoning blocks such as `<think>`/`<reasoning>`) and no further tool calls. Completion requires at least one tool result anywhere in the run's history — the model must have actually done work, not just answered from first-turn text. Reasoning-only interleaves between tool results and the final answer do not block completion; the immediately-preceding message is not required to be a tool result. The synthetic `submit_final_answer` tool is removed. Heuristic keyword matching ("task complete", "summary") is not used. When the model returns empty after tools, the agent issues a single nag ("continue with the task") rather than looping indefinitely; if the response is still empty, a fallback answer (saved content-with-tools or best-available assistant text) is used.
8.  **Context-Preserving Normalization**: Messages transmitted to LLM engines must be stripped of non-essential metadata fields. `role`, `content`, `tool_calls`, and `tool_call_id` are preserved.
    *   When native tools are disabled: `tool` role → `user` role with tool_call_id embedded in content (`Tool result [call_N]: <json>`) to maintain call/result association while avoiding Jinja template errors in llama.cpp backends.
    *   When native tools are enabled: tool roles pass through as-is.
    *   **No Auto-Nags in Normalization**: Nag injection (prompt corrections when the model produces malformed tool calls, duplicate detection, feedback injection) belongs in the agent loop (`agent.go`), NOT in `NormalizeHistory()`. The history normalizer must not modify message content beyond role conversion and metadata stripping.
9.  **Structural Truncation**: High-volume tool outputs must be structurally truncated (Head/Mid/Tail) before being returned to the context window.
10. **Guardrail Decision Flow**: When a tool call is blocked by the guardrail engine, the agent loop must pause and wait for user approval rather than silently failing.
    *   `ValidateToolCall()` fails → creates `GuardrailBlockedPayload` with decision_id.
    *   `onGuardrail` callback registers a channel in `GuardrailDecisionStore` + publishes SSE event to frontend.
    *   Agent blocks on channel (60s timeout).
    *   User approves/denies via `POST /admin/api/conversation/guardrail-decision`.
    *   If approved with `persist: true` → `PersistOverride()` writes to workspace `config.yaml`.
    *   In automation mode (no user present), the callback is nil and guardrail violations fail immediately.
11. **Per-Model Config Flow**: `ModelConfig.MaxSteps`, `ContextBudget`, `MaxTokens`, `ToolCallFormat`, and `Prefill` flow from the runtime to the agent without global defaults interfering.
     *   Read by the executor from `Runtime.ListModels()`.
     *   Passed to `AgentOptions` when creating the Agent.
     *   The agent uses them directly — no global defaults override per-model settings.
     *   Provider-tier defaults (`assistant/agent.go`) define baseline values for each provider type (local, gemini, openai, openrouter, etc.). These defaults are applied when creating new model entries but do not override explicitly set per-model values.
12. **Agent Memory System**: The agent can persist and recall facts across conversations via a SQLite-backed memory store.
      *   **Storage**: Memories are stored in a dedicated `memories` table with an FTS5 full-text index (`memories_fts`) in `orchestrator.db`, sharing the same database file as the ledger. The `memory.Store` package (`internal/platform/memory/`) provides all CRUD and search operations.
      *   **Tools**: `memory_search` and `memory_update` are registered tools available to all agents. `memory_search(query, scope, tags)` performs FTS5 keyword search with BM25 ranking and optional scope/tag filtering; `memory_update(content, scope, mode, keep)` saves a new memory with three-tier routing.
      *   **Active Memory Injection**: Once per session (first LLM turn only), the agent searches memory using the last user message as query. Relevant memories are injected as a separate system message wrapped in `<memory>` tags at the end of prepared messages. The injection is capped at 3000 runes and happens only once to avoid per-turn noise — memories saved mid-session become visible on the next session. For automation tasks, memory injection is disabled entirely (the full task prompt is too generic as a search query, and the `<memory>` block at the end of 18+ turns of history overwrites the finalization instruction in the model's attention). See `docs/audits/memory-injection-investigation.md` for the rationale. All prompt strings live in `templates.go` (Constitution II.12 applies).
     *   **Pre-Sieve Flush**: When context usage exceeds the flush threshold (default 70% of `ContextBudget`), a nudge message is appended asking the agent to save important context to memory via `memory_update`. A `memoryFlushSent` flag prevents repeated nudges across consecutive turns; the flag resets when the physical sieve actually prunes history.
     *   **Configuration**: Memory is configurable via `UserSettings.Memory` (Tier 2 settings.yml). `Enabled`, `SearchTopK`, `FlushThreshold`, and `RetentionDays` are user-settable. When disabled, the memory store is nil and all operations are no-ops.
     *   **Workspace Isolation**: All queries filter by `workspace_id`. Agents in different workspaces have independent memories.
     *   **Lifecycle Events**: `memory_recall` and `memory_flush` events are emitted to the UI for observability.

13. **Prompt Centralization** (SINGLE SOURCE OF TRUTH): `internal/core/assistant/prompts/templates.go` is the ONLY location for ALL prompt strings.
14. **Goroutine Lifecycle Discipline**: Every goroutine must have an explicit termination path via `context.Context` or channel close. No goroutine shall be started with `context.Background()` unless it is tethered to the application's root lifecycle (`rootCtx`). Orphan goroutines — those without cancellation, timeout, or completion signaling — are prohibited. This applies to background workers, heartbeat loops, stream processors, and timer-based routines.
    *   System messages, nag prompts, parse-error feedback, JSON error translations.
    *   Escalation prefixes, protocol instructions, system rule text.
    *   No hardcoded prompt strings anywhere else — not in `agent.go`, not in `tool_call_parser.go`, not in any other logic file.
     *   When adding or modifying a prompt, `templates.go` is the only valid location.

## III. Data & Metadata (The High-Fidelity Rule)

1.  **GGUF Parsing**: Parsing of GGUF metadata must use the authorized parsing library (`internal/core/llm/metadata/gguf_scanner.go`). String manipulation or regex-based extraction from filenames is forbidden. The scanner reads only the GGUF header (KB, not GB) using `SkipLargeMetadata` + `MMap`.
2.  **Atomic Persistence**: All system configuration writes must be atomic. Use `storage.Update` patterns to prevent partial state corruption.
3.  **Workspace Jail**: File system access for agents must be strictly jailed to the assigned workspace directory using `IsSecurePath`. The system root is never a valid target for agentic I/O. Terminal commands must be validated using segment-aware parsing to ensure chained commands (e.g., `&&`, `||`, `;`, `|`) do not bypass security guardrails.
4.  **Three-Tier Configuration Model**:
    *   **Tier 1 — `config.json`** (system): Infrastructure settings (bind, model_host, idle_timeout, environment, workspaces_dir).
    *   **Tier 2 — `settings.yml`** (user): Local paths (llama_server_binary, model_dir, default_args), guardrail overrides, per-model agent tuning overrides.
    *   **Tier 3 — `registry.json`** (dynamic state): Model catalogue, provider definitions, MCP server list, communication connector configs, primary/fallback models.
    *   **Workspace overrides** — `{metadata}/{workspace}/config.yaml`: Per-workspace guardrails, automations, and settings.
5.  **Model Persistence (Two-Tier)**: Model configuration is split across two persistence locations:
    *   **Base model info** → `registry.json`: name, provider, filename/model_id, port, args, credential_id, prefill flag.
    *   **Agent tuning overrides** → `settings.yml` under `model_overrides:`: max_steps, context_budget, tool_call_format, prefill.
    *   On save: both tiers are updated simultaneously.
    *   On load: registry.json base is loaded first, then `ApplyModelOverrides()` merges settings.yml overrides onto runtime models.
    *   On settings change: `ApplyModelOverrides()` re-applies overrides to all runtime models.
    *   **Never put agent tuning overrides directly in registry.json entries.**
6.  **Secrets Are Encrypted**: API keys and tool secrets are stored in `secrets.json` encrypted with AES-256-GCM. The `SecretsStore` interface is the only access path.
7.  **Key Deletion/Update Cascades to Models**: Deleting or updating API keys automatically removes all model catalogue entries whose `CredentialID` no longer matches any remaining key name. Models with an empty `CredentialID` are only removed if no keys remain for the provider. This prevents orphaned model configurations that reference deleted or renamed credentials. Cascade applies on both `DELETE /admin/api/secrets/keys` (single-key) and `PUT /admin/api/secrets/keys` (batch save).
8.  **Unified Provider Management**: All cloud provider configuration (API keys, provider settings, and model entries) is managed under a single provider tab in Settings. The Dashboard cloud tab is read-only — model management ("Add Model", edit, delete) is done exclusively in Settings. This eliminates the previous split where API keys were managed in Settings and models were managed in the Dashboard.
9.  **Secrets Change Notification**: The secrets store publishes an `OnChange` event when credentials are modified. The `AppContext` subscribes to this event and triggers a runtime `Sync()` to ensure credential changes propagate without requiring a server restart.

## IV. Code Standards (The Clean Signal)

1.  **Context Propagation**: Every function performing I/O or network calls MUST accept `context.Context` as its first argument.
2.  **Error Integrity**: Never return raw strings as errors. Always use `fmt.Errorf` with the `%w` verb to maintain the error chain for diagnostic trace-back.
3.  **Failover Clarity**: Distinguish between "Transitional States" (e.g., loading weights) and "Terminal Errors". Fallback logic should only trigger on terminal failures.
4.  **No Dead Code**: Remove unused imports, variables, and functions. Do not leave `_` prefixed variables as placeholders.
5.  **Testing Standards**: Mock the interface, not the implementation. All `RuntimeManager` mocks implement the full interface (`MockManager`). Test the agent loop with real tool providers where possible; mock only the LLM client.

## V. Governance (The Living Law)

1.  **Constitution over Convenience**: If a feature requires violating these laws, the law must be formally amended in the Constitution after a security review, rather than bypassed in the implementation.
2.  **Spec-First Development**: Before modifying a subsystem, read its corresponding SPEC (`docs/SPECS/`, cataloged in `docs/INDEX.md`). After implementing, update the relevant spec and amend this Constitution if architectural rules changed.
3.  **No Half-Finished Implementations**: Complete the feature or don't start. No stubs, no `// TODO`, no feature flags for incomplete work.

## VI. Resource-Aware Orchestration (The Budget Deck)

The full ICU budget system, context length resolution (5-tier), slot persistence, stream interception, and nil-safe operational rules are defined in **SPEC-005** (Resource-Aware Orchestration). All orchestration code must comply with that specification.
