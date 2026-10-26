---
name: assistant-ui-patterns
description: "AgentIde UI shell patterns: chat shell composition, sidebar/drawer states, mobile breakpoints, the shared chat renderer, and UI gotchas. Use when working on AgentIde layout/components."
metadata:
  last_reviewed: "2026-10-15"
---

# Assistant UI Chat Patterns

**Source docs:** SPEC-003, `docs/PLANS/assistant-ui/`, `.agents/skills/event-streaming-patterns/SKILL.md`

---

## Architecture Overview

```
AssistantChat.vue
  ├── useResponsiveLayout()              → breakpoint ('base' = below sm → drawer)
  ├── useAssistant (singleton composable) → messages, sessions, currentSessionId
  ├── ChatSessionList.vue                 → column or drawer (see Sidebar States)
  ├── ChatMessages.vue                    → rendered messages + tool blocks (mode: 'chat' | 'automation')
  └── ChatInput.vue                      → text input + send/cancel

AutomationDetails.vue
  └── useLiveConsole → useMessageBuilder → ChatMessages (mode="automation")

SSE: useAssistantSSE → EventSource → /admin/api/dispatcher/workspaces/{ws}/live?channel=assistant
SSE: useLiveConsole  → EventSource → /admin/api/dispatcher/workspaces/{ws}/live?channel=automation&automation={name}  (that automation's events only)
```

**Critical invariant:** `useAssistant` is a **module-level singleton**. State is shared across all components that import it. Never create a local instance.

**Renderer unification:** assistant chat and automation runs share ONE renderer — `ChatMessages.vue` — and ONE event→message consumer, `useMessageBuilder` (`utils/message/messageBuilder.ts`). `useLiveConsole` feeds the builder directly; the old `automationEventsToMessages` bridge and the `LiveConsole`/`TerminalOutput` terminal stack were deleted (the bridge concatenated cumulative re-emits into a cascade) — do NOT reintroduce either. In `automation` mode `ChatMessages` hides the welcome card and `UserMessage` bubble and passes a static `phase`. New automation event types are handled in the builder, not in a forked mapping.

---

## Sidebar States (ChatSessionList.vue)

The conversation list has three placements, chosen by `AssistantChat`:

| Where | Placement | Open state |
|-------|-----------|------------|
| Assistant page, roomy screen | a 260px column beside the chat | persisted per browser (`assistant-history-open`, `usePersistedState`); closed until opened |
| Assistant page on a phone (`base`), and the **ask panel** (`variant="panel"`) | a drawer sliding over the chat (`fixed` on a phone, `absolute` inside the panel) | local, always starts closed |

`compact = variant === 'panel' || isMobile` selects the second row. History and New are one header control (`.header-cap`: toggle | New) while the list is closed; with a column open the list holds both (a "New chat" footer row, a "Hide conversations" header button) and the cap is hidden. The list is toggled manually only.

**Do NOT add mouseenter/mouseleave auto-expand.** The user explicitly requested manual-only toggle.

## The ask panel and the one-chat rule

`AssistantChat variant="panel"` is the chat in the shell's ask panel (`components/layout/AskPanel.vue`) beside any page. Only **one** `AssistantChat` may be mounted: the panel never opens on the Assistant route, and the Workspaces view closes its chat when deactivated (kept-alive otherwise). Details and the test that guards it: `docs/architecture.md` pitfall 49. The panel resumes `openConversationId` (a read-only export of `useAssistant.ts` that creates no connection).

### List contents (retro redesign, 2026-09-30)

`ChatSessionList` groups via `utils/assistant/sessionSections.ts` (pure): Pinned ·
Today · Yesterday · This week · Older, then webhook conversations in a foldable
folder. Search is client-side over titles. Pins are per workspace in
`composables/assistant/usePinnedSessions.ts` (a `usePersistedState` singleton);
the chat calls `forget()` on delete. Row actions must stay reachable by keyboard
(shown on hover **or** focus-within, always on mobile) and be named after the
conversation. Destructive actions go through `ConfirmDialog`, never `window.confirm`.
The chat header's status tag is derived from real state (approval › model busy ›
reconnecting › working › ready) — never a fixed "online" label.

### CSS Transitions

