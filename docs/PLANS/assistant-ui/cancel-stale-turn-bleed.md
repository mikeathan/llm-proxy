# Fix: stale turn bleed on cancel + new message

**Date**: 2026-06-26
**Subsystem**: assistant-ui
**Files**:
- `frontend/src/utils/messageBuilder.ts`
- `frontend/src/utils/turnGrouper.ts`

## What

After cancelling a turn mid-stream and sending a new message, stale reasoning from the cancelled turn could appear in the new bubble, and multi-message turns (agent flow with tool calls) rendered segments from the wrong assistant message. Two targeted fixes:

1. `messageBuilder.reset()` now clears `liveReasoning.value`. Previously the ref held the cancelled turn's last in-flight reasoning string, so the new bubble briefly rendered it before the new turn's first reasoning event overwrote it.
2. `turnGrouper.groupTurns()` for multi-message turns now takes segments from `last` (the final assistant message) instead of `first` (which is typically incomplete — e.g. the first intermediate message of a tool-call agent flow has only the start of the work).

## Why

- `liveReasoning` is a shared ref passed to every `ChatBubble`. Any leftover value from a cancelled turn shows in the new turn's bubble via the `v-if="loading && isLastTurn && thinking && liveReasoning"` slot. Reset missed it.
- Multi-message turn segments: the first assistant message in an agent tool-call flow contains only the initial reasoning/tool-call start. The final message has the complete segments (the LLM streams intermediate messages then a final summary). Using `first.segments` truncated the visual segments in the bubble.

## Decisions

- **Did NOT call `builder.finalize()` in `cancel()`**: it requires a `reply: string` arg and pushes a NEW assistant message to `messages[]`. On cancel, the partial message is already in `messages[]` at `assistantIdx` from streaming — `reset()` just orphans that index so further updates don't mutate it. The partial message stays as a frozen snapshot. Final state is correct without `finalize()`.
- **Removed unused `first` const** in `turnGrouper.ts` after the swap.
- **Backend SSE bleed deferred**: the user's original report also showed prior turn text appearing in the new turn's `Result` section (rendered from `turn.finalAnswer = last.content`). This is a backend issue — the new turn's last assistant message `content` field apparently contains accumulated text from the cancelled turn. Investigating the backend SSE handler is out of scope for this plan; will be a follow-up.

## Tradeoffs

- Multi-message turn segments now come from the LAST message, which may be the final summary message without intermediate reasoning. For typical tool-call flows this is correct (the final message is what the user wants to see). If a flow has only one assistant message, the `if (assistantMsgs.length === 1)` branch (line 39) handles it correctly.
- The cancelled turn's partial message has no `finalAnswer` content (the reply was aborted). The `Result` section won't render for it (since `v-if="turn.finalAnswer"`). The work section still renders any segments that were streamed. Acceptable.

## Verification

- `npm run build` (vue-tsc + vite) clean.
- Manual: send msg → cancel mid-stream → send new msg → confirm:
  - Cancelled bubble shows segments streamed so far, no `Result` section.
  - New bubble has clean reasoning text, no stale carry-over.
  - If the prior turn's text still appears in the new turn's `Result` section: that is the deferred backend bleed.

## Follow-up

Plan: investigate backend SSE handler for `content` field accumulation on resume after cancel. Likely location: `backend/internal/core/assistant/stream.go` and `agent.go` event emission. Until fixed, the symptom is: prior turn reasoning text may appear at the start of the new turn's `finalAnswer` HTML.
