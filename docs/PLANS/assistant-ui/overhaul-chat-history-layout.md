---
status: active
last_reviewed: 2026-07-11
---

# Assistant UI Overhaul — Chat, History & Layout

**Status:** active  
**Date:** 2026-06-25  
**Phases:** 1 ✅ | 2 ✅ | 3 ✅ | 4 ⬜ (3/7) | 5 ☐
**Related:** `simple-three-bubble.md`, `consolidated-streaming-bubbles.md` (predecessor — complete)

## Problem

The assistant UI has these structural issues:

1. **Buried entry point** — The "Open Assistant" button lives inside `SystemPulseDashboard.vue` (line 53). No permanent assistant button exists. User must select a workspace, then find the button inside the dashboard.

2. **No conversation history** — Despite the backend having full session CRUD (`ListSessions`, `ReadSession`, `WriteSession`, `DeleteSession`) and the frontend service having `listSessions`/`getSession`/`deleteSession` wired, the UI exposes none of it. Every conversation is ephemeral in the UI. Sessions persist on disk but are invisible.

3. **Monolithic chat component** — `AssistantChat.vue` is 675 lines with multiple concerns: SSE connection, message building, segment management, layout, template rendering, and scrolling. No subcomponents extracted.

4. **Missing UI polish** — No error states for failed tool calls or agent errors. No loading states beyond the thinking-gap dots. The 200ms inactivity timer fires during slow model generation. No mobile-responsive layout for the chat/panel combination.

5. **Stop button broken** — The cancel function (`useAssistant.ts:33`) calls `AbortController.abort()` and `sse.disconnect()`, but the backend agent continues running because the HTTP context cancellation may not propagate during streaming. The agent's `processStream` goroutine and tool execution continue after the UI disconnects.

6. **No page-refresh resilience** — If the browser is refreshed during an active assistant run, the SSE connection is lost. All streaming state is gone and the run is invisible. There is no reconnection or run-state recovery mechanism.

7. **Conversation list UX issues**:
   - The "New Conversation" button is hidden inside the expandable conversation list panel — user must expand the panel just to start a new chat
   - No "Clear all conversations" button for bulk deletion
   - The expand/collapse toggle needs a better visual design
   - The conversation panel placement feels tacked-on

## Goals

### Phase 1 ✅: Layout & Assistant Button

**Completed 2026-06-25.**

Changes:
- `AgentIde.vue`: Added `assistantOpen` state + permanent Chat toggle button in sidebar action row (uses `lightning` icon — no `chat.svg` asset). Renders `AssistantChat` as slide-in overlay (Teleport to body, z-40, max-w-2xl) with backdrop instead of v-if replacement.
- `SystemPulseDashboard.vue`: Removed embedded "Open Assistant" button and unused `open-chat` emit.
- `AssistantChat.vue`: Added `overlay` prop. Moved New Chat button from CollapsiblePanel slot to chat header (always visible). Replaced text chevrons (▸/▾) with SVG icons (chevron-right/chevron-down). Added `btn-header-action` style for header buttons.
- Permanent assistant entry point visible at all times, not buried in dashboard
- Assistant chat as a split-pane or overlay that doesn't replace the entire main view
- Workspace context preserved while chatting
- New Conversation button always visible outside the conversation list panel

### Phase 2: Conversation History
- Sidebar or panel listing past conversations for the active workspace
- Load/resume any past conversation
- Create new conversation
- Delete individual conversations
- Clear all conversations (bulk delete with confirmation)
- Search conversations by snippet
- Better expand/collapse toggle styling
- Panels positioned consistently with the rest of the IDE layout

### Phase 3: Component Refactor
- Extract `AssistantChat.vue` into subcomponents:
  - `ChatMessages.vue` — message rendering loop
  - `ChatInput.vue` — input textarea + send button
  - `ChatSessionList.vue` — conversation history sidebar
  - `ChatBubble.vue` — individual message bubble (user/assistant/result)
- Reduce `AssistantChat.vue` to orchestration only (~150 lines)

