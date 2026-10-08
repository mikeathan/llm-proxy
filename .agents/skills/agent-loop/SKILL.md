---
name: agent-loop
description: "Agent-loop mechanics (reference): execution flow, structural/reactive sieve, stuck/spiral detection, reasoning budget, fallback chain, GBNF, and key constants. Load alongside debugging when the failure is in the loop."
last_reviewed: 2026-10-08
---

# Agent Loop — Execution, Sieve, Stuck Detection & Fallback

**Source docs:** SPEC-001, `docs/PLANS/ARCHIVE/agent-loop/refactor-assistant-clean-code.md`, `docs/audits/agent-stability-report.md`

> Debugging workflow: [`debugging`](../debugging/SKILL.md). Planning a loop change: [`task-planning`](../task-planning/SKILL.md).

---

## Execution Flow

`session.run()` resolves the configured **loop strategy** and dispatches to it (SPEC-010: `react` is
the default; `plan_execute` and `evaluator_optimizer` are registered in `loop_strategy.go`). The
diagram below is the shared turn path every strategy composes.

```
executor.go Execute()
  ├── Get LLM client
  ├── buildAgentOptions() — reads ModelConfig, applies overrides
  ├── NewAgent() — creates Agent with resolved opts
  ├── agent.Execute(history)
  │     ├── run() — main loop (runSession)
  │     │     ├── s.steps increments each turn
  │     │     ├── s.agent.executeTurn() — one LLM call + tool parsing
  │     │     │     ├── applyPhysicalSieve() — truncates history if over budget
  │     │     │     ├── computeNextResponse() — streaming Chat request
  │     │     │     │     ├── processStream() — accumulates chunks, detects stuck
  │     │     │     │     └── handleEmptyStream() — XML fallback on empty
  │     │     │     ├── rd.check() — repetition/spiral detector
  │     │     │     ├── rd.checkAlternating() — tool oscillation detector
  │     │     │     └── execute tools → append to history → loop
  │     │     │           ├── executeSingleToolStep() — guardrail check + single tool exec
  │     │     │           │     └── timeout: ToolTimeout / FilesystemToolTimeout / GuardrailTimeout
  │     │     │           └── executePlan() — multi-step plan bypass path
  │     │     │                 ├── pre-check: steps > MaxPlanSteps → fail fast
  │     │     │                 ├── plan-level timeout: context.WithTimeout(MaxPlanDuration)
  │     │     │                 ├── per-step: executeSingleToolStep() with inline check i >= MaxPlanSteps
  │     │     │                 └── guardrail checks: per step, per tool
  │     │     └── max_steps reached or done
  │     └── Return result
```

## Context Budget & Physical Sieve

- `context_budget` in chars (default 8000, overridable per-model via settings.yml)
- When total chars exceeds budget, `applyPhysicalSieve()` fires:
  1. Keep locked head (system message + first user message)
  2. Insert sieve marker: `[System Note: History distilled...]`, then the progress ledger message (`ledger.go`; facts only, one per prune, bounded to a share of `ContextBudget`). Each prune REPLACES earlier sieve messages (`withoutSieveMessages`); the "deliver final answer NOW" warning is sent once per run
  3. Keep priority tail (last 5-10 messages depending on stuck count)
  4. Before dropping, compress long `Content` (>4000 chars) and `ReasoningContent` (>2000 chars) with head+tail truncation

## Stuck Detection

**Reasoning stuck (length ceiling):** When reasoning content exceeds `maxTokens * 2` chars (= `max_tokens / 2` tokens at 4 chars/token; floor 2000), the stream is aborted. A `lifecycle` event with phase `stuck_detected` is emitted. The same ceiling applies to every model: there is no budget-based early cut (the old `reasoningBudget == 0` branch compared characters with a token count and aborted thinking Qwen at ~2K tokens, wasting ~80 s per abort).

**Reasoning-loop guard (`reasoning_repetition.go`):** while the stream is only thinking, once per 1 KB of growth the last 4 KB (whitespace collapsed, digits as written) are scanned: if ≥ half of the 64-byte windows already occurred earlier in the tail, abort into `[stuck]` recovery (`stuck_detected`, `reason: reasoning_repetition`). Judges content, not length, so loops stop in ~3K chars; on real thinking the top score was 0.06 vs the 0.5 threshold. Stays on when `SkipStuckCheck` is set. Paraphrased, count-up or long-period loops fall back to the ceiling / duration cap; digits are not normalised so drafted CSV / tables / checklists are not flagged.

