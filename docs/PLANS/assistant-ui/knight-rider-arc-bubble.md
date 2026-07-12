---
status: active
last_reviewed: 2026-07-11
---

# Knight rider arc on assistant bubble

**Date**: 2026-06-26
**Subsystem**: assistant-ui
**File**: `frontend/src/components/AgentIde/assistant/ChatBubble.vue`

## What

Added a glowing arc that orbits the assistant message bubble while it is the latest turn and still loading. Roughly 60° bright indigo (`rgb(129, 140, 248)`) trailing into transparency, full revolution every 1.8s. Pure CSS — `conic-gradient` + `mask-composite: exclude` + `@property --arc-angle` for smooth angle interpolation. Fades in on `is-loading`, fades out 0.25s after.

## Why

Visual cue that the agent is alive and producing, beyond the existing 3-dot "thinking" line. Mirrors the Gemini/Google AI Mode loading affordance the user referenced. Tells the eye "this bubble is the one currently being filled" when multiple turns are on screen.

## Decisions

- **Conic gradient over SVG/SVG-animate**: GPU-friendly, no JS, no resize observer — bubble height grows during streaming, gradient tracks border-radius automatically.
- **`@property --arc-angle`**: lets the browser interpolate `<angle>` as a typed value; without it, CSS falls back to discrete jumps.
- **`z-index: -1` + `isolation: isolate` on the arc layer**: paints behind bubble content so streamed text is never tinted.
- **`pointer-events: none` + `aria-hidden="true"`**: decorative; no AT noise, no click capture.
- **Trigger matches existing `is-loading`**: `loading && isLastTurn` — only one bubble glows at a time.
- **`prefers-reduced-motion` fallback**: solid ring instead of motion. A11y.
- **Color matches existing `dot-pulse` keyframe in same file** (`rgb(129, 140, 248)`): visual coherence with the existing header loading dot.

## Tradeoffs

- Conic gradient on rounded rects clips slightly at corners. Acceptable — same artifact Google's own implementation has.
- Safari < 16.4 lacks `@property` — angle jumps in steps. Acceptable, low user share.
- No behavioral contract change → no SPEC file updates needed.

## Verification

- `npm run build` (vue-tsc + vite) clean.
- Manual: trigger an assistant turn, confirm arc orbits last bubble only, stops on completion.
- Manual: enable OS reduced-motion, confirm static ring fallback.

---

# Refactor: extract `ArcOrbitLoader`, apply to input, remove header dot

**Date**: 2026-06-26
**Subsystem**: assistant-ui
**Files**:
- `frontend/src/components/common/ArcOrbitLoader.vue` (new)
- `frontend/src/components/AgentIde/assistant/ChatBubble.vue`
- `frontend/src/components/AgentIde/assistant/ChatInput.vue`

## What

Promoted the inline arc orbit CSS into a reusable `<ArcOrbitLoader>` component, then applied it to both the assistant bubble and the chat input. Removed the redundant pulsing dot in the assistant header. Replaced the input's `input-glow` box-shadow pulse with the same arc orbit.

## Why

- Single source of truth for the loading affordance — no drift between bubble and input.
- Header dot + arc orbit was redundant; one signal is cleaner.
- `input-glow` (pulsing box-shadow on a textarea) felt inconsistent with the bubble's animated ring; same animation across both surfaces is calmer and more cohesive.

## Decisions

- **Component over composable**: DOM + CSS live together; no need for a runtime CSS-injection utility.
- **Props via `v-bind()` in scoped CSS**: Vue 3.2+ feature. Lets the consumer override `radius`, `thickness`, `duration`, `color`, `glow` per use site without a class explosion. Compile-time string substitution — zero runtime cost.
- **Bubble radius = `1rem 1rem 1rem 0.125rem`**: matches existing `rounded-2xl` (1rem) and `rounded-tl-sm` (0.125rem) tokens. Preserves the asymmetric "tail" corner that signals an assistant message.
- **Input radius = `0.75rem`**: matches existing `rounded-xl` textarea.
- **Removed `z-index: -1` trick**: switched to `z-index: 1` on arc with `isolation: isolate` on the host (bubble / input-wrap) and `z-index: 0` on all other children. Simpler, no risk of arc being painted behind the host's own background.
- **Removed header dot rule and `@keyframes dot-pulse`**: no longer referenced. Net CSS delta is negative (~50 lines out of the bubble file).
- **Removed `input-glow` keyframe and `is-loading` class on textarea**: replaced by arc.

## Tradeoffs

- `v-bind()` in CSS requires Vue 3.2+ — confirmed via Vite + `vue-tsc` build pass. Already on Vue 3.x.
- Conic gradient corner clipping still present (same as before). Acceptable.
- Reduced-motion fallback changed from a solid conic to a translucent ring (no `0% 100%` syntax inside the component for simplicity) — still clearly indicates "loading" without motion.
- 3-dot "Thinking" row inside bubble preserved per user preference — different layer (in-flow text vs perimeter).

## Verification

- `npm run build` clean.
- Manual: send message → arc on bubble, no header dot pulsing.
- Manual: during loading → arc on textarea (rounded-xl), no box-shadow pulse.
- Manual: reduced-motion ON → static translucent ring on both surfaces.

