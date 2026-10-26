# Assistant UI Chat Architecture — Animation, GPU and Auto-scroll

Reference for the [`assistant-ui-chat`](../SKILL.md) skill, loaded on demand. You are adding animation or changing scroll behavior (check GPU cost first).

## Animation States & GPU / Compositing Notes

| Element | Loading | Not Loading |
|---------|---------|-------------|
| Arc-orbit loader (message input only; the assistant turn has no box to ring since 2026-10-01) | Static accent ring **only while `.is-active`** (thinking-gaps) | Inert (`opacity: 0`) |
| Thinking-gap dots | `thinking-pulse` opacity animation **only while visible** (`.bubble-paused:not(.bubble-paused--hidden)`) | Inert (`visibility: hidden`, animation stopped) |
| Generating pulse-dots | Only rendered while `phase === 'generating'` | Not in DOM |

**GPU / compositing rules (see the consolidated `docs/audits/gpu-performance-audit.md` and
forward-looking `docs/PLANS/gpu-performance.md`):**
- **Never leave a CSS animation running on a hidden/invisible element.** CSS animations run even at `opacity: 0` / `visibility: hidden`. Every animation is gated to its visible state — the arc-orbit animation lives on `.arc-orbit-loader.is-active::before`, and the thinking-gap dots animate only under `.bubble-paused:not(.bubble-paused--hidden)`.
- **`content-visibility: auto` on non-live turns** (`.message-wrapper--virtualized`, applied when `!(loading && isLastTurn)`) skips paint/composite of offscreen history so a growing session doesn't re-composite the whole pane. `contain-intrinsic-size: auto 120px` keeps scroll heights stable.
- **Avoid `backdrop-blur` on always-visible elements** — each layer is a constant macOS compositing cost. Always-visible chrome (the sidebar rail, the top strip) uses solid token backgrounds (`bg-canvas`) instead.
- The live reasoning box renders via `MarkdownViewer`, throttled to a 100ms flush and 250ms outer
  auto-scroll; the inset keeps its 320px cap + internal scroll and scrolls only when it overflows
  (the outer `scrollHeight` read is throttled to ≤4x/sec). The bubble uses `z-index: 0`, not
  `isolation: isolate`. The remaining GPU cost during a run has no known root cause: profile with
  Chrome DevTools Performance before changing more. **The arc-orbit loader has NO animation**: its
  per-frame `--arc-angle` gradient repaint was the dominant during-run GPU cost (~11 points), and
  a transform-based spin would distort the rounded-rectangle ring (only a circle rotates without
  changing shape), so it is a static accent ring and the thinking dots carry the affordance.
  Do NOT re-add an animated conic-gradient loader without a compositor-safe (circular) design
  and a re-measurement.

### Empty inset suppression

The `.bubble-inset` (reasoning/tool panel) is shown via **`v-show` (kept mounted, `display:none` when hidden)** — never `v-if`. `insetVisible` in `ChatBubble.vue` is gated by `insetHasContent` (plus the waiting placeholder), so during the initial wait (`thinking` with no streamed text yet) it is hidden and never paints an empty rounded panel. Keeping it mounted (not `v-if`) makes **collapse/expand during streaming flicker-free**: a `v-if` inset unmounts on collapse and re-mounts + re-parses markdown on expand, which flashes while a stream is live (user-reported 2026-08-28). The trade-off is that a hidden inset still re-renders its live content on each flush (bounded CPU by the adaptive cadence; zero paint while `display:none`).

**Waiting-on-model placeholder.** One exception: `showWaitingPlaceholder` renders a generic "Waiting on the model…" row inside the inset while the last live turn is in `thinking` with nothing streamed yet. It is provider/model-agnostic and reuses the existing phase state (no new heartbeat or event). It disappears as soon as any reasoning/content/segment/`notice`/`generating` state arrives — the `insetHasContent` gate then takes over. This gives the user feedback during a long provider TTFB or a hanging connection before the first upstream-retry notice appears.

## Auto-scroll Behavior

- `watch(segments.length)` scrolls the new segment into view (`behavior: instant`, `block: nearest`)
- **There is NO `watch(messages)` that calls `scrollToBottom()`**. This was removed because it fires on EVERY stream chunk (every 10-50ms), constantly recalculating scroll position and causing visible flickering at the bottom edge of the bubble.
- User has scrolled up: `useAutoScroll` pauses following immediately. If new content arrives and the user stops scrolling for `idleMs` (2s), it resumes and snaps to the bottom; scrolling back to the bottom re-arms it right away. A finished/static conversation stays paused.
