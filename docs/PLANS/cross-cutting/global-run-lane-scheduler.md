# Global Run Lane — Serialized Scheduling for Automations & Assistant Runs

**Status:** proposed — design pending acceptance
**Date:** 2026-09-18
**Related Specs:** SPEC-007 (Automation Dispatcher), SPEC-001 (Agent Loop), SPEC-003 (Discovery/AgentIde UI)

## Goal

Exactly **one agent run executes at a time, system-wide**. When a scheduled automation cannot start
because a run is already in flight, it is **queued** and executes after the current run finishes —
instead of being silently dropped. Interactive chat **preempts** a running automation (cancel +
re-queue at the front) so the user is never blocked behind a scheduled task.

### Decisions taken (operator-confirmed)

| Decision | Choice |
|---|---|
| Lane scope | **Global single lane** — one run at a time across all workspaces |
| Chat vs automation | **Chat preempts**: cancels the running automation and re-queues it at the front |
| Preemption cost | Accepted: in-flight automation work is discarded; the run restarts from scratch after the chat |
| Preemption hang | **Bounded**: preemption waits ≤10 s for the automation to stop; on timeout the chat gets an explicit busy error instead of hanging |
| Duplicate fires | **At most one queued entry per `workspace/automation`.** A fire while that automation is already queued is absorbed; a fire while it is *running* becomes its single pending next run (no backlog, no duplicates) |
| Durability | **In-memory only** — the pending queue is dropped on restart (consistent with today's lack of missed-run catch-up) |
| Delivery scope | Backend + full UI (queued badge, position, cancel-queued, waiting hints) |

## Problem — current behaviour

- Each cron job runs in its own `robfig/cron` goroutine: `scheduleAutomation` → `executeAutomation`
  (`backend/internal/core/automation/dispatcher.go:441-493`, `:527`). There is **no worker pool** —
  `workerCount` / `WithWorkerCount` (`dispatcher.go:59`, `:139-145`; set at `bootstrap.go:505`) is
  dead code, never read.
- The only concurrency gate is a per-workspace **non-blocking** flock: `TryAcquireLock`
  (`dispatcher.go:553-558` → `platform/persistence/workspace.go:76-86`). On contention the run is
  **skipped, not queued** (`"automation skipped (workspace locked)"`, counted as
  `SkippedExecutions`).
- Chat never takes that lock, so an automation and a chat run can execute **simultaneously in the same
  workspace**. Chat has its own `running sync.Map` handler field
  (`assistant_handlers.go:41`) and *cancels the prior chat run*
  (`cancelPriorForWorkspace` `:94-110`).
- Manual trigger returns **409 Conflict** when the persisted workspace state says a run is active
  (`dispatcher_handlers.go:173-177`, keyed off `state.IsRunning()` = `ActiveAutomation != ""` — not
  the flock).
- The only "what is running" surface is per-workspace `GET /admin/api/workspaces/{ws}/active-runs`
  (`active_runs_handlers.go`), polled every 10 s by `useRunningActivity.ts`.

**Net effect today:** overlapping runs are lost, and automation-vs-chat is uncoordinated.

## Design

### 1. New primitive — `backend/internal/core/runlane/lane.go`

A self-contained gate + FIFO queue with a single runner goroutine. It imports neither `automation`
nor `assistant` (no cycle risk, trivially testable).

```go
package runlane

type Kind string
const (KindAutomation Kind = "automation"; KindInteractive Kind = "interactive")

var (
    ErrAlreadyQueued  = errors.New("already queued")
    ErrClosed         = errors.New("run lane closed")
    ErrPreemptTimeout = errors.New("preempted run did not stop in time")
)

type Disposition string
const (DispositionStarted Disposition = "started"; DispositionQueued Disposition = "queued")

type Job struct {
    Key         string // "<workspace>/<automation>" — dedupe + cancel handle
    WorkspaceID string
    Automation  string
    Label       string
    Run         func(ctx context.Context) error
}

func New() *Lane
func (l *Lane) Start(ctx context.Context)         // idempotent (sync.Once); spawns the runner
func (l *Lane) Close(ctx context.Context) error   // cancel holder, stop runner, reject new work

func (l *Lane) Submit(j Job) (Disposition, error) // ErrAlreadyQueued when Key is already queued
func (l *Lane) Cancel(key string) bool            // drop queued entry and/or cancel running ctx

// ClaimInteractive preempts a running automation, then blocks until the lane is
// free; holds it until release(). Waiting chats are FIFO among themselves.
// Preemption waits at most preemptGrace (10 s) for the automation to stop; on
// timeout it returns ErrPreemptTimeout and leaves the lane untouched. A waiter
// whose ctx is cancelled removes itself from the waiters before returning —
// release is only produced on success.
func (l *Lane) ClaimInteractive(ctx context.Context, workspaceID, conversationID string,
) (runCtx context.Context, release func(), err error)

func (l *Lane) Snapshot() Snapshot   // read model for the API/UI
func Preempted(ctx context.Context) bool // ctx value set when a lane preemption fired

type Snapshot struct {
    Holder *Holder // nil when idle
    Queued []Entry // global FIFO order
}
type Holder struct { Key, Kind, WorkspaceID, Automation, Label string; Since time.Time }
type Entry  struct { Key, WorkspaceID, Automation, Label string; Position int; QueuedAt time.Time }
```

**Run rules (owned in one place):**

1. A single runner goroutine pops the front of the queue only when `running == nil`, no interactive
   holder exists, and the lane is not closed. It executes `job.Run(runCtx)`, where `runCtx` derives
   from the lane root context and carries the preemption marker.
2. **Invariant: the queue never contains two entries with the same `Key`.** `enqueueLocked` dedupes,
   and the preempt re-insert uses the same helper — so a fire that landed while the automation was
   running *merges* into the re-queued entry rather than duplicating it.
3. **Interactive claims jump the queue.** They preempt only a running *automation* (mark its ctx
   preempted → cancel → wait up to `preemptGrace` for the runner to clear `running`); they never
   preempt another chat. Chat-vs-chat is FIFO. On grace timeout the claim fails with
   `ErrPreemptTimeout` — the mutual-exclusion invariant is never broken (the automation either stops
   and frees the lane, or keeps holding it; the chat never overlaps it). A waiting claim whose ctx is
   cancelled detaches itself from the waiter list before returning (no ghost queue positions).
4. The runner **re-inserts a preempted job at the front** before releasing the lane, so the automation
   restarts as soon as the chat finishes.
5. **Panic containment:** the runner wraps `job.Run` in `recover()` (reuse `platform/safe`
   conventions) — a panic logs, clears `running`, and continues, so a lease is never leaked.
6. `Disposition` (started vs queued) and the 1-based queue position are computed **inside the same
   critical section as the enqueue** — never from a separate `Snapshot()` read (racy off-by-one).

### 2. Automation dispatcher on the lane — `internal/core/automation/dispatcher.go`

- Add a **required** `lane *runlane.Lane` parameter to `NewDispatcher` — no default, no hidden
  private lane; a missing injection must fail loudly, not silently lose the global guarantee. Tests
  pass `runlane.New()`. While here, delete the dead `workerCount` / `WithWorkerCount` code
  (`:59`, `:139-145`; set at `bootstrap.go:505`) — Constitution IV.4.
- `scheduleAutomation.jobFunc` (`:452-481`): after the existing `ShouldRun` gate, **submit instead of
  execute**:

  ```go
  key := runKey(entry)
  _, err := d.lane.Submit(runlane.Job{
      Key: key, WorkspaceID: entry.Workspace, Automation: entry.Name,
      Label: entry.Workspace + "/" + entry.Name,
      Run:   func(ctx context.Context) error { return d.executeAutomation(ctx, entry, "") },
  })
  if errors.Is(err, runlane.ErrAlreadyQueued) { return } // fire absorbed — not an error
  ```

- `Trigger` (`:302-314`) submits the same job and returns a new `automation.TriggerResult`
  (`{Status: "started"|"queued", Position int}`) so the HTTP/webhook callers can report the
  disposition. `ErrAlreadyQueued` maps to `queued` + the existing entry's position — never an error.
- `executeAutomation` (`:527-686`):
  - **Preemption is not a failure.** In the error branch (`:635-663`), when
    `runlane.Preempted(execCtx)`: drop the failed tail `History` entry the executor appended for
    this run (the executor mutates the shared state pointer mid-run and `RecordActivity` propagates
    it at `:656-659`), clear running + `WriteState`, count as **skipped**
    (`RecordExecution(false, true, elapsed)`), publish an informational `EventMessage` ("Run paused
    for an interactive request — it will restart when the lane is free") **instead of** `EventError`,
    and return `nil`. Extract this into a small helper method so `executeAutomation` doesn't grow
    its grandfathered complexity debt (`knownComplexityExceptions`,
    `tools/check-complexity/main.go:60`).
  - Keep `state.SetRunning(entry.Name)` where it is: `ActiveAutomation` keeps meaning *actually
    executing*, while "queued" is reported from the lane. `AutomationInfo.IsRunning` stays honest.
  - **Flock contention should no longer drop a queued run.** Replace the drop-on-contention
    `TryAcquireLock` (`:553-558`) with a bounded, ctx-aware retry helper — new
    `persistence.AcquireLockWithRetry(ctx, workspaceID, 250*time.Millisecond, 5*time.Second)`, so a
    transient `config.yaml` write (`AcquireLock`, `workspace.go:63-73`) no longer discards the run.
    Final failure keeps today's skipped-metric behaviour.
- `StopAutomation` (`:784-842`): additionally drop the workspace's queued entries by calling
  `lane.Cancel(key)` for every `Snapshot().Queued` entry of that workspace — "Stop" means stop the run
  *and* clear pending. Keep the existing force-kill diagnostic goroutine tethered to a cancellable
  ctx (Constitution II.14).
- New on `Dispatcher`: `LaneSnapshot() runlane.Snapshot` and
  `CancelQueued(workspaceID, automationName string) error` (error when nothing matched).
- `DispatcherMetrics` (`:158-165`): add `QueuedExecutions` and `PreemptedExecutions`, and include them
  in the `LoadHistory` reset-and-rebuild (`:398-405`, recount at `:407-415`) so they stay consistent
  across a restart.

### 3. Assistant integration + wiring

- `handlers.AssistantService` (`internal/transport/http/handlers/services.go:210-235`): add
  `Lane() *runlane.Lane`. Implement on `*AppServices` (`bootstrap.go:264-280`) and on
  `mocks.MockAssistantService` (`internal/testing/mocks/assistant_service.go`).
- `AssistantMessageHandler` (`assistant_handlers.go:31-62`, `running sync.Map` field at `:41`): new
  `lane` field from `service.Lane()` (nil-tolerant — behaves exactly as today when nil). Add
  `granted atomic.Bool` to `runningAgent` (`:44-48`).
  - `RunWithCancel` (`:315-334`): after `h.running.Store(...)` (so `cancelPriorForWorkspace` still
    works while waiting) and **before** `return h.handleAssistant(...)`:

    ```go
    runCtx := execCtx
    if h.lane != nil {
        claimed, release, err := h.lane.ClaimInteractive(execCtx, workspaceID, conversationID)
        if err != nil {
            // ctx cancelled while waiting, or ErrPreemptTimeout → same shape as the
            // existing cancel path: clean informational response, no failed-run artifacts.
            return
        }
        defer release()
        ra.granted.Store(true)
        runCtx = claimed
    }
    ```

    Waiting holds the HTTP request exactly the way a running chat already does (the exec ctx is
    intentionally detached from client disconnect at `:325`), so no new plumbing — the UI learns
    about waiting via `assistant_queued` on the 10 s poll.

  - New `RunningQueued(workspaceID) bool` = "a running entry exists that has not been granted the
    lane yet", so the UI can show *waiting* instead of a running glow.
- **Centralization (AGENTS.md):** create the single lane in `Container.BuildAppServices`
  (`bootstrap.go:64-150`), store it on both `Container` and `AppServices`, pass it into
  `BuildDispatcher` (`bootstrap.go:496-512`) into the required `NewDispatcher` lane param, and start
  it explicitly in `app.New` (`app.go:106-115`) alongside `go disp.Start(ctx)`. In `App.Shutdown`
  (`app.go:33-59`) call `lane.Close(ctx)` **after** `dispatcher.Stop` and **before**
  `services.Shutdown`, so the run context is cancelled and the runner exits before the shell pool /
  egress proxy are torn down.
- `webhook_handlers.go:127` — update for the new `Trigger` signature (ignore the disposition).

### 4. API contract

- `ActiveRunsResponse` (`active_runs_handlers.go:14-22`) += `assistant_queued bool`,
  `queued []runlane.Entry`, `lane *runlane.Holder`. Wiring follows the handler's existing
  decoupled-func pattern: `NewActiveRunsHandler` gains `assistantQueued func(ws string) bool` and
  `laneSnapshot func() runlane.Snapshot` (`app/routes.go:40-44`).
- `AutomationInfo` (`dispatcher_handlers.go:106-122`) += `Queued bool` / `QueuePosition int`,
  populated in `ListAutomations` (`:124-165`) from one snapshot per request.
- `TriggerAutomation` (`:167-196`): **remove the 409 path**, respond `202` with
  `{"status":"started"|"queued","position":n}`.
- New route in the dispatcher block (`app/routes.go:193-218`):
  `DELETE /admin/api/dispatcher/queue/{workspace}/{automation}` → `CancelQueued` → `204` / `404`.
- Extend the handlers' `Dispatcher` interface (`dispatcher_handlers.go:35-47`) with
  `Trigger(...) (automation.TriggerResult, error)`, `CancelQueued(...) error`,
  `LaneSnapshot() runlane.Snapshot`; update the `testDispatcher` fake
  (`dispatcher_handlers_test.go:20-42`).
- Queue updates ride the **existing 10 s `/active-runs` poll** — no new SSE plumbing. Automation
  actions already call `fetchAutomations()` afterwards, so a badge appears immediately after a
  trigger.

### 5. Frontend

| File | Change |
|---|---|
| `frontend/src/types/assistant.ts:85-92` | extend `ActiveRunsResponse` with `assistant_queued`, `queued`, `lane` |
| `frontend/src/types/dispatcher.ts:20-37` | add `is_queued?`, `queue_position?` to `Automation` |
| `frontend/src/services/automation/dispatcherService.ts` | `cancelQueued(workspace, automation)` → `del(...)` |
| `frontend/src/constants/api.ts:30` | `cancelQueued` route builder beside `activeRuns` |
| `frontend/src/composables/assistant/useRunningActivity.ts:14-55` | new singleton refs `assistantQueued`, `queuedRuns`, `laneHolder` |
| `frontend/src/composables/automation/useDispatcher.ts:73-95` | `cancelQueued` action → service + `fetchAutomations()` + `useAppBanner()` (mirror `stopAutomation`); `triggerAutomation` surfaces the 202 `queued` + `position` immediately ("Queued #n") instead of silence |
| `frontend/src/components/AgentIde/automation/AutomationsPanel.vue:55-61` | `Queued #n` badge (new `.status-queued`, reuse the `pulse-dot` pattern) + cancel button via `InlineConfirm` (pattern at `:85-92`) |
| `frontend/src/components/AgentIde/common/RightPane.vue:39-64` | hint when the lane holder belongs to another workspace ("Waiting for `<label>`") |
| `frontend/src/components/AgentIde/AgentIde.vue:83` | destructure `assistantQueued`; show "Waiting for `<holder>`" instead of the running glow |

## Files to change (backend)

| File | Change |
|---|---|
| `backend/internal/core/runlane/lane.go` | **NEW** — lane primitive (queue, runner, preempt, snapshot) |
| `backend/internal/core/runlane/lane_test.go` | **NEW** — unit tests |
| `backend/internal/core/automation/dispatcher.go` | required `lane` param, submit-not-execute, preempt handling (bounded grace + history-cleanup helper), `CancelQueued`, `LaneSnapshot`, flock retry, metrics; dead `workerCount` removed |
| `backend/internal/core/automation/dispatcher_test.go` | queue / preempt / dedupe / stop-clears-queue tests |
| `backend/internal/core/automation/registry.go` | `runKey(entry)` helper (or co-locate in `dispatcher.go`) |
| `backend/internal/platform/persistence/workspace.go` | `AcquireLockWithRetry` |
| `backend/internal/transport/http/handlers/services.go` | `AssistantService.Lane()` |
| `backend/internal/transport/http/handlers/assistant_handlers.go` | lane claim + `RunningQueued` + `granted` flag |
| `backend/internal/transport/http/handlers/active_runs_handlers.go` | queued fields + new func deps |
| `backend/internal/transport/http/handlers/dispatcher_handlers.go` | `AutomationInfo` queued fields, trigger 202, cancel-queued handler, interface |
| `backend/internal/transport/http/handlers/webhook_handlers.go` | `Trigger` signature |
| `backend/internal/app/bootstrap.go` | create + store + inject the lane; `AppServices.Lane()`; dead `WithWorkerCount` call removed |
| `backend/internal/app/app.go` | start lane in `New`, close in `Shutdown` |
| `backend/internal/app/routes.go` | cancel-queued route; `ActiveRuns` wiring |
| `backend/internal/testing/mocks/assistant_service.go` | `Lane()` |

## Tests

- **`runlane/lane_test.go`** (table-driven): mutual exclusion; FIFO order; dedupe (`Submit` twice →
  `ErrAlreadyQueued`); submit-while-running merges into one pending entry; interactive jump-ahead;
  preempt cancels the running ctx, marks `Preempted(ctx)`, re-inserts at the front, and starts after
  release; preempt grace timeout → `ErrPreemptTimeout` with the lane invariant untouched; `Cancel`
  removes queued / cancels running; `Snapshot` positions; a waiting `ClaimInteractive` honours `ctx`
  cancellation and leaves no ghost waiter; the runner survives a panicking `Job.Run`; `Close(ctx)`
  unblocks waiters with `ErrClosed`.
- **`automation/dispatcher_test.go`** (existing conventions — `mockExecutor` at `:17-27`, a real
  `persistence.NewWorkspaceManager` over temp dirs, `logging.NewNopLogger()`, setup at `:29-59`;
  construct the dispatcher with `runlane.New()`): two overlapping fires → one executes, one is
  queued; a preempted run records **skipped**, drops the failed tail `History` entry, and emits no
  `EventError`; `Trigger` reports started vs queued (already-queued → `queued` + position, not an
  error); `StopAutomation` clears the queue.
- **`handlers`**: `active_runs_handlers_test.go` for the new fields; `dispatcher_handlers_test.go` for
  the 202 trigger response and the cancel-queued route.
- **Frontend** (`npm test`, vitest; tests live in `src/__TESTS__/`): `useRunningActivity` queued refs
  and the `cancelQueued` / queued-trigger-feedback actions in `useDispatcher` (composable tests —
  there is no existing services-layer test pattern; don't invent one for a thin `del()` wrapper).

## Docs

- `docs/SPECS/automation-dispatcher.md` (SPEC-007): new section — lane semantics (single global lane,
  queue-not-skip, dedupe invariant, preemption + restart-from-scratch, cancel, in-memory durability).
- `docs/PLANS/README.md` + `docs/INDEX.md`: the rows already exist (`:56` / `:86`) — flip the status
  from *proposed* to *implemented* on acceptance.
- `.agents/skills/automation/SKILL.md`: "Run Lane" section (queue rules + the "never hold a lease
  without a deferred release" pitfall).
- `docs/architecture.md`: one-line pitfall pointer.

## Implementation order

1. **P1 — `runlane` primitive** + tests. No behaviour change yet.
2. **P2 — automation serialization**: dispatcher submits to the lane, preemption handling (bounded
   grace + history-cleanup helper), flock retry, `CancelQueued`/`LaneSnapshot`, metrics, required
   lane param, dead `workerCount` removal. (Automation-vs-automation only; chat still uncoordinated.)
3. **P3 — assistant + wiring**: `Lane()` on `AssistantService`/`AppServices`/mocks, claim + preempt in
   `RunWithCancel`, `RunningQueued`, lane creation/start/close in `BuildAppServices`/`app.New`/
   `App.Shutdown`.
4. **P4 — API**: `/active-runs` fields, `AutomationInfo` queued fields, cancel-queued route, trigger
   response, interface + fake updates.
5. **P5 — frontend**: types, service, composables, panel badge + cancel, waiting hints.
6. **P6 — docs**: SPEC-007, flip the existing plan registry rows, skill, architecture pointer.

Each phase ends with its own build + test gate.

## Verification

```bash
cd backend && go build ./... && go test ./... && go run ./tools/check-complexity/   # complexity ≤ 12
cd frontend && npm test && npm run build
```

Manual scenario (local model, two workspaces):

1. Automation A on a slow task (`@every 1m`) and automation B (`@every 1m`) in a second workspace →
   A runs while B shows **Queued #1**; when A finishes, B starts. A re-firing while running does not
   add a second pending entry.
2. Send a chat message while A is running → A stops, produces **no** failure and no new `History`
   entry, is re-queued at the front, chat runs; when the chat ends, A restarts.
3. Cancel B while queued (panel button) → 204 from
   `DELETE /admin/api/dispatcher/queue/{ws}/{automation}`, badge clears.
4. Restart the service with a queued run → queue is empty and no stale "running" state (the existing
   `Dispatcher.Start` reset at `dispatcher.go:194-199` still applies).

## Out of scope

- Persisting the queue across restarts (explicitly deferred by the operator).
- Per-model / per-workspace lanes, concurrency > 1, priorities beyond interactive-vs-automation.
- Missed-run catch-up for ticks that occurred while the service was down.
- Preempting interactive runs in favour of other interactive runs (chat-vs-chat stays FIFO).
- Any change to run resumability: a preempted automation restarts from its task file.

## Risks / trade-offs

- **Cross-workspace blocking:** a 30-minute chat run (`AgentGlobalTimeout`,
  `assistant/agent.go:42`) blocks automations in every workspace. That is the chosen model; a future
  model-aware lane (one lane per model: local serialized, cloud parallel) is the natural escape hatch
  and is the main follow-up if this proves too coarse.
- **Preemption discards automation progress** (accepted): a restarted automation re-reads its task
  file and re-runs from the top; partial file writes from the cancelled attempt remain in the
  workspace — the same caveat that already applies to the existing `StopAutomation`.
- **A leaked lease is a global stall.** Mitigations: `defer release()` on every path, panic recovery
  inside the runner, bounded preemption grace (`ErrPreemptTimeout` — a wedged automation can delay a
  chat by ≤10 s, never hang it), `Lane.Snapshot().Holder` exposed for diagnosis, and `Close(ctx)` on
  shutdown.
- **Schedules shorter than the run time chain continuously** (coalescing prevents a backlog, not a
  restart-every-time chain). Documented in SPEC-007.
- `assistant_running` remains `true` while a chat is *waiting* for the lane; consumers must read
  `assistant_queued` to distinguish — addressed by the P5 UI.

## Open decisions

1. **Lane scope refinement:** ship global (chosen) vs one-lane-per-model from the start. Consider
   leaving a documented seam (`Lane` keyed by a lane ID) so per-model lanes are a config change later.
   *(Recommendation: keep v1 simple — one global lane, revisit only if cross-workspace blocking
   actually hurts.)*
2. **Preempt vs yield knob:** add `lane.preempt_automations` (default `true`) so the operator can
   switch to "chat jumps the queue but never cancels" without a code change?
   *(Recommendation: no knob in v1 — one behaviour, tested and documented.)*
3. **Queued visibility latency:** accept the 10 s `/active-runs` poll, or emit a lane event on the
   existing `EventBus` for instant updates?
   *(Recommendation: keep the poll — no new plumbing for a badge.)*
4. **`ShouldRun` re-check at dequeue:** currently a queued run executes unconditionally. Re-evaluate
   the trigger when it is dequeued (drop a queue entry that is no longer due)?
   *(Recommendation: defer — unconditional dequeue keeps the dedupe/merge semantics simple; document
   in SPEC-007.)*
