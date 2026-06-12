---
id: SPEC-001
title: Agent Loop
version: "1.0"
status: stable
last_updated: 2026-06-08
constitution_references: [II.4, II.5, II.6, II.7, II.8, II.10]
related_specs: [SPEC-002, SPEC-004, SPEC-005]
supersedes:
---

# SPEC: Agent Loop

## I. Intent
The agent loop (`assistant/agent.go`) executes multi-turn tool-augmented conversations. It orchestrates model inference, tool call parsing, tool execution, and history management. It must be model-agnostic — supporting both local models (via text-based XML tool calls) and cloud models (via native tool schemas) — and resilient to model-specific failures.

## II. Functional Requirements

### 1. Main Execute Loop
- Accept a conversation `history` and iterate up to `maxSteps` turns (default 25, overridable per-model).
- Each turn: list tools, check context budget, call the LLM, parse tool calls, validate against registry, execute, append results.
- Global timeout: 30 minutes. Per-turn timeout: 10 minutes.

### 2. Context Budget & Physical Sieve (Constitution II.6)
- Default budget: 8,000 characters (overridable via `ModelConfig.ContextBudget`).
- When exceeded, first attempt **compression**: truncate long Content (>4000 chars) and ReasoningContent (>2000 chars) in older messages to head+tail with `...[Truncated]...` marker.
- If compression not enough: keep system message + first user message (Locked Head), insert sieve marker, keep last 10 messages (Priority Tail).
- **Critical**: `recentCalls` (repetition detector) MUST survive the sieve boundary.
- **Reactive Sieve**: When the LLM returns a context-size overflow error (e.g. `request exceeds the available context size`), the agent applies an aggressive sieve (keep only system + task + last 3 turns) and retries.  This catches cases where the character-budget sieve didn't fire because the model's actual token context is smaller than expected (e.g. llama.cpp with `--ctx-size 8192` but `n_ctx_train` reporting 262K).

### 3. Tool Call Extraction (Constitution II.4)
- XML-only: `<tool_call>{"tool":"...","args":{...}}</tool_call>`.
- Parsed by `proxy.ParseContentToolCalls()`.
- Failed parses return a structured `ParseError` with `XMLFound`, `JSONError`, and `ToolName` fields.
- Valid tool calls are validated against the provider's tool list via `proxy.ValidateToolCall()`.

### 4. Repetition & Loop Detection
- Track up to 3 recent `(name, args)` pairs.
- Consecutive identical calls increment `duplicateStreak`.
- At 3 duplicates: hard error ("infinite loop detected").
- `submit_final_answer` and `system_error` tools are exempt.

### 5. Exit Heuristics

**Automation mode:**
- Canonical path: `submit_final_answer` tool call (extracts summary from args).
- Premature termination guard: empty output or 3 consecutive identical assistant messages without tool calls.

**Chat mode:**
- Step 1: single-turn response with no tool calls → exit.
- 2 consecutive assistant messages with no tool calls → exit.
- Assistant reply immediately follows a tool result (`precededByToolResult`) → exit.
- Premature termination → exit.

### 6. Error Recovery
- Parse errors: inject `ParseError.Feedback(availableTools)` — specific format guidance with valid tool names.
- No parse error but no tool calls: inject `AutomationNagPrompt`.
- Tool validation failure: treat as parse error, clear invalid tool calls, inject feedback.
- Content too long (write exceeds server JSON parse limit): inject `AutomationContentTooLongPrompt` — instructs the model to use `write_file` for the first chunk, then `append_file` for subsequent chunks.
- Write file content size is NOT enforced by the Go handler. The manifest `maxLength` was removed entirely — server-side grammar constraints were causing silent truncation at exactly `maxLength` chars. The model's own `max_tokens` is the only output cap. If content exceeds server JSON parse limits, the natural JSON parse error triggers recovery.

### 7. Native Tool Support (Constitution II.5)
- Controlled by `Agent.useNativeTools` (resolved from `AgentOptions.UseNativeTools` > `ToolProvider.UseNativeTools()`).
- When true: native tool schemas passed in `req.Tools`, LLM server sends tool-call deltas natively.
- When false: tools stripped, tool manual injected into system prompt as text.
- The HTTP client (`client.go`) does NOT strip tools — the agent controls this.

