---
name: assistant-ui-chat
description: "Assistant chat UI architecture: event handlers, the segment system, inactivity timer, SSE timing, scroll behavior, and common pitfalls. Use when changing the chat view or its SSE handling."
metadata:
  last_reviewed: "2026-10-26"
---

# Assistant UI Chat Architecture

**Source files:** `frontend/src/components/AgentIde/assistant/AssistantChat.vue`, `frontend/src/utils/message/messageBuilder.ts`, `frontend/src/utils/message/turnGrouper.ts`

**Related docs:** `.agents/skills/event-streaming-patterns/SKILL.md` (event flow, SSE composables, dedup)

---

## Reference files

Open only the one your task needs:

| File | Open it when |
|------|--------------|
| [`references/message-builder.md`](references/message-builder.md) | You are changing how SSE events become chat messages (`messageBuilder.ts`). |
| [`references/sse-timing-and-inactivity.md`](references/sse-timing-and-inactivity.md) | A run looks stuck, events arrive out of order, or the inactivity timer misfires. |
| [`references/segments-and-turns.md`](references/segments-and-turns.md) | You are changing how a turn is split into segments, grouped, or finished. |
| [`references/animation-and-scroll.md`](references/animation-and-scroll.md) | You are adding animation or changing scroll behavior (check GPU cost first). |
| [`references/webhook-sessions.md`](references/webhook-sessions.md) | You are changing how connector/webhook sessions show in the chat UI. |

## Layout (document + step timeline, 2026-10-01)

Each turn renders inside one reading column (`.turn-group`, max 52rem, centred by `ChatMessages.vue`):

1. **User message** — right-aligned pill (`UserMessage.vue`) with hover actions (send again, copy).
2. **Assistant turn** (`ChatBubble.vue`) — no box; reads like a document:
   - an **activity line** button (`aria-expanded`): `Thinking` / `Working · N steps` / `Writing the answer`
     while live, `Worked 42s · 3 steps` when finished. Wording: `activityLabel()` in
     `constants/labels.ts`. The duration shows **only when measured**: live by the turn's timer, after a
     reload from its stored run record (`turn.run.duration_ms`); sessions from before 2026-10-01
     have none and show no duration — never an estimate. A turn that answered without any
     work has no activity line.
   - the **step timeline** (`<ol class="timeline">`): reasoning (`Thought`, compact markdown) and tool
     calls (`ToolCallSegment.vue`) in the order they happened, plus guardrail / error / upstream
     notice items. Each tool step reads as a plain verb + target + outcome in words
     (`utils/assistant/toolSteps.ts`: `describeToolStep`, icon per `ToolKind`); opened, it shows the
     arguments and the result decoded for reading (`formatToolResult`: JSON-encoded strings → real
     lines, `[{title,url}]` → search-hit links limited to http(s), other JSON pretty-printed,
     >4000 chars → "Show all"). Results are untrusted: text only, never `v-html`.
   - the **answer** — `MarkdownViewer variant="document"` (14px, 1.65 line height).
   - a **footer** — copy answer (finished turns only) and the turn's stored run record: model and
     tokens generated (prompt tokens processed in the tooltip, `tokenUsageLabel()`). The record is
     `Message.run` on the turn's user message (`groupTurns` copies it to `turn.run`); the activity
     line uses its `duration_ms` after a reload. Old sessions have none — show only what was
     recorded.

```
                              ┌────────────────────────────┐
                              │ search latest on haiku 5.5 │
                              └────────────────────────────┘
 › • Worked 42s · 3 steps                         ← activity line (toggles timeline)
   ◌ Thought  The user wants the latest news…
   ⌕ Searched the web  Example Model 2 news   ✓ ›
   ◍ Read a web page   cellcog.ai/blog/…       ✓ ›
 Here's a summary of the latest on **Example Model 2**…   ← answer (document)
 ⧉                                                ← footer
```

### Key design rules
- The timeline auto-expands while the session is active (`loading = true`) and auto-collapses when finished (`loading → false`); the user can toggle it via the activity line.
- Tool steps are collapsed by default (click to see arguments + result).
- The answer is always visible, never collapsed by the timeline toggle.
- Only the **live** turn (`loading && isLastTurn`) uses the run's `phase`; every earlier turn renders as finished, so a new run's `thinking` phase never hides older answers.
- The timeline is height-capped (320px, internal scroll) **only while live**; a finished turn opens in full.

## Common Pitfalls

- **Tool_call events must arrive BEFORE the message event** — backend must send `processToolCalls` before `notify(EventMessage)`.
- **`ensureAssistant()` must be called in every handler that touches segments** — `handleToolCall()`, `handleToolResult()`. Missing it causes orphaned segments.
- **Handler first, then refs** — in `handleEvent`, call the handler BEFORE toggling `streaming`/`thinking` to prevent work section flickering.
- **Do NOT reset pause timer on non-stream events** — `tool_call`/`tool_result`/`message` should NOT call `resetPauseTimer()`. Only `tool_stream` resets the timer.
- **Do NOT call `forceUpdate()` during pure text streaming** — `scheduleFlush()` (100ms) updates only `liveReasoning.value`; committed turns stay frozen (memoized-list pattern). `forceUpdate()` is reserved for segment mutations.
- **Throttle auto-scroll (~250ms) and never deep-watch `turns`** — per-flush pane scrolling + `deep: true` history traversal are the GPU/CPU hot path during streaming.
- **Use `visibility: hidden` for the thinking-gap on the live turn**, not `v-show`: its space is reserved so the answer does not jump while the run streams. Finished turns do not render the row at all (`v-if="live"`), so history has no empty gap under each answer.
- **Start the pause timer at agent startup** — call `builder.resetPauseTimer()` after `sse.connect()`.
- **SSE must be connected before HTTP POST** — wait for "ping" event before sending the agent request.
- **Do NOT use `watch(messages, scrollToBottom)`** — it causes flickering. Only scroll on new segments.
- **The final answer is the content-only message** — completion is signaled by the model producing text with no further tool calls. The report content is the final assistant message, not a separate tool call.
- **`finalAnswer` must be set for single-message turns** — `groupTurns()` in `turnGrouper.ts` only sets `turn.finalAnswer` when `assistantMsgs.length > 1`. For webhook runs or direct responses (`=== 1`), set `turn.finalAnswer = only.content` explicitly to avoid an empty Result section.
- **Webhook sessions need a synthetic user message** — `sendMessage()` is bypassed, so no user-role message reaches `messages.value`. In `applySessionUpdate()`, push the `session_started.snippet` as `{ role: 'user', content: snippet }` so `groupTurns()` can create a turn.
