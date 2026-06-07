---
status: complete
date: 2026-06-03
related_specs: [SPEC-004]
---
# Remove Memory Rewriter and Fix FTS5 Query Sanitization

**Status:** Completed
**Date:** 2026-06-03

## Problem

Two performance and correctness issues in the automation system:

### 1. Memory rewriter adds 77-89s startup latency

The `rewriteTaskWithMemories()` call in `executor.go` sends a separate Chat request to the LLM
before the agent loop starts. This LLM call rewrites the task instructions with memory-check
gates embedded into each step. The call takes 77-89 seconds because:

- The rewriter uses a unique system prompt (`TaskRewriterSystemPrompt`) that never hits the
  llama.cpp prompt cache
- Generating up to 2048 tokens at ~21 t/s takes ~97s in the worst case
- This is a cold-model penalty on every automation run

The rewriter was added (see pitfall #19 in AGENTS.md) to fix "agent ignores stored memories."
But the industry-standard approach — injecting `<relevant_memories>` into the system prompt
via `injectActiveMemory()` + telling the agent to check it — already works. The last
successful run proved it: the agent skipped `npm install --save-dev typescript` and
`npx tsc --version` using only the existing injected memories + prompt guidance.

### 2. FTS5 syntax error on `memory_search`

When the agent searches for `"typescript_version OR tool_versions"`, the `sanitiseFTSQuery`
function in `store.go` strips non-alphanumeric characters but doesn't protect FTS5 operator
keywords. The agent's literal `OR` collides with FTS5's boolean `OR` operator:

```
Input:  "typescript_version OR tool_versions"
Filter: typescript version OR tool versions
Terms:  ["typescript", "version", "OR", "tool", "versions"]
Joined: "typescript OR version OR OR OR tool OR versions"
                                    ^^^^
                              Two FTS5 operators with no term between = crash
```

Result: `SQL logic error: fts5: syntax error near "OR"`

### 3. Two recording files per run (side effect of rewriter)

The rewriter call uses `ctx` (no TaskName/RunID), while the agent loop uses `execCtx`
(with TaskName/RunID). The RecordingClient creates one file in `{model}/` for the
rewriter and another in `{model}/{taskName}/` for the agent loop.

---

## Design (informed by industry-standard patterns)

Research of [Hermes Agent](https://hermes-agent.nousresearch.com/docs/developer-guide/prompt-assembly),
[OpenClaw](https://docs.openclaw.ai/concepts/memory), Cursor, and Claude Code shows the
same pattern across every major agent framework:

1. **Write**: Agent saves durable facts to a memory store (database or markdown files)
2. **Inject**: At session start, relevant memories are injected into the system prompt as
   a static text block — **zero additional LLM calls**
3. **Instruct**: The system prompt tells the agent to check memories before each step

No framework uses a secondary LLM call to rewrite task instructions. The rewriter is
over-engineered and the delay is unacceptable.

### Changes

#### 1. Remove `rewriteTaskWithMemories()` from `executor.go`

**File:** `internal/core/automation/executor.go`

- Remove `rewriterMaxTokens` and `rewriterTimeout` constants
- Remove `rewriteTaskWithMemories()` method
- Remove the rewriter call from `Execute()` (lines ~208-219)
- Make `MemoryCheckGate` unconditional — always prefix it to `req.TaskContent` before
  calling `buildPrompt()`. This is a static text block (~100 chars) with no LLM overhead.

The `MemoryCheckGate` + `injectActiveMemory()` + `AutomationTaskPrompt` already provide
the full industry-standard memory injection stack:

| Layer | What | Cost |
|-------|------|------|
| `injectActiveMemory()` | `<relevant_memories>` block appended to system prompt each turn | 0ms, 0 LLM |
| `MemoryCheckGate` | "[Memory Check Gate]" block prepended to task instructions | 0ms, 0 LLM |
| `AutomationTaskPrompt` | "Review the <relevant_memories> block above before each step" | 0ms, 0 LLM |

#### 2. Fix FTS5 sanitization in `store.go`

**File:** `internal/platform/memory/store.go`

In `sanitiseFTSQuery()`, wrap each term in double-quotes after splitting on whitespace.
FTS5 treats quoted strings as literal terms, so `"OR"` is a literal word, not an operator.

```go
// Before:
return strings.Join(terms, " OR ")

// After:
quoted := make([]string, len(terms))
for i, t := range terms {
    quoted[i] = `"` + t + `"`
}
return strings.Join(quoted, " OR ")
```

This is safe: FTS5 supports quoted terms for prefix matching. Unicode and multi-word terms
are handled correctly. Empty terms are already filtered by `strings.Fields()`.

### Non-goals

- Not changing how `injectActiveMemory()` works (industry standard, already correct)
- Not changing `AutomationTaskPrompt` (already has memory review instructions)
- Not changing the warn-only reasoning budget (already implemented in stream.go)
- Not changing `ApplyModelOverrides` sync logic (separate fix, already implemented)

---

## Files changed

| File | What changes | Status |
|------|-------------|--------|
| `internal/core/automation/executor.go` | Remove rewriter constants, method, and call. Make `MemoryCheckGate` unconditional | ✅ |
| `internal/core/automation/executor_test.go` | Remove rewriter tests, add `TestBuildPrompt_IncludesMemoryCheckGate` | ✅ |
| `internal/platform/memory/store.go` | Fix `sanitiseFTSQuery` to quote terms | ✅ |
| `internal/platform/memory/store_test.go` | Add 5 `TestSanitiseFTSQuery_*` cases | ✅ |
| `AGENTS.md` | Pitfall #19: industry-standard memory injection (not rewriter) | ✅ |
| `docs/plans/remove-memory-rewriter.md` | This document | ✅ |

---

## Tests

| Test | File | What it verifies |
|------|------|-----------------|
| `TestSanitiseFTSQuery_NormalTerms` | `store_test.go` | Simple alphanumeric query unchanged |
| `TestSanitiseFTSQuery_OR_Collision` | `store_test.go` | Query containing `OR` keyword doesn't crash |
| `TestSanitiseFTSQuery_Underscore` | `store_test.go` | `typescript_version` → `"typescript" OR "version"` |
| `TestSanitiseFTSQuery_SpecialChars` | `store_test.go` | Non-alphanumeric chars stripped |

---

## Edge cases

| Scenario | Outcome |
|----------|---------|
| Memory store disabled (nil) | Nag not injected, no rewriter called (already handled by nil check) |
| No memories exist | Nag not injected (no prior facts to check), agent runs normally |
| Agent searches for `foo OR bar` | `"foo" OR "bar"` — no FTS5 crash |
| Agent searches for `AND` or `NOT` | `"AND"` or `"NOT"` — no FTS5 crash |
| Agent uses `memory_search` with quotes already | `"typescript"` → `""typescript""` — FTS5 handles doubled quotes as escapes |

---

## Verification

1. `go test ./internal/core/automation/...` — all pass (rewriter tests removed)
2. `go test ./internal/platform/memory/...` — all pass (new FTS5 tests added)
3. `go test ./...` — all 27+ packages pass
4. `go vet ./...` — no issues
5. Manual: run smoke test — verify:
   - No 77-89s startup delay
   - Agent still skips discovery steps when memory has the facts
   - `memory_search "typescript_version OR tool_versions"` no longer crashes
   - Single recording file generated
