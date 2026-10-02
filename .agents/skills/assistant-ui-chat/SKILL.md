---
name: assistant-ui-chat
description: "Assistant chat UI architecture: event handlers, the segment system, inactivity timer, SSE timing, scroll behavior, and common pitfalls. Use when changing the chat view or its SSE handling."
last_reviewed: 2026-10-01
---

# Assistant UI Chat Architecture

**Source files:** `frontend/src/components/AgentIde/assistant/AssistantChat.vue`, `frontend/src/utils/message/messageBuilder.ts`, `frontend/src/utils/message/turnGrouper.ts`

**Related docs:** `.agents/skills/event-streaming-patterns/SKILL.md` (event flow, SSE composables, dedup)

---

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
   ⌕ Searched the web  Claude Haiku 5.5 news   ✓ ›
   ◍ Read a web page   cellcog.ai/blog/…       ✓ ›
 Here's a summary of the latest on **Claude Haiku 5.5**…   ← answer (document)
 ⧉                                                ← footer
```

### Key design rules
- The timeline auto-expands while the session is active (`loading = true`) and auto-collapses when finished (`loading → false`); the user can toggle it via the activity line.
- Tool steps are collapsed by default (click to see arguments + result).
- The answer is always visible, never collapsed by the timeline toggle.
- Only the **live** turn (`loading && isLastTurn`) uses the run's `phase`; every earlier turn renders as finished, so a new run's `thinking` phase never hides older answers.
- The timeline is height-capped (320px, internal scroll) **only while live**; a finished turn opens in full.

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

**Live reasoning renders with full markdown via the `liveReasoning` ref (restored 2026-08-28).** The plain-text experiment (Phase B Round 1) was GPU-neutral — `marked()` is main-thread CPU and the adaptive 100–250ms flush cadence bounds the re-parse — so `ChatBubble.vue` renders the live block through `MarkdownViewer` again, matching committed reasoning. Do NOT re-remove markdown: the GPU residual was never attributed to it. An incremental text-node append experiment (Round 5) was attempted and reverted — it produced no GPU win and caused a rendering regression (content leaking out of the capped inset).

**The inset keeps its 320px cap + internal scroller.** Long reasoning stays bounded inside the bubble and scrolls internally (the inset follows the newest line via the throttled inset auto-scroll, gated to the last turn). Do NOT suspend the cap or set `overflow-y: visible` on `.bubble-inset` — combined with the `grid-template-rows: 1fr/0fr` collapse wrapper and `min-height: 0`, visible overflow lets the streaming text leak outside the panel.

**Auto-scroll is throttled (~250ms), skips no-op passes, and the two scrollers never both scroll.** `ChatMessages.vue` coalesces `scrollIfNearBottom` to ≤1 per 250ms, consumes the throttle window BEFORE the `scrollHeight` read (so the forced sync-layout read runs ≤4x/sec, not every flush), and skips the `scrollTop` mutation when `scrollHeight` didn't grow. `ChatBubble.vue`'s inset watcher only scrolls the inset when it actually overflows (`scrollHeight > clientHeight`) — while reasoning fits under the cap the outer pane's growth check is the single follower; once capped, the inset is the single follower. Pause-on-scroll-up / idle-resume semantics (useAutoScroll) are unchanged on both. It watches `turns` by reference instead of `deep: true` (no whole-history deep-walk per flush). Do not restore per-flush scrolls, remove the overflow guard, or re-add a deep watch on `turns` — they cause redundant scroll+layout+composite passes during streaming.

## Inactivity Detection with `paused` Timer

The thinking-gap dots use a debounced timer to detect inactivity pauses:

```typescript
function resetPauseTimer() {
  if (pauseTimer) clearTimeout(pauseTimer)
  pauseTimer = setTimeout(() => { paused.value = true }, 200)
}
```

### Key rules:

1. **`paused` is only set to `false` by `tool_stream` events** (the only event indicating new output is arriving). `tool_call`, `tool_result`, and `message` events do NOT set `paused = false` — this ensures dots stay visible during the final answer processing.

2. **Timer is only started by `tool_stream` events** and at agent startup (`builder.resetPauseTimer()` in `sendMessage()`). Non-stream events do NOT restart the timer — this ensures the timer fires during the final answer phase even if tool_call/tool_result/message events arrive.

3. **The timer threshold must be short enough** to fire before fast tool executions complete. 200ms is the sweet spot: fires before most tool results, but never during normal streaming (events every 10-50ms keep resetting the timer).

4. **Start the timer at agent startup** by calling `builder.resetPauseTimer()` right after `sse.connect()` in `sendMessage()`. Without this, the initial "waiting" phase has no active timer and dots never appear.

### Template condition

```html
<div :class="{ 'thinking-gap-hidden': !(loading && idx === turns.length - 1 && paused && !turn.finalAnswer) }" class="thinking-gap">
```

The thinking-gap div is **always rendered in the DOM**. When hidden, `visibility: hidden` makes it invisible but the space is reserved in the layout — this prevents the bubble height from changing when dots appear/disappear.

Never use `v-if` or `v-show` for the thinking-gap:
- `v-if` adds/removes the element → bubble height changes → flickering
- `v-show` toggles `display: none` → still removes space from layout → bubble height changes → flickering
- `visibility: hidden` keeps the space → stable bubble height

## Backend: Event Ordering

The backend must send `tool_call`/`tool_result` events BEFORE the `message` event for the same turn. In `session.go`:

```go
// CORRECT:
s.agent.processToolCalls(...)       // sends tool_call, then executes, then tool_result
s.agent.notify(EventMessage, ...)   // THEN sends the message event
```

If `EventMessage` is sent before `processToolCalls`, the frontend receives the `message` event first. `handleMessage()` treats it as the final turn and returns early. Tool call segments are never created because the turn is frozen before any tool_call events arrive.

## Frontend: SSE Connection Timing

The SSE connection must be established before the HTTP POST that starts the agent. In `sendMessage()`:

```typescript
sse.connect()                           // establish SSE first
builder.resetPauseTimer()               // start inactivity timer
await waitForSseConnection()            // wait for "ping" event
await AssistantService.sendMessage(...) // then send HTTP POST
```

Without this wait, the HTTP handler may start executing before the SSE handler subscribes to the EventBus. All agent events go to the `recent` buffer and are replayed at once after the agent finishes — `tool_call` and `tool_result` arrive in the same SSE chunk, the browser processes them in consecutive macrotasks, and the `running` state is never painted.

The SSE "ping" event is sent AFTER the handler subscribes and before it replays recent events. Waiting for "ping" guarantees the subscriber exists before the agent starts.

## Segment System

The `AssistantMessage` type has an optional `segments` array:

```typescript
type Segment =
  | { kind: 'reasoning', text: string }
  | { kind: 'tool_call', name: string, args: string, status: 'running' | 'success' | 'error', result?: string, error?: string }
  | { kind: 'final', text: string }
