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

> **Loop strategies:** the agent loop archetype is selected per-model via a pluggable
> loop-strategy engine (react, plan-and-execute, evaluator-optimizer) defined in
> **SPEC-010** (`docs/SPECS/agent-loop-strategies.md`). All strategies compose the shared
> turn primitives specified here; selection is deterministic (config), never LLM-chosen.

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
- `system_error` tools are exempt.
- Single-tool spiral: 12+ consecutive calls to the same tool that recycle ≤4 distinct argument values (no exploration) → hard error ("spiral detected"). A burst of varied-argument calls to one tool (e.g. an audit automation running a sequence of distinct shell commands) is legitimate batching (Constitution II.1), not a spiral; identical-argument repeats are caught earlier by the duplicate detector.
- Sequence-repeat (n-gram): tool-call cycles of length 3–5 repeating ≥3 times in the last 30 calls → hard error ("repeating N-tool cycle detected"). Cycle keys are `(name, args)` pairs, so runs that legitimately use one tool with varying arguments never form a false cycle; single-tool stagnation with varied args remains bounded by the consecutive spiral detector.
- Alternating oscillation: ≤30% unique (tool, args) calls over the last 20 calls for 15+ turns → hard error ("alternating tool spiral detected"). Keys are full (tool, args) pairs, so a run that dominates on one tool with varying arguments (exploration) never trips it — only genuine recycling of the same calls drives the unique ratio down.
- Same-target oscillation: one path hit by 6+ distinct tools in a single turn → hard error ("same-target oscillation detected").

### 5. Exit Heuristics

