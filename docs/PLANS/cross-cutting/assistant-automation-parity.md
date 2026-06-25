---
status: complete
date: 2026-06-12
related_specs: [SPEC-001, SPEC-006, SPEC-008]
---

# Plan: Assistant-Automation Parity + Frontend UX Improvements

**Status:** proposed
**Date:** 2026-06-12
**Related specs:** SPEC-001 (Agent Loop), SPEC-006 (Frontend), SPEC-008 (Guardrails)

## Problem

The assistant (interactive chat) is a second-class citizen compared to
automation. Despite using the same `Agent` engine, the assistant:
1. Cannot execute tools on the first turn (always says "hello" first)
2. Uses XML text mode (`useNativeTools=false`) — cloud models don't get native
   tool calling
3. Has no streaming UX — users wait silently for full HTTP response
4. Hides tool calls and tool results — the model executes tools invisibly
5. Has no guardrail approval UI — blocked tools cannot be approved
6. Hides lifecycle events (`stuck_detected`, `fallback_*`) — invisible to user
7. Uses a loading spinner instead of live content streaming

Meanwhile, the backend **already publishes all the right events** via
`h.svc.Events().Publish()` — the same SSE endpoint
(`/admin/api/dispatcher/workspaces/{id}/live`) serves both automation and
assistant events. The frontend just never subscribes.

## Scope

This plan covers the **assistant chat UX only** — making the interactive
chat experience as rich as the automation console. It does NOT cover:
- Automation features (scheduling, triggers, run management)
- Backend agent loop changes (those are already shared)
- Performance optimizations

## Design Decision: Reuse LiveConsole Event Components

The `LiveConsole.vue` component already has battle-tested rendering for:
- `tool_call` events (icon + name + args + copy)
- `tool_result` events (collapsible + copy)
- `guardrail_blocked` approval banners (Allow/Allow Once/Deny)
- `lifecycle` events (stuck_detected, fallback_* system messages)
- SSE event subscription + stream deduplication

Rather than duplicating this logic into `AssistantChat.vue`, we **extract
reusable sub-components** that both views import:

```
LiveConsole.vue                  AssistantChat.vue
    │                                 │
    ├── ToolCallBlock.vue ←────────────┤  (shared)
    ├── ToolResultBlock.vue ←──────────┤  (shared)
    ├── GuardrailBanner.vue ←──────────┤  (shared)
    ├── LifecycleMessage.vue ←─────────┤  (shared)
    │                                 │
    ├── uses SSE (useLiveConsole)     ├── uses SSE (new useAssistantSSE)
    └── terminal-style layout         └── chat-bubble layout + session sidebar
```

## Implementation

### Phase 1: Extract Shared Event Components

**New files** under `frontend/src/components/common/`:

| Component | Extracted from LiveConsole | Renders |
|---|---|---|
| `ToolCallBlock.vue` | Lines 205-219 | Tool icon + function name + args copy + `<pre>` args |
| `ToolResultBlock.vue` | Lines 223-241 | Collapsible `<details>` + result copy + result body |
| `GuardrailBanner.vue` | Lines 244-295 | Approval banner + Allow/Allow Once/Deny buttons |
| `LifecycleMessage.vue` | Lines 90-114 logic | Phase text + icon for stuck/fallback events |

**Props interface for ToolCallBlock:**
```typescript
interface ToolCallBlockProps {
  name: string
  args: string
}
```

**Props interface for ToolResultBlock:**
```typescript
interface ToolResultBlockProps {
  name: string
  result: string | object
  error?: string
}
```

**Props interface for GuardrailBanner:**
```typescript
interface GuardrailBannerProps {
  decisionId: string
  tool: string
  args: string
  reason: string
  category: string
}
// Emits: allow(persist: boolean) | deny()
```

**Props interface for LifecycleMessage:**
```typescript
interface LifecycleMessageProps {
  phase: 'stuck_detected' | 'fallback_started' | 'fallback_waiting' | 'fallback_completed'
  payload: Record<string, any>
}
```

