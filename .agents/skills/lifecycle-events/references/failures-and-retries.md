# Session Lifecycle Events — Upstream Retries and Mid-Run Failures

Reference for the [`lifecycle-events`](../SKILL.md) skill, loaded on demand. You are changing retry notices or how a run fails part-way.

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
