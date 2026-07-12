---
status: superseded
last_reviewed: 2026-07-11
---

> **Archived:** No status, no date — archived.

# Running-indicator for webhook-triggered assistant sessions

> **Status: ✅ Completed** — implemented and frontend build (`eslint` + `vue-tsc` + `vite build`) passes clean.

## Problem

When a connector webhook triggers an assistant prompt, the frontend already knows instantly (SSE `session_started` → `running:true`), but there is no always-visible signal. The user must open the chat and dig into history. We add two surfaces plus webhook-source distinction.

## Key facts

- `useAssistant()` is a **module-level singleton** (`composables/assistant/useAssistant.ts:17`). `sessions[]` is a shared ref with `running: boolean`, updated live by `applySessionUpdate` (`useAssistant.ts:295-341`) from SSE lifecycle events. Webhook-triggered sessions flow through the same path (`useAssistant.ts:300-326`) — **no backend change needed**.
- Webhook sessions are identifiable by their `source` field (`webhook-telegram` / `webhook`), supplied by the backend on both the session list and the `session_started` SSE event. Manual chat sends have `source: 'manual'`. The frontend maps `source` to icon/label via `utils/assistant/source.ts` and never parses the session ID. The session key format is `wb_{platformType}_{chatID}_{timestamp}` (per-message fresh sessions).
- `sessions` is scoped to the active workspace (SSE early-returns on mismatched `workspace_id`; `fetchSessions(workspaceId)` populates it), so it's safe to display for `selectedWorkspace`.
- The "chat button" lives in two places: sidebar `WorkspaceExplorer.vue:85` (desktop, Explorer tab only) and `MobileTabBar.vue` Chat tab (mobile, always visible).
- The **pulse/monitor panel** (`RightPane.vue`) is always rendered on desktop and already lists automation runs via `WorkspaceActivity` — the most reliable always-on "what's running" surface.

## Option A — Animated chat button

### `WorkspaceExplorer.vue`
- Add props: `chatRunning?: boolean`, `runningAssistantCount?: number`, `webhookRunning?: boolean`.
- Chat button (`:85-88`): when `chatRunning`, append a pulsing dot (`animate-pulse`) + count badge. When `webhookRunning`, show 📡 badge (or 📞 for telegram).
- Keep `@click="emit('open-chat')"` (→ `toggleAssistant` → opens assistant view).

### `MobileTabBar.vue`
- Add same props (`chatRunning`, `runningAssistantCount`, `webhookRunning`).
- Chat tab (`:39-42`): render pulse dot + count when `chatRunning`.

### `AgentIde.vue` wiring
- `import { useAssistant }` and call it to reach `sessions`.
- Pass `:chat-running` / `:running-assistant-count` / `:webhook-running` to both components.

## Option B — Assistant Activity section in the pulse panel

### New component `AssistantActivity.vue` (`components/AgentIde/assistant/`)
Mirror `WorkspaceActivity` styling. Props: `sessions: SessionBrief[]` (running only), `loading?: boolean`. Per row:
- source badge 📡/📞 (webhook) vs none (manual) via shared util,
- `session.snippet` (truncated),
- green `.pulse-dot` (`animate-pulse`),
- elapsed time (reuse `useElapsedTimer` per row, or relative-time),
- clickable → `emit('select-session', session.id)`.
Empty state: "No assistant runs active".

### `RightPane.vue`
- Add prop `assistantSessions: SessionBrief[]` and emit `select-assistant-session`.
- Insert `<AssistantActivity :sessions="assistantSessions" @select-session="(id)=>emit('select-assistant-session', id)" />` (e.g. between Actions card and `WorkspaceActivity`).

### `AgentIde.vue` wiring
- Compute `runningAssistantSessions = sessions.value.filter(s => s.running)` for the active workspace.
- Pass `:assistant-sessions="runningAssistantSessions"` to `RightPane`.
- Handle `@select-assistant-session`: call `loadSession(selectedWorkspace, id)`, set `currentSessionId`, switch to assistant view (`workspaceMiddleTab='chat'` → `activeMainView='assistant'`).

## Shared source util (enables "distinguish webhook")

`frontend/src/utils/assistant/source.ts` (backend-driven):
- `sourceIcon(source?: string): string | null` (radio / radio / null)
- `sourceLabel(source?: string): string` ('Webhook — Telegram' / 'Webhook' / '')
- `sourceOrder: SessionSource[]` — display grouping order
- `sessionSource(id)` — defensive fallback only; backend supplies `source` on list + SSE

The backend is the authority: `models.SessionSource(id)` derives the value, shipped on `SessionBrief.source` and the `session_started` SSE payload.

## `useAssistant.ts` additions

Add module-level computeds, returned from `useAssistant()`:
- `runningSessions` = `sessions.value.filter(s => s.running)`
- `anyRunningAssistant` = `runningSessions.length > 0`
- `webhookRunningSessions` = `runningSessions.filter(s => s.source?.startsWith('webhook'))` (source comes from backend; no ID parsing)

## Files touched

| File | Change |
|---|---|
| `composables/assistant/useAssistant.ts` | add `runningSessions` / `anyRunningAssistant` / `webhookRunningSessions` computeds + return |
| `utils/assistant/source.ts` | **new** — `sessionSource`, `sourceIcon` |
| `components/AgentIde/workspace/WorkspaceExplorer.vue` | chat button pulse dot + count + webhook badge; new props |
| `components/AgentIde/common/MobileTabBar.vue` | Chat tab pulse dot + count + webhook badge; new props |
| `components/AgentIde/assistant/AssistantActivity.vue` | **new** — running-sessions card |
| `components/AgentIde/common/RightPane.vue` | add `AssistantActivity` + prop + emit |
| `components/AgentIde/AgentIde.vue` | wire computeds, props, `openAssistantSession` handler |
| `components/AgentIde/assistant/ChatSessionList.vue` | (optional) use shared `sourceIcon` |

## Behavior summary

- Webhook fires → SSE `session_started` → chat button pulses with 📡 + count, pulse panel shows "Assistant Activity" with pulse dot + elapsed timer.
- Click chat button → opens assistant view (running session is at top of `ChatSessionList`). Click pulse-panel row → opens that specific session.
- `session_completed` → `running:false` → all indicators clear automatically.

## Verification

- `cd frontend && npm run build` — TS enforces new props/icons; catches missing entries.
- Manual trigger: confirm both indicators appear; webhook badge only on `wb_` sessions; click-through opens session; auto-clear on completion.
- No Go changes required; cyclomatic-complexity rule irrelevant here.