Note: workloads classified local (a llama.cpp server run directly, or one behind an OpenAI-style URL identified by its listing fingerprint, SPEC-005) auto-derive a think-token budget from `max_tokens` (`resolveReasoningSpec(provider, workload, …)` → `DefaultReasoningBudget`, `max_tokens/3`); the derivation is from workload and context size, never the model name.

**Empty tool_call spiral:** When pure-reasoning stream (no content, no native tool deltas) has ≥`emptyToolCallSpiralLimit` (3) closed empty `<tool_call></tool_call>` blocks, stuck fires immediately — same lifecycle + nag recovery as char-threshold stuck. Does **not** kill the run. Dangling open tags (still forming a real call) are not counted. Catches Qwen 3.5 empty-tag loops in ~1s instead of waiting for the char threshold (~19s). See `countEmptyClosedToolCalls()`.

**Content-level repetition guard:** Catches degenerate loops that write visible content with no tool calls and no progress (e.g. a model echoing a malformed tool-call dialect as ~190 repeated closing tags — the deepseek-v4-flash workspace-health-test incident). When `Content` is dominated by verbatim repeats (a single 60+ char window or repeated line covering ≥50% of the text, minimum fragment 400 chars, fail-open) **and** zero native tool calls are parsed, the stream aborts into the same `[stuck]` recovery as char-threshold stuck. Model/provider-agnostic — keys purely off the streamed bytes, independent of grammar/tool format. Real tool calls are never discarded (guard requires zero parsed calls). See `isRepetitionDominated()` (Hermes Agent `repetition_guard` port).

**Per-stream duration cap:** A stream producing no native tool calls and no natural completion beyond `streamMaxDuration` (default 5 min, test-shortenable) is terminated; the clock starts at the **first token**, not the request (prompt processing on a busy local server can take a minute); the wait for the first token of a cloud stream is bounded separately (`streamFirstTokenTimeout`, 3 min; local workloads exempt). Content is preserved (not cleared) so the partial turn is evaluated/salvaged.

**Local thinking budget and recovery:** a local workload sends `thinking_budget_tokens` (`max_tokens/3` unless `reasoning_budget` is set) together with `reasoning_budget_message` (`prompts.ThinkBudgetWrapUp`) so the server closes the thought cleanly at the budget instead of letting the reasoning spill into the visible reply. The server appends it on ANY turn that hits the budget, so it must never say the answer is next — a run skipped its playbook's `memory_update` that way (2026-10-07). After a stream is aborted as stuck (ceiling or loop guard), `Agent.noteStuckThinking` makes the next request carry `enable_thinking=false` once (`reasoning.DisableThinking`); cloud is never switched off.

**Progressive sieve recovery:**
- 1st stuck → reactive sieve (first 2 + last 6 messages) + nag prompt
- 2nd consecutive stuck → aggressive sieve (first 2 + last 3 messages) + stronger nag
- 3rd consecutive stuck → agent fails with clear error

**Stuck detection is skipped on XML fallback retries** (the `SkipStuckCheck` config flag).

## Natural Completion vs Control Plane

Completion (Hermes-aligned, Phase 2b): a content-only assistant message with
at least one tool result anywhere in the run's history → done. The model signals
completion by writing plain text without further tool calls. Reasoning-only
interleaves (empty content, large `ReasoningContent`) do not block completion.

**Invisible reasoning in content:** Local models (Qwen3, Ollama) may emit
`<think>`/`<reasoning>` blocks inline in `content`. `stripThinkBlocks` removes
these blocks before evaluating completion. If nothing substantive remains after
stripping, the turn is NOT a final answer.

**Embedded tool calls in reasoning:** Some models (Qwen3) write `<tool_call>`
blocks inside `reasoning_content` instead of using native tool call deltas.
`tryExtractToolCallFromReasoning()` (stream.go) extracts these tool calls
directly into `fullMsg.ToolCalls` without copying reasoning text into `Content`.
The llama.cpp server already separates `reasoning_content` from `content` at
the wire level — reasoning text stays in `ReasoningContent` and is never
promoted to visible output. This prevents a feedback loop where the model
sees its own thinking text in conversation history and reuses it on subsequent
turns, wasting tokens and producing duplicated output.

Recovery messages (nags, sieve notes, parse feedback, stuck placeholders) are
**control plane** — `isAgentControlMessage` identifies them so they are never
confused with real conversation. See `checkTaskCompletion` in `session.go`.

