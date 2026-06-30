# Plan: Graceful Tool Failure Recovery

**Status:** Completed
**Date:** 2026-05-31
**Problem:** When a tool (e.g. `fetch_url`) repeatedly returns a non-retryable error
(e.g. HTTP 503), the model retries until the repetition detector triggers a fatal
"infinite loop detected" error, killing the entire automation.

**Root cause:** Two gaps in the agent loop:

1. The `repetitionDetector` (agent.go:278) fires a **fatal error** at streak 3 —
   the automation dies instead of recovering gracefully.
2. The detector only catches **consecutive identical** calls. If the model
   intermixes other tools (e.g. tries 503, reads a file, tries again), the
   streak resets and the loop continues indefinitely until starvation/timeout.

---

## Design (informed by Hermes Agent patterns)

[Hermes Agent](https://github.com/NousResearch/hermes-agent) solves the same
problem in `agent/tool_guardrails.py` with a `ToolCallGuardrailController` that:

- Tracks **exact failures** (tool + canonical-args hash) — same call repeated
- Tracks **same-tool failures** (tool name, any args) — same tool, different args
- Has **two severity levels**: `warn` (append guidance to result) and `block`
  (synthetic error, prevent execution)
- **Resets counters on success** — a single successful call clears the state
- **Reset per turn** for within-turn loop detection, but our architecture calls
  tools one-per-turn, so our counters live at the **session level** with the
  Hermes-style "clear on success" behavior

Our approach adapts these ideas to Go/single-call-per-turn:

### Changes

#### 1. Repetition detector: streak 3 → skip instruction, not fatal

**File:** `internal/core/assistant/agent.go:278-279`

Change the streak-3 path from a fatal error to a "skip" instruction injected
into history. The model moves to the next step instead of dying.

```go
// Before:
if rd.duplicateStreak >= 3 {
    return true, "", fmt.Errorf("infinite loop detected: ...")
}
// After:
if rd.duplicateStreak >= 3 {
    return true, prompts.AutomationToolForceSkip(key.name), nil
}
```

**Also:** reset the streak to 0 after injecting the skip so the model gets
a fresh chance if it needs to call the tool again for a different step.

#### 2. Add session-level per-tool failure tracker

**File:** `internal/core/assistant/session.go` — new fields + methods on `runSession`

```go
type failTracker struct {
    exactFailures    map[string]int  // tool_name + args_hash → count
    sameToolFailures map[string]int  // tool_name → count
}

type runSession struct {
    // ... existing fields
    failTracker *failTracker
}
```

Two maps:
- **`exactFailures`**: keyed by `toolName + "\x00" + normalizedArgs`. Captures
  "model calls `fetch_url("https://httpbin.org/headers")` 3 times with the same
  URL and it fails each time."
- **`sameToolFailures`**: keyed by `toolName` only. Captures "model tries
  `httpbin.org/headers` (503), then `example.com/foo` (also 503), then
  `example.com/bar` (also 503)." All same tool, different args.

#### 3. `record()` method with Hermes-style thresholds + guidance

**File:** `internal/core/assistant/session.go` — new method

```go
func (ft *failTracker) record(toolName, argsHash string, errResult bool) string {
    if !errResult {
        // Success: clear counters for this tool
        delete(ft.sameToolFailures, toolName)
        for k := range ft.exactFailures {
            if strings.HasPrefix(k, toolName+"\x00") {
                delete(ft.exactFailures, k)
            }
        }
        return ""  // no guidance needed
    }

    // Increment counters
    exactKey := toolName + "\x00" + argsHash
    ft.exactFailures[exactKey]++
    ft.sameToolFailures[toolName]++

    exact := ft.exactFailures[exactKey]
    same := ft.sameToolFailures[toolName]

    // Heuristic thresholds (matching Hermes defaults):
    //   exact_failure_warn ≥ 2    → "change approach or skip"
    //   same_tool_failure_skip ≥ 3 → "stop using this tool"
    switch {
    case exact >= 3 || same >= 3:
        return fmt.Sprintf("CRITICAL: %s has failed %d times. Stop using it. Skip this step and move on.", toolName, same)
    case exact >= 2:
        return fmt.Sprintf("WARNING: %s failed %d times with identical arguments. This looks like a loop — change your approach or skip this step.", toolName, exact)
    case same >= 2:
        return fmt.Sprintf("WARNING: %s has failed %d times. Check the error output and try a different approach.", toolName, same)
    }
    return ""
}
```

#### 4. Wire tracker into the session loop

**File:** `internal/core/assistant/session.go` — in `run()`

After each `processToolCalls` call, scan the appended tool results for errors
and feed them to the tracker. Inject guidance by appending to the tool result
content (matching Hermes `append_toolguard_guidance`), not as a separate
user message.

```go
// In run(), after s.agent.processToolCalls(...) returns:
if s.failTracker != nil {
    s.applyFailGuidance(turnMsg)
}

// New method:
func (s *runSession) applyFailGuidance(turnMsg proxy.Message) {
    for _, tc := range turnMsg.ToolCalls {
        result := findToolResultByID(s.history, tc.ID)
        if result == nil {
            continue
        }
        isError := isToolErrorResult(result.Content)
        argsHash := normalizeArgs(tc.Function.Arguments)
        guidance := s.failTracker.record(tc.Function.Name, argsHash, isError)
        if guidance != "" {
            result.Content += "\n\n" + guidance
        }
    }
}
```

#### 5. Error detection from result content

**File:** `internal/core/assistant/session.go` — new helpers

`isToolErrorResult` detects errors from the **content** of tool result messages,
not the Go `error` interface. Checks for:

- JSON containing `"error":` field (the format used by `tool_exec.go:166`)
- String starting with `Error`
- Terminal-specific: `exit_code` != 0

This catches errors regardless of whether `processToolCalls` saw the Go error,
matching Hermes `classify_tool_failure`.

#### 6. New prompt constant

**File:** `internal/core/assistant/prompts/templates.go`

```go
// AutomationToolForceSkip is injected when the same tool has failed repeatedly.
// Instructs the model to permanently abandon the tool and move to the next step.
const AutomationToolForceSkip = "CRITICAL: %s has failed repeatedly. Stop using it entirely. Skip the step that required it and continue."
```

#### 7. Reset on success at the session level

After the model is forced to skip a tool (skip instruction injected), the
counters for that tool should be reset so the model gets a clean slate if it
needs to use the tool for a legitimate reason later.

---

## Files changed

| File | What changed |
|------|-------------|
| `prompts/templates.go` | Added `ToolForceSkipMessage()` and `ToolFailureGuidance()` functions |
| `agent.go` | `repetitionDetector.check()`: streak 3 → skip (not fatal) + reset streak AND clear recentCalls |
| `session.go` | Added `failTracker` struct, initialize in `newRunSession()`, add `applyFailGuidance()`, add `findToolResultByID()`, `isToolErrorResult()`, `normalizeArgs()` helpers |
| `agent_test.go` | Added 8 new tests. Updated `TestAgent_Execute_LoopDetection`, `TestAgent_Execute_StreamWithXMLToolCall`, `TestAgent_Execute_StreamEmptyFallback`, `TestAgent_Execute_ReasoningStuckFallback`, `TestAgent_Execute_NativeToolsEmptyStreamFallsBackToXML`, `TestAgent_Execute_StreamWithInterleavedToolCalls`, `TestAgent_ToolCallParseErrorRetry`, `TestAgent_ToolCallParseErrorExhaustRetries` with GlobalTimeout. Updated `TestRepetitionDetector_StreakReset` and `TestRepetitionDetector_SlidingWindow` for new skip behavior. |

## Non-goals

- Not changing how tool errors are returned from `ExecuteTool`
- Not changing guardrail flow (that's separate from execution errors)
- Not changing the smoke test template
- Not adding per-model configuration
- Not removing the starvation counter or context sieve (they remain safety nets)

## Edge cases

| Scenario | Outcome |
|----------|---------|
| Tool fails 2x with same args, 3rd time with different args | `exact_failures` counter resets (different key), `same_tool_failures` triggers skip at 3 |
| Tool fails 1x, model uses different tool successfully, fails same tool 2x more | `same_tool_failures` at 3 → skip (cumulative, not consecutive) |
| Tool fails, model tries again with same args, succeeds 3rd time | Success clears both counters — no skip |
| Tool fails 3x with same args, model gets skip instruction | Skip appended, counters reset. If model ignores and tries again, `repetitionDetector` handles consecutive repeats |
| Guardrail blocks tool (produces `{"error": "Guardrail violation..."}`) | `isToolErrorResult` detects the `"error"` key — counts toward skip threshold. This is correct (tool is effectively failing) |
| Multiple tools fail (fetch_url + read_file + terminal) | Each tool tracked independently — only the failing tool gets a skip instruction |

## Verification

1. `go test ./internal/core/assistant/...` — 7.4s, all pass (was timing out at 90s — bottleneck was 7 streaming tests looping without GlobalTimeout)
2. `go test ./...` — all 26 packages pass across the entire backend
3. `go vet ./...` — no issues
4. Manual: run smoke test with `fetch_url` returning 503 — verify:
   - First retry: model gets error, sees guidance "failed 2x"
   - Repetition detector catches identical calls, injects skip prompts
   - Model gets CRITICAL guidance from failTracker after 3rd error
   - Model moves on instead of getting fatal "infinite loop detected"
   - Final submission includes note about failed network step