---

# Fix: unify bubble corners to `rounded-2xl` for clean arc traversal

**Date**: 2026-06-26
**Subsystem**: assistant-ui
**File**: `frontend/src/components/AgentIde/assistant/ChatBubble.vue`

## What

The assistant bubble had `rounded-tl-sm` (2px) at the top-left corner while the other three corners were `rounded-2xl` (16px). The conic gradient ring on the arc loader could not render cleanly at the 2px corner — the ring thickness (1.5px) almost equalled the radius, producing a notch where the arc appeared to dip below the corner. Removed `rounded-tl-sm` so all four corners are `rounded-2xl` and the arc follows the perimeter smoothly.

## Why

Conic gradient on a near-zero-radius corner clips or renders as a flat line depending on browser. The visual was distracting and undermined the rest of the orbit animation.

## Decisions

- **Unify all corners to `rounded-2xl`**: cleanest fix. The user/assistant distinction still comes from `justify-start` vs `justify-end` and bubble background color — corner shape was a redundant cue.
- **Simplify `<ArcOrbitLoader>` `radius` prop** to `"1rem"` (was `"1rem 1rem 1rem 0.125rem"`). One less knob, no special-casing in the component.

## Tradeoffs

- Loses the "tail" corner convention common in chat UIs. Accepted: visual consistency of the arc animation outweighs the convention.

## Verification

- `npm run build` clean.
- Manual: send message → arc orbits bubble, all 4 corners uniform, no notch at top-left.

---

# Tuning: per-site intensity, slower default, hide input border during load

**Date**: 2026-06-26
**Subsystem**: assistant-ui
**Files**:
- `frontend/src/components/common/ArcOrbitLoader.vue`
- `frontend/src/components/AgentIde/assistant/ChatInput.vue`

## What

The input arc felt too intense vs the bubble arc at the same defaults — same 1.5px ring + 4px glow on a smaller surface read as a spotlight. Slowed the global default to 2.4s/loop, extended the fade transition to 400ms, and dialed the input down via per-site props (1px ring, 2px glow). The textarea's gray-700 border now fades to transparent during loading so the arc is the only ring on the surface.

## Why

First-pass input looked chunky and over-attention-grabbing. Per-site props are the cleanest knob — keeps the bubble's defaults intact while letting the input feel proportionate. Border fade prevents the double-ring stack (gray-700 + arc) which made the perimeter feel noisy.

## Decisions

- **Slower default (1.8s → 2.4s)**: calmer rhythm, less "alarm clock". Bubble inherits.
- **400ms ease-out fade**: soft on/off, matches the 2.4s orbit tempo. Global default.
- **Input `thickness=1`**: same proportion on smaller surface as 1.5px on the bubble.
- **Input `glow=2`**: less halo bleed onto the gray-800 input area.
- **Border-transparent on `.is-loading`**: implicit fade via textarea's existing `transition-all`. No new keyframes. Re-fades in when loading ends.
- **Component default `duration: 2.4`**: both surfaces share; removes redundant prop pass to bubble.

## Tradeoffs

- Border fade uses Tailwind's `border-transparent`, which animates `border-color` via `transition-all`. Works because the textarea already has `transition-all` in its class list — no new rule needed.
- 1px ring on input may look almost invisible on hidpi screens at low zoom. If still off, bump to 1.25 (component supports any number).

## Verification

- `npm run build` clean.
- Manual: send message → bubble: 2.4s orbit, 1.5px ring, 4px glow, gray-700 border intact on bubble.
- Manual: input: 2.4s orbit, 1px ring, 2px glow, gray-700 border fades to transparent.
- Manual: completion → both fade out 400ms, border returns on input.
- Manual: reduced-motion ON → static translucent ring on both surfaces.

---

# Fix: unify bubble corners to `rounded-2xl` for clean arc traversal

**Date**: 2026-06-26
**Subsystem**: assistant-ui
**File**: `frontend/src/components/AgentIde/assistant/ChatBubble.vue`

## What

The assistant bubble had `rounded-tl-sm` (2px) at the top-left corner while the other three corners were `rounded-2xl` (16px). The conic gradient ring on the arc loader could not render cleanly at the 2px corner — the ring thickness (1.5px) almost equalled the radius, producing a notch where the arc appeared to dip below the corner. Removed `rounded-tl-sm` so all four corners are `rounded-2xl` and the arc follows the perimeter smoothly.

## Why

Conic gradient on a near-zero-radius corner clips or renders as a flat line depending on browser. The visual was distracting and undermined the rest of the orbit animation.

## Decisions

- **Unify all corners to `rounded-2xl`**: cleanest fix. The user/assistant distinction still comes from `justify-start` vs `justify-end` and bubble background color — corner shape was a redundant cue.
- **Simplify `<ArcOrbitLoader>` `radius` prop** to `"1rem"` (was `"1rem 1rem 1rem 0.125rem"`). One less knob, no special-casing in the component.

## Tradeoffs

- Loses the "tail" corner convention common in chat UIs. Accepted: visual consistency of the arc animation outweighs the convention.

## Verification

- `npm run build` clean.
- Manual: send message → arc orbits bubble, all 4 corners uniform, no notch at top-left.

