---
status: reverted

## Extension: Per-Model Temperature and Timeout Overrides

Following the revert, a separate feature was added to address the Gemma 4
repetition: per-model `temperature` and `timeout_minutes` overrides.

These are exposed in the UI model panel's Agent Tuning section with tooltips
(input `title` attribute) and input validation (min/max/step).

See `docs/audits/ephemeral-turn-context-failed-run.md` for the full file list.
related_specs: [SPEC-001]
---

# Ephemeral Turn Context — Reduce Recap Overhead

## Problem

The model spends ~30% of its reasoning tokens per turn re-listing all task steps and
checking off completed ones. Over 20 turns this is ~5,000 chars of wasted reasoning
= ~60s of dead time per run.

**Root cause:** reasoning content is stripped from conversation history by
`NormalizeHistory`, so the model has no "where am I?" anchor at the start of each
turn. It re-derives its full plan from scratch — re-listing each step and noting
which are done.

**Not the root cause:** sieve retention, prompt verbosity, or the task structure.

## Design

Inject a compact, ephemeral `[Turn N]` context message immediately before each LLM
call. The message:

- Costs ~25-30 tokens to produce and ~25-30 tokens to read
- Replaces ~500 tokens of self-derived recap per turn
- Is updated before every turn with the current tool counts
- Is **never persisted** to conversation history — appended only to the local slice
  passed to `computeNextResponse`, then freed

Example injection (turn 7 of a smoke test):

```
[Turn 7 | Tools used: list_directory, write_file (×2), read_file, execute_terminal_command (×3)]
```

## Files Changed

### `templates.go`

Add `BuildTurnContext` function and `TurnContextPrefix` constant.

```go
const TurnContextPrefix = "[Turn "

func BuildTurnContext(turn int, counts map[string]int) string {
	if len(counts) == 0 {
		return fmt.Sprintf("[Turn %d | No tools used yet]", turn)
	}
	var parts []string
	for name, n := range counts {
		if n == 1 {
			parts = append(parts, name)
		} else {
			parts = append(parts, fmt.Sprintf("%s (×%d)", name, n))
		}
	}
	sort.Strings(parts)
	return fmt.Sprintf("[Turn %d | Tools used: %s]",
		turn, strings.Join(parts, ", "))
}
```

Import: add `"sort"`. Do **not** add `"llm-proxy/internal/core/proxy"` — `proxy` already
imports `prompts`, so that would create a circular dependency. The counting logic
lives in `session.go` where `proxy.Message` is already available.

Add rule 8 to `DefaultRules`:

```
8. Start each turn directly with your next tool call. Do not recap progress — the
   [Turn N] message injected before each LLM call shows your session state.
```

### `session.go` — `executeTurn` signature

Add `turn int` parameter. Rename the context parameter to `turnCtx` to avoid
shadowing the string variable we'll introduce:

```go
func (a *Agent) executeTurn(turnCtx context.Context, history *[]proxy.Message, turn int) (...)
```

Add a helper to count tool usage from history (counting logic lives here
instead of `templates.go` to avoid the import cycle):

```go
func countTools(history []proxy.Message) map[string]int {
	counts := map[string]int{}
	for _, m := range history {
		for _, tc := range m.ToolCalls {
			counts[tc.Function.Name]++
		}
	}
	return counts
}
```

Inside `executeTurn`, after the physical sieve but before `computeNextResponse`,
inject the turn context into a local copy of history:

```go
callHistory := *history
if a.findAutomationCtx(callHistory) {
	turnContext := prompts.BuildTurnContext(turn, countTools(callHistory))
	callHistory = append(callHistory, proxy.Message{
		Role:    proxy.SystemRole,
		Content: turnContext,
	})
}
msg, err := a.computeNextResponse(turnCtx, callHistory, toolsList)
```

No changes to `computeNextResponse` or any other function. The context is
ephemeral — it only exists in the local `callHistory` slice and is freed after
`computeNextResponse` returns. The real `*history` is unmodified.

