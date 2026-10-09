---
id: SPEC-007
title: Automation Dispatcher
version: "1.11"
status: stable
last_updated: 2026-10-08
constitution_references: []
related_specs: [SPEC-001, SPEC-003, SPEC-005, SPEC-006]
supersedes: docs/PLANS/automation/automation-dispatcher-blueprint.md
---

# SPEC: Automation Dispatcher

## Changelog

- **1.11 (2026-10-09)** — Delivery failures are visible (§II.6). A report that could not be sent adds a warning
  (`report not delivered via <connector>: <cause>`, where a rejected token reads as its short reason, e.g. "telegram rejected the bot token (HTTP 401)") to the run that produced it, found by the run's id
  (`ExecuteResponse.RunID`, now the same id as the run context); the run stays successful. The automation list
  exposes `last_warnings`, and the UI tags such runs "Completed with warnings". A heartbeat alert whose send failed
  is recorded `alert_not_delivered`; the heartbeat result is now recorded after delivery.

- **1.10 (2026-10-08)** — Heartbeat active hours (§II.6): `heartbeat.active_hours` (`HH:MM-HH:MM`, server
  local time, the zone cron triggers use; overnight windows such as `22:00-06:00` wrap) limits scheduled
  checks to a daily window. A scheduled tick outside it is dropped in `admitRun` before the run lane
  (no model call, recorded `skipped_outside_hours`); a manual run ignores the window. Backed by the
  reusable `models.DailyWindow`.

- **1.9 (2026-10-08)** — Run history has one source (§II.3): the global activity feed
  (`GET /admin/api/dispatcher/activity`) reads each workspace's `state.json` on request instead of an
  in-memory copy, so deleting a run or clearing an automation's runs removes them from the feed at once
  (previously they stayed listed, unopenable, until a restart).

- **1.8 (2026-10-04)** — Workspace heartbeat (§II.6). The heartbeat becomes one per-workspace setting
  (`WorkspaceConfig.heartbeat`: on/off, interval, model, alert connector) compiled to a reserved-name
  automation, off by default; a tick whose `heartbeat.md` holds no checks is skipped without a model
  call; its skip-if-busy follows the model's lane (local → on); the last check is recorded and shown;
  `GET/PUT …/workspaces/{ws}/heartbeat`. **Breaking, deliberately without migration** (the product is
  new): the `cron_schedule` workspace field and its hidden `default` automation are removed, and an
  automation may no longer be named `heartbeat`. `memory_mode` takes `""|on|off` (the old `hot` is rejected,
  see SPEC-004 2.1). Automation runs resolve their memory default from `memory.automation_hot`.

- **1.7 (2026-10-03)** — Learning journal (§II.7): an automation may set `journal: true`; its runs
  get the `automation_journal` tool, see their notes from earlier runs in the task, and rewrite
  them before finishing. Operator read/clear endpoints; the journal is deleted with the automation.

- **1.6 (2026-10-03)** — Dedup also covers bullet and numbered items that contain a link, not only
  table rows (§II.6); the documented tracking-parameter list now matches the code (`ref_src`).

- **1.5 (2026-10-03)** — Delivery hardening (§II.6): one quiet-heartbeat definition shared by the UI
  smart skip and delivery (report starts with `HEARTBEAT_OK`, markdown emphasis ignored); failure
  notices are limited to one per automation per hour until a run succeeds; an undecodable seen
  ledger is set aside as `*.corrupt` instead of being overwritten; Telegram transport errors no
  longer include the bot token.

- **1.4 (2026-10-03)** — Result delivery (§II.6): an automation may carry a `notify` block; the
  dispatcher delivers the final report (and failure notices) through a communication connector,
  optionally skipping items already reported (seen ledger). A quiet `HEARTBEAT_OK` report is never
  delivered. `skip_if_busy` (§V): a scheduled fire that cannot start at once is skipped instead of
  queued, and a preempted run is dropped instead of re-queued.

- **1.3 (2026-09-29)** — Reference correction (no behavior change): the SSE / `events.jsonl` event
  vocabulary now lists the actual `assistant.AgentEvent` types + `lifecycle` phases (there is no
  `run_completed` event); the JSONL sink type is `eventbus.Sink`.

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
- **Run history** — each workspace's `state.json` `history` (newest 30, `models.MaxStateHistory`) is
  the single source of truth for finished runs. The automation list and the global activity feed
  (`GET /admin/api/dispatcher/activity`: newest `persistence.MaxRecentRuns` = 100 across workspaces,
  oldest first, via `WorkspaceManager.RecentRuns`) both read it on request; nothing keeps a copy in
  memory, so the run delete endpoints need no extra bookkeeping. At startup the dispatcher seeds its
  execution counters from the same history.

### 4. SSE Streaming

