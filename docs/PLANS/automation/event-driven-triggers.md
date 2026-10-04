---
status: proposed
date: 2026-10-04
last_reviewed: 2026-10-04
related_specs: [SPEC-007, SPEC-006, SPEC-003]
constitution_references: [I.1, I.2, II.2, II.14, III.2, III.3, V.2, V.3]
related_plans: [../cross-cutting/connector-auto-reply.md, ../unattended-run-safety-hardening.md]
---

# Event-Driven Automation Triggers (file-drop, watch-for-change, run chaining)

**Status:** proposed — needs the decisions in [§ Decisions for the user](#decisions-for-the-user) before
Phase 2. Phase 0 changes no behaviour and can start immediately.

## Why this plan exists

Automations can only start on a clock or by hand. `models/workspace.go:8-10` defines exactly three
trigger types (`cron`, `interval`, `manual`). Most useful automation reacts to *something happening*:
a file arrives, a page changes, a previous job finishes. Today a user who wants that has to poll with a
short `interval` trigger and make the agent itself decide "is there anything to do?" — which spends a
full LLM run (and on a local model, the only GPU slot) just to find out the answer is "no".

| User benefit | Service benefit |
|---|---|
| **Drop a file, get a result** — put a PDF/CSV/log into `inbox/`, the automation processes it. No UI, no API call. | No wasted model runs on empty polls; the local lane (`local_concurrency` 1) stays free for real work. |
| **"Tell me when this changes"** — watch a URL or command output; the agent runs only when the content actually changed. | The change check is a cheap fetch + hash, not an LLM call. |
| **Pipelines without a mega task file** — automation B runs when A completes (or fails). Each step stays small, testable, and can use a different model. | Smaller task files fit small local context windows (see `memory/small-context-memory.md`); failures are isolated to one step. |

## Ground truth (verified in code, 2026-10-04)

| Fact | Source |
|---|---|
| `Trigger` interface is clock-shaped: `ShouldRun(lastRun, now)`, `NextRun(now)`, `Type()`, `Value()` | `backend/internal/core/automation/trigger.go:11-16` |
| Factory `automation.New` switches on the three types; unknown → error | `trigger.go:19-30` |
| Scheduling converts every trigger to a cron spec (`triggerToCron`) and registers it on `robfig/cron`; `manual` removes the job; any other type makes `scheduleAutomation` fail | `backend/internal/core/automation/scheduling.go:184-241` |
| Every fire goes through `admitRun` → run lane. The lane **dedupes by key**: a fire while the automation runs becomes its single pending rerun; a fire while queued is absorbed | `backend/internal/core/automation/admission.go:44-77` |
| `Dispatcher.Trigger(ws, name, recordingRef)` is the public manual entry point (also planned for connector `/run`) | `admission.go:30-36`; `docs/PLANS/cross-cutting/connector-auto-reply.md` |
| The dispatcher already owns an `fsnotify` watcher, but only on the **metadata** root (config hot-reload), not workspace files | `scheduling.go:244-258` |
| Run outcome is finalised in two places: `failRun` and `succeedRun`, both append to the global ledger via `RecordActivity` | `backend/internal/core/automation/execution.go:225-278` |
| `ExecuteRequest` carries `TaskContent` (the task file text); there is no field for "why this run fired" | `execution.go:397-414` |
| Outbound fetch with guardrails exists outside the agent: `NetworkTools.FetchURL(ctx, url)` | `backend/internal/core/tools/network.go:107` |
| Automation validation lives in `validateAutomation` and does not inspect the trigger (the factory does at register time) | `backend/internal/transport/http/handlers/dispatcher_handlers.go:85-112` |

## Design

### Two families of trigger

The clock-shaped `Trigger` interface stays as is. Event triggers are a **separate interface** so the
cron path is untouched:

```go
// EventSource starts watching and calls fire(reason) when its condition is met.
// It must return when ctx is cancelled (Constitution II.2/II.14).
type EventSource interface {
    Type() models.TriggerType
    Start(ctx context.Context, fire func(FireReason)) error
}
```

`scheduleAutomation` branches: clock triggers → cron job (unchanged); event triggers → a supervisor
goroutine per automation, tethered to the dispatcher's root context and cancelled on
unregister/update/workspace delete (the same lifecycle points that call `removeJob` today).

### The three new types

| Type | `value` | Fires when | Notes |
|---|---|---|---|
| `file` | path glob relative to the workspace, e.g. `inbox/*.pdf` | a matching file is created or finished writing (debounced: size stable for N ms) | Path resolved with `IsSecurePath` against the workspace root (Constitution III.3). Dotfiles and `.sandbox` ignored. |
| `watch` | `url:<https://…>` or `cmd:<command>`, plus `every` (min 5m) | the fetched body / command output **hash** differs from the last stored hash | URL via `NetworkTools.FetchURL` only (I.1, I.2) and the automation's `network_grant`. `cmd:` runs through the sandboxed shell (II.3) with the automation's allow-list. First check stores a baseline and does not fire. |
| `after` | `<automation-name>[:completed|failed|any]` | the named automation in the **same workspace** finishes with that outcome | Hooked in `succeedRun`/`failRun`. Cycle detection at register time (A→B→A refused). |

### Telling the agent why it was started

Add `FireReason` (type, a short human summary, and a bounded list of items — file paths, a unified diff
truncated Head/Mid/Tail per Constitution II.9, or the upstream run's ID/status/output summary). It is
passed through `admitRun` → `newExecuteRequest` and rendered into the run's first user message by a
template in `core/assistant/prompts/templates.go` (II.13 — no prompt strings elsewhere).

**Batching rule (because the lane dedupes):** two files dropped while a run is in progress collapse
into one rerun. So `file` triggers keep a per-automation pending set; the rerun receives **all**
pending items, not just the last one. No event is lost; the run gets a list.

### Persistence

Per-automation trigger state (`watch` baseline hash, `file` items already processed) lives in the
workspace `state.json` next to `LastRuns`, written atomically (III.2). Processed-file tracking uses
path + size + mtime so a restart does not reprocess the inbox.

## Decisions for the user

| # | Decision | Recommendation |
|---|---|---|
| D1 | After a `file` run, what happens to the input file? (a) leave it, (b) move to `inbox/done/`, (c) agent decides | (b) — predictable and visible; failed items go to `inbox/failed/` |
| D2 | Minimum `watch` interval | 5 minutes (protects cloud quotas and remote sites) |
| D3 | May `after` chain across workspaces? | No for v1 — same workspace only (keeps the workspace jail and lock simple) |
| D4 | Max chain depth for `after` | 5 |

## Phases

### Phase 0 — Characterise (no behaviour change)
- Add tests pinning today's behaviour: unknown trigger type refused by `automation.New`;
  `scheduleAutomation` with a non-clock type errors; lane dedupe collapses two fires into one rerun.
- **Acceptance:** `cd backend && go test ./internal/core/automation/ -count=1` green with the new tests.

### Phase 1 — Event-source plumbing + `after` (smallest real trigger)
- `EventSource` interface, supervisor lifecycle in `scheduling.go`, `FireReason` through `admitRun`
  and `ExecuteRequest`, prompt template for the reason block.
- `after` trigger hooked into `succeedRun`/`failRun`; cycle check in register.
- **Acceptance:** test — A completes → B admitted once with `FireReason{type: after, upstream: A}`;
  A→B→A refused at register; unregistering B stops it firing. `go test -race ./internal/core/automation/`
  green (lifecycle code changed).

### Phase 2 — `file` trigger (needs D1)
- Watcher on the workspace directory (separate `fsnotify` instance from the metadata watcher),
  debounce, pending-set batching, processed tracking, D1 move semantics.
- **Acceptance:** tests — three files dropped during a run → exactly one rerun receiving all three;
  restart does not reprocess; a symlink escaping the workspace is ignored; `go test -race` green.

### Phase 3 — `watch` trigger (needs D2)
- URL fetch via `NetworkTools`, `cmd:` via sandboxed shell, hash baseline in state, diff summary.
- **Acceptance:** tests with an `httptest` server — unchanged body → no fire; changed body → one fire
  with a truncated diff; blocked domain → trigger reports an error state and never fires.

### Phase 4 — API, UI, docs
- `validateAutomation` validates the new types' values up front (400 with a clear message).
- Automation form (`frontend/src/composables/automation/useAutomationForm.ts` and the automation
  components under `frontend/src/components/AgentIde/automation/`) gets the three types with
  type-specific inputs; automation list shows "waiting for: inbox/*.pdf" instead of a next-run time.
- SPEC-007 §II.1 amended (new trigger types, `FireReason`, batching rule); `docs/api-reference.md`.
- **Acceptance:** `npm test && npm run build`; `npm run test:visual` (UI change);
  `./scripts/check-agent-harness.sh` green.

## Risks and non-goals
- **Event storms** (a tool writing 1 000 files into `inbox/`) — bounded by debounce + batching + the
  lane's single pending rerun; the item list passed to the agent is capped.
- **Self-triggering** — an automation writing into its own watched folder. Mitigation: ignore files
  written by the same automation's run (tracked by run ID), and document it.
- **Non-goal:** generic inbound webhook triggers — owned by `../cross-cutting/connector-auto-reply.md`.
- **Non-goal:** cross-workspace pipelines and DAG editors (D3).

## Remaining Work
All phases (0–4). Blocked: Phase 2 on D1, Phase 3 on D2.
