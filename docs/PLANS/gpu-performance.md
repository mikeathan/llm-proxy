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

### 2026-08-28 round (residual-driver investigation — see audit Phase D)
- **P0 fixed:** `refreshMetricsService()` now passes the FULL metrics config
  (`GPUSampleIntervalSec` + `GPUSmoothingAlpha` from `System().Get()`) into the rebuilt
  `MetricsService` — the background sampler actually runs at the configured cadence, and
  `ioreg`/`nvidia-smi` sampling is decoupled from poll volume. Live changes apply via
  `System().OnChange → SetGPUConfig → refreshMetricsService` (no restart). New exported
  `MetricsService.SampleInterval()`/`EffectiveSmoothingAlpha()` getters + wiring test
  `TestRefreshMetricsService_WiresGPUMetricsConfig`.
- **Arc-orbit: animation REMOVED (measurement-driven).** The transform-rotation experiment
  was reverted (visually rotated the rounded-rect shape), and the A/B measurement then showed
  the restored `--arc-angle` animation was the DOMINANT during-run GPU cost (~11 points:
  baseline 21% peaks vs 9.9% with the loader off, ≈ idle). The loader is now a static accent
  ring (painted once); thinking dots + label carry the activity affordance. A
  compositor-safe spinning-arc redesign (circular element) is optional future work.
- **Always-on `animate-pulse` dots removed** (`AssistantActivity.vue`, `AutomationsPanel.vue`,
  `RecordingsPanel.vue`) — static dots.
- **Double-scroll redundancy removed:** the 320px inset only scrolls itself when it actually
  overflows (`scrollHeight > clientHeight`); `ChatMessages.maybeScroll` consumes its 250ms
  throttle before the `scrollHeight` layout read so forced sync layout runs ≤4x/sec. All
  pause-on-scroll-up / idle-resume behaviour unchanged.
- **Live reasoning renders full markdown again** (plain-text experiment was GPU-neutral;
  adaptive flush cadence bounds `marked()`).
- **`isolation: isolate` → `z-index: 0`** on `.message-bubble` (same stacking context;
  removes the isolate property).
- **`useLiveConsole` event dedup → O(1) Set** (was `liveEvents.some()` — O(n²) over long
  automation runs), matching chat's `useAssistantSSE`.
- Docs: audit Phase D + this plan updated; skill `assistant-ui-chat.md` updated.

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

### P0 — ~~Wire the metrics config through `refreshMetricsService()` (backend)~~ **DONE (2026-08-28)**
`app_context.refreshMetricsService()` previously built `NewMetricsService(&models.Config{
Metrics:{GPU: …}})`, dropping `GPUSampleIntervalSec` and `GPUSmoothingAlpha`. Net effect:
`gpu_sample_interval_seconds: 10` in config was **dead** — no background ticker ran, sampling
was on-demand per `Snapshot()` (coupled to request volume), and the documented "background
sampler" never activated in production.
- **Fixed:** the rebuild now reads the full metrics config from the system config; live
  changes apply through the existing `System().OnChange → SetGPUConfig` subscriber.
- **Follow-up (shipped 2026-08-28):** `MetricsConfig`/`GPUConfig` previously had no `yaml`
  tags, so `settings.yml` stored field-name-derived keys (`gpusampleintervalsec`). The
  canonical snake_case keys (`gpu_sample_interval_seconds`, `gpu_smoothing_alpha`,
  `sysfs_path`) are now tagged, and custom `UnmarshalYAML` accepts the legacy keys too, so
  existing files keep loading and migrate on the next write (canonical wins when both
  present). See `docs/audits/2026-08-28-ops-performance-review.md`.

### P1 — Clean re-measure before any more rendering changes
The residual during-run GPU (≈22–30%) has **not** been cleanly measured since the 29↔41
contradiction and the Round 5 revert. Protocol (Round 6 lesson — measure first):
- One UI tab, DevTools closed, fixed display refresh rate (close ProMotion variability).
- Chrome DevTools Performance trace during a reasoning-only run (per-frame paint/composite,
  longest tasks) + Activity Monitor CPU-vs-GPU side-by-side (separate compositor work from
  mislabeled main-thread).
- Record the animation census (a cheap, non-rAF probe — do NOT reintroduce an rAF loop).

### P2 — Test the remaining during-run suspects (gated on P1)
- ~~Outer-pane programmatic `scrollTo` every 250ms~~ — redundancy removed 2026-08-28 (inset
  scrolls only when overflowing; outer `scrollHeight` read throttled). Verify in the trace
  that scroll passes are now single-container and rare.
- **Bubble `box-shadow` + `z-index: 0` re-raster on a growing element.** Audit the layer in
  the trace; replace the shadow with a cheaper accent if it shows. (`isolation: isolate` was
  swapped for `z-index: 0` 2026-08-28 — same stacking context, one less property.)
- **`content-visibility: auto` + frequent scroll interplay** — confirm it helps (not harms)
  the large-list case under the 2026-08-28 scroll cadence.

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
- Bound `liveEvents` growth (O(1) watcher already removed the per-event cost; chat +
  automation arrays still grow unboundedly for the session — cap or prune on completion).
- Confirm `content-visibility: auto` helps (not harms) the large-list case.
- Add `yaml` tags to `MetricsConfig` + one-time migration so hand-edited snake_case keys
  (`gpu_sample_interval_seconds`) read back (see P0 follow-up).
- **Sequencing constraint:** the stuck/nudge-loop bug ("empty finalization turn" —
  `nagSent` one-shot vs. re-arming) **is already specified and fixed in
  `fix-final-report-realignment.md`**. Do NOT re-investigate here. That fix must land
  *before* any P1 re-measure, because the bug skews GPU measurement windows. This GPU plan
  owns only pure rendering/metrics items (P1–P5); the agent-loop fix lives in the other doc.

## Verification checklist
- `cd backend && go build ./... && go test ./...`
- `cd backend && go run ./tools/check-complexity/`
- `cd frontend && npm run build`
- Mac re-measure per P1 protocol before and after any P2/P3 change.