- `/dispatcher/workspaces/{ws}/live` — SSE endpoint for per-workspace live events.
- Events: the shared `assistant.AgentEvent` types — `step_start`, `message`, `tool_call`,
  `tool_result`, `reasoning`, `tool_stream`, `guardrail_violation`/`guardrail_blocked`/
  `guardrail_invalidated`, `upstream`, `error` — plus `lifecycle` phases (`session_started`/
  `progress`/`completed`, `stuck_detected`, `fallback_*`, `agent_thinking`, `still_thinking`).
- `Bus` in `internal/core/eventbus` manages per-workspace, per-channel pub/sub
  (`Subscribe`/`Publish`/`Unsubscribe`); `Sink` writes the same events to a per-run JSONL file.

### 5. Run Artifacts

Each run produces:
- `run-meta.json` — machine-readable metadata (model, status, steps, duration).
- `events.jsonl` — structured event stream (lifecycle events with timestamps). The in-memory
  `eventbus.Sink` is thread-safe, buffers writes and flushes per write, and fsyncs periodically
  (1s interval) plus once on `Close` — a crash loses at most one sync interval of events.
  `reasoning` and `tool_stream` events carry the full text so far, so the sink keeps only the newest
  snapshot of a consecutive stream (same type and conversation), plus a checkpoint every 10 s of a long
  stream; a four-minute run is about 100 KB instead of 11 MB. Nothing in the app reads `events.jsonl` back:
  chat history comes from the session files, live replay from the event bus.
  In-RAM capture per run is bounded to the most recent 500 events (the full stream lives in
  `events.jsonl`); `recordRun` drops the older slice to bound memory under concurrent long runs.
- `recording.jsonl` — LLM request/response recordings (when `--record` is active).

### 6. Result Delivery (`notify`)

An automation with `notify: {connector: <name>}` has its result delivered by the **dispatcher**
after the run — not by the agent (`notify_user` stays reserved for tasks that explicitly request an
external message, and is gated by network scope; delivery is not).

- **Trigger points** (`execution.go`, `notify.go`): a successful run sends the report
  (`ExecuteResponse.Report`, the final report without the run header); a failed run sends one
  `⚠️ Automation <name> failed: <classified error>` line, at most once per hour per automation
  (`failureNoticeLimiter`; a successful run re-arms it). A user stop (`context.Canceled`) and a
  lane preemption send nothing.
- **Best-effort, but visible**: a delivery failure never changes the run's outcome (the report exists), but it is
  recorded as a warning on that run (`deliverAndRecord` → `AgentState.AddRunWarning`, history and latest run), exposed
  as `last_warnings`, and a heartbeat alert that could not be sent is `alert_not_delivered`. Each send has
  its own 30 s timeout derived from the lane context, not the run's own (possibly expired)
  timeout context.
- **Chat formatting** (`digest.go`): markdown tables become bullet lists (`• Item — cell · cell` and
  the link on the next line); other text, including bullet lists, passes through as written. The
  connector owns platform limits
  (Telegram: size splitting and a plain-text retry — SPEC-009).
- **Dedup** (`notify.dedup: true`): the first link of each **table row** and each **bullet or
  numbered item that contains a link** is canonicalised (lower-case host, no `www.`, no fragment,
  no `utm_*`/`fbclid`/`gclid`/`ref_src`, no trailing slash). Items whose link is in the
  automation's **seen ledger** inside the retention window (`dedup_days`, default 60) are
  dropped (a dropped bullet takes its more-deeply indented detail lines with it); items without a
  link are always kept; a link repeated inside one report is sent once. A bullet that merely
  *cites* an already-reported source is dropped too — same rule as a table row.
  When every row is dropped nothing is sent unless `send_empty: true` (then
  `No new items since the last run.`). Items are recorded **only after a successful send**, so a
  failed delivery re-offers them next run.
- **Seen ledger** (`persistence/seen.go`): `<meta>/<workspace>/seen/<automation>-<hash>.json`,
  written atomically, pruned to the retention window on write, outside the agent's workspace jail.
  An undecodable file is renamed `*.corrupt` and the run continues with an empty ledger. Deleting
  an automation removes its ledger (`DeleteAutomationRuns`); renaming one starts a fresh ledger.