### 8. Reasoning Stuck Detection & Fallback
- Scaled threshold: `stuckThreshold()` returns `max(maxTokens * 2, MinReasoningStuckThreshold)`. Safety net for servers not enforcing reasoning budgets.
- Early detection for models without reasoning budget (`reasoningBudget == 0`): fires at `maxTokens / stuckNonReasoningDivisor` chars (currently divisor=1 → threshold = `maxTokens`). NOTE: `reasoningBudget == 0` does NOT mean the model can't reason — local GGUF models (Gemma 4, GPT-OSS-20B) produce legitimate `<think>` blocks without an explicit budget. Divisor=1 avoids false positives on these models while catching stuck states 2x faster than the `maxTokens * 2` baseline. Reasoning models with an explicit budget (`reasoningBudget > 0`) skip this check entirely.
- Detection: stream produces only reasoning content (no text, no tool call deltas) exceeding the derived threshold.
- Embedded tool call recovery: before declaring stuck, scan reasoning content for `<tool_call>` blocks. If found, strip `<think>` tags and promote to `Content` so the XML parser can process it. See `cleanReasoningContent()`.
- On stuck: stream is aborted, `lifecycle` event (`stuck_detected`) emitted with `reasoning_chars`.
- Fallback chain:
  a. **Native tools + empty stream** → depends on `usePrefill`:
     i. **Native-only** (`usePrefill=false`): skip XML retry, go directly to non-streaming + nag prompt. The XML retry would waste ~60s for models that can't produce `<tool_call>` blocks.
     ii. **XML-text** (`usePrefill=true`, e.g. local models): retry via XML streaming — temporarily disable `useNativeTools`, suppress `tool_choice`, suppress `reasoningBudget`, skip stuck check (user sees tokens in real-time), emit `lifecycle` event (`fallback_started`).
  b. **XML stream also empty** → fall back to `computeNextResponseNonStreaming` as last resort.
  c. **Non-streaming also fails** → starvation count increments, sieve nudges model.
- Progressive sieve recovery handles repeated stuck events (1st: reactive sieve + nag, 2nd: aggressive sieve + stronger nag, 3rd: fail with clear error).

### 9. Lifecycle Events for UI Progress
- `lifecycle` event type with `phase` field:
  - `stuck_detected`: model stuck in reasoning loop, `reasoning_chars` included.
  - `fallback_started`: fallback mode engaged, `reason` and `mode` included.
  - `fallback_waiting`: non-streaming fallback in progress, `elapsed` time included (15s heartbeat).
  - `fallback_completed`: fallback succeeded.
- Lifecycle events are appended as system messages in the frontend (never overwrite assistant streaming content).
- The heartbeat in `computeNextResponseNonStreaming` now uses `lifecycle` (`fallback_waiting`) with elapsed time instead of `tool_stream`.

### 10. Goroutine Lifecycle in processStream
- A `streamDone` channel (closed via `defer`) ensures the 30-second heartbeat goroutine exits when `processStream` returns for ANY reason — not just `ctx.Done()`. This prevents misleading "stream still generating" log lines after stuck detection or stream EOF.

### 11. Guardrail Decision Flow (Constitution II.10)
- When `guardrails.ValidateToolCall()` rejects a tool call, the agent invokes `onGuardrail(ctx, payload)` if set.
- The payload includes: `DecisionID`, `Tool`, `Args`, `Reason`, `Category` (terminal/filesystem/network/search/communication).
- The callback blocks until: user approves (`Allow: true`), user rejects (`Allow: false`), timeout fires (60s default), or context is cancelled.
- If approved with `Persist: true`: the guardrail engine saves an override to the workspace config (`config.yaml`) so future matching calls pass without blocking.
- In automation mode (no user present), the callback is nil and guardrail violations fail immediately with an error tool result.
- The decision store (`assistant/guardrail_decision.go`) provides concurrent-safe Register/Resolve/Remove operations.

### 12. History Normalization (Constitution II.8)
- `NormalizeHistory()`: strips `ToolCalls` when `useNativeTools=false`. Converts `tool` role → `user` role with `tool_call_id` embedded in content (`Tool result [call_N]: <json>`) to avoid Jinja template errors in llama.cpp while preserving call/result association. No auto-nags. Consolidates consecutive same-role messages.
- `SanitizeHistory()`: preserves `role`, `content`, `tool_calls`, `tool_call_id`.

### 13. Per-Model Temperature and Timeout Overrides
- `ModelConfig.Temperature` (float64) overrides the hardcoded `DefaultAutomationTemperature` (0.1) for automation tasks. 0 = use default.
- `ModelConfig.TimeoutMinutes` (int) overrides `AgentGlobalTimeout` (30 min) per execution. 0 = use default.
- Both are stored in `settings.yml` under `model_overrides.<name>`. Persisted via `writeModelOverrides()` in `registry_handlers.go`.
- Applied in `executor.go` `buildAgentOptions()`: `cfg.Temperature` → `opts.Temperature`, `cfg.TimeoutMinutes` → `opts.GlobalTimeout`.
- At the LLM request level: `buildChatRequest()` uses `a.temperature` if set (>0), else falls back to `DefaultAutomationTemperature`. Llama.cpp server-level `--repeat-penalty`, `--frequency-penalty`, and `--presence-penalty` are set server-side (not in ChatRequest).
- Frontend exposes these as number inputs in the Agent Tuning grid with `title`-attribute tooltips and input constraints (min/max/step).