### Phase 4: UI Polish
- [x] Error banner for agent failures (stall, timeout, tool failure)
- [-] Tool call inline errors with retry UI (rejected — not currently worth the effort. Error display already shows failures; retry would need agent-level support. Can revisit.)
- [ ] Better streaming progress (spinner, estimated progress)
- [ ] Mobile-responsive: stacked layout with toggleable panels
- [x] Keyboard shortcut: `Ctrl+K` → focus input
- [x] Empty state for new conversation
- [x] "Agent is thinking..." with consistent animation

### Phase 5: Stop Button & Refresh Resilience
- **Stop button** — ensure backend agent cancels when user clicks stop:
  - Investigate if `AbortController.abort()` properly cancels the HTTP request
  - Verify `r.Context()` cancellation propagates to `agent.Execute` during streaming
  - Add explicit check for context cancellation before each tool execution
  - Show "Run stopped" feedback in UI after cancellation
- **Page refresh resilience**:
  - Backend: add `GET /admin/api/conversation/status/{workspace}` to check if a run is active for a session
  - Backend: add `POST /admin/api/conversation/cancel/{workspace}` to explicitly cancel a running agent
  - Frontend: on mount, poll for active runs and offer to reconnect or cancel
  - Frontend: store minimal state in `sessionStorage` to survive refreshes

## Current State

### What works
- SSE streaming with event dedup (see `event-streaming-patterns.md`)
- Three-bubble per-turn layout (see `simple-three-bubble.md`)
- Consolidated streaming bubbles (see `consolidated-streaming-bubbles.md`)
- AgentMessage builder with segment tracking
- Inactivity detection with `paused` timer
- Auto-scroll on new segments

### Backend is ready
- `GET /admin/api/conversation/sessions/{workspace}` — list sessions
- `GET /admin/api/conversation/sessions/{workspace}/{session}` — read session
- `DELETE /admin/api/conversation/sessions/{workspace}/{session}` — delete
- `POST /admin/api/conversation/message` — send message (creates session if new `conversation_id`)
- `AssistantSession` model with `ID`, `Snippet`, `UpdatedAt`
- `SessionBrief` model for list summaries

### Frontend service is ready
- `AssistantService.listSessions(workspaceId)` — returns `SessionBrief[]`
- `AssistantService.getSession(workspaceId, sessionId)` — returns `AssistantSession`
- `AssistantService.deleteSession(workspaceId, sessionId)`
- `AssistantService.sendMessage(workspaceId, payload)` — already passes `conversation_id`

## Implementation Plan

### Phase 1: Assistant Button & Layout

**Problem**: Button is hidden in `SystemPulseDashboard.vue:53` inside a dashboard card. Only visible when `activeMainView === 'dashboard'`.

**Fix**: Add a permanent assistant toggle in the workspace header area of `AgentIde.vue`:
- Workspace selector row gets a `💬 Chat` button/icon
- Clicking it toggles the assistant overlay or split-pane
- The assistant view no longer replaces the dashboard — it opens alongside or as an overlay

**Files**:
- `AgentIde.vue` — add permanent assistant button in header, change layout from `v-if` replacement to overlay/split
- `SystemPulseDashboard.vue` — remove the embedded "Open Assistant" button (deprecated)
- `AssistantChat.vue` — accept `overlay` prop for overlay mode styling

### Phase 2 ✅: Conversation History

**Completed 2026-06-25.**

Changes:
- `backend/router.go`: Added `Patch` method for PATCH route support.
- `backend/assistant_handlers.go`: Added `RenameSession` handler (reads session, updates `Metadata["title"]`, writes back).
- `backend/bootstrap.go`: Registered `PATCH /admin/api/conversation/sessions/{workspace}/{session}`.
- `backend/assistant_handlers_test.go`: Added 3 tests — successful rename, 404 on missing session, 400 on empty title.
- `frontend/ChatSessionList.vue`: New component extracted from AssistantChat.vue — session list with inline rename, new chat, delete, hover actions.
- `frontend/assistantService.ts`: Added `renameSession` method (PATCH).
- `frontend/AssistantChat.vue`: Replaced CollapsiblePanel with ChatSessionList component. Wired rename events. Removed unused CSS and imports (CollapsiblePanel, formatTime, session list styles).