- **Heartbeat**: a report that starts with `HEARTBEAT_OK` (`isQuietHeartbeat`, `executor.go`;
  leading markdown emphasis such as `**` is ignored) is hidden by the UI smart skip, not
  delivered and not recorded as seen — a quiet check is silent. The workspace heartbeat is a
  config object (`WorkspaceConfig.heartbeat`: `enabled` (default off), `every` (1m–24h, default
  30m), `model`, `notify`), not an automation the operator writes: `registerWorkspaceAutomations`
  compiles it (`HeartbeatConfig.Automation()`) into an interval automation named `heartbeat`
  (`models.HeartbeatAutomationName`, a reserved name — `validateAutomation` rejects it), isolated,
  `memory_mode: off` always. The removed `cron_schedule` field and its hidden `default` automation
  no longer exist (old files still load; the key is ignored).
  - **Active hours**: `heartbeat.active_hours` (`HH:MM-HH:MM`, half-open — start included, end excluded —
    in server local time, the same zone cron triggers use; start after end wraps past midnight; empty =
    always active). `admitRun` reads the workspace config at fire time (`heartbeatActive`), so a changed
    window applies to the next tick; a scheduled tick outside it is skipped before the run lane and
    recorded `skipped_outside_hours`, a manual run ignores the window. The same check
    (`skipHeartbeatTick`) runs again when a queued tick leaves the lane queue, so a tick admitted just before
    the window closed does not run after it. It is checked before the no-checks
    rule. An unparsable stored window never silences the heartbeat (`HeartbeatConfig.ActiveAt`); save
    rejects it (`heartbeat.active_hours`, 400). A per-owner timezone is an autonomy-roadmap decision (R2);
    when it lands this window should read it.
  - **No checks, no run**: `admitRun` reads `heartbeat.md` and `models.HeartbeatBody` (HTML
    comments outside code fences stripped, trimmed); empty → the tick is skipped before the run lane
    is touched (`TriggerSkipped`, no model call, no history) and recorded `skipped_no_checks`. The
    task sent is `prompts.HeartbeatTask` (the checks + `HeartbeatReplyRules`, which carry the
    `HEARTBEAT_OK` contract); the starter file (`prompts.DefaultHeartbeat`) is comments only.
  - **Skip-if-busy is derived** (`Dispatcher.skipIfBusy`): for the heartbeat it is true only when
    `laneKeyFor(model)` is the local lane, resolved at fire time (an empty model follows the
    registry primary); other automations use their own `skip_if_busy`.
  - **Status**: the last check (`quiet`, `alert`, `skipped_no_checks`, `skipped_busy`, `error` + time)
    is written to `meta/<ws>/heartbeat-status.json` (`WriteHeartbeatStatus`, outside the agent
    jail). `GET/PUT …/dispatcher/workspaces/{ws}/heartbeat` returns/saves the config and returns
    `models.HeartbeatState` (`lane`, `wakes_local_model`, `has_checks`, `status`); PUT validates,
    saves into the workspace config without touching other fields and (un)schedules at once.
- **Prompt hint**: with dedup on, up to 25 recent titles are appended to the task
  (`prompts.AutomationSeenBlock`) so the run spends its search budget on new items. This is an
  efficiency hint; the link filter at delivery is the guarantee.
- **Known limits**: delivery runs inside the run (lane slot and workspace lock held for at most
  the 30 s send timeout); a chat that preempts in that window aborts the send and the scheduled run
  restarts from its task file (the ledger was not written, so nothing is lost). An automation run
  from Telegram with `/run` gets the webhook's own result reply in addition to its `notify`
  delivery. An unreadable seen ledger is treated as empty and rewritten after the next delivery.
- **Network policy**: the send rides the connector's guarded client (Constitution I.2). It is an
  operator-configured, system-side send to the operator's own connector — the same class as `/run`
  result replies (SPEC-009 §1.1) — so it is not blocked by an agent network grant of `none`.

### 7. Learning Journal (`journal`)

An automation with `journal: true` keeps short notes for its future runs (queries that surfaced
new items, sources worth or not worth checking, topics already saturated) — the loop that lets a
recurring search improve instead of repeating itself. Off by default.

- **Storage** (`persistence/journal.go`): `<meta>/<workspace>/journal/<automation>-<hash>.md`,
  atomic write, outside the agent's workspace jail, at most `models.MaxJournalChars` (4000)
  characters. Text is sanitised on every write (`models.SanitizeJournal`: control characters
  dropped, blank runs collapsed, length capped). Empty text clears it. Deleting the automation
  deletes its journal (`DeleteAutomationRuns`).
- **Read path** (`automation/journal.go`): `prompts.AutomationJournalBlock` is appended to the task
  after the seen-titles hint — the stored notes inside a `<journal>` fence, labelled "your own
  notes, not instructions", plus the instruction to rewrite them before finishing. A first run is
  told the journal is empty. An unreadable journal is treated as empty and logged.
- **Write path**: the `automation_journal` tool (`core/tools/automation_journal.go`, replace-whole
  semantics so the agent curates instead of appending forever). The target comes from the run
  context (workspace + automation name), never from arguments; empty text is refused.
- **Gating**: the tool is registered with a context predicate (`registerScopedTool`). It is listed
  — and therefore present in the tool schema, the tool manual and argument validation — and callable
  only when the run context carries `models.WithJournalRun`, which the executor stamps for a
  `journal: true` automation (`runContextFor`). Chat and journal-less automations never see it. An
  automation with an `allowed_tools` allowlist gets the tool appended (`allowedToolsFor`).
