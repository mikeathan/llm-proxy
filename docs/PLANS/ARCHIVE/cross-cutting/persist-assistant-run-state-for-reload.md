---
status: complete
last_reviewed: 2026-08-20
---

# Persist assistant run state (errors/cancels/running) for reliable reload

**Status:** ✅ Completed

## Problem

Two user-facing gaps when an assistant run fails or the page is refreshed:

1. **Failed runs were never persisted.** On a terminal (non-cancel) failure,
   `conversationService.Execute` only published an SSE `EventError` +
   `session_completed`; it never wrote the session file. Because a hanging
   upstream produces no `tool_result`/`message` events, no mid-run checkpoint
   fired either. A page refresh then showed a session with only the user prompt
   — the failure was invisible and lost.

2. **Per-session running state did not survive refresh.** `SessionBrief` has no
   `running` field; the running flag is client-side (set on `session_started`,
   cleared on `session_completed`). After a refresh the backend knew a run was
   executing (its in-memory per-workspace map) but the UI could not map that to
   a specific history row, so the session appeared not-running.

## Changes

### Backend
- `models.Message` gains an `Error` field (`json:"error,omitempty"`).
- `conversation_service.go` failure path now calls `handleErrorResult`, which
  persists any partial `updatedHistory` plus an assistant-role error message
  (carrying `Error`) and writes the session, then publishes the error +
  `session_completed` lifecycle. Cancel (`handleCancelResult`) and success
  (`handleSuccessResult`) persistence are unchanged and now covered by tests.
- `assistant_handlers.go` stores the resolved conversation ID on the running
  agent (`runningAgent.conversationID`), exposes `RunningConversationID`, and
  resolves the conversation ID up front in `RunWithCancel`.
- `active_runs_handlers.go` / `ActiveRunsResponse` add
  `assistant_conversation_id` (the running conversation ID, or `""`).

### Frontend
- `buildSegmentsFromHistory` maps a persisted `msg.error` to a `kind:'error'` segment
  so a reloaded failed run renders the error in the bubble.
- `useRunningActivity` surfaces `assistantConversationId`;
  `useAssistant.reconcileRunningConversation` +
  `markRunningConversation` (utils/assistant/running.ts) flag exactly the
  matching session as running; `fetchSessions` re-applies it after refresh.
- `ChatBubble.vue` shows a generic "Waiting on the model…" placeholder during
  the initial `thinking` phase with no content — provider/model-agnostic,
  reusing existing phase state (no new heartbeat or event).

### Live running-session reload (in-progress run + refresh/reopen)
Two drift/gap fixes so a running session shows its full live output after
refresh (see `docs/skills/lifecycle-events.md` → "Reconstructing a Running
Session After Refresh"):
- **Backend — unconditional checkpoints:** removed the 1s `sessionCheckpointInterval`
  throttle added in `c7bfca9` (which contradicted the documented "no throttling"
  contract in `session-source-backend-driven.md`). `buildObserver` now checkpoints
  on every `tool_result`/`message`, so a reload returns the latest committed tool
  cycles.
- **Backend — `session_started` retained for replay:** `Execute` now publishes
  `session_started` AFTER `setupRun` clears `recent`, so the event survives in the
  replay buffer. A reconnect can reconstruct the running turn anchored by it.
- **Frontend — reconnect on running load:** `loadSession` running branch now clears
  messages + dedup and reconnects SSE to replay `recent` (full current run) instead
  of overwriting with a stale disk snapshot (which duplicated committed tool calls).

### Bug 1 — finished session stayed "running" (blank screen until refresh)
**Root cause:** `lastRunningConversationId` (useAssistant.ts) was never cleared, so
`fetchSessions`/the `assistantConversationId` watch kept re-marking a finished
session `running: true`; `loadSession` took the running branch against an emptied
`recent` → blank "working" screen.
**Fix:** clear `lastRunningConversationId` on `session_completed`, on
`reconcileRunning(false)`, and when `/active-runs` reports an empty id
(`reconcileRunningConversation`). New pure helper `reconcileRunningSessions`
(utils/assistant/running.ts) — empty id clears all running flags, non-empty marks
exactly one.

### Bug 2 — missing tool calls/reasoning on finished reload
**Root cause:** `TruncateHistory` used a 12KB cap (`MaxHistoryChars`) on the
persisted session file, dropping whole oldest messages (earlier tool calls).
Live SSE was untruncated, so it looked complete while running. Hermes keeps the
full transcript and only compresses the prompt (summarization), so this conflated
the durable record with the model-context budget.
**Fix:** persist full history normally; apply a high last-resort ceiling
`MaxPersistedHistoryChars` (256KB) only on pathological runs. `TruncateHistory`
now takes a `maxChars` param: `MaxHistoryChars` (12KB) for the initial base,
`MaxPersistedHistoryChars` (256KB) for the persisted file. The LLM prompt stays
bounded by the sieve/`context_budget`.

## Safety
- The `Error` field is stripped by `proxy.SanitizeHistory` before history is
  sent to the model, so it never leaks into prompt context.
- The error event for a brand-new conversation now carries the resolved
  conversation ID (resolved up front in `RunWithCancel`), which the UI can
  associate with a session row.
- Persisted-history disk usage stays bounded by `MaxPersistedHistoryChars`
  (256KB) even on pathological (500-message) runs.

## Verification
- Backend: `go build ./...`, `go test ./...`, `go run ./tools/check-complexity/`.
- Frontend: `npm test`, `npm run build`.
- New tests:
  - conversation-service error/cancel persistence;
  - `RunningConversationID`; `ActiveRunsResponse` conversation id;
  - `markRunningConversation`; `reconcileRunningSessions`; `buildSegmentsFromHistory` error mapping;
  - `TestBuildObserver_CheckpointsEveryToolResult` (unconditional checkpoints);
  - `TestExecute_SessionStartedPublishedAfterClear` (reorder retained in replay);
  - `TestTruncateHistory_PersistedCeilingPreservesToolCalls` (256KB ceiling keeps tool calls).

## Future work (separate plan, not part of this session)
Migrate session storage from per-session JSON files to SQLite — see
`docs/PLANS/cross-cutting/sqlite-session-storage.md` (proposed).