**File changes:**
- `frontend/src/components/common/ToolCallBlock.vue` — New
- `frontend/src/components/common/ToolResultBlock.vue` — New
- `frontend/src/components/common/GuardrailBanner.vue` — New
- `frontend/src/components/common/LifecycleMessage.vue` — New

**Refactor LiveConsole.vue** to import from these shared components instead
of inlining the rendering. This ensures both views stay in sync as features
evolve.

**Files changed:**
- `frontend/src/components/AgentIde/automation/LiveConsole.vue` — Replace
  inline tool_call/tool_result/guardrail rendering with component imports

### Phase 2: Add SSE Subscription to Assistant

The backend already publishes assistant events via
`h.svc.Events().Publish(workspaceID, ev)` at `assistant_handlers.go:159`.
The frontend just needs to subscribe.

**New composable:** `frontend/src/composables/useAssistantSSE.ts`

A composable that connects to the same `/live` SSE endpoint as automation,
but tailored for the assistant lifecycle:
- Connect when assistant starts executing
- Disconnect when response completes
- Accumulate `tool_stream` events into a streaming message
- Forward `tool_call`, `tool_result`, `lifecycle`, `guardrail_blocked` events
- Deduplicate against SSE reconnect replay (same pattern as `useLiveConsole.ts`)

```typescript
export function useAssistantSSE(workspaceId: () => string) {
  const streamingContent = ref('')
  const liveEvents = ref<AgentEvent[]>([])
  const isConnected = ref(false)
  const pendingDecision = ref<GuardrailBlockedPayload | null>(null)

  const connect()  // EventSource → /admin/api/dispatcher/workspaces/{id}/live
  const disconnect()  // close EventSource

  return { streamingContent, liveEvents, isConnected, pendingDecision, connect, disconnect }
}
```

**Integrate into `useAssistant.ts`:**
- Add `assistantSSE` composable
- On `sendMessage`, connect SSE before the POST, disconnect after response
- Pipe `streamingContent` and `liveEvents` into the messages array

**Files changed:**
- `frontend/src/composables/useAssistantSSE.ts` — New (80 lines)
- `frontend/src/composables/useAssistant.ts` — Integrate SSE (add ~30 lines)

### Phase 3: Update AssistantChat.vue

**Files changed:** `frontend/src/components/AgentIde/assistant/AssistantChat.vue`

Replace the current flat message rendering with:

1. **Streaming message** — When SSE is connected, render `streamingContent`
   as a live-updating assistant message bubble using `marked.parse()`.
   Replace the current `loading` spinner with actual streaming text.

2. **Tool call blocks** — When `liveEvents` contains a `tool_call` event,
   render `<ToolCallBlock>` between message bubbles at the correct position.

3. **Tool result blocks** — When `liveEvents` contains a `tool_result`,
   render `<ToolResultBlock>` after the corresponding tool call.

4. **Lifecycle messages** — When `liveEvents` contains a `lifecycle` event,
   render `<LifecycleMessage>` as a system indicator between messages.

5. **Guardrail banner** — When `pendingDecision` is set, render
   `<GuardrailBanner>` at the top of the message area.

6. **Input stays disabled** during SSE connection (same as current `loading`
   behaviour), not re-enabled until the final response is received.

**Layout structure (after changes):**
```
message-container
  ├── [existing messages: user + assistant bubbles]
  ├── ToolCallBlock      (when tool_call event received)
  ├── ToolResultBlock    (when tool_result event received)
  ├── LifecycleMessage   (when lifecycle event received)
  ├── streaming bubble   (during SSE, replaces loading spinner)
  └── GuardrailBanner    (when pendingDecision is set, pinned to top)
```

### Phase 4: Backend Changes

#### 4a. Enable Native Tools for Assistant

**File:** `assistant_handlers.go`

Currently, `AgentOptions` sets no `UseNativeTools`, so it falls through to
`LocalToolRegistry.UseNativeTools()` which returns `false` (XML text mode).

