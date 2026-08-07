---
status: reference
last_reviewed: 2026-07-11
---

# Audit: stale turn bleed on cancel + new message

**Date**: 2026-06-26
**Severity**: high
**Subsystem**: assistant-ui / assistant backend

## Symptom

User reports: send message "list all files" → click Stop → response interrupted bar shows → send "tell me a joke" → new turn's `Result` section contains the prior turn's reasoning text ("The user wants to list files. I should list the contents of the workspace root directory to see what files are available.") concatenated with the new turn's joke.

## Root cause

`AssistantMessageHandler.ServeHTTP` (backend/internal/transport/http/assistant_handlers.go:69) ran the agent on `context.Background()` so the agent could not be canceled by the frontend's `AbortController`. The agent kept executing after the frontend's HTTP request aborted, kept invoking tools, and persisted the partial session via the cancel handler at line 249-252.

When the user sent a new message:
1. Frontend `abortController.abort()` cancelled the HTTP request, disconnecting the SSE pipe.
2. The orphan agent on the backend kept running and finished its turn, writing to the session DB.
3. The new message was a separate request; depending on `conversation_id` it may have appended to the same session the orphan was mutating, OR the orphan's persisted messages were loaded into `messages.value` on the next `loadSession`/send, showing up in the new turn's bubble.

There was no explicit cancel endpoint, so the user had no way to stop the orphan.

## Detection

User reported screenshot. Backend logs confirmed: `stream closed by context cancel` followed by continued tool calls (`list_directory`).

## Resolution

- `assistant_handlers.go`: agent now runs on `context.WithCancel(context.Background())`. Cancel func stored in `sync.Map` registry keyed by `conversation_id`. On request completion, registry entry is removed via defer.
- New `POST /admin/api/conversation/cancel` endpoint. Handler looks up the registry, fires the cancel func.
- Frontend `cancel()` in `useAssistant.ts` now calls `AssistantService.cancelAgent(ws, sid)` before aborting the HTTP request.

## Side fixes (frontend only)

While the backend was broken, two frontend bugs were also fixed:
- `messageBuilder.reset()` was missing `liveReasoning.value = ''`. Stale reasoning from the cancelled turn could show in the new turn's `live-reasoning` slot before the first new reasoning event overwrote it.
- `turnGrouper.groupTurns()` for multi-message turns used `first.segments` instead of `last.segments`. The first message in an agent tool-call flow contains only the start of the work; the final message has the complete segments.

## Lessons

- A cancel path is a first-class concern, not an afterthought. When you decouple a long-running process from the HTTP request lifecycle, you MUST provide an explicit cancel mechanism, otherwise the only signal you accept is "agent ran to completion or hit a timeout".
- Frontend `AbortController` is not a substitute for backend cancel. The HTTP request can abort, but the work on the other side may continue.
- "Detached from r.Context()" comments in production code are a code smell when there is no other cancel path. Either accept the proxy-idle risk and tie to r.Context(), or add a cancel registry. The original comment claimed a benefit (idle resilience) but the cost (uncancellable agents) was never weighed.
