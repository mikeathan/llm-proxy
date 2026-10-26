# Session Lifecycle Events — Publishing Helper and Frontend Handling

Reference for the [`lifecycle-events`](../SKILL.md) skill, loaded on demand. You are emitting a lifecycle event or handling one in the frontend.

## Publishing Helper

```go
// publishSessionLifecycle publishes a lifecycle event to the workspace event bus
// so the frontend can update the conversation sidebar in real time.
func (h *AssistantMessageHandler) publishSessionLifecycle(workspaceID, conversationID, snippet, phase string) {
    if workspaceID == "" || conversationID == "" {
        return
    }
    h.svc.Events().Publish(workspaceID, assistant.AgentEvent{
        ID:        fmt.Sprintf("sse_%d", time.Now().UnixNano()),
        Type:      assistant.EventLifecycle,
        Payload: map[string]any{
            "phase":           phase,
            "conversation_id": conversationID,
            "workspace_id":    workspaceID,
            "snippet":         snippet,
        },
        Timestamp: time.Now(),
    })
}
```

## Frontend Handling

In `useAssistantSSE.ts`, the lifecycle handler checks if the event is a session lifecycle event (phase starts with `session_`) and routes it to the `onSessionUpdate` callback instead of the live events list.

The callback in `useAssistant.ts` updates `sessions.value`:
- `session_started` — adds a new entry with `running: true` or updates existing entry
- `session_progress` — updates the snippet on the existing entry
- `session_completed` — sets `running: false`

```typescript
function applySessionUpdate(p: SessionLifecyclePayload) {
  const cid = p.conversation_id
  if (!cid) return
  const idx = sessions.value.findIndex(s => s.id === cid)
  // ... update logic
}
```