```

- `reasoning` segments are pushed by `commitReasoning()` when a turn boundary is detected or a tool call occurs
- `tool_call` segments are pushed by `handleToolCall()` with status `'running'`, updated by `handleToolResult()` to `'success'`
- Status transition from `'running'` to `'success'` is synchronous (no setTimeout, no rAF). The real time gap between `tool_call` and `tool_result` events (the tool execution time) provides natural visibility for the spinner.

## Natural Completion Flow

The agent completes a task by producing a content-only assistant message (no further tool calls). The frontend handles this as a normal `message` event: `commitReasoning()` moves streamed text to a reasoning segment, and the final answer text becomes `turn.finalAnswer`. There is no special frontend code for completion — the `message` event is treated the same as any other turn boundary.

**Tool-call-markup guard (2026-08-28):** a content-only `message` whose content contains tool-call markup (`<tool_call`, `[TOOL_CALLS]`) is **NOT** treated as the final answer. Malformed/failed `<tool_call>` attempts are streamed as visible text inside the message payload (the backend cuts them from the live stream via `FilterStreamingMarkup`, but the payload carries the full content); finalizing on them rendered raw JSON as the result and suppressed the real answer (observed with Qwen3.6-35B-A3B). The guard lives in `messageBuilder.ts`'s `message` case.

The `message` event is sent after `processToolCalls` returns and must NOT push a tool-call segment — the tool calls are already represented by their own `tool_call`/`tool_result` events.

## Thinking-Gap Dots Indicator

The thinking-gap shows animated dots during inactivity pauses (no events for 200ms+). It is the ONLY thinking indicator.

- **During streaming**: events every 10-50ms → timer keeps resetting → dots hidden → correct (text is visible)
- **Between stream chunks during a pause**: events stop → 200ms → dots appear → correct
- **Between tool call and result**: no tool_stream events → 200ms → dots appear → correct
- **During final answer**: last tool_stream → 200ms → dots appear and stay through tool_call/tool_result/message → correct
- **Initial startup**: `builder.resetPauseTimer()` starts timer → 200ms → dots appear → correct

The dots NEVER appear while text is streaming. The user needs to see the text, not be distracted by animation.

## Animation States & GPU / Compositing Notes

| Element | Loading | Not Loading |
|---------|---------|-------------|
| Arc-orbit loader (message input only; the assistant turn has no box to ring since 2026-10-01) | Static accent ring **only while `.is-active`** (thinking-gaps) | Inert (`opacity: 0`) |
| Thinking-gap dots | `thinking-pulse` opacity animation **only while visible** (`.bubble-paused:not(.bubble-paused--hidden)`) | Inert (`visibility: hidden`, animation stopped) |
| Generating pulse-dots | Only rendered while `phase === 'generating'` | Not in DOM |

**GPU / compositing rules (see the consolidated `docs/audits/gpu-performance-audit.md` and
forward-looking `docs/PLANS/gpu-performance.md`):**
- **Never leave a CSS animation running on a hidden/invisible element.** CSS animations run even at `opacity: 0` / `visibility: hidden`. Every animation is gated to its visible state — the arc-orbit animation lives on `.arc-orbit-loader.is-active::before`, and the thinking-gap dots animate only under `.bubble-paused:not(.bubble-paused--hidden)`.
- **`content-visibility: auto` on non-live turns** (`.message-wrapper--virtualized`, applied when `!(loading && isLastTurn)`) skips paint/composite of offscreen history so a growing session doesn't re-composite the whole pane. `contain-intrinsic-size: auto 120px` keeps scroll heights stable.
- **Avoid `backdrop-blur` on always-visible elements** — each layer is a constant macOS compositing cost. Always-visible chrome (the sidebar rail, the top strip) uses solid token backgrounds (`bg-canvas`) instead.
- The live reasoning box renders via `MarkdownViewer` (restored 2026-08-28 — GPU-neutral),
  throttled 100ms flush / 250ms outer auto-scroll; the inset keeps its 320px cap + internal
  scroll. Parity check (2026-08-05): same browser, Gemini ≈3% GPU vs llm-proxy ≈30% — the
  residual is NOT a platform floor, but the Round-5 append/uncap experiment was reverted (no
  GPU win + rendering regression). Root cause of the residual still open; profile with Chrome
  DevTools Performance before more changes (`?perf=1` telemetry was removed — its rAF loop
  was itself a GPU confound). 2026-08-28 shipped: double-scroll redundancy removed (the inset
  scrolls only when overflowing; outer `scrollHeight` read throttled ≤4x/sec), always-on
  pulse dots removed, `isolation: isolate` → `z-index: 0` on the bubble. **The arc-orbit
  loader has NO animation** (since 2026-08-28): the `--arc-angle` per-frame gradient repaint
  measured as the dominant during-run GPU cost (~11 points), and a transform-based spin is
  impossible without distorting the rounded-rectangle ring (only a circle can rotate without
  shape change) — so it is a static accent ring and the thinking dots carry the affordance.
  Do NOT re-add an animated conic-gradient loader without a compositor-safe (circular)
  design and a re-measurement.

### Empty inset suppression

The `.bubble-inset` (reasoning/tool panel) is shown via **`v-show` (kept mounted, `display:none` when hidden)** — never `v-if`. `insetVisible` in `ChatBubble.vue` is gated by `insetHasContent` (plus the waiting placeholder), so during the initial wait (`thinking` with no streamed text yet) it is hidden and never paints an empty rounded panel. Keeping it mounted (not `v-if`) makes **collapse/expand during streaming flicker-free**: a `v-if` inset unmounts on collapse and re-mounts + re-parses markdown on expand, which flashes while a stream is live (user-reported 2026-08-28). The trade-off is that a hidden inset still re-renders its live content on each flush (bounded CPU by the adaptive cadence; zero paint while `display:none`).

**Waiting-on-model placeholder.** One exception: `showWaitingPlaceholder` renders a generic "Waiting on the model…" row inside the inset while the last live turn is in `thinking` with nothing streamed yet. It is provider/model-agnostic and reuses the existing phase state (no new heartbeat or event). It disappears as soon as any reasoning/content/segment/`notice`/`generating` state arrives — the `insetHasContent` gate then takes over. This gives the user feedback during a long provider TTFB or a hanging connection before the first upstream-retry notice appears.

## Auto-scroll Behavior

- `watch(segments.length)` scrolls the new segment into view (`behavior: instant`, `block: nearest`)
- **There is NO `watch(messages)` that calls `scrollToBottom()`**. This was removed because it fires on EVERY stream chunk (every 10-50ms), constantly recalculating scroll position and causing visible flickering at the bottom edge of the bubble.
- User has scrolled up: `useAutoScroll` pauses following immediately. If new content arrives and the user stops scrolling for `idleMs` (2s), it resumes and snaps to the bottom; scrolling back to the bottom re-arms it right away. A finished/static conversation stays paused.

## Turn Grouper (`turnGrouper.ts`)

The `groupTurns()` function converts the flat `messages[]` array into structured `Turn[]` objects for rendering. Each turn is anchored by a user-role message followed by zero or more assistant-role messages.

### Agent control messages (contract test)

The backend injects control messages as `role: "user"` (nags, format/retry feedback, the context sieve's `[System Note: …]` and `[Progress ledger …]`, length continuation) plus an empty `[stuck]` assistant placeholder. `isInternalMessage()` hides them and keeps them from starting a turn — when it missed the sieve note and ledger (2026-10-01) they showed as the operator's own messages and split one run into fake turns with a lone "Thought:" as the answer. `CONTROL_PREFIXES` mirrors `isAgentControlMessage` (`backend/internal/core/assistant/session.go`); `__TESTS__/utils/message/controlMessages.contract.test.ts` reads that Go function and `prompts/templates.go` and fails for any control prompt the UI does not hide. **Adding a control prompt in Go → add its prefix here.**

### Single-message turn edge case

When `assistantMsgs.length === 1` (e.g., for webhook-triggered runs or direct text responses), `turn.finalAnswer` must be set explicitly from the single message's content. The `> 1` branch sets `finalAnswer = last.content` automatically, but the `=== 1` branch historically left it empty — causing the "Result" section to be hidden.

**Rule:** When exactly one assistant message exists, set `turn.finalAnswer = only.content` in the same block where `agentOutput` and `segments` are set. The `finalAnswer` content is still filtered by `buildSegmentsFromHistory()` (tool-call content is moved to reasoning segments, so only actual report text ends up in `finalAnswer`).

### `buildSegmentsFromHistory()` content-routing logic

When reconstructing segments from persisted history, assistant messages with tool calls have their `content` moved to a reasoning segment. This prevents intermediate planning text from appearing as raw output:

1. Messages with `reasoning_content` → reasoning segment
2. Messages with `tool_calls + content` → content moved to reasoning segment, message content cleared
3. Content-only messages (natural completion) → content preserved (it IS the report)

## Webhook Sessions

Webhook-triggered agent runs (Telegram, etc.) bypass the frontend's `sendMessage()` flow. Events arrive via SSE as normal, but there's a critical difference:

- **`sendMessage()`** pushes `{ role: 'user', content: text }` to `messages.value` at line 168. The builder finds this user message when processing subsequent events.
- **Webhook sessions** have NO user message in `messages.value` because `sendMessage()` was never called.

Without a user-role message, `groupTurns()` at line 28 (`m.role !== 'user'`) skips all messages, producing zero turns. The assistant bubble never renders.

**Fix:** In `applySessionUpdate()`, when receiving a `session_started` lifecycle event:
1. Check `!loading.value` — avoid pushing during active manual `sendMessage()`
2. Check `p.snippet` is present (contains the user message text)
3. Check for duplicates — don't push if `messages.value` already has the same text
4. Push `{ role: 'user', content: p.snippet }` to `messages.value`

This makes the user message available for `groupTurns()` to anchor the turn, and subsequent SSE events flow through the normal builder pipeline.

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
