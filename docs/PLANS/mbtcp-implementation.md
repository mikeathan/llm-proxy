---
status: proposed
date: 2026-06-03
related_specs: [SPEC-004]
---
# MBTCP: Memory-Backed Tool Call Pre-emption

**Status:** Proposed
**Date:** 2026-06-03

## Problem

The model re-runs discovery commands (`uname -a`, `npx tsc --version`) even when
memory already has the answer. Every prompt-level approach (system prompt injection,
prefill, annotations, MemoryCheckGate, Rule 7 rewrite) has failed for 4B models
because the explicit step instruction overrides general memory guidance.

## Solution

Intercept tool calls in Go code AFTER the model decides to call them but BEFORE
the tool executes. Search the memory store with the command string. If a match
is found with word overlap and recency, inject the stored result as a synthetic
tool result. The model sees the output and moves on — it never knows the tool
was intercepted.

This is the only approach (besides the removed LLM rewriter) that does NOT require
the model to follow any instruction about skipping.

## Architecture

### Interception point

`session.go` `run()` — between the turn message append and `processToolCalls()`:

```
Current flow:
  line 257: s.history = append(s.history, turnMsg)
  line 258: s.agent.notify(EventMessage, turnMsg)
  line 259: s.agent.processToolCalls(s.ctx, turnMsg, &s.history)

Proposed flow:
  line 257: s.history = append(s.history, turnMsg)
  line 258: s.agent.notify(EventMessage, turnMsg)
  [NEW]:   s.interceptRedundantToolCalls(&turnMsg, &s.history)
  line 259: s.agent.processToolCalls(s.ctx, turnMsg, &s.history)
```

### Interception function

```
For each tool call in turnMsg.ToolCalls:
  1. Skip non-terminal tools (only execute_terminal_command is intercepted)
  2. Search FTS5 with the full command string as query
  3. If no match → tool executes normally (safe failure)
  4. Precision filter: command must share a non-stop-word with the memory entry
  5. Recency check: entry must be less than 30 days old
  6. Remove the tool call from turnMsg.ToolCalls
  7. Append a synthetic ToolRole message via appendToolResult()
  8. Log: "memory pre-empted tool call | tool X | command Y | match Z"
```

### Memory matching with precision filter

The search uses the full command string (`"npx tsc --version"`) to maximize FTS5
recall. A precision filter then verifies the result shares a non-stop-word with
the command. This prevents `ls -la` from matching a compliance memory entry.

| Command | Memory entry | FTS5 match | Word overlap | Outcome |
|---|---|---|---|---|
| `npx tsc --version` | `tool_versions: TypeScript 6.0.3` | ✅ | ✅ `version` | Intercepted |
| `uname -a` | `system_os_info: OS: Darwin...` | ❌ (no shared words) | N/A | Executes |
| `npm install --save-dev typescript` | `tool_versions: TypeScript 6.0.3` | ✅ | ✅ `typescript` | Intercepted |
| `ls -la` | `workspace_init: file list...` | ✅ (maybe "file") | ❌ no non-stop match | Executes |
| `date -u +...` | any | ❌ | N/A | Executes |

### Safety

- **Recency check**: entries older than 30 days are skipped (tool runs fresh)
- **Precision filter**: shared non-stop-word required — prevents coincidental matches
- **Replay safety**: skipped when `RecordingRef != ""` or memory store is nil
- **Failure mode**: no match → tool runs normally, identical to current behavior
- **Duplicate safety**: if model re-queues same tool after synthetic result, the
  repetition detector catches it as a normal duplicate (no special handling needed)

## Files changed

| File | Change | Lines |
|---|---|---|
| `session.go` | Add `interceptRedundantToolCalls()`, word overlap helpers, wire into `run()` | ~55 |
| `session_test.go` | Add test with mock memory store — verify interception and non-interception cases | ~80 |

## Tests

| Test | What it verifies |
|---|---|
| `TestIntercept_ExecCommandWithMemoryMatch` | `npx tsc --version` with matching `tool_versions` entry → tool removed from turnMsg, synthetic result appended |
| `TestIntercept_NoMemoryMatch_ExecutesNormally` | `uname -a` with no matching memory → tool calls unchanged |
| `TestIntercept_NilStore_NoOp` | `memoryStore == nil` → no interception |
| `TestIntercept_StaleMemory_Skipped` | Entry with `updated_at` > 30 days ago → tool executes |

## Non-goals

- Not changing the existing memory injection pipeline (injectActiveMemory stays)
- Not adding auto-save or classification logic for tool results
- Not changing the prompt, system prompt, or task content
- Not adding any LLM calls or startup latency