**Content-with-tools fallback:** When the model writes visible text AND calls
tools in the same turn, and every call is housekeeping (`hasOnlyHousekeepingTools`
→ `toolpolicy.EffectHousekeeping`: `memory_update`, `automation_journal`), the text
is saved (`lastContentWithTools`). If the next turn is empty, the saved text is used
as the final answer. This handles the common pattern where a model delivers its
report while calling `memory_update`. Text beside a substantive tool (`write_file`,
terminal, reads) is mid-task narration and is not saved, and such a turn clears any
text saved earlier (`noteContentWithTools`). Until 2026-10-08 the housekeeping switch
matched no tool, so this path never fired in production.
A natural-completion turn that is at least `signOffRatio` (2x) shorter than the saved text is a sign-off, so
`keepReportOverSignOff` keeps the saved report (a playbook's "save to memory before you answer" puts the report beside
the `memory_update` call; the next turn is a one-line "done"). A comparable or longer final answer wins.

**One-shot nag:** Empty turns after tool results trigger a single
`AutomationNagPrompt` injection. If the next turn is still empty, a fallback
(fallback content or best-available assistant text) finishes the run — the
agent does not nag perpetually.

**Finalization-turn failure fallback:** When the tools-disabled `finalizeReport`
turn fails (LLM/provider error) or returns empty, the fallback chain is
`bestAvailableAnswer()` (last substantive assistant text) → `synthesizeRunSummary()`
(a degraded-but-real summary of the run's tool activity: per-tool call counts +
recorded `{"error": ...}` failures) → error only when the run did no tool work.
This is what keeps plan_execute runs alive when a provider outage hits the report
turn — their history is pure tool calls, so there is no assistant text to salvage.

## Truncated write_file salvage

When `write_file` / `append_file` args JSON is truncated mid-stream, the agent:
1. Recovers `content` (≥100 chars) from partial args
2. Best-effort extracts `path` and persists via Engine (guardrails + FS)
3. Completes with the recovered report even if path is missing or persist fails

No path is invented. See `salvageTruncatedWrite` in `tool_exec.go`.

## Reasoning Budget

- Auto-computed as `maxTokens / 3` (divisor = 3, do NOT change)
- Sent as both `reasoning_budget` and `thinking_budget_tokens` for broad provider compatibility
- **Warn-only, never terminate** — the proxy warns when exceeded but does NOT kill the stream
- Model-specific: llama.cpp enforces at API level, OpenAI ignores

## Output Constraint (GBNF Grammar)

