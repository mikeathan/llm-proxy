# Assistant UI Chat Architecture — MessageBuilder

Reference for the [`assistant-ui-chat`](../SKILL.md) skill, loaded on demand. You are changing how SSE events become chat messages (`messageBuilder.ts`).

## MessageBuilder (`messageBuilder.ts`)

Central state machine for processing SSE events into renderable messages and segments.

### State variables

| Variable | Type | Purpose |
|----------|------|---------|
| `assistantIdx` | `number \| null` | Index of current assistant message in `messages` array |
| `reasoningBuffer` | string | Raw uncommitted streaming text (current model reply) |
| `reasoningCommitted` | string | Accumulated committed text (persisted to segments) |
| `liveReasoning` | `Ref<string>` | Reactive ref for live streaming display (uncommitted text) |
| `streaming` | `Ref<boolean>` | True while tool_stream events are being received |
| `thinking` | `Ref<boolean>` | True during reasoning phase — set by `agent_thinking` lifecycle (pre-response compute wait) OR `tool_stream` events |
| `paused` | `Ref<boolean>` | True when no tool_stream events for 200ms (inactivity detection) |
| `lastClean` | string | Last tool_stream text for detecting contiguous streaming |
| `inReasoningPhase` | boolean | True during `reasoning` events, false during `tool_stream` events. Used by `handleToolStream` to detect reasoning→content transitions without a fragile length heuristic. |

### Flow per event

| Event | Action |
|-------|--------|
| `tool_stream` | If `stripToolCallXml(text)` is non-empty: call `ensureAssistant()`, update `reasoningBuffer` + `liveReasoning`, `render()`, restart 200ms pause timer. |
| `tool_call` | Call `ensureAssistant()` + `commitReasoning()`. Push `tool_call` segment with `status: 'running'`. **Does NOT restart pause timer.** |
| `tool_result` | Call `ensureAssistant()`. Find matching `tool_call` segment, update to `status: 'success'`. **Does NOT restart pause timer.** |
| `guardrail_violation` | Call `ensureAssistant()`, push `{kind: 'guardrail', tool, error}` segment, `render()`. Synchronous rejections emit this with NO preceding `tool_call`/`tool_result` pair, so it must create the segment itself. |
| `message` | `commitReasoning()`, `render()`. **Does NOT restart pause timer.** |
| `finalize(reply)` | Ensure message content is `reasoningCommitted`, push result message. |

### CRITICAL: `ensureAssistant()` must be called before pushing segments

The most insidious bug in this system was that `handleToolCall()` and `handleToolResult()` did NOT call `ensureAssistant()`. They assumed `handleToolStream()` would have already created the assistant message. If the model produces no reasoning text before a tool call (or if all streaming text is stripped by `stripToolCallXml`), the assistant message is never created. `getSegments()` returns a fresh empty `[]` on every call — segments are pushed to orphan arrays, `forceUpdate()` does nothing because `assistantIdx` is null. The tool call is invisible.

**Always call `ensureAssistant()` at the start of `handleToolCall()` and `handleToolResult()`.**

### Event handler ordering: handler first, then refs

In `handleEvent()`, the event-specific handler (`handleToolCall`, etc.) must run BEFORE toggling `streaming`/`thinking` refs. If refs are toggled first, Vue may process the conditional work section rendering (which depends on `thinking` + `liveReasoning`) before the handler has pushed segments. The work section briefly hides, then shows again when the handler's `forceUpdate()` runs — causing visible flickering.

**Wrong:**
```typescript
case 'tool_call':
  streaming.value = false   // work section might hide
  thinking.value = false     //   if segments not pushed yet
  handleToolCall(...)        // pushed segments, too late
  return
```

**Correct:**
```typescript
case 'tool_call':
  handleToolCall(...)        // push segments FIRST
  streaming.value = false
  thinking.value = false
  return
```

### Live reasoning text: use a separate ref, not `turn.agentOutput`

During streaming, the model's text is accumulated in `reasoningBuffer`. The template shows it via `liveReasoning` — a separate `Ref<string>` set to the current buffer. When `commitReasoning()` runs (on tool_call), it pushes a reasoning segment AND clears `liveReasoning.value = ''`. This prevents duplicate text display (the committed segment shows the same text).

The `turn.agentOutput` field was removed — it was never consumed by any Vue template.

### Force update pattern

Vue's `ref([])` does NOT make array elements deeply reactive by default. Mutating a message's `content` or `segments` inside the array does not reliably trigger computed re-evaluation. The `forceUpdate()` function creates a shallow copy of the message at its array index, forcing Vue to detect the change:

```typescript
function forceUpdate() {
  const idx = assistantIdx
  if (idx !== null && idx < messages.value.length) {
    messages.value[idx] = { ...messages.value[idx] } as AssistantMessage
  }
}
```

Call `forceUpdate()` after every mutation that must be reflected in the template:
- After pushing a segment (tool_call, reasoning, guardrail)
- After updating a segment (tool_result)
- After `commitReasoning()` pushes a reasoning segment
- **NOT during pure text streaming** — see below

**Streaming renders via the `liveReasoning` ref only; committed turns stay frozen.** `handleToolStream()` buffers streamed text in `pendingText` and flushes `liveReasoning.value` at most once per 100ms (`scheduleFlush()`). The flush does NOT replace the message object — no `forceUpdate()`. This keeps the `turns` computed frozen during text streaming, so historical turns never re-render and their `MarkdownViewer` computeds never re-run (the memoized-list pattern ChatGPT/Gemini use). `forceUpdate()` runs only on segment mutations (`tool_call`/`tool_result`/`commitReasoning`/`guardrail`), where the turn genuinely changes. `clearPendingFlush()` runs in `commitReasoning()`/`finalize()`/`reset()` so committed or finished text is never re-flashed as live.

**Live reasoning renders with full markdown via the `liveReasoning` ref.** `marked()` is main-thread CPU and the adaptive 100–250ms flush cadence bounds the re-parse, so `ChatBubble.vue` renders the live block through `MarkdownViewer`, matching committed reasoning. Do NOT replace it with plain text: the GPU residual was never attributed to markdown. Do NOT switch to incremental text-node appends either: they gave no GPU win and let content leak out of the capped inset.

**The inset keeps its 320px cap + internal scroller.** Long reasoning stays bounded inside the bubble and scrolls internally (the inset follows the newest line via the throttled inset auto-scroll, gated to the last turn). Do NOT suspend the cap or set `overflow-y: visible` on `.bubble-inset` — combined with the `grid-template-rows: 1fr/0fr` collapse wrapper and `min-height: 0`, visible overflow lets the streaming text leak outside the panel.

**Auto-scroll is throttled (~250ms), skips no-op passes, and the two scrollers never both scroll.** `ChatMessages.vue` coalesces `scrollIfNearBottom` to ≤1 per 250ms, consumes the throttle window BEFORE the `scrollHeight` read (so the forced sync-layout read runs ≤4x/sec, not every flush), and skips the `scrollTop` mutation when `scrollHeight` didn't grow. `ChatBubble.vue`'s inset watcher only scrolls the inset when it actually overflows (`scrollHeight > clientHeight`) — while reasoning fits under the cap the outer pane's growth check is the single follower; once capped, the inset is the single follower. Pause-on-scroll-up / idle-resume semantics (useAutoScroll) are unchanged on both. It watches `turns` by reference instead of `deep: true` (no whole-history deep-walk per flush). Do not restore per-flush scrolls, remove the overflow guard, or re-add a deep watch on `turns` — they cause redundant scroll+layout+composite passes during streaming.
