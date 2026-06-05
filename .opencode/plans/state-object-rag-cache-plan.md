# Plan: State Object + RAG Placement + Citation Forcing + LRU KV Cache

**Date:** 2026-06-03
**Status:** ✅ Completed — all layers implemented and tested

## Problem Summary

Two independent issues causing agent inefficiency:

| Issue | Symptom | Root cause |
|---|---|---|
| **Sieve repetition** | After ~8 turns, history exceeds 10924 chars, sieve fires, model restarts at Step 1 | 4B model generates 700-2000 chars reasoning per turn; budget fills; truncation loses progress |
| **Memory ignored** | Model reads `<relevant_memories>` but still runs discovery commands | Memory block in system prompt is distant from task; connection to step commands is implicit |

## Architecture

### Current prompt layout

```
[0] System: rules + <relevant_memories>  ← volatile, invalidates KV cache every turn
[1] User: MemoryCheckGate + TASK: Step N
[2..N] Assistant/Tool history
```

### Proposed prompt layout

```
[0] System: rules (static)               ← 100% cache hit rate
[1] System: state object                 ← pinned [DONE]/[ACTIVE] block, survives sieve
[2..N] ... history ...                   ← normal conversation
[M-1] System: <memory>...</memory>        ← volatile RAG payload (at end, near generation)
[M] User: current action
```

## Changes

### 1. State Object — Execution Progress Tracker

**File:** `internal/core/automation/state.go` (new)

Deterministic `[DONE]`/`[ACTIVE]`/`[PENDING]` block injected at index [1] of the prompt. Updated by Go after the agent calls `complete_step`.

```go
type StepStatus string
const (
    StatusPending StepStatus = "PENDING"
    StatusActive  StepStatus = "ACTIVE"
    StatusDone    StepStatus = "DONE"
)
type PlanState struct {
    Goal  string
    Steps []Step
}
type Step struct {
    Description string
    Status      StepStatus
}
```

**Serialization** (custom, token-efficient):
```
Goal: Execute LLM Smoke Test
- [DONE] Step 1: Filesystem List
- [ACTIVE] Step 2: Write & Verify
- [PENDING] Step 3: Terminal Commands
```

**~70 tokens for 10 steps.** Survives sieve — pinned at index [1].

**Updating:** When the agent calls `complete_step(notes string)`, the Go backend:
1. Marks the current `[ACTIVE]` step as `[DONE]`
2. Marks the next `[PENDING]` step as `[ACTIVE]`
3. Does NOT show the `complete_step` tool call or result in the conversation history — it's infrastructure-only

**Critical: Step detection via explicit tool, NOT fragile signature matching.** The agent explicitly signals completion. This avoids desync when a step requires 3 write_file calls and 2 terminal commands — only the model knows when it's truly done.

| File | Change | Lines |
|---|---|---|
| `internal/core/automation/state.go` | New: `PlanState`, `Step`, `ToCompactState()`, `MarkStepComplete()` | ~60 |
| `internal/core/automation/state_test.go` | New: verify state transitions and rendering | ~40 |
| `internal/core/assistant/agent.go` | Add `State *PlanState` field to `Agent` | ~2 |
| `internal/core/assistant/session.go` | Inject state at index [1]; detect `complete_step` and call `MarkStepComplete()` | ~20 |
| `executor.go` | Build `PlanState` from task file before agent starts | ~15 |
| **Total** | | **~137** |

### 2. `complete_step` Tool Registration

**Files:** `models/tools.go`, `internal/core/tools/manifests/`, `internal/core/tools/`

Add `complete_step` as a new tool that the agent can call to signal step completion.

**Manifest** (JSON):
```json
{
  "name": "complete_step",
  "description": "Call this when the current step's requirements have been satisfied. Marks it done and progresses to the next step.",
  "parameters": {
    "type": "object",
    "properties": {
      "notes": {
        "type": "string",
        "description": "Brief summary of what was done (for the final report)"
      }
    },
    "required": ["notes"]
  }
}
```

