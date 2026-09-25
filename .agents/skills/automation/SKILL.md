---
name: automation
description: "Automation system: dispatcher, executor, run lifecycle, and templates. Use when working on scheduled tasks or automations."
when_to_use: "Working on scheduled tasks, cron/interval triggers, run lifecycle, or task templates."
status: reference
last_reviewed: 2026-07-11
---

# Automation System — Dispatcher, Executor & Task Lifecycle

**Source docs:** SPEC-007 (the dispatcher + run-scheduler/model-residency contract), `docs/architecture.md` (package layout, pitfalls #34–#36)

---

## Architecture

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│  Automations UI  │────▶│  Dispatcher      │────▶│  LLMTaskExecutor │
│  (frontend)      │     │  (cron/interval  │     │  (per-run)       │
│                  │     │   /manual)       │     │                  │
└─────────────────┘     └──────────────────┘     └─────────────────┘
                              │                           │
                              ▼                           ▼
                       ┌──────────────┐          ┌─────────────────┐
                       │  Workspace   │          │  Agent.Execute()│
                       │  isolation   │          │  (tool loop)    │
                       └──────────────┘          └─────────────────┘
```

## Execution Lifecycle

1. **Trigger** — Scheduled (cron/interval) or manual via UI
2. **Start** — Dispatcher creates a run, emits SSE `run_start`
3. **Execute** — `LLMTaskExecutor.Execute()` runs the agent loop
4. **Progress** — SSE events stream turn-by-turn to the UI
5. **Complete/Fail** — Result saved to run history, SSE `run_complete` or `run_fail`

## Key Components

### Dispatcher (`internal/core/automation/dispatcher.go`)
- Manages schedules (cron, interval, manual)
- Emits SSE events for live UI updates
- Handles run lifecycle (start, cancel, complete)
- Run timeout = `max(automationTimeout (default 10m), pinned model's timeout_minutes)`, so a slow
  model configured with a longer timeout is not cut off mid-run (`runContext` in dispatcher.go)
- Cron jobs register their **real schedule** (`triggerToCron`), so pre-executor failures no longer
  retry every minute and a new automation does not fire immediately
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
(`waitForModelReady`) instead of failing the run immediately, so an unattended run can auto-start
the local LLM and wait for it. The poll runs every `modelStartPollInterval` (3s) up to
`modelStartWaitTimeout` (5 min), mirroring the idle reaper's own 5-minute startup window, and
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
message via `waitForModelReady` rather than falsely proceeding on a dead model.

### Agent Loop (`internal/core/assistant/`)
- `run()` in `session.go` — main loop, handles turn-by-turn execution
- `executeTurn()` — sieve → computeNextResponse → parse tools → execute → check repetition
- Returns final answer via natural completion (content-only message)

## Run Output Structure

```
backend/data/runs/workspace-<id>/<task-name>/<model>/<timestamp>_<uuid>/
├── run-meta.json      # Duration, LLM calls, tool calls, result
├── recording.jsonl    # Full request/response pairs (for replay)
├── events.jsonl       # SSE events (lifecycle, stuck, fallback)
└── final-report.md    # Agent's final output
```

## Model Config Overrides Applied Per-Run

| Field | Source | Example |
|-------|--------|---------|
| `max_tokens` | Model config + metadata | 2730 |
| `reasoning_budget` | Model config or max_tokens/3 | 910 |
| `temperature` | Model config (default 0.1) | 0.1 |
| `max_steps` | Model config (default 25) | 35 |
| `context_budget` | Model config (default 8000, `(ctx−max_tokens)×4` chars/token for local) | 21848 |
| `timeout_minutes` | Model config (default 30) | 0 (use default) |
| `tool_call_format` | Model config | "native" or "xml" |
| `prefill` | Model config | true/false |

## Task Templates

Templates live in `backend/data/templates/`. They're plain markdown files copied to the workspace when a task starts. The template is the first user message the agent sees.

**Template writing conventions:**
- Write for the agent, not for a human tester. Use "Save these facts to memory" not "Present the agent with these facts."
- For search-heavy tests, use "Search at most once" to prevent search spirals.
- The template must produce a clear PASS/FAIL result at the end.
- After changing a template, also update the workspace copy if one exists.

## Important Gotchas

- The workspace is NOT cleaned between runs. Leftover files from previous runs pollute the agent's context and cause confusion.
- `notify_user` is NOT a valid tool for automation. The guardrail blocks it. Write the final report directly as natural-language output.
- A trailing comma in a tools manifest (`system.json`) can silently drop a tool entry — validate manifests after edits.
- The spiral detector kills runs that repeat the same tool call with varying args 12+ times. This prevents infinite search loops.
- Memory can accumulate across runs in orchestrator.db. To fully reset, delete the db file.
- For memory-tags tests, the persona count for `tags:["persona"]` should be 4 (fact 6 is appended into the same topic).
- **State object concept (not implemented)** — A `[DONE]/[ACTIVE]/[PENDING]` progress block pinned at index [1] in the prompt was proposed but never built. The idea: the model calls a `complete_step` tool, Go updates the block, and the block survives the sieve as ground truth. If reviving this, use an explicit `complete_step` tool (not fragile signature matching) and pin the block at index [1] as a system message to survive truncation.

## Run Lane (Scheduler)

Every agent run — chat and automation — is admitted through `internal/core/runlane`
before executing (SPEC-007 §V). Two workload-class lanes: `local` (default concurrency 1)
and `cloud` (default 3), configured in Settings → Local (`settings.yml → scheduler:`) and
applied live via the settings `OnChange` hook.

- **Queue, never drop** — an automation that cannot start queues FIFO
  (`AutomationInfo.queued` / `queue_position`; `DELETE /dispatcher/queue/{ws}/{automation}`
  cancels a queued entry). At most one queued entry per automation: a fire while queued is
  absorbed (reported queued at its position, never an error), a fire while running becomes
  the single pending rerun. Cancel-queued drops only a pending entry — never a running run
  (`StopAutomation` owns that). Deleted automations are re-resolved at dequeue and dropped.
- **Preemption** — a chat preempts a running automation in the same lane only: the run's
  ctx is marked `runlane.Preempted` then cancelled (≤10 s grace); the run records as
  **skipped**, its failed tail history entry is dropped, and an informational event
  (not `EventError`) is published. Scheduled runs re-queue at the front; manual runs drop.
- **Same-workspace runs queue on the flock** (`acquireWorkspaceLock`) — a run whose ctx
  expires while waiting counts as skipped, never failed.

### Where the UI reads run state

Two scopes, and lane state has exactly **one** owner (the snapshot is global, so
the per-workspace route deliberately does not carry it):

- `GET /admin/api/active-runs` (global, no workspace param) → `lane_holders` +
  `queued`; polled once by `useGlobalRunActivity` and shared by the header
  `RunActivityPill` **and** AgentIde's `laneWaitingLabel`.
- `GET /admin/api/workspaces/{ws}/active-runs` → only the workspace-scoped fields
  (`assistant_running` / `automation_running` / `assistant_conversation_id` /
  `assistant_queued`); polled by `useRunningActivity`.

Two pitfalls this replaced: `laneWaitingLabel` used to pick a holder with **no
`workspace_id` filter** (naming another workspace's run as the blocker), and the
header indicator used to keep showing the last lists after a failed poll —
reading as live progress. Filter holders by `workspace_id`, and surface poll
failure ("run state unavailable") instead of presenting stale counts.

### Pitfall: never hold a lane slot without a deferred release

An interactive claim's returned ctx derives from the **caller's** ctx (never the lane
root) — otherwise `/assistant/cancel` stops reaching the run. Every path between a
successful claim and `handleAssistant` must `defer release()`; a granted slot without a
release stalls the whole lane. The preempted job's goroutine is joined before its slot
is handed to the claimant, so the chat never overlaps the unwinding automation.

### Model residency gate (external /v1 callers)

The same `runlane` gate arbitrates **model residency**, not just lane slots: the local
slot serves one model at a time, so a request for a different model would otherwise stop
the running server and kill any run using it. `LLMRuntimeManager.GetInstance` refuses the
switch when a run or inbound caller still holds the model (`llm.ErrLocalModelBusy`), and
the gate queues the caller instead. A caller that asks with `X-Queue-Wait: N` parks up to
`inbound_wait_seconds`; a caller with no header parks only when the host enabled
`inbound_wait_by_default` (default off — refused `429` + `X-LLM-Status: busy` +
`Retry-After`, explicit `X-Queue-Wait: 0` always refuses). Parked callers surface in the
run-activity panel
(Serve now / Dismiss) and can cancel out of band. Nothing evicts on a caller's behalf —
only the operator's promote (`POST /admin/api/queue/{key}/promote`) or the opt-in
`inbound_preempt` policy cancels the blocker, and the waiter is granted only after it
unwinds. While the gate holds an entry, the local lane suspends queued starts (or a
re-queued automation instantly re-takes the model ahead of the caller). Settings:
`inbound_wait_seconds`, `inbound_wait_by_default`, `inbound_max_queued`,
`inbound_preempt`. Full contract: SPEC-007 §V.1.
