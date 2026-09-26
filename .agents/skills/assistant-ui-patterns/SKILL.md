---
name: assistant-ui-patterns
description: "AgentIde UI shell patterns: chat shell composition, sidebar/drawer states, mobile breakpoints, the shared chat renderer, and UI gotchas. Use when working on AgentIde layout/components."
when_to_use: "Editing AgentIde UI layout, sidebar/drawer states, mobile breakpoints, or the shared ChatMessages renderer."
status: reference
last_reviewed: 2026-07-11
---

# Assistant UI Chat Patterns

**Source docs:** SPEC-003, `docs/PLANS/assistant-ui/`, `.agents/skills/event-streaming-patterns/SKILL.md`

---

## Architecture Overview

```
AssistantChat.vue
  ├── useResponsiveLayout(breakpoint)    → isMobile
  ├── useAssistant (singleton composable) → messages, sessions, currentSessionId
  ├── ChatSessionList.vue                 → sidebar / drawer (3 states)
  ├── ChatMessages.vue                    → rendered messages + tool blocks (mode: 'chat' | 'automation')
  └── ChatInput.vue                      → text input + send/cancel

AutomationDetails.vue
  └── useLiveConsole → useMessageBuilder → ChatMessages (mode="automation")

SSE: useAssistantSSE → EventSource → /admin/api/dispatcher/workspaces/{ws}/live?channel=assistant
SSE: useLiveConsole  → EventSource → /admin/api/dispatcher/workspaces/{ws}/live?channel=automation
```

**Critical invariant:** `useAssistant` is a **module-level singleton**. State is shared across all components that import it. Never create a local instance.

**Renderer unification:** assistant chat and automation runs share ONE renderer — `ChatMessages.vue` — and ONE event→message consumer, `useMessageBuilder` (`utils/message/messageBuilder.ts`). `useLiveConsole` feeds the builder directly; the old `automationEventsToMessages` bridge and the `LiveConsole`/`TerminalOutput` terminal stack were deleted (the bridge concatenated cumulative re-emits into a cascade) — do NOT reintroduce either. In `automation` mode `ChatMessages` hides the welcome card and `UserMessage` bubble and passes a static `phase`. New automation event types are handled in the builder, not in a forked mapping.

---

## Sidebar States (ChatSessionList.vue)

The sidebar has 3 visual states driven by the parent:

| State | Width | Desktop | Mobile |
|-------|-------|---------|--------|
| Collapsed (default) | 0px / hidden | No visible bar | Drawer closed |
| Hovered (not supported) | — | Do NOT use mouse-position auto-expand | — |
| Toggled | 260px / 85vw | Manual `sidebarOpen` ref | Manual `sidebarOpen` ref |

Current implementation uses a single `sidebarOpen` boolean. Desktop aside transitions `width: 0 ↔ 260px`. Mobile drawer uses `Transition name="drawer"` with `transform: translateX(-100%)`.

**Do NOT add mouseenter/mouseleave auto-expand.** The user explicitly requested manual-only toggle.

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
| `< 640px` | AssistantChat.vue | Mobile sidebar drawer |
| `< 1024px` | AgentIde.vue | Mobile panel tabs |

Always match the breakpoint to the component's context. The assistant chat uses 640px because the chat panel is narrower than the full IDE view.

---

## Common Gotchas

1. **`useResponsiveLayout` registers `onMounted`/`onUnmounted`** — These hooks are scoped to the calling component. If the parent unmounts, the resize listener is cleaned up. Safe to use in multiple components simultaneously (each gets its own `isMobile` ref).

2. **Sidebar transitions on resize** — When resizing from desktop to mobile with the sidebar open, the `<aside>` hides and the `<Transition>` drawer appears instantly. The drawer has no enter animation in this case because the component mounts already open. This is acceptable.

3. **`Transition` requires `v-if`** — The drawer uses `v-if="isMobile && sidebarOpen"`. Using `v-show` with `Transition` will not animate. Always use `v-if` with `Transition`.

4. **Backdrop z-index** — Backdrop should be `z-30`, drawer `z-40`. This keeps the drawer above the backdrop.

5. **`Icon` component icons exist as SVGs in `src/assets/svg/`** — Add a new SVG file there, then reference it via `<Icon name="filename" size="..." />`. No manual registration needed for simple SVGs — the dynamic import system auto-discovers files in that directory.

6. **Never mutate `useAssistant` state directly** — Always call methods like `sendMessage()`, `loadSession()`, `newSession()`. Direct mutation of `messages` ref will be overwritten by the composable on the next state update.