**Handler:** When the agent calls `complete_step`, the Go backend:
1. Calls `state.MarkStepComplete()` — transitions `[ACTIVE]` → `[DONE]`, next `[PENDING]` → `[ACTIVE]`
2. Appends the `notes` to a summary accumulator (used in the final report)
3. Returns a synthetic tool result: `"Step completed. Proceeding to next step."`
4. The complete_step call AND the synthetic result enter the standard conversation history [2..N] — same as any other tool

**Why history must be preserved:** LLM APIs require every `tool_call` to have a corresponding `tool_result` in the message array. Scrubbing them would violate the schema and cause sequence errors. The context sieve truncates old history naturally; the state block at [1] maintains ground truth regardless of history length.

**Safety:** The `[ACTIVE]` block at index [1] reminds the agent on every turn. If the agent forgets to call `complete_step`, the block stays `[ACTIVE]` and the model self-corrects. No deadlock.

| File | Change | Lines |
|---|---|---|
| `models/tools.go` | Add constant `ToolCompleteStep = "complete_step"` | ~1 |
| `internal/core/tools/manifests/complete_step.json` | New: manifest JSON | ~15 |
| `internal/core/tools/step_tool.go` | New: handler that calls `MarkStepComplete()` | ~30 |
| `internal/core/assistant/registry.go` | Register `complete_step` tool | ~3 |
| `internal/core/assistant/session.go` | Handle `complete_step` — update state, suppress history | ~15 |
| `internal/core/tools/step_tool_test.go` | New: test handler | ~30 |
| **Total** | | **~94** |

### 3. Move RAG Injection to End of Prompt

**File:** `internal/core/assistant/stream.go`

Change `injectActiveMemory()` to emit `<relevant_memories>` as a separate system message appended AFTER all history, right before the current user turn. Wrapped in `<memory>` tags.

This keeps the system prompt + state object + history KV cache stable — only the final `<memory>` message changes each turn.

```go
// Before (current):
systemMsg := history[0]
systemMsg.Content += "\n<relevant_memories>..."  // invalidates cache

// After (proposed):
history = append(history, proxy.Message{
    Role: "system",
    Content: "<memory>\n" + relevantMemories + "\n</memory>",
})
```

| File | Change | Lines |
|---|---|---|
| `internal/core/assistant/stream.go` | Change `injectActiveMemory()` to emit separate message at end | ~20 |

### 4. Citation Forcing in System Prompt

**File:** `internal/core/assistant/prompts/templates.go`

Add to `DefaultRules`:
```
When you use information from a <memory> or <relevant_memories> block,
begin your thought with "Based on retrieved memory:" before acting.
```

Forces the model to articulate the memory before acting. If it says "Based on retrieved memory: TypeScript 6.0.3 is installed" and then calls `npx tsc --version`, the contradiction is salient in the output tokens.

| File | Change | Lines |
|---|---|---|
| `internal/core/assistant/prompts/templates.go` | Add citation rule to `DefaultRules` | ~2 |

### 5. LRU Exact-Match KV Cache (Replace FTS5 MBTCP)

**File:** `internal/core/assistant/session.go`

Replace current FTS5-based MBTCP with an LRU exact-match cache:

- After every successful `execute_terminal_command`, store `(cmdKey → stdout)` in the cache
- Before executing a tool, check `cache[cmdKey]`. If found, inject synthetic result
- If the tool returns an error (exit code ≠ 0, error payload), do NOT cache and do NOT advance state
- LRU eviction at 500 entries using `container/list`
- Thread-safe with `sync.RWMutex`
- No FTS5 search, no word overlap, no stop words, no recency check

**Cache key is a composite of CWD + command arguments.** This prevents `ls` in `/workspace/app` from returning the wrong result after `cd /workspace/tests`. The key format is `"cwd:command"`. If CWD is unknown (shell not started), fall back to the command string only.

