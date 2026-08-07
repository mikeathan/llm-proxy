---
status: active
date: 2026-08-06
---

# GPU Performance Plan — Consolidated

**Status:** active. The single forward-looking GPU plan (consolidates the removed
`gpu-utilization-followup.md` and `gpu-metrics-background-sampler.md` plans).
Full history, fixes, and lessons live in the audit: `docs/audits/gpu-performance-audit.md`.

**Goal:** the UI uses as little GPU as possible; the GPU reading is precise. No polling
reduction (deliberately held at the current 10s cadence).

## Completed (verified in the working tree)

### Frontend compositing / rendering
- Animation gating to visible/active state only (arc-orbit, thinking-gap-dot, 1s activity
  tick) — idle 7% → 2%.
- `backdrop-blur` → solid surfaces; `content-visibility: auto` on non-live turns; frozen
  turns (no message-object replacement during text streaming).
- Removed all per-frame-paint animations: pulse-dot box-shadow, `animate-ping` status dots
  (`ProcessLogs.vue`, `Logs.vue`), `animate-pulse` session dots (`ChatSessionList.vue`),
  `animate-alert-glow` + `notif-dot` box-shadow keyframes (`tailwind.config.js`,
  `WorkspaceExplorer.vue`, `MobileTabBar.vue`, `NotificationDot.vue`).
- `transition-all` → specific properties on always-mounted hot elements.
- `liveEvents` deep watch → O(1) length watch (`AssistantChat.vue`).
- `liveReasoning` inset-scroll watcher gated to `isLastTurn` (`ChatBubble.vue`).
- `maybeScroll` skips the `scrollTop` mutation when `scrollHeight` didn't grow
  (`ChatMessages.vue`); 250ms scroll throttle.
- Live reasoning rendered as plain text while streaming; committed content keeps markdown.
- Adaptive flush cadence (100–250ms) with no flush starvation.
- `?perf=1` telemetry removed (its rAF loop was a measurement confound).
- Vitest harness added (`vitest`, `vitest.config.ts`, `npm test`).

### Backend notify path (protocol-preserving, all full snapshots)
- Coalesce `EventReasoning`/`EventToolStream` emits to ≤50ms (`stream.go`
  `streamNotifyCoalesceInterval`) — `sse.events` ≤29/2s during fast streams.
- Byte-identical snapshot dedupe (`lastEmittedReasoning`/`lastEmittedContent`) — no
  redundant emits during provider stalls.
- Tests: `TestProcessStream_NotifyCoalescing{,_Reasoning,_ReasoningThenContent,_Dedupe}`.

### Metrics sampling + display
- Background-sample caching machinery (`gpuCached`/`gpuMu`/`stopCh`) and frontend poll
  5s→10s (decoupled sampling; note P0 — the interval is not wired through
  `refreshMetricsService()` in production).
- EMA smoothing of `UtilizationPct`/`MemoryUtilizationPct` (default α=0.3, provider-agnostic),
  seeded from the mean of the first 3 samples (no warm-up spike).
- Hot-reloadable `gpu_smoothing_alpha` via `SetSmoothingAlpha` + `ApplySystemUpdate`
  (smoothing applies immediately; sample interval still needs a backend restart).
- UI controls (Settings → GPU Status Configuration): GPU Sample Interval (s) + GPU Smoothing
  (0–1), each with an `InfoTooltip`. Static, light.
- Fixed a data race on `gpuSmoothingAlpha` (read under `RLock`); race-clean under `-race`.

## Next steps / suggested work

Prioritized. P0–P1 are measurement/wiring; P2+ are code only after measurement.