Notes:
- `useConversations.ts` composable was skipped — `useAssistant.ts` already manages session state (fetch, load, new, delete). Adding a separate composable would create duplicate `sessions` refs.
- Session files ARE deleted from disk when a conversation is deleted (`os.Remove` in `DeleteSession`).

### Phase 3 ✅: Component Refactor

**Completed 2026-06-25.**

Extracted 3 new subcomponents:
- `ChatBubble.vue` (~120 lines) — Single assistant message bubble with header, work section (segments/reasoning), thinking dots, and result section. Contains all segment rendering, tool call display, and markdown rendering.
- `ChatMessages.vue` (~90 lines) — Message container with empty state, turn loop (UserMessage + ChatBubble per turn), scroll management, segment auto-scroll, and interrupted bar.
- `ChatInput.vue` (~60 lines) — Textarea with Enter-to-send, loading glow animation, stop/send buttons.

`AssistantChat.vue` reduced from 675→210 lines. Now acts as orchestrator: composes composables, defines event handlers, routes data to subcomponents. Contains only sidebar state, work/segment collapse state, session handlers, and the loading watcher.

### Phase 4: UI Polish

| Feature | File | Description |
|---------|------|-------------|
| Error banner | `AssistantChat.vue` / `ChatMessages.vue` | Show agent errors inline as a dismissible banner |
| Tool call error | `ChatBubble.vue` | Show failed tool calls with red indicator + error message |
| Loading states | `ChatInput.vue` | Disabled input + spinner during agent execution |
| Empty state | `ChatMessages.vue` | "Start a conversation" placeholder when no messages |
| Mobile layout | `AgentIde.vue` + `AssistantChat.vue` | Full-screen chat on mobile, toggleable panels |
| Keyboard shortcut | `ChatInput.vue` | `Ctrl+K` / `Cmd+K` focuses the input |
| Delete conversation | `ChatSessionList.vue` | Confirm dialog → delete API → refresh list |

### Phase 5: Stop Button & Refresh Resilience

**Problem 5a — Stop button broken**: The cancel function calls `AbortController.abort()` and `sse.disconnect()`, which closes the HTTP connection. But the backend agent continues running because:
- The Go HTTP server does not always cancel `r.Context()` when the client disconnects during a streaming response
- The `processStream` goroutine and tool execution may not check context before every operation

**Fix — Backend**:
- In `handleAssistant`, use a dedicated `context.WithCancel` derived from the HTTP request context
- Expose cancellation via a new endpoint (`POST /admin/api/conversation/cancel/{workspace}/{session}`)
- In `processStream`, check `ctx.Err()` before every stream chunk append
- In `processToolCalls`, check `ctx.Err()` before each tool execution

**Fix — Frontend**:
- Change cancel to call `POST .../cancel` BEFORE closing the SSE connection
- Show a "Stopping..." indicator while waiting for cancellation confirmation
- On successful cancel, show "Run stopped by user" banner

**Problem 5b — Page refresh loses active run**:

**Fix — Backend**:
- `GET /admin/api/conversation/status/{workspace}` — returns `{active: bool, session_id: string}` if a run is in progress
- `POST /admin/api/conversation/cancel/{workspace}/{session}` — cancels the agent context for a specific session

**Fix — Frontend**:
- On `AssistantChat` mount, call the status endpoint. If active:
  - Show "A run is in progress — reconnecting..." banner
  - Re-establish SSE and replay recent events from the event buffer
  - Or offer "Stop run" button if reconnect fails
- Store `conversation_id` in `sessionStorage` so the correct session is restored after refresh