**Natural completion (automation + chat tool loops):**
- Content-only assistant message (≥20 chars after stripping inline reasoning blocks such as `<think>`/`<reasoning>` tags, no tool calls) with at least one tool result anywhere in the run's history → complete.
- Reasoning-only interleaves (empty content, large `ReasoningContent`) between tool results and the final answer do not block completion; the completion gate searches the entire history for a tool result, not just the immediately-preceding message.
- Premature termination guard: empty output or 3 consecutive identical assistant messages without tool calls.
- Content-with-tools fallback: when the model writes visible text AND calls tools in the same turn, the text is saved. If the next turn is empty, the saved text is used as the final answer.
- **Output-cap truncation guard** (`finish_reason="length"`): the stream layer surfaces the upstream `finish_reason` on the turn message (choice-level JSON, `models.Message.FinishReason`; never serialized into replayed history because strict providers reject unknown message keys). A content-only turn whose generation hit the output-token cap (`finish_reason == "length"`) is a **truncated final answer, not a complete one** — `checkTaskCompletion` must not finalize a cut-off report. Both `handleTextTurn` (natural completion) and `finalizeReport` (deterministic finalization turn) instead keep the partial, inject the bounded continuation nudge `LengthContinuationPrompt` ("Continue exactly where you left off. Do not restart or repeat prior text. Finish the answer directly." — mirrors Hermes's `_LENGTH_CONTINUATION_OUTPUT_LIMIT`), and run the next turn with the partial in history. Fragments accumulate in `truncatedParts` and are stitched at completion by `joinTruncatedParts` (newline glue where two fragments would join without whitespace, port of Hermes `_join_truncated_parts`), bounded per run by `lengthContinuationMax` (2). A clean `finish_reason="stop"` is untouched — this guard only fires on genuine output-cap truncation, never on a model choosing to stop. See `docs/audits/2026-08-30-llm-smoke-test-incomplete-run.md` (Resolution section).
- **Truncated ReAct scaffold guard** (`endsWithBareActionMarker`): a turn whose **last non-empty line is a bare `Action:` marker** — the delimiter the system prompt's own `Thought -> Action` contract places on its own line before every tool call — is a truncated tool-call attempt, never a final report. Matching the full line (not a word suffix) means a report that merely ends with "...the action:" is still a valid completion. Enforced uniformly at every completion surface so all loop strategies behave identically: `checkTaskCompletion` + the parse-error trust branch (react / evaluator-optimizer), `finalizeReport` (plan-execute + the react recovery ladder), and `bestAvailableAnswer` (fallback answers must never surface a scaffold). It is rejected by `checkTaskCompletion` **and** by the parse-error trust branch (`handleParseErrorFeedback`), which previously re-accepted any non-empty XMLFound=false content as a completion — the back door that let the llm-smoke-test run finalize as "completed" after the model stopped mid-`Action:`. The turn then flows into the recovery ladder (format feedback / nag) so the run continues. See `docs/audits/2026-08-30-llm-smoke-test-incomplete-run.md`.

**Chat mode (additional):**
- Step 1: single-turn response with no tool calls → exit.
- 2 consecutive assistant messages with no tool calls → exit.
- Assistant reply with substantive content and no tool calls, at least one tool result in history → exit.
- Premature termination → exit.

### 6. Error Recovery
- Parse errors: inject `ParseError.Feedback(availableTools)` — specific format guidance with valid tool names.
- Empty response after tool calls: inject ONE `AutomationNagPrompt` (one-shot nag). If the next turn is still empty, use the saved content-with-tools fallback or best-available assistant text — the agent does not nag perpetually.
- Plain text without tool calls in native-tools mode: accepted as completion (Hermes-aligned). No nag, no rejection — text without tools IS the completion signal.
- Tool validation failure: treat as parse error, clear invalid tool calls, inject feedback.
- Content too long (write exceeds server JSON parse limit): inject `AutomationContentTooLongPrompt` — instructs the model to use `write_file` for the first chunk, then `append_file` for subsequent chunks.
- Write file content size is NOT enforced by the Go handler. The manifest `maxLength` was removed entirely — server-side grammar constraints were causing silent truncation at exactly `maxLength` chars. The model's own `max_tokens` is the only output cap. If content exceeds server JSON parse limits, the natural JSON parse error triggers recovery.

### 7. Native Tool Support (Constitution II.5)
- Controlled by `Agent.useNativeTools` (resolved from `AgentOptions.UseNativeTools` > `ToolProvider.UseNativeTools()`).
- When true: native tool schemas passed in `req.Tools`, LLM server sends tool-call deltas natively.
- When false: tools stripped, tool manual injected into system prompt as text.
- The HTTP client (`client.go`) does NOT strip tools — the agent controls this.
- **Effective format resolution** (`LLMRuntimeManager.EffectiveToolCallFormat`): explicit `tool_call_format` ("native"/"xml") wins. Cloud workloads default to "native" (`ApplyMetadataDefaults`). A **local** OpenAI-compatible model with an unset format is probed once against its serving endpoint (`OpenAICompatibleProvider.ProbeNativeTools` — a minimal chat request with a tiny `tools` schema; `finish_reason == "tool_calls"` ⇒ supported) and the result is persisted onto the stored model config. Probe failures are not cached (XML fallback for that turn, re-probed next resolution). This removes the per-model `tool_call_format` patching that weak XML-format local models required — llama.cpp `--jinja`, Ollama, LM Studio, vLLM and friends are detected automatically, while XML-only servers keep XML text mode. **Ordering invariant**: the format must be resolved **before** the agent is built — `executor.go` calls `EffectiveToolCallFormat` ahead of `buildAgentOptions`/`NewAgent` (and the chat path resolves in `resolveModelConfig` before the agent builder). `ApplyModelConfig` locks `UseNativeTools` from the stored config at build time; resolving after `NewAgent` would leave the first cold-cache run (post-restart) in XML text mode with no `tools` array (2026-08-31 14:26 regression).

### 8. Reasoning Stuck Detection & Fallback
- Scaled threshold: `stuckThreshold()` returns `max(maxTokens * 2, MinReasoningStuckThreshold)`. Safety net for servers not enforcing reasoning budgets.
- Early detection for models without reasoning budget (`reasoningBudget == 0`): fires at `maxTokens / stuckNonReasoningDivisor` chars (currently divisor=1 → threshold = `maxTokens`). NOTE: local/GGUF models now auto-derive a think-token budget from `max_tokens` (`resolveReasoningSpec` → `DefaultReasoningBudget`, `max_tokens/3`), so local reasoning is normally enforced server-side and this early-stuck branch mainly covers cloud/opaque providers that emit no readable reasoning stream. The derivation is from context size, never the model name. Divisor=1 avoids false positives while catching stuck states 2x faster than the `maxTokens * 2` baseline. Reasoning models with an explicit budget (`reasoningBudget > 0`) skip this check entirely.
- Detection: stream produces only reasoning content (no text, no tool call deltas) exceeding the derived threshold.
- Empty tool_call spiral: ≥`emptyToolCallSpiralLimit` (3) closed empty `<tool_call></tool_call>` blocks in pure reasoning also triggers stuck (before char threshold). Dangling open tags not counted. Same recovery path as char-threshold stuck — does not fail the run. Lifecycle payload includes `empty_tool_calls` count. See `countEmptyClosedToolCalls()`.
- Embedded tool call recovery: before declaring stuck, scan reasoning content for `<tool_call>` blocks. If found, extract the tool calls directly into `fullMsg.ToolCalls` — reasoning text stays in `ReasoningContent` and is never promoted to `Content`. The llama.cpp server already separates `reasoning_content` from `content` at the wire level; this function bridges the gap when the model writes tool calls as text inside reasoning instead of using native deltas. See `tryExtractToolCallFromReasoning()`.
- Content-level repetition guard: when the streamed visible `Content` is dominated by verbatim repeated fragments (a single 60+ char window or repeated line covering ≥50% of the text, minimum fragment 400 chars, fail-open) **and** no native tool calls have been parsed, the stream is aborted and routed into the same `[stuck]` recovery as char-threshold stuck. This catches degenerate loops that write visible content (e.g. a model echoing a malformed tool-call dialect as ~190 closing tags) with no tool calls and no progress — a shape the reasoning-only stuck detector and the tool-call repetition detector both miss. See `isRepetitionDominated()` (Hermes Agent `repetition_guard` port). Real tool calls are never discarded: the guard requires zero parsed calls.
- Per-stream duration cap: a stream producing no native tool calls and no natural completion beyond `streamMaxDuration` (default 90s, test-shortenable) is terminated to bound worst-case degenerate-stream runtime well under the 10-minute per-turn timeout. Unlike the repetition guard, the accumulated content is preserved (it may be a genuine slow report) — mirroring char-cap termination so the partial turn is evaluated/salvaged rather than dropped.
- On stuck: stream is aborted, `lifecycle` event (`stuck_detected`) emitted with `reasoning_chars` (and `empty_tool_calls` when spiral).
- **Token budget enforcement at the stream layer**: when the orchestrator's `StreamInterceptor` signals `ShouldTerminate` (token or reasoning budget exceeded), `processStream` returns nil, ending the stream client-side. Upstream servers do not always enforce `max_tokens`; without this client-side termination the model can generate indefinitely. A char cap of `maxTokens * 4` (via `exceedsContentCharCap`) is a fallback safety net for the case where the token counter underestimates output. The agent loop evaluates the partial turn (e.g. `isPrematureTermination`, `handleEmptyStream`, or normal turn processing) so the response is not silently dropped.
- Fallback chain (after a stuck stream):
  a. **Native tools + empty stream** → depends on `usePrefill`:
     i. **Native-only** (`usePrefill=false`): `[stuck]` placeholder injected as assistant message. On the next turn, `handleNoToolCalls` fires the one-shot nag — the model gets one recovery attempt with `AutomationNagPrompt`. If still empty, the saved content-with-tools fallback or best-available answer is used. No perpetual nagging.
     ii. **XML-text** (`usePrefill=true`, e.g. local models): retry via XML streaming — temporarily disable `useNativeTools`, suppress `tool_choice`, suppress `reasoningBudget`, skip stuck check (user sees tokens in real-time), emit `lifecycle` event (`fallback_started`).
  b. **XML stream also empty** → fall back to `computeNextResponseNonStreaming` as last resort.
  c. **Non-streaming also fails** → starvation count increments. After `DefaultStarvationLimit` consecutive no-tool-call turns, the run fails with a stall error.

### 9. Lifecycle Events for UI Progress
- `lifecycle` event type with `phase` field:
  - `agent_thinking`: emitted at the start of every LLM call (pre-response compute wait) — frontend flips to a neutral "thinking…" status. Carries no content/reasoning fields; never fabricated model output.
  - `stuck_detected`: model stuck in reasoning loop, `reasoning_chars` included.
  - `fallback_started`: fallback mode engaged, `reason` and `mode` included.
  - `fallback_waiting`: non-streaming fallback in progress, `elapsed` time included (15s heartbeat).
  - `fallback_completed`: fallback succeeded.
  - `guardrail_violation`: synchronous guardrail rejection (path/workspace boundary, no approval flow) — payload `{tool, error}`. Surfaced as its own chat segment.
- Lifecycle events are appended as system messages in the frontend (never overwrite assistant streaming content).
- The heartbeat in `computeNextResponseNonStreaming` now uses `lifecycle` (`fallback_waiting`) with elapsed time instead of `tool_stream`.

### 10. Goroutine Lifecycle in processStream
- A `streamDone` channel (closed via `defer`) ensures the 30-second heartbeat goroutine exits when `processStream` returns for ANY reason — not just `ctx.Done()`. This prevents misleading "stream still generating" log lines after stuck detection or stream EOF.

### 11. Guardrail Decision Flow (Constitution II.10)
- When `guardrails.ValidateToolCall()` rejects a tool call, the agent invokes `onGuardrail(ctx, payload)` if set.
- The payload includes: `DecisionID`, `Tool`, `Args`, `Reason`, `Category` (terminal/filesystem/network/search/communication), `WorkspaceID`.
- The callback blocks until: user approves (`Allow: true`), user rejects (`Allow: false`), timeout fires (default 5 min, per-model configurable via `guardrail_approval_timeout_seconds`), or context is cancelled.
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

### 14. executePlan Step Limit and Timeout
- The `executePlan` tool bypasses the standard single-tool-per-turn loop and executes a multi-step plan atomically.
- **Pre-check:** if `len(plan.Steps) > MaxPlanSteps` (default 50), the plan is rejected before any step executes.
- **Plan-level timeout:** the entire plan is wrapped in `context.WithTimeout(MaxPlanDuration)` (default 15 minutes, overridable per-model via `ModelConfig.MaxPlanDurationMinutes`).
- **Per-step timeout:** each step uses `executeSingleToolStep` with the same `ToolTimeout` / `FilesystemToolTimeout` as regular tool calls. If any step times out, the plan aborts.
- **Inline step cap:** after each step, `if i >= MaxPlanSteps → abort` catches any discrepancy between pre-check count and actual iterations.
- **Guardrail checks:** each step runs through `resolveGuardrail` with the configured `GuardrailTimeout` and `GuardrailTimeoutBehavior` (fail-open or fail-closed).
- **Schema validation:** each step's args are validated against the tool manifest schema exactly like the react loop (`validateToolArgs`): required parameters must be present and non-empty. A step that guesses a parameter name (e.g. `file_path` instead of `path`) fails fast with `plan step N: invalid tool call: …` before any tool executes — it never falls through to an empty-value write.
- **Config flow:** `MaxPlanSteps` and `MaxPlanDuration` flow through `AgentOptions ← ModelConfig ← adminTuningDefaults` so users can tune every limit per-model.

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

Provider-specific defaults are defined in `models/tuning.go` (`ProviderTuningDefaults()`) for the numeric rows and `assistant/reasoning_param.go` (`providerReasoningTable`) for the reasoning wire, composed in `assistant/agent.go`, and exposed to the frontend via `adminTuningDefaults` in `GET /admin/api/state`. The UI uses these to prefill model forms with reasonable values per provider (e.g. Gemini/OpenAI/NVIDIA/OpenRouter get 8192 output-cap tokens and `native` tools; local models get 2048 prefill tokens and XML text mode).

## IV. Testing Strategy
- Unit tests: `agent_test.go` covers simple execution, tool calls, loop detection, streaming, premature termination, `precededByToolResult`.
- Parser tests: `tool_call_parser_test.go` covers XML format, malformed JSON, missing tool fields, unsupported tags, tool validation, `ParseError.Feedback`.
- Content filter tests: `content_filter_test.go` covers markup detection edge cases.

## V. Constitutional References
- II.4: Unambiguous Tool Boundaries (XML-only)
- II.5: Text-First Tool Interface (native tools for cloud)
- II.6: Token Budgeting & Structural Sieve
- II.7: Natural Task Completion
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
   - **Tool-level kill (per-Execute):** A kill goroutine started per `shell.Execute` call receives `ctx.Done()` and calls `killAll()` → `syscall.Kill(-pgid, SIGTERM)`. This is the immediate tool-specific kill, not the dispatcher's general force-kill.
   - SIGTERM terminates bash and any running child processes
   - The blocked `stdout.ReadString` returns with an error (pipe closed)
   - `Execute` returns the partial output and a context-cancelled error
   - The agent exits
4. If the shell tool-level kill does not terminate the run:
   - **Dispatcher force-kill (after 30s):** `StopAutomation`'s diagnostic goroutine checks if the run is still active after 30 seconds. If still running and a shell PGID exists, it sends `syscall.Kill(-pgid, SIGKILL)` and removes the run from `activeRuns`. If no shell PGID (network-only run), it logs a warning.
   - This is a secondary safety net, distinct from the per-Execute tool kill.

The background `HostShellManager` reaper also uses `killAll` to cleanly terminate sessions on shutdown or recycle, ensuring no orphaned processes remain.

### Cleanup Sequence
1. Close stdin (signals bash to exit after current command)
2. Wait for bash process to exit (`ps.done` channel)
3. If context is cancelled while waiting, `killAll` terminates the process group
