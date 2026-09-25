---
id: SPEC-007
title: Automation Dispatcher
version: "1.2"
status: stable
last_updated: 2026-09-25
constitution_references: []
related_specs: [SPEC-001, SPEC-003, SPEC-005, SPEC-006]
supersedes: docs/PLANS/automation/automation-dispatcher-blueprint.md
---

# SPEC: Automation Dispatcher

## I. Intent

The automation dispatcher schedules, triggers, and manages autonomous task executions (runs).
It provides cron-based scheduling, manual triggers, workspace isolation, and live SSE streaming
of run events to the frontend.

## II. Functional Requirements

### 1. Scheduling

- `CronTrigger` — standard cron expressions for recurring automations.
- `IntervalTrigger` — fixed-interval scheduling.
- `ManualTrigger` — ad-hoc execution via API.
- All triggers are per-workspace.

### 2. Execution

- `LLMTaskExecutor` creates an `Agent` with the configured model and runs the task file content.
- Two strategies:
  - `IsolatedStrategy` — clean workspace each run (no state carryover).
  - `PersistentStrategy` — state continuity across runs (working directory persists).
- Task files are stored per-workspace with CRUD via the API.

### 3. Run Lifecycle

- A run starts when a trigger fires → creates `AutomationRun` with status `pending`.
- Transitions: `pending` → `running` → `completed` | `failed` | `cancelled`.
- Runs are tracked with: model, task file, start time, duration, LLM call count, tool call count,
  step count, output summary, error.
- Run artifacts are stored at `data/runs/{workspace}/{model}/{timestamp}_{session}/`.

### 4. SSE Streaming

- `/dispatcher/workspaces/{ws}/live` — SSE endpoint for per-workspace live events.
- Events: `step_start`, `tool_call`, `tool_result`, `stuck_detected`, `fallback_*`,
  `guardrails:blocked`, `guardrails:resolved`, `run_completed`, `error`.
- `Bus` in `internal/core/eventbus` manages per-workspace, per-channel pub/sub
  (`Subscribe`/`Publish`/`Unsubscribe`); `Sink` writes the same events to a per-run JSONL file.

### 5. Run Artifacts

Each run produces:
- `run-meta.json` — machine-readable metadata (model, status, steps, duration).
- `events.jsonl` — structured event stream (lifecycle events with timestamps). The in-memory
  `EventSink` is thread-safe, buffers writes and flushes per write, and fsyncs periodically
  (1s interval) plus once on `Close` — a crash loses at most one sync interval of events.
  In-RAM capture per run is bounded to the most recent 500 events (the full stream lives in
  `events.jsonl`); `recordRun` drops the older slice to bound memory under concurrent long runs.
- `recording.jsonl` — LLM request/response recordings (when `--record` is active).

## III. Error Handling

- Model not found: run fails with `ErrUnknownModel`.
- Model context maxed: agent sieves and retries.
- Agent times out: per-turn timeout is 10 minutes. The full automation run is bounded by
  `max(defaultAutomationTimeout (10m), the pinned model's timeout_minutes)` (`runContext`,
  `execution.go`), applied at dequeue for **all** trigger paths (cron, webhook, manual). The
  lane owns the run context, so the timeout starts when the run leaves the queue — not when it
  was triggered.
- Dispatch to stopped workspace: 404.

## IV. Configuration

- Per-workspace automation definitions in `config.yaml`:
  ```yaml
  automations:
    smoke-test:
      trigger: cron
      cron: "0 */6 * * *"
      model: gemma-4-4b-it
      task_file: llm-smoke-test.md
      strategy: isolated
  ```

## V. Run Scheduler (global-run-lane plan)

Every agent run — chat and automation — is admitted through the
`internal/core/runlane` scheduler before executing. Concurrency is a
**workload-class-aware, operator-configurable policy**:

- **Lanes** — two fixed lanes keyed by workload class (`local`, `cloud`),
  resolved via `AppServices.LaneKeyFor` (one `WorkloadClassifier` instance,
  shared with the client factory). Unresolvable models serialize on the local
  lane (protects the GPU).
- **Limits** — `settings.yml → scheduler`: `local_concurrency` (default 1,
  one GPU / one llama.cpp slot), `cloud_concurrency` (default 3),
  `preempt_automations` (default true). Applied live via the settings
  `OnChange` hook — no restart.
- **Queue-not-skip** — a scheduled automation that cannot start queues FIFO
  (position reported via `AutomationInfo.queued` / `queue_position` and the
  `/active-runs` `queued` list). At most one queued entry per
  `workspace/automation`: a fire while queued is absorbed (reported queued at
  its existing position, never an error), a fire while running becomes the
  single pending rerun. Cancelling a queued entry
  (`DELETE /dispatcher/queue/{ws}/{automation}` → `CancelQueued`) drops only the
  pending entry and never stops a running run. Deleted/renamed automations are
  re-resolved at dequeue and dropped.
- **Admission ordering** — the scheduler rejects a run with `ErrNotStarted`
  until `Start` has tethered it to the app root, so no run executes on an
  untethered context.
- **Preemption** — an interactive chat preempts a running automation in the
  same lane only (ctx marked `runlane.Preempted` then cancelled; ≤10 s grace;
  the run is recorded skipped, its failed tail history entry dropped, and an
  informational event published — never `EventError`). Scheduled runs re-queue
  at the front; manual runs are dropped.
