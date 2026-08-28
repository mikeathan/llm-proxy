---
status: reference
last_reviewed: 2026-07-11
---

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
  "phase":           "session_started" | "session_progress" | "session_completed" | "agent_thinking" | "still_thinking",
  "conversation_id": "conv_20260630154205",
  "workspace_id":    "workspace-test",
  "snippet":         "first 80 chars of user message or current step"
}
```

Two lifecycle families exist:
- **Session lifecycle** (`phase` starts with `session_`) — routes to `onSessionUpdate` in `useAssistantSSE.ts`.
- **Agent lifecycle** (`phase` = `agent_thinking`, `stuck_detected`, `fallback_*`, `guardrail_violation`) — per-LLM-call status; routes to the live events list / message builder.

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

### `agent_thinking`

Published at the start of every LLM call (all three compute functions: `computeNextResponse`,
`computeNextResponseNonStreaming`, `computeNextResponseStreamXML`).

- **Payload:** `conversation_id`, `workspace_id`, `step` (current step)
- **Sender:** `notifyLifecycle` in `stream.go` (constant `PhaseAgentThinking` in `agent_events.go`)
- **Purpose:** Frontend flips `phase → 'thinking'` + `thinking = true` for the pre-response compute wait. Status-only — no reasoning/content fields, so it can never be mistaken for model output. Real reasoning arrives later via a `reasoning` event and fills the inset. Emitted for ALL providers (opaque OpenAI included). May double-emit on re-entrancy (XML fallback / prefill retry) — harmless, frontend treats it idempotently.

### `still_thinking`

Published periodically on the heartbeat cadence (`streamHeartbeatInterval` / `nonStreamHeartbeatInterval`, 30s / 15s) while a call is still running but has NOT advanced since the previous tick (a silent-stall liveness signal). Suppressed while content/reasoning is actively streaming, so the bus stays quiet during normal streaming.

- **Payload:** `conversation_id`, `workspace_id`, `elapsed` (rounded duration since the call started, e.g. `"30s"`)
- **Sender:** `core.Heartbeat` (see `core/heartbeat.go`), consumed in `processStream` (stream) and `computeNextResponseNonStreaming` (non-stream `wait` wrapper) in `stream.go`
- **Purpose:** keeps the UI bubble alive (shows `· Ns` elapsed) during a long provider TTFT or silent stall, so the user never sees a dead/blank bubble. Status-only like `agent_thinking` — carries no content. Frontend treats it like `agent_thinking` but idempotent from `idle` ONLY (a completed turn must not re-enter thinking).

## Upstream Retry Notices

`type: upstream` events surface a transient upstream LLM failure that is being retried, so the UI shows why a turn is paused instead of a silent multi-minute stall. Observational only — the retry/backoff policy is unchanged.

**Wire payload** (mirrors `UpstreamEventPayload` in `backend/internal/core/assistant/agent_events.go`; frontend type in `dispatcher.ts` — keep in sync):

```
type: upstream
payload: {
  "event":        "retry",
  "reason":       "transport" | "status",
  "attempt":      2,        // 1-based attempt being retried
  "max_attempts": 3,
  "error":        "unexpected EOF",  // only when reason=transport
  "status":       529,               // only when reason=status
  "elapsed_ms":   1500
}
```

- **Sender:** the retry observer wired in `Agent.Execute` (`proxy.WithRetryObserver`) → `notifyUpstream`. The observer lives on the request context (per-request, safe under the shared `RuntimeClientProvider` client) and fires only on *actual* retries (attempt < max), not on the terminal failure.
- **Frontend:** `messageBuilder.ts` renders each `upstream` event as a `{ kind: 'notice', status: 'pending' }` segment inside the turn inset. When streaming resumes (`tool_stream`/`reasoning`), the most recent pending notice is marked `status: 'resolved'`. Retries do **not** touch phase/thinking/streaming flags (the call is still running).

## Terminal Mid-Run Failure

When the agent run fails mid-execution (e.g. upstream retries exhausted), `conversationService.Execute` (chat path) now publishes **both**:
1. an `EventError` (`type: error`, payload `{ error: ... }`) so the frontend renders an error segment and clears loading, and
2. a `session_completed` lifecycle event so the sidebar stops showing the session as running.

Previously this failure only reached the terminal logs, leaving the bubble stuck on "thinking". Automation runs already publish their own error event (`automation/dispatcher.go`), so the chat-side failure is published in the conversation service only to avoid a duplicate error segment.

**Persisted for reload:** the failure is also written to the session file (`handleErrorResult` in `conversation_service.go`) as an assistant-role message carrying the `Error` field (`models.Message.Error`, JSON `error`). This survives a page refresh: `buildSegmentsFromHistory` (frontend) maps it back to a `kind:'error'` segment so a reloaded failed run shows the error instead of a turn with only the user prompt. The `Error` field is stripped by `proxy.SanitizeHistory` before the history is sent to the model, so it never leaks into prompt context — it is a UI/observability marker only.

## Per-Session Running State After Refresh

The `SessionBrief` returned by `ListSessions` has no `running` field — the per-session running flag is client-side and lost on refresh. The authoritative source is `GET /workspaces/{id}/active-runs`, which now also returns `assistant_conversation_id` (the conversation ID of the agent currently running, or `""`):

- **Backend:** `assistant_handlers.go` stores the resolved conversation ID on the running agent (`runningAgent.conversationID`), exposed via `RunningConversationID` and threaded through `ActiveRunsHandler` into the `ActiveRunsResponse`.
- **Frontend:** `useRunningActivity` surfaces `assistantConversationId`; `useAssistant.reconcileRunningConversation` / `markRunningConversation` (utils/assistant/running.ts) flag exactly the matching session as running. `fetchSessions` re-applies it so a refresh marks the correct history row immediately.

## Reconstructing a Running Session After Refresh

When a run is **in progress** and the user refreshes (or closes and reopens the session from history), the frontend reconstructs the complete live turn from the backend's `recent` replay buffer rather than the disk snapshot:

- **Disk checkpoints** persist committed tool cycles only (`conversation_service.go` `buildObserver` — unconditional, every `tool_result`/`message`, no throttle). In-flight streaming/reasoning is intentionally not persisted, and replaying `recent` on top of the disk snapshot would duplicate committed tool calls.
- **`recent` replay** holds the entire current run (cleared by `setupRun` at run start/end). `session_started` is published **after** `setupRun` clears `recent` so it survives in the buffer for replay — otherwise a reconnect would have no user anchor and the reconstructed turn would not render.
- **Frontend `loadSession` running branch** (`useAssistant.ts`): sets the current session, clears messages, resets the builder, clears the SSE dedup set (`sse.reset()`), then **reconnects** (`connectSSE()`). The replayed `session_started` pushes the user anchor + sets loading, then the replayed tool/reasoning/message/upstream/error events rebuild the turn; live events continue. `loading` starts `false` so the replayed `session_started` can set it and push the anchor.

**Finished runs** (not running) take the non-running `loadSession` branch and read the persisted history from disk. For this branch to be taken, the finished session's `running` flag must be cleared — see "Running flag lost on page refresh" + the `lastRunningConversationId` clearing below.

**Bug 1 fix — stale running flag after finish:** the module-level `lastRunningConversationId` (useAssistant.ts) is now cleared on `session_completed` (`applySessionUpdate`), on `reconcileRunning(false)`, and when `/active-runs` reports an empty id (`reconcileRunningConversation`). Previously the stale id was never cleared, so `fetchSessions`/the `assistantConversationId` watch kept re-marking a finished session `running: true`; `loadSession` then took the running branch against an emptied `recent` → blank "working" screen until a manual refresh reset module state. See `utils/assistant/running.ts` → `reconcileRunningSessions` (empty id clears all flags).

**Bug 2 fix — persisted history ceiling:** the persisted session file is bounded by `MaxPersistedHistoryChars` (256KB) via `TruncateHistory(history, maxChars)`, not the small `MaxHistoryChars` (12KB). Normal runs persist full history so a reload shows the complete tool-call/reasoning trail; the 256KB ceiling only fires on pathological runs (see `docs/PLANS/unattended-run-safety-hardening.md` PL-1). `MaxHistoryChars` (12KB) now bounds only the initial system+user base history.

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

## Known Issues Fixed

### Duplicate user messages in sessions

**Root cause:** `handleAgentMessage` appended the user message to `session.History`, then `handleAssistant` appended it a second time (line 217-222). The session file ended up with `[user, user, assistant]` → two user bubbles in the chat.

**Fix:** Removed the append from `handleAgentMessage`. `handleAssistant` is the single source of truth for appending the user message to the session. Both HTTP chat and webhook flows use the same append path.

### Running flag lost on page refresh

**Root cause:** In `fetchSessions`, the `runningIds` set was captured at function entry (before the API call). SSE events (delivered during the call via `connectSSE`) updated `sessions.value` with `running: true`, but the stale `runningIds` didn't reflect this. When the API call returned, `sessions.value` was reassigned using the stale capture, overwriting the `running: true` flag.

**Fix:** Moved `runningIds` capture to after the API call, right before the reassignment. Any SSE events that arrived during the request are now reflected.

### No result visible after agent completes

**Root cause:** The `AssistantChat.vue` watch on `loading` collapses all work sections when `loading` goes from `true` to `false`. For webhook-originated sessions, `loading` never changes (no `sendMessage` call), so the work section stays in its default state. After the agent completes, the last turn's work remains collapsed, hiding the result text.

**Fix:** Added a watcher on the current session's `running` flag. When it transitions from `true` to `false`, the last turn's work section is expanded.

## Webhook Flow

When a Telegram (or other connector) webhook triggers the agent, the flow is:

1. Webhook receives message → `handleAgentMessage` persists it to disk, then returns immediately (200 OK to Telegram)
2. Agent runs in a background goroutine via `RunWithCancel` (same helper used by the HTTP chat handler)
3. Lifecycle events (`session_started`, `session_progress`, `session_completed`) are published to the workspace EventBus
4. Frontend SSE receives them → sidebar shows `running: true`, auto-selects the new session
5. When the agent finishes, `replyToChat` sends the reply via the connector's `Send()` method (the canonical reply path)
6. If cancelled via the cancel button (or a subsequent request), the context is cancelled and the agent stops

The `notify_user` tool is excluded from the agent in webhook context via the guardrail-derived tool schema (`DisabledToolNames` → `communication.enabled: false` manifest **default**, resolved at the agent's `resolveToolProvider` narrow waist), so with the default policy the model cannot call it. This prevents double-sends — the single reply path through `replyToChat` is the only mechanism. ⚠️ The exclusion is policy-conditional: if an operator enables Communication guardrails (or grants an override), `notify_user` is advertised to webhook sessions too, so the double-send guard no longer holds — the webhook channel has no structural `notify_user` exclusion anymore.

**Key files:**
- `RunWithCancel` in `assistant_handlers.go` — registers/unregisters in the shared `running` map
- `handleAgentMessage` / `runAgentReply` in `webhook_handlers.go` — async agent execution
- `replyToChat` in `webhook_handlers.go` — sends via connector's `Send()` method
- `filtered_provider.go` / `tool_availability.go` in `core/assistant/` — wraps `ToolProvider` to exclude tools per context / guardrail policy

## Backward Compatibility

- New lifecycle phases are additive — existing consumers ignore unknown phase values.
- `SessionBrief` gains an optional `running` field. Old data (no field) is treated as `undefined` (falsy, no badge).
- The `running` flag is client-side only (not persisted to disk). Sessions appear running only while SSE is connected.
