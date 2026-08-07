---
status: reference
last_reviewed: 2026-08-06
---

# GPU Performance Audit — Consolidated

**Date:** 2026-08-06 (consolidation of work done 2026-07-23 → 2026-08-06)

**Canonical record.** This audit consolidates every GPU-performance investigation, fix, and
lesson learned from the work done 2026-07-23 → 2026-08-06 (previously spread across the now
removed `2026-08-03-gpu-utilization-investigation.md` audit and the `gpu-utilization-followup.md`
/ `gpu-metrics-background-sampler.md` plans).

The forward-looking work items live in the single plan: `docs/PLANS/gpu-performance.md`.

## Goal

Two user-facing goals drive all work in this audit:
1. **The UI uses as little GPU as possible.**
2. **The GPU reading is precise** (stable, honest, not noisy).

## What the GPU metric actually measures (attribution caveat)

On macOS, `ioreg -c IOAccelerator -r -l` exposes an **aggregate, delayed host signal** —
the first matching `Device Utilization %` / `GPU Activity` field — with **no per-process
attribution**. It includes WindowServer compositing, the browser, the terminal, and every
other app on the machine. It is **not** proof that the chat UI caused a sampled percentage.
Attribution requires Activity Monitor (per-process GPU) or a browser DevTools GPU process
view. Linux providers (sysfs `gpu_busy_percent`, `nvidia-smi` `utilization.gpu`, rocm-smi,
amdgpu_top) are similarly instantaneous and noisy.

Consequences: a gauge reading can dip to 0 at idle (raw noise floor) while its **average**
is 15–17; and a high reading is not automatically the frontend's fault. Design decisions in
this audit respect this by (a) attacking provable code paths (animations, watchers, backend
payloads) and (b) using smoothing only to make the *display* stable, never to hide real work.

## Root-cause history (chronological)

### Phase A — frontend compositing hot paths (2026-08-03, audit Rounds 1–6)
Original problem: 48–56% GPU while running assistant prompts on a local Mac (llama.cpp on a
remote host; the local GPU should do no inference). Findings in order:

1. **`go:embed` shipping trap.** `npm run build` alone is not enough; the running backend
   keeps serving the old embedded bundle until the Go binary is rebuilt **and** restarted
   **and** the browser hard-refreshed. Several measurement rounds were invalid for this reason.
2. **Always-on animations on invisible elements.** `arc-orbit-loader` (2 conic-gradient
   repaint animations) and `thinking-gap-dot` (3 opacity dots) kept running at
   `opacity:0`/`visibility:hidden` 24/7.
3. **Deep watcher over an unbounded live-event list.** `AssistantChat.vue` deep-watched
   `liveEvents`; every SSE event appended to the array and the whole history was traversed
   per event → O(n²) over a long stream.
4. **Update fan-out to every historical bubble.** `liveReasoning` was passed to **every**
   `ChatBubble`, not only the last active turn; each bubble recomputed on every live flush.
5. **`backdrop-blur`** on always-visible surfaces = constant macOS blur-composite cost.
6. **Scroll churn.** Outer + inset auto-scroll read `scrollHeight`, wrote `scrollTo`, and
   re-read position on every flush (two independently scheduled scroll paths).
7. **Box-shadow keyframes** (`animate-alert-glow`, `notif-dot`, pulse-dot) = per-frame paint.

Round outcomes (measurements in the audit's telemetry runs): R1 no change (build never
shipped); R2 UX regression, reverted; R3 first real lever (scroll throttle); R4 idle wins
(1s tick gated to running, pulse-dot box-shadow removed); R5 idle waste removal
(animation gating, `content-visibility`, `backdrop-blur` → solid) → **idle 7%→2%,
during-run peak 54–56%→29%**; R6 adaptive flush cadence — anomalous measurement
(fps collapse + idle jank), environment suspected, never cleanly re-measured at the time.

### Phase B — closing missed drivers (2026-08-04/05, followup Rounds 0–6)
1. **Backend notify coalescing (Phase 0).** `stream.go` sent the FULL accumulated
   reasoning/content buffer on every LLM chunk → O(n²) payload + frontend full-buffer
   re-parse/Markdown re-render per event. Fixed: buffer latest snapshot, flush at most every
   50ms (`streamNotifyCoalesceInterval`); first chunk flushes immediately; `defer` guarantees
   the final tail. Protocol-preserving: every payload is still a full snapshot; only the rate
   is capped.
