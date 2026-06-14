# Event Streaming Patterns — SSE, Observers & Lifecycle Events

**Source docs:** `SPEC-006`, `internal/core/assistant/agent_events.go`, `frontend/src/composables/useAssistantSSE.ts`, `frontend/src/composables/automation/useLiveConsole.ts`

---

## Architecture

The agent publishes real-time events during execution via an **Observer** function.
Events flow through two channels:

```
Agent.notify() → Observer(AgentEvent)
                    ├── h.svc.Events().Publish() → SSE endpoint → Frontend
                    └── collectingObserver (optional) → HTTP response events[]
```

The same SSE endpoint (`/admin/api/dispatcher/workspaces/{id}/live`) serves both
automation and assistant events. The `agent_update` SSE event carries `AgentEvent`
as JSON payload.

## AgentEvent Types

| Type | When Published | Payload | Rendered As |
|------|---------------|---------|-------------|
| `step_start` | Each turn begins | `{step: number}` | Step counter in console |
| `message` | Model produces text | `{role, content}` | Streaming text bubble |
| `tool_call` | Model calls a tool | `{function: {name, arguments}}` | `ToolCallBlock.vue` |
| `tool_result` | Tool execution completes | `{name, result, error?}` | `ToolResultBlock.vue` |
| `tool_stream` | Mid-generation text delta | `string` | Live streaming content |
| `guardrail_blocked` | Tool blocked by guardrail | `{decision_id, tool, args, reason, category}` | `GuardrailBanner.vue` |
| `guardrail_invalidated` | Decision auto-resolved | `{decision_id, reason}` | Clears pending banner |
| `guardrail_violation` | Tool blocked (no approval) | `{tool, error}` | Error message |
| `lifecycle` | Stuck/fallback events | `{phase, ...}` | `LifecycleMessage.vue` |
| `error` | Execution error | `string` | Error display |

## Observer Pattern

### Observer is a raw function type

```go
type Observer func(AgentEvent)
```

There can **only be one observer per agent** — setting a new one silently replaces
the prior one. If both test capture and production publish are needed, chain them:

```go
var captured []AgentEvent
agent := NewAgent(..., AgentOptions{
    Observer: func(ev AgentEvent) {
        captured = append(captured, ev)
        productionObserver(ev)  // forward to SSE
    },
})
```

### CollectingObserver for HTTP responses

For assistant chat responses, events must also be returned in the HTTP body
(to fill the gap between SSE disconnect and response arrival):

```go
// In assistant_handlers.go:
var collected []AgentEvent
publishObs := func(ev AgentEvent) {
    collected = append(collected, ev)
    h.svc.Events().Publish(workspaceID, ev)
}
// ... Execute() ...
// Return both reply + collected events:
return map[string]any{
    "reply":  reply,
    "events": collected,  // serialised as JSON-friendly map
}
```

## Guardrail Decision Flow

```
Model calls tool → GuardrailEngine blocks → GuardrailDecisionCallback invoked
  ├── Decision stored in GuardrailDecisionStore with unique `decision_id`
  ├── `guardrail_blocked` event published (SSE)
  ├── Frontend shows GuardrailBanner → user clicks Allow/Deny
  │     └── POST /admin/api/conversation/guardrail-decision {decision_id, allow, persist}
  │           └── DecisionStore.Resolve() → wakes callback
  ├── Callback returns {Allow, Persist} to processToolCalls
  └── Guardrail times out after 60s → auto-invalidated → guardrail_invalidated event
```

### Frontend guardrail state management

```typescript
// useLiveConsole.ts / useAssistantSSE.ts:
const pendingDecision = ref<GuardrailBlockedPayload | null>(null)
const handledDecisionIds = new Set<string>()

// On guardrail_blocked event:
if (!handledDecisionIds.has(payload.decision_id)) {
    pendingDecision.value = payload
}

// On guardrail_invalidated event:
handledDecisionIds.add(payload.decision_id)
if (pendingDecision.value?.decision_id === payload.decision_id) {
    pendingDecision.value = null
}

// After submitting decision:
handledDecisionIds.add(id)
pendingDecision.value = null
```

## SSE Composables

### Automation: `useLiveConsole`

```typescript
export function useLiveConsole(workspaceId, isExecuting, historyEvents) {
    const liveEvents = ref<AgentEvent[]>([])
    const displayEvents = computed(() => liveEvents.value.length > 0
        ? liveEvents.value
        : historyEvents() || [])

    connect()  // EventSource → /live endpoint
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

SSE reconnects may replay events. The frontend handles this via:

1. **Server-assigned event IDs** — Check `ev.id` against a `receivedEventIds` Set.
   If already seen, skip processing.
2. **`handledDecisionIds` Set** — Persists across SSE reconnects so guardrail
   decision prompts aren't re-shown.
3. **HTTP response fill** — Events from the HTTP response fill gaps between
   SSE disconnect and final response. Frontend skips events already seen via SSE.

```typescript
const handleAgentEvent = (ev: AgentEvent) => {
    if (ev.id && receivedEventIds.has(ev.id)) return
    if (ev.id) receivedEventIds.add(ev.id)

    // ... handle event by type ...
}
```

## Lifecycle Event Format

| Phase | Payload | Message |
|-------|---------|---------|
| `stuck_detected` | `{reasoning_chars}` | ⚠️ Model stuck in reasoning loop (N chars) — retrying... |
| `fallback_started` | `{reason, mode}` | 🔄 Switching to {mode} mode — {reason} |
| `fallback_waiting` | `{elapsed}` | ⏳ Waiting for non-streaming response... (elapsed: {time}) |
| `fallback_completed` | `{}` | ✅ Fallback completed successfully |

## Heartbeat Goroutine Cleanup

`computeNextResponseNonStreaming` spawns a heartbeat goroutine that ticks every
`nonStreamHeartbeatInterval` to emit `fallback_waiting` events. The goroutine
must exit cleanly when the stream ends:

```go
heartbeatDone := make(chan struct{})

go func() {
    defer close(heartbeatDone)
    for {
        select {
        case <-ctx.Done():
            return
        case <-streamDone:
            return
        case <-time.After(interval):
            notifyLifecycle("fallback_waiting", elapsed)
        }
    }
}()

// When computeNextResponseNonStreaming returns:
close(streamDone)
<-heartbeatDone  // wait for goroutine to exit
```

The `streamDone` channel ensures the goroutine exits when `processStream` returns
for ANY reason (stuck detection, stream EOF, errors), not just context cancellation.
Always pair `select { case <-ctx.Done(): case <-streamDone: }` in new goroutines.

## Key Files

| File | Purpose |
|------|---------|
| `internal/core/assistant/agent_events.go` | Observer type, lifecycle event helpers |
| `internal/transport/http/dispatcher_handlers.go` | SSE endpoint, event publish |
| `frontend/src/composables/useAssistantSSE.ts` | Assistant SSE composable |
| `frontend/src/composables/automation/useLiveConsole.ts` | Automation SSE composable |
| `frontend/src/constants/icons.ts` | `getPhaseMessage()` — lifecycle → emoji mapping |
| `frontend/src/components/common/GuardrailBanner.vue` | Guardrail approval UI |
| `frontend/src/components/common/LifecycleMessage.vue` | Lifecycle event rendering |
| `frontend/src/components/common/ToolCallBlock.vue` | Tool call rendering |
| `frontend/src/components/common/ToolResultBlock.vue` | Tool result rendering |
