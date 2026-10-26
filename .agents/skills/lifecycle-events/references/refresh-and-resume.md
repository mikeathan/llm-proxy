# Session Lifecycle Events — Running State and Reconstruction After Refresh

Reference for the [`lifecycle-events`](../SKILL.md) skill, loaded on demand. A page refresh shows the wrong state for a running session.

## Per-Session Running State After Refresh

The `SessionBrief` returned by `ListSessions` has no `running` field — the per-session running flag is client-side and lost on refresh. The authoritative source is `GET /workspaces/{id}/active-runs`, which now also returns `assistant_conversation_id` (the conversation ID of the agent currently running, or `""`):

- **Backend:** `chat.Service` (`core/chat/service.go`) stores the resolved conversation ID on the workspace's running turn (`runningTurn.conversationID`), exposed via `RunningConversation` and threaded through `ActiveRunsHandler` into the `ActiveRunsResponse`.
- **Frontend:** `useRunningActivity` surfaces `assistantConversationId`; `useAssistant.reconcileRunningConversation` / `markRunningConversation` (utils/assistant/running.ts) flag exactly the matching session as running. `fetchSessions` re-applies it so a refresh marks the correct history row immediately.

## Reconstructing a Running Session After Refresh

When a run is **in progress** and the user refreshes (or closes and reopens the session from history), the frontend reconstructs the complete live turn from the backend's `recent` replay buffer rather than the disk snapshot:

- **Disk checkpoints** persist committed tool cycles only (`conversation_service.go` `buildObserver` — unconditional, every `tool_result`/`message`, no throttle). In-flight streaming/reasoning is intentionally not persisted, and replaying `recent` on top of the disk snapshot would duplicate committed tool calls.
- **`recent` replay** holds the entire current run (cleared by `setupRun` at run start/end). `session_started` is published **after** `setupRun` clears `recent` so it survives in the buffer for replay — otherwise a reconnect would have no user anchor and the reconstructed turn would not render.
- **Frontend `loadSession` running branch** (`useAssistant.ts`): sets the current session, clears messages, resets the builder, clears the SSE dedup set (`sse.reset()`), then **reconnects** (`connectSSE()`). The replayed `session_started` pushes the user anchor + sets loading, then the replayed tool/reasoning/message/upstream/error events rebuild the turn; live events continue. `loading` starts `false` so the replayed `session_started` can set it and push the anchor.

**Finished runs** (not running) take the non-running `loadSession` branch and read the persisted history from disk. For this branch to be taken, the finished session's `running` flag must be cleared — see "Running flag lost on page refresh" + the `lastRunningConversationId` clearing below.

**Bug 1 fix — stale running flag after finish:** the module-level `lastRunningConversationId` (useAssistant.ts) is now cleared on `session_completed` (`applySessionUpdate`), on `reconcileRunning(false)`, and when `/active-runs` reports an empty id (`reconcileRunningConversation`). Previously the stale id was never cleared, so `fetchSessions`/the `assistantConversationId` watch kept re-marking a finished session `running: true`; `loadSession` then took the running branch against an emptied `recent` → blank "working" screen until a manual refresh reset module state. See `utils/assistant/running.ts` → `reconcileRunningSessions` (empty id clears all flags).

**Bug 2 fix — persisted history ceiling:** the persisted session file is bounded by `MaxPersistedHistoryChars` (256KB) via `TruncateHistory(history, maxChars)`, not the small `MaxHistoryChars` (12KB). Normal runs persist full history so a reload shows the complete tool-call/reasoning trail; the 256KB ceiling only fires on pathological runs (see `docs/PLANS/ARCHIVE/cross-cutting/unattended-run-safety-hardening.md` PL-1). A 12KB cap that `initSession` applied at the start of every turn kept a long chat to its first question and newest exchange (the model lost the conversation, a reload lost messages); it was removed on 2026-10-23, so every path uses the 256KB ceiling and the model's prompt is bounded by the sieve (`context_budget`).
