# Session Lifecycle Events

**Source:** `backend/internal/core/assistant/agent_events.go` (phase constants), `backend/internal/transport/http/assistant_handlers.go` (publishing)

---

## Overview

The assistant publishes lifecycle events to the per-workspace SSE event bus so the frontend can track session state in real time. These events power the running ● indicator, the live sidebar session list, and per-session cancel buttons.

## Event Contract

All events share the same structure:

```
type: lifecycle
payload: {
  "phase":           "session_started" | "session_progress" | "session_completed",
  "conversation_id": "conv_20260630154205",
  "workspace_id":    "workspace-test",
  "snippet":         "list all files..."   // first 80 chars of user message or current step
}
```

## Phases

### `session_started`

Published immediately after the user message is written to disk (`WriteSession`), before the agent starts executing.

- **Payload:** `conversation_id`, `workspace_id`, `snippet` (user message, truncated to 80 chars)
- **Sender:** `handleAssistant` in `assistant_handlers.go`
- **Purpose:** Frontend adds the session to the sidebar with `running: true`

### `session_progress`

Published each time the agent fires a `tool_call` event during execution.

- **Payload:** `conversation_id`, `workspace_id`, `snippet` (e.g. "Step 3: query_device")
- **Sender:** `publishObs` callback in `handleAssistant`
- **Purpose:** Frontend updates the sidebar snippet and confirms the session is still running

### `session_completed`

Published after the final `WriteSession` on the success path, or after `WriteSession` on the cancel path.

- **Payload:** `conversation_id`, `workspace_id`, `snippet` (empty — the disk file has the full history)
- **Sender:** `handleAssistant` in both success and cancel paths
- **Purpose:** Frontend sets `running: false` on the session

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

## Multi-Workspace Correctness

Events are scoped per workspace. The SSE connection connects to `/admin/api/dispatcher/workspaces/{ws}/live`. Each workspace has its own event bus subscribers. Lifecycle events for workspace A never reach workspace B's sidebar.

## Testing

Three test functions cover lifecycle events:

| Test | What it verifies |
|------|-----------------|
| `TestHandleAssistant_PublishesSessionLifecycleEvents` | Integration: end-to-end lifecycle events (started, progress, completed) during a full agent run |
| `TestPublishSessionLifecycle_SkipsEmptyIDs` | Unit: helper skips publishing when workspace or conversation ID is empty |
| `TestPublishSessionLifecycle_PublishesWithCorrectPayload` | Unit: helper publishes correct payload fields (type, phase, conversation_id, snippet) |

## Backward Compatibility

- New lifecycle phases are additive — existing consumers ignore unknown phase values.
- `SessionBrief` gains an optional `running` field. Old data (no field) is treated as `undefined` (falsy, no badge).
- The `running` flag is client-side only (not persisted to disk). Sessions appear running only while SSE is connected.