2. **Coalescing dedupe (Phase 3).** Skip byte-identical re-emits (`lastEmittedReasoning` /
   `lastEmittedContent`) — provider stalls were re-sending the same frozen snapshot repeatedly.
3. **Missed always-on animations (Phase 2.5).** `ProcessLogs.vue`/`Logs.vue` `animate-ping`
   green dot; `ChatSessionList.vue` `animate-pulse` per running session → static dots.
4. **Box-shadow keyframes removed (Phase 3.3).** `animate-alert-glow` (action-pill) +
   `notif-dot` → static accents.
5. **`?perf=1` telemetry was a measurement confound (Round 2).** `perfTelemetry.ts` ran an
   infinite `requestAnimationFrame` loop at module load whenever `?perf=1` was set. A perpetual
   60fps rAF keeps the compositor awake → **phantom ~20% idle GPU**. This confounded **every**
   prior measurement (including the audit's 54%→29% numbers). Removed entirely.
6. **Transition narrowing (Round 2).** `transition-all` → specific properties on
   always-mounted hot elements so transitions don't replay on unrelated re-renders.
7. **Live reasoning plain-text render (Phase 1).** Live streaming reasoning rendered as plain
   `pre-wrap` text instead of `MarkdownViewer` (avoids the O(n²) `marked()` re-parse per
   flush); committed segments and the final answer still render full markdown.
8. **Round 3 verification** confirmed the above in the tree and surfaced two missed hot paths:
   the `liveEvents` deep watcher (audit #3, unaddressed) and the `liveReasoning` fan-out
   (audit #4, unaddressed).
9. **Round 4 safe fixes.** `liveEvents` deep watch → `watch(() => liveEvents.value.length, …)`
   (O(n)→O(1)); the `ChatBubble` inset-scroll watcher early-returns unless `props.isLastTurn`
   (kills the per-flush fan-out into every historical bubble).
10. **Parity check overturned the "inherent floor" (Round 5).** Same Mac + same browser
    streaming a reply: **Gemini ≈3% GPU vs llm-proxy ≈27–30%** (CPU ≤17% → real compositor
    work, not mislabeled main-thread). Three mechanisms identified in
    `ChatBubble.vue`/`ChatMessages.vue`: full text-block re-raster per flush; nested
    auto-scrolling inset (`max-height:320px; overflow-y:auto`) force-scrolled every 250ms
    while content grew; bubble layer + `box-shadow` re-raster on a growing element.
11. **Round 5 implemented + reverted (Round 6).** Incremental text-node append (A), inset
    uncap (B), scroll-skip (C) shipped, but B+A **regressed rendering** (streamed reasoning
    leaked outside the inset) with **no GPU win** (≈22, same as before). Reverted in full.
    **Lesson: do not touch rendering internals again until the cost is measured** (Chrome
    DevTools Performance trace). Retained (safe, orthogonal): Round-4 `liveEvents` length
    watcher, the `isLastTurn` inset-scroll gate, and `ChatMessages.maybeScroll` scroll-height
    skip (C).

### Phase C — metrics display precision (2026-08-06)
The displayed GPU% was a jumpy raw snapshot (observed 9 → 0 → 23 → 0 during a stream; 0-dips
at idle). Work delivered:

1. **EMA smoothing.** `MetricsService.smoothGPU` applies an exponential moving average
   (default `gpu_smoothing_alpha = 0.3`) to `UtilizationPct`/`MemoryUtilizationPct` after any
   provider `Sample()`. Provider-agnostic (macOS + Linux). Temperature and raw memory bytes
   stay untouched. Purely display math — no added sampling, goroutines, or processes.
2. **Seed from mean of first 3 samples.** Prevents the "warm-up spike" (gauge seeding on one
   high first reading after a metrics-service restart and decaying slowly). Accurate from ~30s.
3. **Hot-reloadable alpha.** `MetricsService.SetSmoothingAlpha` + `SystemUpdatePayload`
   `gpu_smoothing_alpha`; a config change applies immediately via `ApplySystemUpdate` without
   a backend restart (sample interval still requires restart — it governs the poller).
4. **UI controls.** Settings → GPU Status Configuration now has GPU Sample Interval (seconds)
   and GPU Smoothing (0–1) inputs, each with an `InfoTooltip`. Static, light, no polling.
5. **Data race fixed.** `gpuSmoothingAlpha` was read without a lock in
   `effectiveSmoothingAlpha` while `SetSmoothingAlpha` wrote under `gpuMu` → now read under
   `RLock`. Race-clean under `go test -race`.

## Key learnings / lessons

1. **`ioreg` is aggregate host telemetry, not per-process proof.** Use Activity Monitor for
   attribution. A stable gauge value can never exceed the raw readings — the EMA is a bounded
   weighted average, so "15 at idle" means the raw average is ~15, not a display bug.
2. **Never animate hidden elements.** CSS animations keep running at `opacity:0` /
   `visibility:hidden`. Gate the animation itself (`.is-active::before`, `:not(.bubble-paused--hidden)`).
3. **`backdrop-blur` on always-visible surfaces is a constant macOS GPU cost.** Prefer solid backgrounds.
4. **Box-shadow keyframes are per-frame paint.** Use opacity/transform-only indicators.
5. **Measurement tooling must not animate.** A `requestAnimationFrame` loop in the telemetry
   itself sustained phantom GPU and confounded every number it produced.
6. **`go:embed` trap:** frontend changes need Go rebuild + backend restart + browser
   hard-refresh before they can be measured.
7. **Deep `watch` on unbounded arrays is O(n²).** Watch by reference/scalar (`length`).
8. **Prop fan-out to every list child is wasted child-update traversal.** Gate to the live/last element.
9. **Nested auto-scrollers re-raster their whole region each pass** on top of the outer scroll.
10. **Full text-node replacement re-rasters the whole block per flush.** Incremental append is
    O(1) per event; full re-render is O(n).
11. **Don't conclude "inherent platform floor" without a parity check.** Gemini ≈3% vs
    llm-proxy ≈30% on the same machine proved the residual was ours.
12. **Don't ship rendering-internal changes unmeasured** (Round 5 → 6 regression). Measure
    (Chrome DevTools Performance trace) before writing rendering code.
13. **EMA shows the mean, not the dips.** "Idle used to hit 0" was the raw noise floor; the
    smoothed gauge shows the sustained average. Setting expectations beats chasing the number.

## Current state (as of 2026-08-06)

**Done and verified:**
- Idle GPU ~2% under strict conditions (single tab, no DevTools, fixed refresh) — animation
  gating, `backdrop-blur` removal, `content-visibility`, frozen turns, adaptive flush.
- Backend notify coalescing (50ms) + byte-dedupe — `sse.events` ≤29/2s during fast streams.
- All always-on/paint-cost animations removed (`animate-ping`, `animate-pulse` dots,
  box-shadow keyframes); only cheap opacity/transform animations remain during runs.
- `liveEvents` deep-watch and `liveReasoning` fan-out eliminated (O(1), last-turn gated).
- `?perf=1` telemetry removed (was a measurement confound).
- Live reasoning rendered as plain text during streaming; committed content keeps markdown.
- Metrics display: EMA smoothing (default 0.3), mean-of-3 seed, hot-reloadable alpha, UI
  controls, race-free.
- Frontend Vitest harness added (`textAppend` removed with Round 6; harness stays).

**Open / not yet resolved:**
- Root cause of the residual during-run GPU (≈22–30%) still **not measured cleanly** since the
  29↔41 measurement contradiction and the Round 5 revert. Round 6 suspects: outer-pane
  programmatic `scrollTo` every 250ms (forced sync layout + full-pane composite) and bubble
  `box-shadow`/`isolation` re-raster on a growing element.
- `gpu_sample_interval_seconds` in config is **not wired through**
  `refreshMetricsService()` (only `GPU` provider is passed), so the documented background
  sampler does not run in production; sampling is on-demand, driven by request volume.
- Display-policy decision: idle reads the raw average (~15–17) rather than the raw 0-dips.
  Whether that should be a stable average (current), the raw snapshot, or a floor is a
  product call — see the plan.

**Measurement evidence (condensed):**
- Audit runs A–D: idle 7%→2%; during-run peak 54–56%→29% (post-R5); Run D fps collapse was
  an environment/refresh signature, not a code regression.
- Followup live re-measure (R1): active tab fps ~75, janky 0, `sse.events` ≤29/2s, idle
  animation census empty; `frames=0` windows = backgrounded tab (rAF pauses), not a stall.
- Round 2: ~41% during a **stuck/incomplete** run (invalid measurement window; do not drive
  code off it).
- Round 5 parity: Gemini ≈3% vs llm-proxy ≈27–30% on the same machine/browser.
- Round 6: revert restored Round-4 rendering; GPU ≈22 during a reasoning run (no win from A+B).

## Related documents

- Plan (forward-looking): `docs/PLANS/gpu-performance.md`
- Skill rules (live): `docs/skills/assistant-ui-chat.md`
