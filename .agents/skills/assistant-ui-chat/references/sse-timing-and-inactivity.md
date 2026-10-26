# Assistant UI Chat Architecture — SSE Timing, Event Ordering and Inactivity

Reference for the [`assistant-ui-chat`](../SKILL.md) skill, loaded on demand. A run looks stuck, events arrive out of order, or the inactivity timer misfires.

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