## III. Technical Architecture

### Component Map
```
automation/executor.go          — task dispatch, per-model config → AgentOptions
assistant/agent.go              — Execute loop, sieve, tool processing
assistant/registry.go           — wires LocalToolRegistry + MCP → provider/engine
assistant/tool_provider.go      — MultiToolProvider, CompositeEngine, ToolProvider interface
assistant/content_filter.go     — streaming markup filter
assistant/guardrails/           — tool-call guardrail validation
assistant/prompts/              — system prompts, nag prompts, tool manual builder
proxy/client.go                 — HTTP pipe to LLM servers (Chat + Stream)
proxy/history.go                — NormalizeHistory, SanitizeHistory, NormalizeContextSieve
proxy/tool_call_parser.go       — XML-only tool call extraction, ParseError, validation
models/config.go                — ModelConfig with per-model agent tuning fields
```

### Data Flow (single turn)
```
Executor → Agent.Execute()
  → provider.ListTools()
  → check context budget (sieve if needed)
  → computeNextResponse()
    → prepareMessages() → NormalizeHistory()
    → client.Stream() / client.Chat()
    → processStream() or computeNextResponseNonStreaming()
  → handleContentToolCalls() → ParseContentToolCalls()
  → ValidateToolCall() against tool list
  → deduplicate + repetition check
  → processToolCalls() → guardrails.ValidateToolCall() → onGuardrail() (if blocked) → engine.ExecuteTool()
  → append tool results to history
  → check exit conditions
```

### Per-Model Configuration
| Field | Default | Purpose |
|---|---|---|
| `ModelConfig.MaxSteps` | 25 | Max agent loop iterations |
| `ModelConfig.ContextBudget` | 8000 | Char count triggering sieve |
| `ModelConfig.ToolCallFormat` | (empty) | `"native"` to force native tools |
| `ModelConfig.MaxTokens` | 3072 | Per-request token limit sent in `max_tokens` to the LLM |

Provider-specific defaults are defined in `assistant/tiers.go` (`ProviderTiers()`) and exposed to the frontend via `adminTuningDefaults` in `GET /admin/api/state`. The UI uses these to prefill model forms with reasonable values per provider (e.g. Gemini/Vertex/OpenAI get 4096 tokens and `native` tools; local models get 2048 tokens and XML text mode).

## IV. Testing Strategy
- Unit tests: `agent_test.go` covers simple execution, tool calls, loop detection, streaming, premature termination, `precededByToolResult`.
- Parser tests: `tool_call_parser_test.go` covers XML format, malformed JSON, missing tool fields, unsupported tags, tool validation, `ParseError.Feedback`.
- Content filter tests: `content_filter_test.go` covers markup detection edge cases.

## V. Constitutional References
- II.4: Unambiguous Tool Boundaries (XML-only)
- II.5: Text-First Tool Interface (native tools for cloud)
- II.6: Token Budgeting & Structural Sieve
- II.7: Explicit Task Completion (submit_final_answer)
- II.8: Context-Preserving Normalization
- II.10: Guardrail Decision Flow (user approval for blocked tool calls)

## VI. Shell Session Lifecycle

The shell provider (`internal/shell/terminal.go`) manages persistent bash sessions per workspace.

### Process Group Isolation
Each bash session is started with `Setpgid: true` (`syscall.SysProcAttr`), creating a dedicated process group. This ensures that when the session is terminated, all child processes (running commands) are killed alongside the bash shell — orphaned processes are prevented.

### Stop / Context Cancellation
When the user clicks "Stop Automation" or the agent's context is cancelled:

1. `StopAutomation` calls `cancel()` on the execution context
2. The agent loop checks `execCtx.Err()` on the next iteration and exits
3. If the agent is blocked inside a running shell command:
   - A kill goroutine (started per-`Execute` call) receives `ctx.Done()` and calls `killAll()` → `syscall.Kill(-pgid, SIGTERM)`
   - The SIGTERM terminates bash and any running child processes
   - The blocked `stdout.ReadString` returns with an error (pipe closed)
   - `Execute` returns the partial output and a context-cancelled error
   - The agent exits

The background `HostShellManager` reaper also uses `killAll` to cleanly terminate sessions on shutdown or recycle, ensuring no orphaned processes remain.

### Cleanup Sequence
1. Close stdin (signals bash to exit after current command)
2. Wait for bash process to exit (`ps.done` channel)
3. If context is cancelled while waiting, `killAll` terminates the process group
