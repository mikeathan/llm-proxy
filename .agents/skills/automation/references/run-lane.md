# Automation System — Dispatcher, Executor & Task Lifecycle — Run Lane (Scheduler)

Reference for the [`automation`](../SKILL.md) skill, loaded on demand. You are changing admission, concurrency or model residency for runs.

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
