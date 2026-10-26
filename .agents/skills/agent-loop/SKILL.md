---
name: agent-loop
description: "Agent-loop mechanics (reference): execution flow, structural/reactive sieve, stuck/spiral detection, reasoning budget, fallback chain, GBNF, and key constants. Load alongside debugging when the failure is in the loop."
metadata:
  last_reviewed: "2026-10-26"
---

# Agent Loop — Execution, Sieve, Stuck Detection & Fallback

**Source docs:** SPEC-001, `docs/PLANS/ARCHIVE/agent-loop/refactor-assistant-clean-code.md`, `docs/audits/agent-stability-report.md`

> Debugging workflow: [`debugging`](../debugging/SKILL.md). Planning a loop change: [`task-planning`](../task-planning/SKILL.md).

---

## Reference files

Open only the one your task needs:

| File | Open it when |
|------|--------------|
| [`references/stuck-and-repetition.md`](references/stuck-and-repetition.md) | A run loops, repeats a tool call, or is wrongly declared stuck. |
| [`references/completion-and-control-plane.md`](references/completion-and-control-plane.md) | A run finishes too early, never finishes, or a truncated write is salvaged. |
| [`references/tool-calls.md`](references/tool-calls.md) | You are changing how tool calls are parsed, batched, constrained by grammar, or executed. |
| [`references/plan-execute.md`](references/plan-execute.md) | You are changing the plan-execute strategy. |

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
  1. Keep locked head (system message + first user message; never a tool call, `keptHead`) and, in a later turn of a conversation, the current question between it and the note (`keptQuestion`)
  2. Insert sieve marker: `[System Note: History distilled...]`, then the progress ledger message (`ledger.go`; facts only, one per prune, bounded to a share of `ContextBudget`). Each prune REPLACES earlier sieve messages (`withoutSieveMessages`); the "deliver final answer NOW" warning is sent once per run
  3. Keep priority tail (last 5-10 messages depending on stuck count); it starts at the call that made its first result (`tailStart`), so a call is never kept without its result. `proxy.RepairToolPairing` heals a saved history once at `newRunSession`
  4. `TokenUsage.CachedTokens` (llama.cpp `cache_n`) is summed per run and logged per call: a history rewrite shows as `cached_tokens` collapsing to ~0
  4. Before dropping, compress long `Content` (>4000 chars) and `ReasoningContent` (>2000 chars) with head+tail truncation

## Reasoning Budget

- Auto-computed as `maxTokens / 3` (divisor = 3, do NOT change)
- Sent as both `reasoning_budget` and `thinking_budget_tokens` for broad provider compatibility
- **Warn-only, never terminate** — the proxy warns when exceeded but does NOT kill the stream
- Model-specific: llama.cpp enforces at API level, OpenAI ignores

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
| `AgentRetryTimeout` | 5 min | `agent.go` | Timeout for non-streaming fallback |
| `DefaultStarvationLimit` | 15 | `session.go` | Consecutive no-tool-call turns before stall error |
| `streamReasoningBudgetDivisor` | 3 | `stream.go` | Divisor for reasoning_budget |
| `emptyToolCallSpiralLimit` | 3 | `stream.go` | Closed empty `<tool_call>` blocks → early stuck |
| `streamMaxDuration` | 5 min | `stream.go` | Max generation time (from the first token) with no tool calls → terminate (preserves content) |
| `reasoningRepeat*` | 64 B window, 4 KB tail, 1 KB step, ≥ 1/2 | `reasoning_repetition.go` | Reasoning that mostly repeats itself → `[stuck]` recovery |
| `minRepetitionFragmentLen` | 400 | `stream.go` | Min content length before repetition guard runs (fail-open) |

## Important Gotchas

- Do NOT terminate on reasoning budget exceeded. The server enforces it; the proxy only warns.
- Do NOT change `streamReasoningBudgetDivisor` from 3. Divisor 4 caused recompilation loops.
- The `budgetWarned` flag prevents log spam — warning fires once per stream, not 200+ times.
- Empty stream fallback must use XML streaming, NOT Chat directly (the Chat path was the old buggy behaviour).
- **Citation forcing** — A 2-line prompt rule in `templates.go` can force the model to articulate memory before acting: "When you use information from a `<memory>` or `<relevant_memories>` block, begin your thought with 'Based on retrieved memory:' before acting." This makes cache hits salient in the output and self-correcting on misses.
