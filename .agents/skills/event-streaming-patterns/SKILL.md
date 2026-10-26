---
name: event-streaming-patterns
description: "Event streaming patterns: SSE composables, observer chaining, guardrail flow, heartbeat cleanup, and dedup. Use when working with event streams or SSE handlers."
metadata:
  last_reviewed: "2026-10-26"
---

# Event Streaming Patterns — SSE, Observers & Lifecycle Events

**Source docs:** `SPEC-006`, `internal/core/assistant/agent_events.go`, `frontend/src/composables/assistant/useAssistantSSE.ts`, `frontend/src/composables/automation/useLiveConsole.ts`

---

## Reference files

Open only the one your task needs:

| File | Open it when |
|------|--------------|
| [`references/observers-and-guardrail-flow.md`](references/observers-and-guardrail-flow.md) | You are adding an observer or changing how guardrail approvals flow through events. |
| [`references/sse-composables-and-dedup.md`](references/sse-composables-and-dedup.md) | You are changing a frontend SSE composable or seeing duplicate/missing events. |
| [`references/heartbeat-cleanup.md`](references/heartbeat-cleanup.md) | You are touching stream heartbeats or chasing a goroutine leak. |

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
as JSON payload. Chat asks `?channel=assistant`; an automation console asks
`?channel=automation&automation={name}` (required, else 400) and gets only that automation's
events: every automation event carries `AgentEvent.Automation` and is routed to
`assistant.AutomationStream(name)`, with its own replay buffer (cleared by that automation's boot,
Clear runs / Unregister). Publish through `Dispatcher.publish` / `LLMTaskExecutor.publishRunEvent`,
never `Events().Publish` directly.

## AgentEvent Types

| Type | When Published | Payload | Rendered As |
|------|---------------|---------|-------------|
| `step_start` | Each turn begins | `{step: number}` | Step counter in console |
| `message` | Model produces text | `{role, content}` | Streaming text bubble |
| `tool_call` | Model calls a tool | `{function: {name, arguments}}` | `ToolCallSegment.vue` |
| `tool_result` | Tool execution completes | `{name, result, error?}` | `ToolCallSegment.vue` |
| `tool_stream` | Mid-generation text delta | `string` | Live streaming content |
| `guardrail_blocked` | Tool blocked by guardrail | `{decision_id, tool, args, reason, category}` | `GuardrailBanner.vue` |
| `guardrail_invalidated` | Decision auto-resolved | `{decision_id, reason}` | Clears pending banner |
| `guardrail_violation` | Tool blocked (no approval) | `{tool, error}` | Error message |
| `lifecycle` | Agent/session status events | `{phase, ...}` | `ChatBubble.vue` |
| `error` | Execution error | `string` | Error display |

`lifecycle` phases: `session_*` (session started/progress/completed), `agent_thinking`
(per-LLM-call pre-response compute wait — frontend flips to neutral "thinking…" status;
content-free, never fabricated model output), `stuck_detected`, `fallback_*`,
`guardrail_violation` (sync rejection, no approval flow).

## Lifecycle Event Format

| Phase | Payload | Message |
|-------|---------|---------|
| `stuck_detected` | `{reasoning_chars}` | ⚠️ Model stuck in reasoning loop (N chars) — retrying... |
| `fallback_started` | `{reason, mode}` | 🔄 Switching to {mode} mode — {reason} |
| `fallback_waiting` | `{elapsed}` | ⏳ Waiting for non-streaming response... (elapsed: {time}) |
| `fallback_completed` | `{}` | ✅ Fallback completed successfully |

## Key Files

| File | Purpose |
|------|---------|
| `internal/core/assistant/agent_events.go` | Observer type, lifecycle event helpers |
| `internal/transport/http/handlers/dispatcher_handlers.go` | SSE endpoint, event publish |
| `frontend/src/composables/assistant/useAssistantSSE.ts` | Assistant SSE composable |
| `frontend/src/composables/automation/useLiveConsole.ts` | Automation SSE composable |
| `frontend/src/constants/icons.ts` | `getPhaseMessage()` — lifecycle → emoji mapping |
| `frontend/src/components/common/chat/GuardrailBanner.vue` | Guardrail approval UI |
| `frontend/src/components/AgentIde/assistant/ChatBubble.vue` | Assistant bubble: reasoning + tool segments |
| `frontend/src/components/AgentIde/assistant/ToolCallSegment.vue` | Tool call/result segment rendering |