The agent does **not** send a GBNF grammar on native-tools requests to local
llama.cpp. llama.cpp — the only server that accepted GBNF — rejects a custom
grammar combined with native `tools` in one request (HTTP 400 "Cannot use
custom grammar constraints with tools."), and every local llama.cpp is reached
over the OpenAI-compatible wire. Attaching the grammar made each tool-using
turn fail before generation (2026-09-05: assistant execution failed on the
locally-managed llama.cpp path while the same model via an OpenAI-style
registration worked). Native tool-call arguments are already constrained by the
server's own tool template, and llm-proxy validates results post-hoc
(`handleContentToolCalls` + `ValidateToolCall`), so no grammar is attached on
any path today:

- **Native tools (local or cloud):** no grammar — `buildChatRequest` never sets
  `req.Grammar`; the server returns structured JSON that the parser validates.
- **XML text mode:** no grammar either — the model writes
  `<tool_call>{"tool":…}</tool_call>` free-text and parse failures feed the
  format-error feedback loop (SPEC-002), never a generation-level constraint.
- The bare-JSON GBNF builder remains available as `proxy.GBNFConstraint`
  (unit-tested) for a future non-tools/JSON-mode use; it is deliberately not
  wired to native-tools requests.
- **Re-enabling grammar is tracked, not forgotten:** an opt-in, llama.cpp-safe
  re-enablement (envelope-aware grammar for XML text mode, never with native
  tools, per-model toggle) is planned in
  `docs/PLANS/cross-cutting/tool-call-grammar-reenable.md`.

## Fallback Chain

When native tools stream returns empty:
1. **Native-only models** (`usePrefill=false`): skip XML retry, go directly to **non-streaming Chat** + nag prompt.
2. **XML-text models** (`usePrefill=true`): retry via **XML streaming** (disables `useNativeTools`, suppresses `tool_choice` and `reasoningBudget`, stuck detection skipped). If also empty → **non-streaming Chat**.
3. Non-streaming heartbeat uses `fallback_waiting` lifecycle events with elapsed time.

## Tool-Call Batches

All calls of one assistant turn are emitted before any result, so a call depends only on
another's side effects. `toolpolicy.EffectFor(name)` classifies each tool:

| Effect | Tools | Batch after a non-fatal failure |
|---|---|---|
| `EffectReadOnly` | `fetch_url`, `internet_search`, `read_file`, `list_directory`, `memory_search`, `get_network_info`, `scan_local_network` | continues |
| `EffectHousekeeping` | `memory_update`, `automation_journal` | stops |
| `EffectMutating` (default: every unlisted and MCP tool) | `write_file`, terminal, `notify_user`, … | stops |

Invalid arguments, a guardrail denial and a run-fatal error always stop the batch. Every early stop
answers the unrun calls with `prompts.ToolCallNotExecuted` (`appendSkippedToolResults`, under `mu`,
bypassing `appendToolResult` so the guardrail streak and the ledger are untouched) — an unanswered
tool_call id was resent on every later request and is rejected by strict APIs. Failures inside one
batch count once toward `toolFailureStreakLimit` (`toolFailureState.beginBatch`); plan steps and
lone `executeSingleToolStep` calls count one by one.

## Key Constants

| Constant | Value | File | Purpose |
|----------|-------|------|---------|
| `DefaultMaxSteps` | 25 | `agent.go` | Max loop iterations |
| `DefaultContextBudget` | 8000 | `agent.go` | Char limit before sieve |
| `DefaultMaxTokens` | 3072 | `agent.go` | Max tokens per LLM response |
| `DefaultAutomationTemperature` | 0.1 | `agent.go` | Low temp for automation |
| `MinReasoningStuckThreshold` | 2000 | `agent.go` | Floor for stuck detection |
| `AgentGlobalTimeout` | 30 min | `agent.go` | Total wall-clock per Execute |
| `AgentTurnTimeout` | 10 min | `agent.go` | Per-LLM-call timeout |
| `AgentRetryTimeout` | 5 min | `agent.go` | Timeout for non-streaming fallback |
| `DefaultStarvationLimit` | 15 | `session.go` | Consecutive no-tool-call turns before stall error |
| `streamReasoningBudgetDivisor` | 3 | `stream.go` | Divisor for reasoning_budget |
| `emptyToolCallSpiralLimit` | 3 | `stream.go` | Closed empty `<tool_call>` blocks → early stuck |
| `streamMaxDuration` | 5 min | `stream.go` | Max generation time (from the first token) with no tool calls → terminate (preserves content) |
| `reasoningRepeat*` | 64 B window, 4 KB tail, 1 KB step, ≥ 1/2 | `reasoning_repetition.go` | Reasoning that mostly repeats itself → `[stuck]` recovery |
| `minRepetitionFragmentLen` | 400 | `stream.go` | Min content length before repetition guard runs (fail-open) |

## Repetition/Spiral Detector

`repetitionDetector.check()` in `agent.go`:

1. **Exact duplicate args** — streak ≥ 3 → aborts with "infinite loop"
2. **Single-tool spiral** — 12+ consecutive calls to the same tool that **recycle ≤4 distinct argument values** → aborts with "spiral detected". A burst of varied-argument calls to one tool (e.g. an audit automation running a sequence of distinct `execute_terminal_command` calls) is legitimate batching (Constitution II.1), not a spiral — identical-argument repeats are caught earlier by the duplicate detector.
3. Streak < 3 → injects AutomationDuplicateNagPrompt and continues

**Sequence-repeat (n-gram) detector** — `checkSequenceRepeat()`: catches
repeating tool-call cycles of length 3–5 in the last 30 calls. Keys are
**name + args** (full `toolKey`), so a run that legitimately uses one tool
with varying arguments (e.g. a series of distinct `execute_terminal_command`
calls in a storage audit) is never flagged as a cycle; true degenerate cycles
recur with identical arguments and are caught after 3 repeats. Single-tool
stagnation with varied args is still bounded by the 12-consecutive spiral
detector. `checkAlternating()` catches 2-tool oscillation (≤30% unique
(tool, args) calls over 20 calls for 15+ turns), and `checkSameTarget()` catches
many-tools-one-path oscillation within a turn.

## Tool Call Data Flow

### History Normalization (`NormalizeHistory` in `history.go`)

Before each LLM call, history is normalized based on `useNativeTools`:

**When `useNativeTools=true` (native mode):**
- `ToolRole` messages kept as-is with `Content` = raw JSON result
- `ToolCalls` arrays preserved on assistant messages
- Standard OpenAI-style `tools` + `tool_choice` format

**When `useNativeTools=false` (XML mode):**
- `ToolRole` messages **converted to `UserRole`** with `"Tool result [callID]: ..."` prefix
- `ToolCalls` arrays **emptied** and serialized to XML in `Content`:
  ```
  <tool_call>
  {"tool": "memory_search", "args": {"query": "foo"}}
  </tool_call>
  ```
- Consecutive same-role messages are merged (except messages with `ToolCallID`)
- Empty content is filled with placeholders (`"Thinking..."`, `"Tool result: ..."`)
- `ReasoningContent`, `Name`, and non-standard fields are stripped by `SanitizeHistory`

**Effect on debugging:** A history dump with `useNativeTools=false` looks fundamentally
different from one with `true`. There will be no `ToolRole` messages or `ToolCalls` arrays.
The model sees `UserRole` messages with `"Tool result"` text prefixes instead.

### `useNativeTools` Resolution (Three-State `*bool`)

`AgentOptions.UseNativeTools` is a `*bool` — nil, `&false`, or `&true`:

| Value | Behaviour |
|-------|-----------|
| `nil` (unset) | Use `provider.UseNativeTools()` — `LocalToolRegistry` returns `false`, `MockNodeHerder` returns `true` |
| `&true` | Force native mode — `tool_choice: "required"` in automation, `NormalizeHistory` keeps ToolRole |
| `&false` | Force XML mode — `tool_choice` not set, `NormalizeHistory` converts ToolRole → UserRole |

**Important:** `&false` is NOT the same as `nil`. Setting `UseNativeTools: &false`
silently overrides the provider's `UseNativeTools()` return value. This changed the
tool role format in the assistant handler and broke the agnostic flow test.

### `appendToolResult` Double-JSON-Marshaling

In `processToolCalls`, the tool result is marshaled to JSON TWICE:
1. In the caller (`processToolCalls` lines 159-170) to create `finalResult`
2. In `appendToolResult` (line 443) via `json.Marshal(result)`

This is wasteful and can produce different output if `result` is a pointer type
modified between the two calls. When adding new tool execution paths, prefer
passing pre-serialized strings to `appendToolResult`: `string(json.Marshal(result))`.

```go
// Correct — single marshal in caller:
raw, _ := json.Marshal(result)
a.appendToolResult(history, tc, string(raw))
```

## executePlan Execution (plan_execute primitive)

`executePlan` is the shared plan primitive used by `PlanExecuteStrategy` (SPEC-010); the old
`execute_plan` tool is gone. When a plan is generated it bypasses the standard turn loop:

- **Pre-check:** if `len(plan.Steps) > MaxPlanSteps` (default 50), plan fails before any step executes.
- **Plan-level timeout:** whole plan wrapped in `context.WithTimeout(MaxPlanDuration)` (default 15 min).
- **Per-step timeout:** each step uses `executeSingleToolStep` with the same `ToolTimeout`/`FilesystemToolTimeout` as regular tool calls.
- **Inline check:** after each step, `if i >= MaxPlanSteps → abort`.
- **Guardrail checks:** each step/tool goes through `resolveGuardrail` with `GuardrailTimeout`.
- **Step execution failures continue (record-and-continue):** a step whose tool *execution* fails (shell exit code, compile error, missing file, tool timeout) is logged as `plan step failed, continuing` and the next step runs — the failure is already in history as a tool result, so the final report notes it. A single failed step never aborts the run, matching the react loop's `processToolCalls` behavior (model-agnostic: a weak model's mistakes become reported issues, not run-killers). Only structural errors abort: args marshal failure, `validateToolArgs` failure, `MaxPlanSteps` exceeded, plan deadline exceeded.
- **Guardrail-denied steps continue** (`plan step guardrail denied, continuing`); in unattended automation the denial is immediate (Constitution II.10).

## Important Gotchas

- Do NOT terminate on reasoning budget exceeded. The server enforces it; the proxy only warns.
- Do NOT change `streamReasoningBudgetDivisor` from 3. Divisor 4 caused recompilation loops.
- The `budgetWarned` flag prevents log spam — warning fires once per stream, not 200+ times.
- Empty stream fallback must use XML streaming, NOT Chat directly (the Chat path was the old buggy behaviour).
- **Citation forcing** — A 2-line prompt rule in `templates.go` can force the model to articulate memory before acting: "When you use information from a `<memory>` or `<relevant_memories>` block, begin your thought with 'Based on retrieved memory:' before acting." This makes cache hits salient in the output and self-correcting on misses.