```css
.chat-sidebar {
  transition: width 200ms ease-out;
  width: 0;                        /* collapsed */
  overflow: hidden;
}
.chat-sidebar--open { width: 260px; }

/* Mobile drawer */
.drawer-enter-active  { transition: transform 250ms cubic-bezier(0.4, 0, 0.2, 1); }
.drawer-leave-active  { transition: transform 200ms cubic-bezier(0.4, 0, 0.2, 1); }
.drawer-enter-from,
.drawer-leave-to      { transform: translateX(-100%); }

.fade-enter-active,
.fade-leave-active    { transition: opacity 200ms ease; }
.fade-enter-from,
.fade-leave-to        { opacity: 0; }
```

---

## SSE wiring (UI side)

The event plumbing (observer → EventBus → `/live` SSE → dedup), the event-type/phase tables, and the guardrail flow are owned by [`event-streaming-patterns`](../event-streaming-patterns/SKILL.md); session phases by [`lifecycle-events`](../lifecycle-events/SKILL.md). Only the UI-side wiring belongs here:

**Key files:** `composables/assistant/useAssistant.ts` (singleton state), `useAssistantSSE.ts` (EventSource lifecycle), `backend/internal/core/eventbus/` (bus).

**UI integration rules:**
- Connect SSE **before** the `POST /admin/api/conversation/message`; otherwise the agent runs to completion with no streamed events.
- Never mutate `messages` directly — call the composable's methods; the singleton ref is overwritten on the next update.
- Do not disconnect before the agent completes (component unmount included).

**Chat + automation share ONE event→message consumer** — `useMessageBuilder`. Chat uses `finalizeOn: 'explicit'`; automation passes `finalizeOn: 'lifecycle'` + a synthetic run-header message. Customize automation chrome via the `ChatMessages` `mode` prop + `#run-header` slot, never a forked mapping.

## Tool message rendering

Tool/reasoning segments render inside `ChatBubble.vue` via `ToolCallSegment.vue` (`components/AgentIde/assistant/`); guardrail approval is `components/common/chat/GuardrailBanner.vue`. There is no separate `ToolCallBlock`/`ToolResultBlock`/`LifecycleMessage` component — the event-type → rendering map is in [`event-streaming-patterns`](../event-streaming-patterns/SKILL.md).

---

## Mobile Breakpoints

| Breakpoint | Used By | Behavior |
|-----------|---------|----------|
| `< 640px` (`base`) | AssistantChat.vue | Conversation list as a drawer; the message box opens a full-screen editor for a long draft |
| `< 1024px` (below `lg`) | App shell, `views/WorkspacesView.vue`, `views/AutomationsView.vue` | Sidebar is replaced by the bottom tab bar (`BottomTabs.vue`); one pane at a time; the assistant fills the screen between the top strip and the bar |

Breakpoints are named (`sm | md | lg | xl | 2xl`) and come from `src/theme/breakpoints.ts`, which also feeds Tailwind's `screens` — never hardcode a pixel width. The assistant chat switches at `sm` because the chat panel is narrower than the full view.

---

## Common Gotchas

1. **`useResponsiveLayout` registers `onMounted`/`onUnmounted`** — It listens to one `matchMedia` query per breakpoint, scoped to the calling component and removed when it unmounts. Safe to use in multiple components simultaneously (each gets its own `breakpoint` / `isMobile` refs).

2. **Sidebar transitions on resize** — When resizing from desktop to mobile with the sidebar open, the `<aside>` hides and the `<Transition>` drawer appears instantly. The drawer has no enter animation in this case because the component mounts already open. This is acceptable.

3. **`Transition` requires `v-if`** — The drawer uses `v-if="isMobile && sidebarOpen"`. Using `v-show` with `Transition` will not animate. Always use `v-if` with `Transition`.

4. **Backdrop z-index** — Backdrop should be `z-30`, drawer `z-40`. This keeps the drawer above the backdrop.

5. **`Icon` component icons exist as SVGs in `src/assets/svg/`** — Add a new SVG file there, then reference it via `<Icon name="filename" size="..." />`. No manual registration needed for simple SVGs — the dynamic import system auto-discovers files in that directory.

6. **Never mutate `useAssistant` state directly** — Always call methods like `sendMessage()`, `loadSession()`, `newSession()`. Direct mutation of `messages` ref will be overwritten by the composable on the next state update.