For the assistant to use native tools (same as automation):
```go
useNative := false
if tier, ok := assistant.ProviderTiers()[providerType]; ok {
    useNative = tier.ToolCallFormat == "native"
}
```

Set `UseNativeTools: &useNative` in `AgentOptions`. This mirrors the same
logic as `executor.go:buildAgentOptions()`.

**Note:** The `AssistantMessageHandler` doesn't currently have access to
the `providerType`. This field comes from `ModelConfig.Provider` which is
set during model loading. The handler currently uses a hardcoded tool
provider. We need to thread `providerType` through the `AssistantService`
interface so the handler can pass it to `NewAgent`.

#### 4b. Remove First-Turn Special Case

**File:** `session.go` in `handleNoToolCalls`

Remove the logic that exits immediately on the first assistant turn:
```go
// Before:
if !s.isAutomation && len(s.history) <= 3 {
    return turnMsg.Content, true, nil
}
```

The assistant should execute tools on the first turn like automation does.
The model will decide whether to respond with text or call a tool.

**Risk:** The model might immediately call tools on greeting messages
("Hello, what tools do you have?"). Mitigation: the model's system prompt
already instructs it to follow the ReAct loop — only call tools when needed.

#### 4c. Add ProviderType to AssistantService Interface

**Files:**
- `internal/transport/http/assistant_service.go`
- `internal/transport/http/assistant_handlers.go`

The `AssistantService` interface (defined in `assistant_handlers.go` or
a separate file) needs a `ProviderType() string` method so the handler
can pass it to `AgentOptions`.

### Phase 5: Update Assistant Chat Response Type

**File:** `frontend/src/types/assistant.ts`

Update `ChatResponsePayload` to include events alongside the reply text:

```typescript
export interface ChatResponsePayload {
  reply: string
  conversation_id: string
  events?: AgentEvent[]  // NEW: events from the agent run
}
```

This allows the frontend to render events that arrived between the SSE
connection dropping and the final HTTP response. The backend already has
all the events in memory — they just need to be returned in the response.

**Backend change** in `assistant_handlers.go` `handleAssistant()`:
After `agent.Execute()` completes, collect events from the observer and
include them in the response alongside `reply`.

### Phase 6: Backend Events Collection

**File:** `assistant_handlers.go`

The current observer publishes events via `h.svc.Events().Publish()` but
doesn't store them. To return events in the HTTP response:

```go
type collectingObserver struct {
    events []assistant.AgentEvent
    next   assistant.Observer  // chain to SSE publisher
}

func (o *collectingObserver) Observe(ev assistant.AgentEvent) {
    o.events = append(o.events, ev)
    if o.next != nil {
        o.next(ev)
    }
}
```

This observer collects events during execution AND forwards them to the
SSE publisher. After execution, events are included in the API response.

## Files to Change

| File | Change | Est. Lines |
|---|---|---|
| `frontend/src/components/common/ToolCallBlock.vue` | **New** | ~40 |
| `frontend/src/components/common/ToolResultBlock.vue` | **New** | ~50 |
| `frontend/src/components/common/GuardrailBanner.vue` | **New** | ~70 |
| `frontend/src/components/common/LifecycleMessage.vue` | **New** | ~30 |
| `frontend/src/composables/useAssistantSSE.ts` | **New** | ~80 |
| `frontend/src/composables/useAssistant.ts` | Integrate SSE | ~30 |
| `frontend/src/components/AgentIde/assistant/AssistantChat.vue` | Add streaming + event rendering + guardrail UI | ~150 |
| `frontend/src/components/AgentIde/automation/LiveConsole.vue` | Refactor to use shared components | ~80 |
| `frontend/src/types/assistant.ts` | Add `events` to `ChatResponsePayload` | 2 |
| `backend/internal/transport/http/assistant_handlers.go` | Enable native tools, collecting observer, include events in response | ~40 |
| `backend/internal/transport/http/assistant_service.go` | Add `ProviderType()` to interface | 3 |
| `backend/internal/core/assistant/session.go` | Remove first-turn special case | ~5 |

## Frontend Architecture