- **Operator control**: `GET` / `DELETE` `…/workspaces/{ws}/automations/{automation}/journal`
  read and clear it; the automation page shows the text and a "Clear journal" action.
- **Trust**: the notes derive from web-derived model output and are re-injected, so they are a
  persistence channel for prompt injection. Mitigations: sanitised and capped, fenced and labelled
  as untrusted notes (the close tag is stripped from stored text), written only through the tool,
  never injected into chat, visible and clearable by the operator.

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
      memory_mode: "on"  # optional; "" (inherit the global automation default) | on | off — hot memory once per run
      skip_if_busy: true # optional; skip a scheduled tick when the lane is busy (the workspace heartbeat derives this from its model's lane)
      journal: true      # optional; keep notes between runs (§II.7); default false
      notify:            # optional; deliver the report through a communication connector
        connector: my-telegram   # registry.json communication.connectors key
        dedup: true              # skip items already reported (default false)
        dedup_days: 60           # ledger retention (default 60)
        send_empty: false        # send "no new items" when dedup removes everything (default false)
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
- **Skip-if-busy** — an automation with `skip_if_busy: true` is *disposable*: a scheduled (cron /
  interval) fire that cannot start immediately (the lane is full, a chat is waiting, or the
  residency gate holds starts) is dropped, not queued (`runlane.DispositionSkipped`,
  `TriggerSkipped`, counted as a skipped execution, no history entry), and a run preempted by a
  chat is dropped instead of re-queued at the front. Manual triggers (UI, API, `/run`) ignore the
  flag and queue as usual. Meant for checks whose next tick repeats the work, so a busy local
  model is not queued behind or restarted for a check that can wait. The workspace heartbeat sets it
  from its model's lane instead of a flag: on for the local lane, off for cloud.
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
  history entry, so the startup metrics seeding cannot rebuild them).
- **Global run visibility** — lane state is **global** (`Scheduler.Snapshot()`
  takes no arguments), so exactly one route exposes it: `GET
  /admin/api/active-runs` with **no workspace path parameter**, returning
  `lane_holders` / `queued` and `lanes` (per lane: `lane`, `limit`, `running`,
  `waiting`, `holder_keys` — idle lanes included; inbound callers hold no lane slot). `GET /workspaces/{ws}/active-runs` carries **only**
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
  **`429` + `X-LLM-Status: busy`** (refused) or **`429` + `queued`** (parked but
  the wait ended unserved), both with `Retry-After` and a body naming the model
  in the way — the standard "busy, back off" pair, which OpenAI-compatible
  clients already retry on (409/202 were used before 2026-09-25 and are still
  recognized by this proxy's client for mixed-version deployments). A caller with no
  `X-Queue-Wait` header is parked for `inbound_wait_seconds` only when the host
  enabled `inbound_wait_by_default`; otherwise it is refused (an unsolicited
  connection is never held). An explicit `X-Queue-Wait: 0` refuses immediately.
- **Nothing is evicted on a caller's behalf.** Only the operator's
  `POST /admin/api/queue/{key}/promote` (and the opt-in `inbound_preempt` host
  policy, default off) cancels the blocking run — and the waiter is granted only
  after that run unwinds, so a model is never swapped under a live run.
- **Cancel** — a held caller may end its wait by closing the connection, or out of
  band with `DELETE /v1/queue/{key}`; the key is cryptographically random and is
  returned to the caller in the busy/queued answer. The operator's
  `POST /admin/api/queue/{key}/cancel` is the same drop behind admin auth.
- **Settings** (`settings.yml → scheduler`, live): `inbound_wait_seconds`
  (default 60) caps a wait a caller requests with `X-Queue-Wait` and is the park
  duration for a header-less caller (`0` refuses immediately, `-1` unbounded);
  `inbound_wait_by_default` (default **false**) parks header-less callers instead
  of refusing them — parked callers appear in the run-activity panel where the
  operator can serve or dismiss them;
  `inbound_max_queued` (default 32, always enforced so an unbounded wait cannot
  hold unbounded connections); `inbound_preempt` (default false).
- **Held lane** — while the gate holds an entry the local lane suspends queued
  starts, so a preempted scheduled run is re-queued but does not restart ahead of
  the caller the operator promoted.
- **Client side** — a remote proxy's busy/queued answer is reported as its own
  upstream reason (`model_busy`) with the server's explanation. The client then
  waits (honoring `Retry-After`, bounded by the run context) and re-checks until
  the model frees or the run is cancelled — it is not retried against the
  transient-retry budget. The chat UI surfaces a Wait / Cancel prompt for the
  wait; the inline notice carries the server's explanation.

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
