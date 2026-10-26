---
name: lifecycle-events
description: "Session lifecycle: phases, SSE contract, frontend handler, and testing. Use when changing session phases or refresh/resume handling."
metadata:
  last_reviewed: "2026-10-26"
---

# Session Lifecycle Events

**Source:** `backend/internal/core/assistant/agent_events.go` (phase constants), `backend/internal/core/chat/` (publishing: `service.go` for the turn lifecycle, `conversation_service.go` for the session phases)

---

## Reference files

Open only the one your task needs:

| File | Open it when |
|------|--------------|
| [`references/failures-and-retries.md`](references/failures-and-retries.md) | You are changing retry notices or how a run fails part-way. |
| [`references/refresh-and-resume.md`](references/refresh-and-resume.md) | A page refresh shows the wrong state for a running session. |
| [`references/publishing-and-frontend.md`](references/publishing-and-frontend.md) | You are emitting a lifecycle event or handling one in the frontend. |
| [`references/known-issues-and-webhooks.md`](references/known-issues-and-webhooks.md) | You are debugging a lifecycle regression or webhook-started sessions. |

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
- **Sender:** `conversationService.Execute` in `core/chat/conversation_service.go`
- **Purpose:** Frontend adds the session to the sidebar with `running: true`

### `session_progress`

Published each time the agent fires a `tool_call` event during execution.

- **Payload:** `conversation_id`, `workspace_id`, `snippet` (e.g. "Step 3: query_device")
- **Sender:** the observer `conversationService.buildObserver` builds
- **Purpose:** Frontend updates the sidebar snippet and confirms the session is still running

### `session_completed`

Published after the final `WriteSession` on the success path, or after `WriteSession` on the cancel path.

- **Payload:** `conversation_id`, `workspace_id`, `snippet` (empty — the disk file has the full history)
- **Sender:** `conversationService` in both success and cancel paths
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

### `stuck_detected`

Published when the backend aborts a stream that is looping in its thinking (`reasoning_chars`, `empty_tool_calls`) and asks the model again (`stream.go`, `abortStreamAsStuck`).

- **Payload:** `phase`, `reasoning_chars`, `empty_tool_calls` (the repetition guard sends its own `extra`)
- **Purpose:** the frontend (`handleStuckRestart` in `messageBuilder.ts`) keeps the reasoning that already streamed and shows a pending "The model got stuck thinking — trying again" notice, resolved when the restarted stream resumes. Without it the turn emptied with no explanation.

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
