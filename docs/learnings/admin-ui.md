---
subsystem: admin-ui
last_updated: 2026-10-26
---

# Learnings — Admin UI and chat rendering (SPEC-003)

What was tried and what happened. Read before proposing an approach in this area; add a line when a run,
eval, audit or merged phase teaches something (newest last within each section). Format and rules:
[`README.md`](README.md).

## Dead ends — do not retry without new evidence

- **Tried:** Animating hidden elements, `backdrop-blur` on always-visible surfaces, box-shadow keyframes → **constant GPU cost on macOS**. **Lesson:** gate the animation itself; solid backgrounds; opacity/transform-only indicators. Evidence: [gpu-performance-audit.md](../audits/gpu-performance-audit.md).
- **Tried:** Measuring GPU with a telemetry loop that uses `requestAnimationFrame` → **the meter itself created phantom GPU load**. **Lesson:** measurement tooling must not animate. Evidence: [gpu-performance-audit.md](../audits/gpu-performance-audit.md).

## Root causes and fixes that held

- **Tried:** Deep `watch` on unbounded arrays; `.some()` dedup over all live events → **O(n²) per event**. **Lesson:** watch by reference/length; one shared `Set`-based event-id deduper. Evidence: [gpu-performance-audit.md](../audits/gpu-performance-audit.md).
- **Tried:** Full text-node replacement and nested auto-scrollers while streaming → **whole-region re-raster per flush**. **Lesson:** incremental append; one scroller. Evidence: [gpu-performance-audit.md](../audits/gpu-performance-audit.md).
- **Tried:** Measuring a frontend change without rebuilding the Go binary → **old UI measured (`go:embed`)**. **Lesson:** rebuild Go, restart, hard-refresh before measuring. Evidence: [gpu-performance-audit.md](../audits/gpu-performance-audit.md).
- **Tried:** `messageBuilder.reset()` and multi-message turn grouping → **stale reasoning in the next turn; grouper used the first message's segments**. **Lesson:** reset `liveReasoning`; use the last message's segments. Evidence: [2026-06-26-stale-turn-bleed.md](../audits/2026-06-26-stale-turn-bleed.md).
- 2026-10-29: a command that writes a file the running service must read has to act on the service's own folder as the service's account. `llm-proxy auth setup-code` typed from a checkout wrote to the typist's `~/.config/llm-proxy` and the deployed service (own user, `LLM_PROXY_HOME=/var/lib/llm-proxy`) never saw the code; `setup_open` stayed false and `/admin/setup` redirected home. The command now chooses the folder as the server does, becomes the account that owns the folder (not the unit's `User=`), and checks a marker (`server.json`) the running server keeps in its folder; guessing from the unit alone has blind spots (drop-ins), a check does not. A plain `sudo` with an explicit `--data` would be worse: a root-owned 0600 file the service cannot read, so every admin request answers 503.