**Files**:
| File | Action |
|------|--------|
| `backend/internal/transport/http/assistant_handlers.go` | Add RunStatus and CancelRun handlers |
| `backend/internal/app/bootstrap.go` | Register new routes |
| `backend/internal/core/assistant/stream.go` | Add context check in `processStream` |
| `backend/internal/core/assistant/tool_exec.go` | Add context check in `processToolCalls` |
| `frontend/src/composables/useAssistant.ts` | Change cancel to call backend, add reconnect logic |
| `frontend/src/services/assistantService.ts` | Add `cancelRun` and `getRunStatus` methods |

1. **Overlay, not replacement** — The assistant chat opens as a sliding overlay (right side) on desktop. The dashboard remains visible behind it. This keeps workspace context (active runs, metrics) while chatting. On mobile, the overlay goes full-screen.

2. **Sessions auto-save** — Every message persists immediately via the existing `WriteSession` call in `handleAssistant`. No explicit "save" action needed. Session list refreshes on load and after each message exchange. Running sessions show a green ● indicator that updates via workspace-level SSE lifecycle events (`session_started`, `session_progress`, `session_completed`).

3. **Conversation ID as route param** — `?conversation=conv_20260625140500` in the URL so deep-linking and browser back/forward work with the session history.

4. **No backend changes for the basic flow** — listing, loading, deleting sessions are already implemented. Only `rename` needs a new endpoint (PATCH).

## File Change Checklist

| File | Phase | Action |
|------|-------|--------|
| `frontend/src/components/AgentIde/AgentIde.vue` | 1 | Add permanent chat button, overlay layout |
| `frontend/src/components/AgentIde/system/SystemPulseDashboard.vue` | 1 | Remove embedded "Open Assistant" button |
| `frontend/src/components/AgentIde/assistant/AssistantChat.vue` | 1,3 | Refactor to orchestrator, add overlay prop |
| `frontend/src/components/AgentIde/assistant/ChatMessages.vue` | 3 | New — message rendering |
| `frontend/src/components/AgentIde/assistant/ChatBubble.vue` | 3 | New — single bubble |
| `frontend/src/components/AgentIde/assistant/ChatInput.vue` | 3,4 | New — input bar |
| `frontend/src/components/AgentIde/assistant/ChatSessionList.vue` | 2 | New — session history sidebar |
| `frontend/src/composables/useConversations.ts` | 2 | New — session state |
| `frontend/src/composables/useAssistant.ts` | 2,5 | Modify — session load + cancel + reconnect |
| `frontend/src/types/assistant.ts` | 2 | Add session types + running badge |
| `frontend/src/services/assistantService.ts` | 2,5 | Add rename, cancelRun, getRunStatus, clearAllSessions |
| `backend/internal/transport/http/assistant_handlers.go` | 2,5 | Add RenameSession, RunStatus, CancelRun handlers, session lifecycle events |
| `backend/internal/core/assistant/agent_events.go` | 5 | Add PhaseSessionStarted/Progress/Completed constants |
| `backend/internal/app/bootstrap.go` | 2,5 | Register new routes |
| `backend/internal/core/assistant/stream.go` | 5 | Add context check in processStream |
| `backend/internal/core/assistant/tool_exec.go` | 5 | Add context check in processToolCalls |

## Risks

- **Session list performance** — `ListSessions` reads all session files to build snippets. With 100+ sessions per workspace, this could be slow. Mitigation: paginate (backend already has `SessionBrief`, add limit/offset to handler).
- **SSE reconnection on session switch** — When loading a previous session, the SSE connection must be closed and re-established. `useAssistant.ts` already handles `sse.disconnect()` in cleanup — a `switchSession()` method needs to call this.
- **Overlay z-index conflicts** — The overlay must sit above the dashboard but below any modals/toasts. Use z-40 (below toast at z-50).
- **Context cancellation during streaming** — Go's `http.Server` does not always propagate client disconnection to `r.Context()` during active response writes. The cancel endpoint approach avoids relying on automatic context propagation.
- **Run state recovery after refresh** — The event buffer on the backend (`EventSink`) holds recent events. Reconnecting SSE and replaying the buffer should restore most state, but in-flight tool executions may be lost. A "Recovering state..." indicator covers the gap.