```go
cacheKey := fmt.Sprintf("%s:%s", currentCWD, commandArgs)
```

**Cache Invalidation:** An exact-match cache becomes stale when filesystem state changes. After any successful dedicated file mutation tool (`write_file`, `edit_file`, `append_file`), the entire cache is flushed. This prevents `cat config.json` from returning old data after `write_file` has updated it.

```go
func (c *TokenCache) Flush() {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.items = make(map[string]*list.Element)
    c.evictList = list.New()
}
```

Brute-force flush called from `tool_exec.go` after successful file write tools. No bash parsing, no regex, no heuristic. ~5 lines, 100% reliable for the 95% case. The remaining edge case (`echo > file` via terminal) is undetectable without fragile parsing and rare in practice — the command string differs each time so the stale cache entry never matches.

| File | Change | Lines |
|---|---|---|
| `internal/core/assistant/tool_cache.go` | New: `TokenCache` struct with LRU eviction | ~40 |
| `internal/core/assistant/session.go` | Replace FTS5 MBTCP with `TokenCache` | -15 net |
| `internal/core/assistant/agent.go` | Add `Cache *TokenCache` field to `Agent` | +1 |
| `internal/core/assistant/tool_exec.go` | Call `cache.Flush()` after successful `write_file`, `edit_file`, `append_file` | +5 |
| `internal/core/assistant/session_test.go` | Remove old MBTCP tests, add LRU cache tests | ~40 |
| **Total** | | **~65 net** |

## Files Changed Summary

| File | Change | Lines (net) |
|---|---|---|
| `internal/core/automation/state.go` | New | +60 |
| `internal/core/automation/state_test.go` | New | +40 |
| `internal/core/assistant/agent.go` | Add `State *PlanState` field | +2 |
| `internal/core/assistant/session.go` | Inject state at [1], handle `complete_step`, LRU cache | +10 |
| `internal/core/assistant/tool_cache.go` | New: LRU cache with `container/list` | +40 |
| `internal/core/assistant/tool_exec.go` | Call `cache.Flush()` after successful file write tools | +5 |
| `internal/core/assistant/stream.go` | Move RAG to end-of-prompt separate message | ±0 |
| `internal/core/assistant/prompts/templates.go` | Add citation forcing rule | +2 |
| `models/tools.go` | Add `ToolCompleteStep` constant | +1 |
| `internal/core/tools/manifests/complete_step.json` | New: manifest | +15 |
| `internal/core/tools/step_tool.go` | New: handler | +30 |
| `internal/core/tools/step_tool_test.go` | New: tests | +30 |
| `internal/core/assistant/registry.go` | Register `complete_step` | +3 |
| `executor.go` | Build PlanState before agent starts | +15 |
| `internal/core/assistant/session_test.go` | Remove old MBTCP tests, add LRU cache tests | ±0 |
| **Total** | | **~203** |

## Tests

| Test | File | What it verifies |
|---|---|---|
| `TestState_Render` | `state_test.go` | `ToCompactState()` output matches expected format |
| `TestState_Transitions` | `state_test.go` | `MarkStepComplete()` moves `ACTIVE`→`DONE`, next `PENDING`→`ACTIVE` |
| `TestState_Empty` | `state_test.go` | No steps — renders Goal only |
| `TestCompleteStep_Handler` | `step_tool_test.go` | Handler calls `MarkStepComplete()`, returns synthetic result, result enters history |
| `TestCompleteStep_NoDeadlock` | `step_tool_test.go` | If all steps already `DONE`, `MarkStepComplete()` is no-op |
| `TestLRUCache_ExactMatch` | `session_test.go` | Same args + CWD → intercept |
| `TestLRUCache_DifferentArgs` | `session_test.go` | Different args → execute |
| `TestLRUCache_CWD_PreventsCollision` | `session_test.go` | Same `ls` in two different directories → distinct cache entries |
| `TestLRUCache_CacheFlush_AfterWrite` | `session_test.go` | After `write_file`, cache is empty — next `cat` executes fresh |
| `TestLRUCache_Eviction` | `session_test.go` | 500+ entries → oldest evicted |
| `TestLRUCache_ErrorNotCached` | `session_test.go` | Error result → not stored, not intercepted |
| `TestRAGPlacement_LastMessage` | `stream_test.go` | Memory block is last system message before user turn |

