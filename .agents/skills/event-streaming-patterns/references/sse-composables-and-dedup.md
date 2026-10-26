# Event Streaming Patterns — SSE, Observers & Lifecycle Events — SSE Composables and Event Dedup

Reference for the [`event-streaming-patterns`](../SKILL.md) skill, loaded on demand. You are changing a frontend SSE composable or seeing duplicate/missing events.

## SSE Composables

### Automation: `useLiveConsole`

```typescript
export function useLiveConsole(workspaceId, isExecuting, historyEvents, automationName) {
    const liveEvents = ref<AgentEvent[]>([])
    const displayEvents = computed(() => liveEvents.value.length > 0
        ? liveEvents.value
        : historyEvents() || [])

    connect()  // EventSource → /live?channel=automation&automation=<name>
    disconnect()
    submitDecision(allow, persist)
}
```

- Always connected when the automation console is mounted
- Displays `displayEvents` — live events during run, history events after run
- Guardrail decisions submitted via `ApiService.submitGuardrailDecision()`

### Assistant: `useAssistantSSE`

```typescript
export function useAssistantSSE(workspaceId) {
    const streamingContent = ref('')      // tool_stream → live text
    const liveEvents = ref<AgentEvent[]>([])  // all other event types
    const pendingDecision = ref<GuardrailBlockedPayload | null>(null)

    connect()   // connect before POST
    disconnect() // disconnect after HTTP response
    reset()      // clear all state for new message
}
```

- Connected only during message execution (not persistent like automation)
- `streamingContent` replaces the loading spinner with live text
- `liveEvents` accumulates tool_call, tool_result, lifecycle events
- Disconnected as soon as the HTTP response arrives

## SSE Event Deduplication Strategy

The bus keeps a per-workspace/stream `recent` buffer (1000 events / 4 MiB; an automation's stream is its own, so a workspace holds at most that per automation) that a new subscriber receives before live events: it is how a reopened chat rebuilds a running turn. `reasoning` and `tool_stream` events are **full snapshots** (the text so far), so `Bus.Publish` replaces the newest buffered event when the new one supersedes it (same type and conversation, string payloads, nothing between: `assistant.SupersedesSnapshot`). Without that, a four-minute run published ~3,300 snapshots (11 MB), evicted `session_started` and the tool cycles, and a reopened chat got only a window of snapshots with no start. `eventbus.Sink` (`events.jsonl`) applies the same rule plus a 10 s checkpoint. The chat opens through `useAssistant.openWorkspace`, which connects to the live stream once (loading a running conversation connects itself).

SSE reconnects may replay events. The frontend handles this via:

1. **Server-assigned event IDs** — Check `ev.id` against a seen-id Set.
   If already seen, skip processing. Both SSE consumers (chat
   `useAssistantSSE`, automation `useLiveConsole`) share the single
   `createEventIdDeduper()` implementation in
   `frontend/src/utils/events/eventIdDedup.ts` — **never hand-roll a second
   dedup** (the automation variant previously drifted to a
   `liveEvents.some()` array scan, O(n) per event → O(n²) over a long run).
   An event without an id always passes and gets one assigned.
2. **`handledDecisionIds` Set** — Persists across SSE reconnects so guardrail
   decision prompts aren't re-shown.
3. **HTTP response fill** — Events from the HTTP response fill gaps between
   SSE disconnect and final response. Frontend skips events already seen via SSE.

```typescript
const eventIds = createEventIdDeduper()

const handleAgentEvent = (ev: AgentEvent) => {
    if (!eventIds.accept(ev)) return

    // ... handle event by type ...
}
```