- **Same-workspace serialization** — the per-workspace flock is now a waiting
  acquire (`acquireWorkspaceLock`, 250 ms retry, bounded by the run ctx): two
  same-workspace runs queue instead of discarding each other. A run whose ctx
  expires while waiting counts as skipped, never failed.
- **Lifecycle** — the scheduler is created in `BuildAppServices` (single
  composition root), started in `app.New` before anything admits runs, and
  closed in `App.Shutdown` after `dispatcher.Stop` and before
  `services.Shutdown`.
- **Durability** — lanes, queue, and the queued/preempted counters are
  in-memory; they reset on restart (skipped/preempted runs produce no persisted
  history entry, so `LoadHistory` cannot rebuild them).
- **Global run visibility** — lane state is **global** (`Scheduler.Snapshot()`
  takes no arguments), so exactly one route exposes it: `GET
  /admin/api/active-runs` with **no workspace path parameter**, returning
  `lane_holders` / `queued`. `GET /workspaces/{ws}/active-runs` carries **only**
  the workspace-scoped fields (`assistant_running`, `automation_running`,
  `assistant_conversation_id`, `assistant_queued`). The frontend polls the global
  route once (`useGlobalRunActivity`, 10 s) and shares that state between the
  always-visible header indicator (`RunActivityPill`) and the per-chat "waiting
  for a lane" hint — the latter filters holders by `workspace_id`, so it never
  names another workspace's run. The indicator reports "run state unavailable" on
  a failed poll instead of presenting the last known lists as live.
- **Trade-offs & caveats** (accepted, documented):
  - *The lane is claimed by the primary model.* If a chat's execution falls back to a model
    of the other workload class, the run keeps its original lane (conservative; may
    under-use the other lane) and is not re-claimed mid-run.
  - *`preempt_automations: false`* means a chat joins the FIFO behind a long automation
    instead of cancelling it; the wait is unbounded by design and the UI shows the waiting
    state (`assistant_queued`).
  - *A schedule shorter than the run time chains continuously* — coalescing prevents a
    backlog (one pending rerun), not a restart-every-time chain.
  - *Preemption discards in-flight automation work* (the run restarts from its task file);
    partial file writes from the cancelled attempt remain in the workspace — the same caveat
    as `StopAutomation`.
  - *Clear-runtime-data is blocked while work is queued*: the guard treats queued lane
    entries and lane-waiting chats as active, so a queued run is never silently dropped.
  - *Dequeue re-resolves existence only*, not the trigger's `ShouldRun` — dedupe/merge
    semantics stay simple; revisit if stale runs become a problem.
  - **Open:** whether "all agent runs are admitted through the run scheduler" becomes a
    Constitution II invariant is undecided; SPEC-007 §V is the current contract.

### V.1 Model residency — external /v1 callers are admitted, never evicting

The local slot serves **one model at a time** (`LLMRuntimeManager.activeModel`), so
serving a different model stops the running server — killing any run using it. That
eviction is now a **decided** act, not a side effect:

- **The gate** (`runlane` model gate) is the single decision point. A switch is
  allowed when nothing is active, when it is the same model, or when the caller is
  the holder; otherwise it is refused and the caller is queued behind the run that
  holds the model.
- **Callers** are the agent runs (holders carry the model they use) and external
  `/v1` callers (identified by a minted `inbound:<n>` key). An admitted inbound
  caller counts as a user of the model it requested, so two external callers cannot
  evict each other.
- **The manager refuses, the transport explains.** `GetInstance` returns
  `llm.ErrLocalModelBusy` for a refused eviction; the proxy handler answers
  `409` + `X-LLM-Status: busy`, or `202` + `queued` when a wait ended unserved, with
  a body naming the model in the way and `Retry-After`. A client asks to wait with
  `X-Queue-Wait: N`; without it the caller is refused rather than held.
- **Nothing is evicted on a caller's behalf.** Only the operator's
  `POST /admin/api/queue/{key}/promote` (and the opt-in `inbound_preempt` host
  policy, default off) cancels the blocking run — and the waiter is granted only
  after that run unwinds, so a model is never swapped under a live run.
- **Cancel** — a held caller may end its wait by closing the connection, or out of
  band with `DELETE /v1/queue/{key}`; the key is cryptographically random and is
  returned to the caller in the busy/queued answer. The operator's
  `POST /admin/api/queue/{key}/cancel` is the same drop behind admin auth.
- **Settings** (`settings.yml → scheduler`, live): `inbound_wait_seconds`
  (default 60; `0` refuses immediately, `-1` unbounded), `inbound_max_queued`
  (default 32, always enforced so an unbounded wait cannot hold unbounded
  connections), `inbound_preempt` (default false).
- **Held lane** — while the gate holds an entry the local lane suspends queued
  starts, so a preempted scheduled run is re-queued but does not restart ahead of
  the caller the operator promoted.
- **Client side** — a remote proxy's busy/queued answer is reported as its own
  upstream reason (`model_busy`) with the server's explanation, not retried as a
  transient fault.

#### V.1.1 Known limits (documented, not hidden)

- **Per-process.** The gate protects the proxy that receives the request. A second
  proxy is a client and cannot be made to respect this slot; a genuinely global
  router would need a distributed per-model lease (out of scope).
- **No ETA.** Busy/queued answers report the blocker and the queue position;
  nothing can predict when the running work ends.
- **`local_concurrency > 1` weakens run-vs-run protection** — two local runs can
  hold different models. The lane default is 1.
- **Cancel is unauthenticated on the public surface**, so the key is
  cryptographically random (`crypto/rand.Text`): a guessable key would let one
  caller drop another's wait.
