# Assistant UI Chat Patterns

**Source docs:** SPEC-003, `docs/PLANS/assistant-ui/`, `docs/skills/event-streaming-patterns.md`

---

## Architecture Overview

```
AssistantChat.vue
  ├── useResponsiveLayout(breakpoint)    → isMobile
  ├── useAssistant (singleton composable) → messages, sessions, currentSessionId
  ├── ChatSessionList.vue                 → sidebar / drawer (3 states)
  ├── ChatMessages.vue                    → rendered messages + tool blocks
  └── ChatInput.vue                      → text input + send/cancel

SSE: useAssistantSSE → EventSource → /admin/api/dispatcher/workspaces/{ws}/live
```

**Critical invariant:** `useAssistant` is a **module-level singleton**. State is shared across all components that import it. Never create a local instance.

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

## SSE Event Flow

```
Agent publishes event via publishObs
  → EventBus.Publish(workspaceID, event)
    → fanned out to all subscribers (EventSource connections)
      → frontend useAssistantSSE receives JSON
        → updates messages ref
          → ChatMessages re-renders
```

**Key files:**
- `frontend/src/composables/assistant/useAssistant.ts` — singleton state
- `frontend/src/composables/assistant/useAssistantSSE.ts` — EventSource lifecycle
- `backend/internal/core/automation/broadcast.go` — EventBus

**Lifecycle:**
1. User sends message → `POST /admin/api/conversation/message`
2. Before the request completes, frontend connects SSE to `/admin/api/dispatcher/workspaces/{ws}/live`
3. Backend agent loop runs, publishing events via `publishObs`
4. SSE streams `agent_update` events to frontend
5. Frontend appends each event to `messages` ref

**Common mistakes:**
- Calling `POST /message` without first connecting SSE — the agent runs to completion but no events stream
- Modifying `messages` directly instead of through `useAssistant` — breaks reactivity (singleton ref)
- Disconnecting SSE early — component unmount must not happen before agent completes

---

## Tool Message Rendering Pattern

Messages from the assistant contain tool calls and results. These are rendered through shared components:

| Event type | Component | Path |
|-----------|-----------|------|
| `tool_call` | `<ToolCallBlock>` | `components/common/chat/ToolCallBlock.vue` |
| `tool_result` | `<ToolResultBlock>` | `components/common/chat/ToolResultBlock.vue` |
| `lifecycle` | `<LifecycleMessage>` | `components/common/chat/LifecycleMessage.vue` |
| `guardrail_violation` | (inline in TerminalOutput) | — |
| `guardrail_blocked` | `<GuardrailBanner>` | `components/common/chat/GuardrailBanner.vue` |

All tool-related UI components moved to `components/common/chat/` during the refactor. Import path:
```typescript
import ToolCallBlock from "../../components/common/chat/ToolCallBlock.vue"
```

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
