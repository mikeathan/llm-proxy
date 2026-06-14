# Agent Loop — Execution, Sieve, Stuck Detection & Fallback

**Source docs:** SPEC-001, `docs/PLANS/agent-loop/refactor-assistant-clean-code.md`, `docs/audits/agent-stability-report.md`

---

## Execution Flow

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
  │     │     │     └── execute tools → append to history → loop
  │     │     └── max_steps reached or done
  │     └── Return result
```

## Context Budget & Physical Sieve

- `context_budget` in chars (default 8000, overridable per-model via settings.yml)
- When total chars exceeds budget, `applyPhysicalSieve()` fires:
  1. Keep locked head (system message + first user message)
  2. Insert sieve marker: `[System Note: History distilled...]`
  3. Keep priority tail (last 5-10 messages depending on stuck count)
  4. Before dropping, compress long `Content` (>4000 chars) and `ReasoningContent` (>2000 chars) with head+tail truncation

## Stuck Detection

**Reasoning stuck (token-level):** When reasoning content exceeds `maxTokens * 2` chars (floor 2000), the stream is aborted. A `lifecycle` event with phase `stuck_detected` is emitted.

**Early stuck for models without reasoning budget:** When `reasoningBudget == 0` (no server-side thinking enforcement), stuck fires at `maxTokens / stuckNonReasoningDivisor` chars instead — currently divisor=1 gives threshold at `maxTokens` (e.g. 2048 chars for a local model). Divisor=2 was tried but caused false positives on Gemma 4 (~1371 chars of legitimate `<think>` blocks before output). See `stream.go` `stuckNonReasoningDivisor` and `checkStreamStuck()`.

**Progressive sieve recovery:**
- 1st stuck → reactive sieve (first 2 + last 6 messages) + nag prompt
- 2nd consecutive stuck → aggressive sieve (first 2 + last 3 messages) + stronger nag
- 3rd consecutive stuck → agent fails with clear error

**Stuck detection is skipped on XML fallback retries** (the `skipStuckCheck` flag).

## Reasoning Budget

- Auto-computed as `maxTokens / 3` (divisor = 3, do NOT change)
- Sent as both `reasoning_budget` and `thinking_budget_tokens` for broad provider compatibility
- **Warn-only, never terminate** — the proxy warns when exceeded but does NOT kill the stream
- Model-specific: llama.cpp enforces at API level, OpenAI ignores

## Output Constraint (GBNF Grammar)

When native tools are active in automation (`useNativeTools = true`, `tool_choice: "required"`),
the agent applies a **GBNF grammar constraint** to prevent malformed JSON in tool call arguments:

- **Local providers** (llama.cpp): `GBNFConstraint` generates a disjunctive GBNF grammar from
  all tool schemas and sets `req.Grammar`. This constrains token generation so the model can
  only produce valid JSON matching one of the available tool schemas.
- **Cloud providers** (OpenAI, Gemini): Skipped — their native tool API already returns
  structured valid JSON.
- **XML text mode**: Skipped — `useNativeTools` is false, so `llmTools` is nil and the
  constraint code is never reached.

The constraint is optional (never fatal). If `Apply()` returns false (unsupported types,
empty schema, etc.), the request is sent unchanged and the existing parsing handles it.

See `proxy/tool_constraint.go` `GBNFConstraint` and `stream.go` `buildChatRequest()`.

## Fallback Chain

When native tools stream returns empty:
1. **Native-only models** (`usePrefill=false`): skip XML retry, go directly to **non-streaming Chat** + nag prompt.
2. **XML-text models** (`usePrefill=true`): retry via **XML streaming** (disables `useNativeTools`, suppresses `tool_choice` and `reasoningBudget`, stuck detection skipped). If also empty → **non-streaming Chat**.
3. Non-streaming heartbeat uses `fallback_waiting` lifecycle events with elapsed time.

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
| `streamReasoningBudgetDivisor` | 3 | `stream.go` | Divisor for reasoning_budget |

## Repetition/Spiral Detector

`repetitionDetector.check()` in `agent.go`:

1. **Exact duplicate args** — streak ≥ 3 → aborts with "infinite loop"
2. **Same tool, any args** — 12+ consecutive calls to same tool → aborts with "spiral detected"
3. Streak < 3 → injects AutomationDuplicateNagPrompt and continues

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

## Important Gotchas

- Do NOT terminate on reasoning budget exceeded (#19 in AGENTS.md). The server enforces it.
- Do NOT change `streamReasoningBudgetDivisor` from 3. Divisor 4 caused recompilation loops.
- The `budgetWarned` flag prevents log spam — warning fires once per stream, not 200+ times.
- Empty stream fallback must use XML streaming, NOT Chat directly (the Chat path was the old buggy behaviour).
- **Citation forcing** — A 2-line prompt rule in `templates.go` can force the model to articulate memory before acting: "When you use information from a `<memory>` or `<relevant_memories>` block, begin your thought with 'Based on retrieved memory:' before acting." This makes cache hits salient in the output and self-correcting on misses.