### P0 — Wire the metrics config through `refreshMetricsService()` (backend)
`app_context.refreshMetricsService()` builds `NewMetricsService(&models.Config{
Metrics:{GPU: …}})`, dropping `GPUSampleIntervalSec` and `GPUSmoothingAlpha`. Net effect:
`gpu_sample_interval_seconds: 10` in `config.json` is **dead** — no background ticker runs,
sampling is on-demand per `Snapshot()` (coupled to request volume), and the documented
"background sampler" never activates in production.
- Fix: pass `GPUSampleIntervalSec` + `GPUSmoothingAlpha` from the system config into the
  rebuilt service. Optionally expose a `SetSampleInterval` (or rebuild) for live application.
- Why it matters: predictable sample cadence, `ioreg`/`nvidia-smi` cost decoupled from poll
  volume, consistent EMA seeding, and the config knob actually working.
- Verify: `go build ./...`, `go test ./...`, `go run ./tools/check-complexity/`.

### P1 — Clean re-measure before any more rendering changes
The residual during-run GPU (≈22–30%) has **not** been cleanly measured since the 29↔41
contradiction and the Round 5 revert. Protocol (Round 6 lesson — measure first):
- One UI tab, DevTools closed, fixed display refresh rate (close ProMotion variability).
- Chrome DevTools Performance trace during a reasoning-only run (per-frame paint/composite,
  longest tasks) + Activity Monitor CPU-vs-GPU side-by-side (separate compositor work from
  mislabeled main-thread).
- Record the animation census (a cheap, non-rAF probe — do NOT reintroduce an rAF loop).

### P2 — Test the two remaining during-run suspects (gated on P1)
- **Outer-pane programmatic `scrollTo` every 250ms during streaming** — forced synchronous
  layout + full-pane composite each pass. If confirmed, consolidate to a single
  scroll-if-near-bottom path and/or use `content-visibility`/`scrollIntoView` sparingly.
- **Bubble `box-shadow` + `isolation: isolate` re-raster** on a growing element (Round 5
  mechanism 3). Audit the layer; replace the shadow with a cheaper accent if it shows in trace.

### P3 — True delta emission + incremental append (gated on P1; large blast radius)
Backend `{ text, full }` envelope with delta computation and full-snapshot fallback on
non-prefix transitions (incl. the `FilterStreamingMarkup` shrink case and `ExtractReasoning()`
tool-call markup); frontend append-delta text-node path (`messageBuilder.ts`). This is the
only path to O(1) per-event cost (full re-render is O(n)). Consumers to update:
`conversation_helpers.go` (payload type asserts), `agent_test.go` + `stream_test.go`
(prefix/snapshot assertions), and `conversation_helpers_test.go`. Requires live
Mac GPU A/B. If P1 shows the residual is the scroll/layer suspects (P2), this may be unnecessary.

### P4 — Display-policy decision for the gauge (product call)
The EMA shows the **mean** of the noisy `ioreg` signal (~15–17 at idle), where the raw gauge
used to dip to 0. Options (no GPU impact either way):
- Keep the stable average (current) — honest, but idle reads non-zero.
- Raw snapshot (`gpu_smoothing_alpha: 1.0` / off) — idle dips to 0, but noisy (original
  23-climb complaint returns).
- A floor/only-show-when-sustained policy if the non-zero idle reading is undesirable.

### P5 — Low priority / hygiene
- Bound `liveEvents` growth (O(1) watcher already removed the per-event cost).
- Confirm `content-visibility: auto` helps (not harms) the large-list case.
- **Sequencing constraint:** the stuck/nudge-loop bug ("empty finalization turn" —
  `nagSent` one-shot vs. re-arming) **is already specified and fixed in
  `fix-final-report-realignment.md`**. Do NOT re-investigate here. That fix must land
  *before* any P1 re-measure, because the bug skews GPU measurement windows. This GPU plan
  owns only pure rendering/metrics items (P0–P4); the agent-loop fix lives in the other doc.

## Verification checklist
- `cd backend && go build ./... && go test ./...`
- `cd backend && go run ./tools/check-complexity/`
- `cd frontend && npm run build`
- Mac re-measure per P1 protocol before and after any P2/P3 change.