### Component Hierarchy (After Changes)

```
AssistantChat.vue
  ├── SessionSidebar (existing)
  ├── MessageContainer
  │   ├── Message Bubble (user) [existing]
  │   ├── Message Bubble (assistant) [existing, updated for streaming]
  │   ├── ToolCallBlock (new, shared)
  │   ├── ToolResultBlock (new, shared)
  │   ├── LifecycleMessage (new, shared)
  │   └── Streaming Bubble (new — replaces loading spinner)
  ├── GuardrailBanner (new, shared — pinned above input)
  └── Input area [existing]

LiveConsole.vue (refactored)
  ├── TerminalHeader [existing]
  ├── GuardrailBanner (new, shared)
  ├── TerminalBody
  │   ├── StepLabel [existing]
  │   ├── Message [existing]
  │   ├── ToolCallBlock (new, shared — replaces inline)
  │   ├── ToolResultBlock (new, shared — replaces inline)
  │   └── LifecycleMessage (new, shared — replaces inline)
```

### Data Flow

```
AssistantChat.vue
  │
  ├─ useAssistant() composable
  │     ├── sendMessage() → POST /admin/api/conversation/message
  │     │     └── response: { reply, conversation_id, events? }
  │     │
  │     └── useAssistantSSE() composable (NEW)
  │           └── EventSource → /admin/api/dispatcher/workspaces/{id}/live
  │                 ├── "tool_stream" → streamingContent
  │                 ├── "tool_call"   → liveEvents (→ ToolCallBlock)
  │                 ├── "tool_result" → liveEvents (→ ToolResultBlock)
  │                 ├── "lifecycle"   → liveEvents (→ LifecycleMessage)
  │                 ├── "guardrail_blocked" → pendingDecision (→ GuardrailBanner)
  │                 └── "message" (final) → append to messages, stop SSE
```

## Event Deduplication Strategy

SSE events and the HTTP response events may overlap. The frontend handles
this via:

1. **SSE event IDs** — Server assigns IDs to each event. Client tracks
   received IDs and skips duplicates (same pattern as `useLiveConsole.ts`
   `handledDecisionIds`).
2. **Response events are additive** — The HTTP response includes only
   events that arrived after SSE disconnect (gap-filling). Events already
   rendered via SSE are skipped.
3. **Streaming content** — The `tool_stream` content is continuously
   updated during SSE. When the HTTP response arrives, it contains the
   final `reply` which replaces the streaming content.

## Test Strategy

| Test | Covers |
|---|---|
| `TestToolCallBlock` | Renders tool name, args, copy button |
| `TestToolResultBlock` | Renders result, expand/collapse, copy |
| `TestGuardrailBanner` | Allow/Allow Once/Deny emits correct events |
| `TestLifecycleMessage` | Each phase renders correct text |
| `TestAssistantSSE` | SSE connects/disconnects, events deduplicated |
| `TestLiveConsoleRefactor` | LiveConsole still renders all event types correctly |

## Risks & Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| **SSE event ordering mismatch** | Tool result before tool call | Agent backend emits events in order; SSE preserves order. Frontend renders in received order. |
| **SSE vs HTTP race** | Duplicate events or missed events | Dedup via event IDs. HTTP response fills gaps after SSE disconnect. |
| **Guardrail decision race** | Decision arrives after SSE reconnect | `handledDecisionIds` set persists across SSE reconnects. Same pattern as automation. |
| **Network disconnect during SSE** | Partial streaming content + no response | SSE reconnect auto-retries. HTTP fallback when SSE fails entirely (existing behaviour). |
| **Backend events not collected** | Empty `events` in HTTP response | Collector observer always captures events. Empty array is valid — frontend handles it. |

## Future Work (Out of Scope)

1. **Multi-model assistant** — Switch models mid-conversation
2. **File upload in assistant** — Attach files to messages
3. **Agent continuation** — Manual "continue" button to extend long responses
4. **Message editing** — Edit and resend user messages
5. **Tool call inline approval** — Approve specific args before execution
