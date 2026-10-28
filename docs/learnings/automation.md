---
subsystem: automation
last_updated: 2026-10-26
---

# Learnings — Automation dispatcher and heartbeat (SPEC-007)

What was tried and what happened. Read before proposing an approach in this area; add a line when a run,
eval, audit or merged phase teaches something (newest last within each section). Format and rules:
[`README.md`](README.md).

## Dead ends — do not retry without new evidence

- **Tried:** Model-written memory as the heartbeat's "seen" ledger → **rejected**. **Lesson:** non-deterministic; the model cannot know delivery succeeded. Evidence: [heartbeat-seen-ledger.md](../PLANS/automation/heartbeat-seen-ledger.md).
- **Tried:** Injecting the whole 60-day seen ledger into every prompt → **rejected**. **Lesson:** unbounded prompt growth; filter by link in code. Evidence: [heartbeat-seen-ledger.md](../PLANS/automation/heartbeat-seen-ledger.md).

## Root causes and fixes that held

- **Tried:** Heartbeat `alert` recorded even when nothing was sent → **found 2026-10-15**. **Lesson:** record delivery honestly (tracked). Evidence: [2026-10-15-next-work-analysis.md](../audits/2026-10-15-next-work-analysis.md), [heartbeat-seen-ledger.md](../PLANS/automation/heartbeat-seen-ledger.md).
- **Tried:** Reading `is_running` right after a manual trigger (2026-10-28) → **"Run now" came back enabled for a few seconds before turning into "Stop", so the click looked lost.** The trigger endpoint answers as soon as the run is admitted to its lane (`status: started`), but `AgentState.ActiveAutomation`, which `is_running` reads, is set later in `prepareRun`, after the workspace lock and the pre-flight (for a network audit, the readiness check); and the page's next refresh was up to 10 s away (`REFRESH_INTERVAL_MS` in `AutomationsView.vue`). **Lesson:** an admitted run is not yet a running run; the runner now holds a `starting` state (button "Starting…", disabled) and polls once a second until the server shows it running or queued, a new run appears in the history (it failed to start), or a minute passes. `useDispatcher.triggerAutomation` returns the result so `queued`/`skipped` do not wait. The server still has the gap; marking the run at admission was not done because the flag is persisted state. Evidence: `useAutomationRunner.test.ts` ("starting"), `Automations.component.test.ts`.

