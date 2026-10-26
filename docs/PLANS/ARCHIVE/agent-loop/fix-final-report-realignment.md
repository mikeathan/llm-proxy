---
status: complete
date: 2026-08-07
related_specs: [SPEC-001]
---

# Plan: Fix automation "Final Report" regression (full clean realignment)

## 0. TL;DR
The broken final report is caused by an **uncommitted staged edit** in
`backend/internal/core/assistant/session.go` that renamed `forcedCompletionSent`
→ `nagSent` and made `resetParseErrorState` keep `nagSent` set, turning the
post-tool empty-response nudge into a **one-shot-per-run** event. A model that
emits an empty response after running tools therefore never gets re-nudged, and
the run limps to `checkForcedCompletion` (MaxSteps×2), which (after the
now-superseded A/B edits) falls back to dumping file contents / synthesizing a
summary instead of a real report.

Fix = delete the layered compensation heuristics (salvage-as-report,
synthesize-summary, preamble scooping) and replace them with a **bounded,
re-arming, Hermes-aligned recovery ladder**, plus a **deterministic
tools-disabled finalization turn**. Works for any LLM, not just Qwen.

---

### Sequencing constraint (added via consolidation with `gpu-performance.md`)

The GPU plan's P5 note deferred this exact stuck/nudge-loop bug ("empty finalization
turn") as a measurement confound. **This document is the canonical fix.** It must land
**before** any GPU P1 re-measure (`gpu-performance.md` §P1), because an unfixed nudge-loop
inflates/skews during-run GPU measurement windows. The GPU plan owns only pure
rendering/metrics items (P0–P4); the agent-loop recovery ladder lives here. Do not
duplicate the investigation in the GPU plan.

---

## 1. Root cause (verified)
- Pre-branch HEAD `f89b2cf` cleared `forcedCompletionSent=false` inside
  `resetParseErrorState` on **every** tool turn → nudge re-armed each tool turn →
  worked for dev + network.
- Staged edit (current `git diff --cached`):
  - `session.go`: `forcedCompletionSent` → `nagSent`; `resetParseErrorState`
    comment "nagSent is intentionally NOT cleared here" → **one-shot**.
  - Split into `nagSent` + `hardCapTriggered` (the split/independence is *good*,
    keep it).
  - Added `repetitionDetector` (`checkAlternating`/`checkSequenceRepeat`/
    `checkSameTarget`) in `handleToolTurn` — *good*, keep.
- My working-tree edits (Change A `salvageTruncatedReportFromHistory` gating;
  Change B `synthesizeToolActivitySummary` + reordered `resolveFallbackAnswer`)
  are **compensating heuristics** to be removed under this plan.
- Tool plumbing already exists and is correct: `models.ToolChoiceNone/Auto/
  Required`; `errors.go:73` degrades "tool_choice is not supported" gracefully;
  `buildChatRequest` omits `ToolChoice` today (defaults to auto).
  `stream.go:handleEmptyStream` returns the `[stuck]` placeholder for
  native-tools models — already correct.

---

## 2. Goals / non-goals
**Goals**
- Any LLM returns a correct final report after real tool work (dev: chat message;
  network: written file via execution-time salvage).
- Empty responses after tool execution are nudged, re-armed, and finally forced
  into a text-only turn.
- Remove all "salvage the file as the answer" / "synthesize summary" /
  "scoop preamble" code paths.
- Recover messages stay synthetic/sentinel (never poison history with real
  user/task text).
- Bounded: no infinite loops; `hardCapTriggered` at MaxSteps×2 remains the
  ultimate backstop.

**Non-goals**
- Don't special-case Qwen. Don't change `checkForcedCompletion`'s hard cap role.
- Don't touch `stream.go:handleEmptyStream` or `processToolCalls`
  execution-time `salvageTruncatedWrite` (those are correct).

---

## 3. Design — the empty-turn recovery ladder (in `handleNoToolCalls`)

Replace the current one-shot `nagSent` branch with an ordered ladder.
Pseudocode:

```
handleNoToolCalls(turnMsg, parseErr, toolsList):
  append turnMsg to history + notify                  # keep
  if isPrematureTermination(...): return content,true  # keep
  if parseErr != nil: handleParseErrorFeedback(...)    # keep
  if len(toolsList)==0 && content!="": return content,true  # keep (cloud no-tools)

  # Housekeeping shortcut (keep): model wrote answer alongside prior housekeeping tools
  if s.lastContentWithTools != "": return it,true

  recentToolResult := s.lastMeaningfulMessageIsToolResult()  # new helper

  # (1) Re-armed post-tool nudge
  if recentToolResult && s.postToolNudgeCount < postToolNudgeMax(=2):
      s.postToolNudgeCount++
      append synthetic assistant "(empty response)"  # sentinel, not surfaced
      append user prompts.AutomationNagPrompt
      return "",false   # continue loop, same tools

  # (2) Deterministic finalization: force ONE text-only turn
  if s.finalizeAttempts < 1:
      s.finalizeAttempts++
      append user prompts.AutomationFinalizePrompt   # new: "Deliver final report now as plain text; do not call tools."
      s.textOnlyNextTurn = true
      return "",false   # loop re-enters executeTurn with tools disabled

  # (3) Terminal: best real answer or honest note
  return s.bestAvailableAnswer(), true
```