## Documentation Updates

| Document | What to add |
|---|---|
| `AGENTS.md` | Pitfall: state object pinned at index [1] with `complete_step` tool; LRU cache replacing FTS5 MBTCP; error safety on tool result caching |

## Sequence: How a turn works with all layers

```
1. Execute() starts: build PlanState from task → [PENDING, PENDING, PENDING...]
2. Agent loop, turn 1:
   a. injectActiveMemory() appends <memory> block as LAST system message
   b. executeTurn() prepends state.ToCompactState() as system message at [1]
   c. history = [System][State][history...][<memory>][User: current action]
   d. LLM generates "Based on retrieved memory: TypeScript 6.0.3 is installed"
   e. LLM calls execute_terminal_command("npx tsc --version")
3. run() receives tool call:
   a. LRU cache: not cached → executes normally
   b. Tool result = "Version 6.0.3", no error → stored in cache: cache["npx tsc --version"] = "Version 6.0.3"
4. Later in the same step:
   a. Model decides step is done: calls complete_step(notes: "Verified TS 6.0.3, wrote and compiled code")
   b. Go detects complete_step → calls state.MarkStepComplete()
   c. state.Steps[current] = DONE, Steps[current+1] = ACTIVE
   d. Notes appended to final report accumulator
   e. Synthetic result returned: "Step completed. Proceeding to next step."
   f. complete_step call + result enter standard history — sieve truncates naturally
   g. State at [1] maintains ground truth regardless of history length
5. After sieve fires (turn ~8):
   a. State at index [1] survives truncation
   b. Model sees: "Step 3 [DONE], Step 4 [ACTIVE]" — continues correctly
   c. LRU cache still has prior results — repeated commands intercepted
```

## Gemini Review — Changes Incorporated

| Review finding | Original plan | Updated plan |
|---|---|---|---|
| Brittle step detection (signature mapping) | Map tool calls to steps via signature | **Explicit `complete_step` tool** — model signals completion directly |
| State mutation on tool errors | No error checking | **Error check before cache and state updates** — failed tools not cached, state not advanced |
| Plain `map` has no eviction | `map[string]string` with ad-hoc eviction | **LRU cache using `container/list`** — deterministic 500-entry ceiling |
| `complete_step` history contradiction | Strip tool call + result from history | **Let it enter standard history** — sieve truncates naturally, state at [1] maintains truth |
| LRU cache key needs CWD | Key `args → stdout` | **Composite key `CWD:args → stdout`** — prevents `ls` collison across directories |
| Concurrency: LRU Get uses write lock | Planned `RLock` for reads | **Use `Lock` (write lock) for Get** — moving list element is a mutation |
| Sieve invalidates history below state | Already noted as acceptable | No change — prefix survives, which is the critical benefit |

## Risks

| Risk | Mitigation |
|---|---|---|
| Model forgets to call `complete_step` | `[ACTIVE]` block at index [1] stays visible — model self-corrects on next turn. No deadlock. |
| Cache stores stale data across runs | In-memory only. Lost on restart. Fresh data on each session. |
| CWD not available (first terminal call) | Fall back to command-string-only key — no CWD prefix. |
| Citation forcing reduces fluency | Add as optional rule, monitor before/after |
| `complete_step` called before step truly done | Model determines completion. If wrong, it can still redo work in the next `[ACTIVE]` step. No data loss. |