**Fallback chain note:** `computeNextResponseStreamXML` and
`computeNextResponseNonStreaming` receive `callHistory` (which includes the
turn context message) via `computeNextResponse`'s `history` parameter. This is
harmless — fallbacks are retries of the same turn, so the model sees identical
turn number and tool counts.

### `session.go` — call site in `run()`

Pass `s.steps` as the turn number:

```go
turnMsg, parseErr, toolsList, err := s.agent.executeTurn(s.ctx, &s.history, s.steps)
```

## What does NOT change

- `computeNextResponse` signature (no changes needed)
- All other callers of `executeTurn` (there's only one — the `run()` loop)
- History persistence (never written to DB or conversation)
- `AutomationTaskPrompt` (not modified)
- `prepareMessagesForTurn` (not modified)
- Sieve constants (not modified)
- Reasoning budget (stays at 910)
- `tool_choice` (stays `required`)
- Non-automation (interactive) sessions are unaffected

## Verification Plan

1. `go build ./...` — must compile clean
2. `go test ./...` — all tests pass
3. Run smoke test against `gemma-4-4b-it-Q4_K_M.gguf`
4. In `run.log`, verify:
   - Each assistant turn starts with the tool call directly — no step-recap block
   - Reasoning character count per turn decreases (especially mid-run turns 5-15)
   - Total reasoning chars drops by ~30%
5. Expected outcome: total duration decreases from ~370s to ~310s
6. Confirm `submit_final_answer` is still called correctly (no regression)

## Expected Benefit

| Metric | Before | After | Savings |
|--------|--------|-------|---------|
| Reasoning chars/turn (avg) | ~820 | ~570 | ~250 |
| Total reasoning chars | ~17,200 | ~12,000 | ~5,150 |
| Total duration | ~370s | ~310s | ~60s |
| LLM calls | 20-21 | 18-20 | ~1-2 |

## Edge Cases

- **Zero tool calls**: First turn — no tool calls yet. Context says "No tools used yet."
- **Tool names with special characters**: All tool names are plain ASCII — no escaping.
- **Non-automation context**: `findAutomationCtx` check keeps interactive sessions
  unaffected.
- **Sieve interaction**: Context is appended after the sieve, so it's never pruned
  mid-turn. Only lives for one LLM call.
- **SystemRole not UserRole**: The injected message uses `proxy.SystemRole` (not `UserRole`) to avoid the model interpreting it as a user instruction. UserRole + imperative "Proceed directly." caused the model to spend MORE reasoning tokens attempting to interpret the injected message. See `docs/audits/ephemeral-turn-context-failed-run.md` for the full investigation.

## Audit: Why the First Attempt Failed

The initial implementation injected the turn context as a `UserRole` message with trailing `| Proceed directly.` instruction. A smoke test run regressed from ~370s to ~555s (9.25 min) — the slowest ever.

**Root cause:** Two compounded issues:

1. **UserRole + imperative language.** The model treats `UserRole` messages as actual human input. `| Proceed directly.` looks like a command, so the model spends extra processing cycles trying to interpret what the user wants. It also breaks the natural `ToolResult → Assistant` alternation that smaller models rely on for coherence.

2. **Recap is invisible — rule 8 can't suppress it.** The model's recap happens in `reasoning_content` (hidden from the visible output). Rule 8 only suppresses recap in the visible `Content`. The model was doing **both**: recapping in reasoning (~500 chars/turn) **plus** reading the injected message (~30 chars/turn) = **+30 tokens of overhead** instead of -470.

**Fix applied:**
- Changed `UserRole` → `SystemRole` so the model reads the turn context as a system annotation, not user input.
- Removed `| Proceed directly.` — the message now just states facts: `[Turn 22 | Tools used: ...]`.
- The imperative instruction was redundant with rule 8 in the system prompt anyway.