**Key mechanics**
- `postToolNudgeCount int` replaces `nagSent`. Cleared in `resetParseErrorState`
  (re-arm on every successful tool turn) — restores pre-branch behavior.
- `finalizeAttempts int` ensures step (2) fires at most once → no infinite
  finalization loop.
- `textOnlyNextTurn bool` (new) tells `executeTurn` to run **one** turn with
  `Tools=nil` and `ToolChoice=models.ToolChoiceNone`, then resets itself. This is
  the model-agnostic "make it talk" lever.
- The text-only turn, when it returns text, flows through `run()` →
  `handleTextTurn` → `checkTaskCompletion` (text present + any `ToolRole` in
  history) → `completeWith(content)` → real report. ✅
- If the text-only turn *still* returns empty or a tool-call marker,
  `finalizeAttempts` is already exhausted → step (3) terminal →
  `bestAvailableAnswer()`.

**`lastMeaningfulMessageIsToolResult()`** (new helper): walk `s.history` backward
past `isAgentControlMessage` entries; if the first meaningful message is
`proxy.ToolRole`, return true. (Reuses `isAgentControlMessage` filter so injected
nags don't count.)

**`bestAvailableAnswer()`** (kept, simplified): return the last `assistant`
message that (a) has substantive content after `stripThinkBlocks`, (b) has
**no** `ToolCalls`, (c) is not a stuck placeholder. Drops all preamble-scooping /
synthesize logic. This yields the genuine report if the model ever emitted it as
content, else empty → caller returns a short honest note.

---

## 4. Tool-choice plumbing for the text-only turn
- `executeTurn` (session.go:702) is called only from `run()`. Add: if
  `s.textOnlyNextTurn`, set `toolsList = nil` and pass a `toolChoice` override
  into `computeNextResponse`.
- `computeNextResponse` (stream.go:281) already short-circuits GBNF when
  `len(llmTools)==0`, and routes stream errors to
  `computeNextResponseNonStreaming` (which also takes `tools`). Extend its
  signature with `toolChoice models.ToolChoice`; default `""` (auto) for normal
  turns; `models.ToolChoiceNone` for the finalization turn.
- After building the request in `computeNextResponse`, when `toolChoice != ""`
  set `req.ToolChoice = toolChoice`. `errors.go:73` already degrades unsupported
  `tool_choice` → safe on all providers.

No new request type needed; `proxy.ChatRequest` already has `ToolChoice` +
`Tools`.

---

## 5. File-by-file changes

### `backend/internal/core/assistant/session.go`
1. **Struct** (`runSession`, ~line 47): remove `nagSent bool`; add
   `postToolNudgeCount int`, `finalizeAttempts int`, `textOnlyNextTurn bool`.
   Keep `hardCapTriggered bool` + `rd repetitionDetector`.
2. **`resetParseErrorState`**: clear `postToolNudgeCount = 0` (re-arm). Leave
   `hardCapTriggered` untouched. (Verify the staged edit's exact text before
   editing.)
3. **`handleNoToolCalls`**: replace the `if s.nagSent {...} s.nagSent=true` block
   with the ladder from §3. Remove the call to `resolveFallbackAnswer()` from
   here.
4. **`handleToolTurn`**: unchanged except it already calls
   `resetParseErrorState` (re-arm happens automatically). Keep
   `lastContentWithTools` housekeeping shortcut.
5. **Remove** `synthesizeToolActivitySummary` (Change B) entirely.
6. **`resolveFallbackAnswer`**: simplify to `return s.bestAvailableAnswer()`.
   Remove preamble/synthesis branches. (Still used by `checkForcedCompletion` +
   `handleTurnError` for the hard-cap backstop.)
7. **`bestAvailableAnswer`**: simplify to "last substantive non-tool,
   non-placeholder assistant text" (remove scooping logic).
8. **`executeTurn`**: honor `s.textOnlyNextTurn` (nil tools + pass
   `ToolChoiceNone`); reset flag after building request.
9. Add helper `lastMeaningfulMessageIsToolResult()`.
10. Add const `postToolNudgeMax = 2`.

### `backend/internal/core/assistant/stream.go`
- `computeNextResponse(ctx, history, tools, toolChoice models.ToolChoice)` (+
  propagate same param to `computeNextResponseNonStreaming`). Set `req.ToolChoice`
  when non-empty.

### `backend/internal/core/assistant/tool_exec.go`
- **Remove** `salvageTruncatedReportFromHistory` and its completion-path call
  (Change A). Keep `salvageTruncatedWrite` / `trySalvageWriteContent` exactly as-
  is — they run at **execution time** inside `processToolCalls` and correctly
  recover the network task's truncated `write_file` → `final-report.md`.
  (Verified: network report salvage is independent of this regression.)

### `backend/internal/core/assistant/prompts/templates.go`
- Add `AutomationFinalizePrompt`:
  `"SYSTEM: You have completed all tool work for this task. Produce your FINAL REPORT now as a plain-text assistant message. Do NOT call any tools. Summarize the actual results of the work you performed."`
- Register it in `isAgentControlMessage` allowlist so it's treated as synthetic,
  not real task text.

---

## 6. Test plan (`backend/internal/core/assistant/*_test.go`)
Add/extend table tests; all must pass `go test ./internal/core/assistant/...`:

1. **Dev happy path (reproduces bug fix):** simulate empty response immediately
   after a `write_file`/`exec` tool result → assert `postToolNudgeCount`
   increments, loop re-enters, and on the text-only turn the model's report text
   becomes the returned `reply` (not file contents).
2. **Re-arm:** empty → nudge → (mock) tool turn → empty again → **second** nudge
   fires (proves `resetParseErrorState` clears the counter). Pre-fix this would
   NOT re-nudge.
3. **Bound:** repeated empties after tools → exactly `postToolNudgeMax` nudges,
   then one finalization turn, then terminal `bestAvailableAnswer`/honest note;
   no infinite loop (assert step count bounded).
4. **Finalization tools-disabled:** assert the finalization turn's outgoing
   `ChatRequest.Tools == nil` and `ToolChoice == models.ToolChoiceNone` (capture
   via a fake Provider/Client).
5. **Network salvage still works:** truncated `write_file` → `salvageTruncatedWrite`
   recovers content at execution time → `final-report.md` written; completion
   returns salvaged content. (Regression guard that the removed
   `salvageTruncatedReportFromHistory` never masked this.)
6. **Model ignores finalization & emits tool marker:** text-only turn returns XML
   tool call → `finalizeAttempts` exhausted → terminal `bestAvailableAnswer`.
7. **Housekeeping shortcut:** model emits text alongside only housekeeping tools
   → `lastContentWithTools` path returns it.

---

## 7. Verification (commands, from `backend/`)
```
cd backend
git diff --cached internal/core/assistant/session.go   # confirm staged regression text
go build ./...                                          # must pass after each edit
go test ./internal/core/assistant/...                   # all green
go vet ./internal/core/...
go run ./tools/check-complexity/                         # ≤12
```
Then a live smoke test (optional): run the `ts-logic-interface-test.md` dev
automation on the local model and confirm the **chat message** is the real
Source/Compilation/Output report (not `app.ts` dump, not preamble).

---

## 8. Risks / notes
- `executeTurn` is called only from `run()` (verified via grep) → adding the
  `textOnlyNextTurn` read is safe; no other caller affected.
- `computeNextResponseNonStreaming` path must also receive `toolChoice` so
  non-streaming providers get `tool_choice=none` (some providers error on
  tools+content-only; the `errors.go:73` degrade covers the rest).
- Keep `hardCapTriggered` backstop; the ladder should normally resolve before
  MaxSteps×2.
- No new dependencies; no config/CI changes (within AGENTS.md boundaries).

---

## 9. Implementation status (marked complete 2026-08-19)

Implemented in `backend/internal/core/assistant/` (session.go ladder,
recovery_ladder_test.go, agent_test.go bounded-ladder tests; GPU P1 re-measure is
unblocked). Two deliberate deviations from the §3 pseudocode, both intentional
and covered by tests:

1. **§3 step (1) nudge gate dropped.** The design gated the re-armed nudge on
   `lastMeaningfulMessageIsToolResult()` (nudge only when the last meaningful
   message is a tool result). The implementation nudges on **any** empty turn
   (`handleNoToolCalls`, `if s.postToolNudgeCount < postToolNudgeMax`), still
   bounded by `postToolNudgeMax = 2` and re-armed in `resetParseErrorState` on
   every successful tool turn. `recovery_ladder_test.go` asserts the
   unconditional march (nudge → nudge → finalize). The helper was never built.
2. **§3 step (2) delegated to the shared `finalizeReport` primitive** (see
   `strategy-agnostic-completion-and-tool-schema.md`): instead of appending the
   finalize prompt and re-entering the loop, `handleNoToolCalls` calls
   `finalizeReport(s.ctx)` inline (which runs the single tools-disabled
   `textOnlyNextTurn` itself and returns the report), then the caller seals via
   `completeWith` — so the "completed" lifecycle fires exactly once and
   plan-execute's completion path cannot drift from react's.
