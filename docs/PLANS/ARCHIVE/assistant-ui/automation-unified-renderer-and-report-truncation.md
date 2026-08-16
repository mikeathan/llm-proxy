# Unify Automation UI on Assistant Renderer + Fix Report Truncation

**Status:** `complete`
**Created:** 2026-07-18
**Completed:** 2026-07-18
**Subsystems:** assistant-ui (SPEC-003), agent-loop (SPEC-001), automation (SPEC-007)
**Branches:** `task/universal_agent_completion`

---

## 0. Problem Statement

Two related defects surfaced during a `network-scan` automation run (model
`Qwen3.5-9B-UD-Q4_K_XL.gguf`, native-tools mode):

1. **Frontend "3× output" illusion.** The automation live terminal
   (`useLiveConsole.ts`) renders `reasoning` and `tool_stream` events by
   **overwriting** the last assistant `message.payload.content`. The model's
   reasoning repeated the phrase "The task has been completed. I have:" across
   257 `reasoning` events; the console showed the latest reasoning chunk as if
   it were the answer, so the user perceived the report produced 3×. In reality
   the report was emitted **once** (to `write_file`, 5237 chars) + **once**
   inline (2732 chars, truncated). Only 1 `message` event carried the real
   report.

2. **Backend wasted generation + truncated report.** After the successful
   `write_file`, the next turn was empty → one-shot nag
   (`AutomationNagPrompt`) → the model regenerated the entire report as inline
   text → the **no-tool content cap** at `stream.go:631` cut it at
   `maxTokens` (2732 chars) mid-sentence. This both wasted tokens (full
   regeneration) and delivered a partial report.

The `max_tokens` value (2730) is **auto-derived** (`ctxLen / 3` = 8192 / 3),
so it is already context-aligned — the defect is the cap *cutting a legitimate
final answer*, not the value itself.

---

## 1. Root-Cause Analysis (verified)

### 1.1 Backend no-tool content cap

`stream.go:631-636`:
```go
if a.config.UseNativeTools && len(fullMsg.ToolCalls) == 0 && len(fullMsg.Content) > a.config.MaxTokens {
    a.deps.Logger.Warn("content exceeded max_tokens chars with no tool calls, terminating stream", ...)
    return nil
}
```

- **Why it exists:** chosen deliberately. Audit
  `docs/audits/2026-07-06-assistant-debug-cycle.md:75,84` rejected fixed
  thresholds (500/1000/2000/2730) as "too fragile". The cap stops the
  **runaway joke-loop** (local Qwen3.5-9B generating 7000+ chars of plain text,
  no EOS, no tool calls — `docs/audits/memory-injection-investigation.md:319`).
- **Scope:** only fires in **native-tools mode** (`a.config.UseNativeTools`).
- **Interaction:** it cuts the stream *before* the agent loop's
  `checkTaskCompletion` (`session.go:270`) — the intended "natural completion"
  gate (no tool calls + ≥20 chars + any prior `ToolRole` in history →
  `completeWith`). So a genuine long final report gets amputated.

**Runaway backstops that remain even if we relax this cap:**
- Token-budget `ShouldTerminate` (`stream.go:564-581`) — fires first on most
  providers that enforce `max_tokens`.
- `*4` char cap `exceedsContentCharCap` (`stream.go:623-628`, 10920 chars) —
  documented fallback for when token counting underestimates output.
- These two already bound *all* runaway output; the no-tool cap is a tighter
  second gate specifically for tool-free text.

### 1.2 Frontend terminal renderer

`useLiveConsole.ts:49-92`:
```ts
// tool_stream AND reasoning both write into the SAME last assistant message:
if (lastEvent.type === "message" && lastEvent.payload.role === "assistant") {
  (lastEvent.payload as AgentMessagePayload).content = content; // overwrites!
}
```
No segmentation, no turn grouping, no reasoning block. The assistant renderer
(`ChatMessages.vue` + `useMessageBuilder` + `turnGrouper`) already solves this
correctly: reasoning becomes a collapsible `segments[kind:'reasoning']` inset,
tool calls become status-tracked `ToolCallSegment`s. There is **no unified
`AgentEvent → AssistantMessage` mapper** (the `toolCallEventToMessage` /
`toolResultEventToMessage` helpers in `utils/dispatcher.ts` are **dead/unused**).

---

## 2. Design Decisions

### 2.1 Backend — relax, do not remove

Relax the no-tool cap so a *legitimate* final answer survives, while the
runaway guard stays intact:

- When `len(fullMsg.ToolCalls) == 0 && len(fullMsg.Content) > maxTokens`:
  - **Do NOT terminate** if `(prior `ToolRole` exists in `s.history`) OR
    `(len(toolsList) == 0)`** — i.e. real work was done, or no tools are
    available so a tool-free answer is expected. Let the stream run to its
    natural stop; `checkTaskCompletion` completes the full answer intact.
  - **Otherwise terminate** as today (no tool result yet, tools available,
    no-tool runaway).
- Keep `max_tokens = ctxLen / 3` auto-derivation (`budget_squeezer.go:135`).
  **Do NOT hardcode, do NOT blindly raise.** The `*4` char cap and token-budget
  `ShouldTerminate` remain as backstops.

**Risk:** the no-tool runaway window grows from `maxTokens` → `maxTokens*4`.
Mitigated by (a) `ShouldTerminate` token budget firing first, (b) the `*4`
backstop, (c) the relaxation is scoped to turns with prior work / no tools.

### 2.2 Frontend — single renderer, delete terminal

Make `ChatMessages.vue` the one renderer for both assistant chat and automation
runs. Delete the terminal stack.

- **Delete** `TerminalOutput.vue`, `LiveConsole.vue`.
- **New** `utils/message/automationToMessages.ts`: `AgentEvent[] →
  AssistantMessage[]` reusing `buildSegmentsFromHistory` (`turnGrouper.ts:77`)
  so reasoning/tool calls become proper `segments` (fixes the overwrite bug at
  the source). Inject one **synthetic user message** ("Automation run:
  <name>") so `groupTurns` forms a single clean turn (automation has no chat
  prompt). Map `lifecycle`/`step_start` → a status `reasoning`/`system` segment
  or `phase` so step visibility is preserved.
- **Modify** `ChatMessages.vue`: add optional `mode?: 'chat' | 'automation'`.
  In `automation` mode skip the `UserMessage` bubble (or render the synthetic
  one), hide retry/welcome, keep the collapsible reasoning inset and
  `GuardrailBanner`.
- **Modify** `AutomationDetails.vue:200-205`: replace `<LiveConsole>` with the
  generic renderer fed by `useLiveConsole` (still SSE `?channel=automation`) →
  bridge → `ChatMessages mode="automation"`.
- **NOTE (superseded):** the bespoke `automationToMessages.ts` bridge described
  below was replaced by routing automation through the shared `useMessageBuilder`
  consumer (see `automation-renderer-unify-consumption.md`). The bridge caused a
  cumulative-re-emit cascade; deleting it and sharing one consumer fixed it.
- **Modify** `useLiveConsole.ts`: drop the `reasoning`/`tool_stream` overwrite
  logic (49-92); feed all events through the shared `useMessageBuilder`
  (`finalizeOn: 'lifecycle'`), seeded with a synthetic run header message.
- **Modify** `utils/dispatcher.ts`: remove dead `toolCallEventToMessage` /
  `toolResultEventToMessage`; relocate `formatEventsToText` (copy-all) into the
  automation shell or keep.
- **Modify** `composables/automation/index.ts`: drop the `useLiveConsole`
  re-export only if the file is removed; otherwise keep.
- **Reuse unchanged:** `ChatBubble.vue`, `ToolCallSegment.vue`,
  `MarkdownViewer.vue`, `GuardrailBanner.vue`, `UserMessage.vue`,
  `useAutoScroll`, `groupTurns`, `turnGrouper` control-message strip (already
  added during the channel-split work).

---

## 3. Implementation Steps

### Backend
1. `internal/core/assistant/stream.go` (631-636): add the relaxation guard
   described in §2.1. The guard needs `s.history` (prior `ToolRole`) and
   `toolsList` — both are available in `processStream`'s caller
   (`executeTurn`/`runSession`). Thread them through or re-check at the
   `handleTextTurn` boundary (the cleaner spot is to NOT cut at the streaming
   layer when the turn is a plausible final answer; let `checkTaskCompletion`
   decide). Prefer: when `UseNativeTools && no tools && content > maxTokens`,
   return the turn **without** terminating **iff** the turn is a valid
   natural-completion candidate (prior tool result OR no tools available);
   otherwise terminate.
2. `internal/core/assistant/agent_test.go`: add regression tests
   (`TestNoToolCap_*`).
3. `go build ./... && go test ./... && go run ./tools/check-complexity/`.

### Frontend
4. Delete `TerminalOutput.vue`, `LiveConsole.vue`.
5. New `utils/message/automationToMessages.ts` bridge.
6. `ChatMessages.vue`: add `mode` prop + automation gating.
7. `AutomationDetails.vue`: swap to generic renderer.
8. `useLiveConsole.ts`: remove overwrite logic.
9. `utils/dispatcher.ts`: remove dead mappers; relocate `formatEventsToText`.
10. `npm run build` (vue-tsc + vite).

---

## 4. Verification

- Backend: `go build ./... && go test ./... && go run ./tools/check-complexity/`
- Frontend: `cd frontend && npm run build`
- Manual: re-run `network-scan` → single clean report delivered; no repeated
  "The task has been completed" phrase in the console; automation view uses
  assistant-style bubbles with collapsible reasoning; no truncated inline
  regeneration; runaway no-tool loop (if it ever occurs) still terminates via
  the `*4` cap / token budget.

---

## 5. Files Touched

**Backend**
- `internal/core/assistant/stream.go` — relax no-tool cap (§2.1)
- `internal/core/assistant/session.go` — `checkTaskCompletion` (270) unchanged,
  is the intended completion gate
- `internal/core/assistant/agent_test.go` — new regression tests

**Frontend**
- `src/components/AgentIde/automation/TerminalOutput.vue` — **DELETE**
- `src/components/AgentIde/automation/LiveConsole.vue` — **DELETE**
- `src/components/AgentIde/automation/AutomationDetails.vue` — swap (200-205)
- `src/components/AgentIde/assistant/ChatMessages.vue` — `mode` prop
- `src/composables/automation/useLiveConsole.ts` — drop overwrite (49-92)
- `src/composables/automation/index.ts` — drop re-export if file gone
- `src/utils/message/automationToMessages.ts` — **NEW** bridge
- `src/utils/dispatcher.ts` — remove dead mappers; relocate `formatEventsToText`
- `src/utils/message/turnGrouper.ts` — `buildSegmentsFromHistory` reused

---

## 6. Open Items / Follow-ups

- Consider a raw/compact "debug" toggle later if operators miss the monospace
  terminal scrollback — out of scope for this plan (user wants the terminal
  removed).
- `docs/architecture.md` Pitfall #6 + `docs/PLANS/ARCHIVE/cross-cutting/universal-agent-completion.md`
  already updated for the channel split; this plan's completion should be noted
  in `docs/skills/assistant-ui-patterns.md` (renderer unification).
