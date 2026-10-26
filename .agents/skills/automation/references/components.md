# Automation System — Dispatcher, Executor & Task Lifecycle — Key Components

Reference for the [`automation`](../SKILL.md) skill, loaded on demand. You need the role of a specific dispatcher/executor component.

## Key Components

### Dispatcher (`internal/core/automation/dispatcher.go`)
- Manages schedules (cron, interval, manual)
- Emits SSE events for live UI updates
- Handles run lifecycle (start, cancel, complete)
- Run timeout = `max(automationTimeout (default 10m), pinned model's timeout_minutes)`, so a slow
  model configured with a longer timeout is not cut off mid-run (`runContext` in dispatcher.go)
- `Dispatcher.NextRuns()` reads each job's next fire time from the scheduler's own entries (`next_run_at` on the automation list); a manual automation has no job and no next run.
- Cron is **one grammar** (`cronParser` in `core/automation/cron.go`: five fields or a descriptor, **no seconds**) shared by
  `NewCronTrigger`, the scheduler and `PreviewSchedule`; `validateAutomation` builds the trigger with `automation.New`
  so a schedule the scheduler cannot read is refused (400) before it is saved. The UI never asks for cron: the schedule
  picker (`SchedulePicker.vue`, `utils/automation/schedule.ts`) writes the trigger and reads it back.
- Cron jobs register their **real schedule** (`triggerToCron`), so pre-executor failures no longer
  retry every minute and a new automation does not fire immediately
- Re-`Register`ing an automation **atomically replaces** its cron entry (`replaceJob`: remove-then-add
  under `d.mu`). A trigger switch or schedule edit must never leave the old entry scheduled — that
  previously made a cron automation switched to `manual` keep firing at its old time (and leaked a
  second entry on every schedule change). `scheduleAutomation` removes the entry for a manual trigger;
  `Unregister` shares `removeJob`
- If an automation still fires after being changed, the orphan lives in the running process —
  **restart the service** to clear it (scheduling state is in-memory only)
- The hot-reload watcher watches the **metadata root** (`<root>/meta`, where `config.yaml` lives) plus
  each workspace metadata dir, because fsnotify is non-recursive. Watching `workspaces/` (the old
  `BaseDir`) never saw a `config.yaml` write, so reconciliation was dead
- Panic containment: every execution goroutine runs through `safe.Go` and cron jobs through
  `cron.Recover`; `executeAutomation` recovers panics and clears the run's running state, so a
  single bad run can never crash the service or leave a workspace stuck marked running
- `RunReaper` (`runreaper.go`, started in `app.New`) prunes completed run directories older than
  `DefaultRunRetention` (30 days) so a long-lived service does not fill the disk

### StopAutomation Behavior
- `StopAutomation()` cancels the execution context immediately (best-effort).
- A 30-second diagnostic goroutine monitors whether the run actually terminated.
- If still running after 30 seconds and a shell PGID is available, force-kills the process group via `syscall.Kill(-pgid, SIGKILL)` and removes the run from `activeRuns`.
- If no shell PGID (network-only run), logs a warning (graceful degradation).
- Subsequent `StopAutomation` calls cancel the previous diagnostic goroutine via a cancellable context (prevents accumulation).
- Shell PGID is polled lazily after agent creation — the persistent shell session is created on first `terminal_execute` call.

### LLMTaskExecutor (`internal/core/automation/executor.go`)
- Gets LLM client
- Builds AgentOptions from model config (max_steps, temperature, reasoning budget, etc.)
- Creates agent, runs `Execute()`
- Returns result to dispatcher

### Cold Local-Model Startup
When an automation targets a local model that is not running (e.g. idle-reaped before a
scheduled run), the first `GetClientForModel` triggers `llama-server` startup and returns
`models.ErrModelStarting` while it warms up. `getLLMClient` now **polls** on that sentinel
(`proxy.WaitForClient`, shared with chat) instead of failing the run immediately, so an unattended run can auto-start
the local LLM and wait for it. The poll runs every `models.ModelStartPollInterval` (3s) up to
`models.ModelStartWaitTimeout` (5 min), mirroring the idle reaper's own 5-minute startup window, and
aborts immediately on any non-starting error or on context cancellation.

### Local Model Failure Detection
A local model that crashes on launch (bad launch args, missing model) is detected via a shared
process-exit watch (`internal/platform/procwatch`, reused by both the model lifecycle and the
persistent shell). `clearCrashedModelLocked` (in `internal/core/llm/lifecycle.go`) is the single
place that turns an exited model into a failure: it records the error (`LastModelError`, surfaced
in the admin status / process-logs endpoints) and clears the dead model. Both `GetInstance` (so a
request surfaces a real error instead of looping on `ErrModelStarting`) and the idle reaper (so a
crashed model is cleared promptly instead of after the 5-minute startup timeout) call it. The
automation path additionally fails with a clear `model <name> did not become ready within 5m`
message via `proxy.WaitForClient` rather than falsely proceeding on a dead model.

### Agent Loop (`internal/core/assistant/`)
- `run()` in `session.go` — main loop, handles turn-by-turn execution
- `executeTurn()` — sieve → computeNextResponse → parse tools → execute → check repetition
- Returns final answer via natural completion (content-only message)
