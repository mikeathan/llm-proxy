---
status: proposed
date: 2026-09-30
last_reviewed: 2026-09-30
related_specs: [SPEC-001, SPEC-007]
constitution_references: [II.2, IV.4]
related_plans: [unattended-run-safety-hardening.md, orchestrator/local-and-cloud-inference-performance.md, gpu-performance.md, memory/small-context-memory.md]
evidence: docs/audits/2026-09-30-platform-scan.md (P2–P8, P10, P11)
---

# Backend Hot Paths, Memory Growth and Leak Hardening

**Status:** proposed. Phase 0 is tooling and a status sweep; later phases are each gated on a number from it.

## Why this plan exists

Two July audits (`backend-audit-report.md`, `codebase-audit-report.md`) and the 2026-08-28 ops review found
and fixed most of the big items. What is left is (a) items that were *deferred on purpose*, (b) items that
moved rather than disappeared, and (c) **no way to prove there are no leaks** — every leak fix so far came from
reading code. A long-lived scheduled service (automations every few hours, hours-long runs) needs a soak test and
a couple of exposed gauges more than another audit.

This plan does **not** cover: model/inference latency (see
[`local-and-cloud-inference-performance.md`](../orchestrator/local-and-cloud-inference-performance.md)); browser
GPU cost ([`gpu-performance.md`](../gpu-performance.md)); the agent-loop safety items still open in
[`unattended-run-safety-hardening.md`](../unattended-run-safety-hardening.md) Steps 6–9 (context-aware I/O
hardening, unattended tool restriction and spiral detection, perf optimisations O1/O3/O5–O7, docs sync) —
execute those there and do not duplicate them here.

## Status sweep of the July findings (verified 2026-09-30)

| July item | Now | Evidence |
|---|---|---|
| `Chat()` goroutine leak on body close (1.1) | **fixed / not present** — only select-based waits remain | `core/proxy/client.go` (`<-ctx.Done()` at 486, 571, 599, 674, 715, 764, all inside `select`) |
| SSE one-byte reads (1.4) | **fixed** | `known-performance-findings.md` §2 |
| Guardrail decision map growth (2.1) | **bounded**: `retained` capped at 100; `pending` entries removed on resolve/remove | `core/assistant/agent.go:351-386, 400-425` |
| `StopAutomation` 30 s goroutine (M3) | **fixed**: cancellable diagnostic | `core/automation/execution.go:258-272` |
| Timer per SSE line (2.3) | **open** | `core/proxy/client.go:725` (`time.AfterFunc` each line) |
| History copies per turn (3.1) | **open** (first copy confirmed) | `core/proxy/history.go:22-60` |
| N+1 workspace-state reads (1.6 / M6) | **moved into the automation package, open** | `core/automation/execution.go:133,145`, `history.go:58`, `scheduling.go:198` |
| `collectedEvents` growth (2.2) | **open**, now in `conversation_service.go` | `core/assistant/conversation_service.go:190` |
| Full session rewrite per tool cycle | **open, undecided** (product call recorded 2026-08-28) | `conversation_service.go:189-213`; ops review "Open product decision" |
| `ListSessions` decodes every file | **open** | `platform/persistence/workspace.go:662-694` |
| Per-chunk O(n) scans in `processStream` | **deferred on purpose** (chunk-boundary correctness risk) | ops review finding 6 |
| Flaky `TestBroadcastCriticalEventDeliveredWhenFull` | open, test-only | ops review finding 7 |

## What was tried before

| Attempt | Outcome | Lesson |
|---|---|---|
| Static audits (two rounds, ~100 findings) | most fixed; no runtime proof | add runtime gauges + a soak |
| Bounding the event-bus replay buffer by count only | tens of MB after a heavy run | bound by **bytes** too (fixed: 4 MiB/channel) |
| Log rotation | fixed | keep the pattern (counter-based, no per-line stat) |
| Throttling session checkpoints | rejected when the reload contract was written ("unconditional, no throttling") and pinned by `TestBuildObserver_CheckpointsEveryToolResult` | change the contract and the test together, with a measurement |
| Incrementalising `FilterStreamingMarkup` / repetition guard | deferred (correctness across chunk boundaries) | only with a property/fuzz test first |

## Phase 0 — Gauges, soak, inventory (tooling only)

1. **Expose runtime gauges**: goroutine count, heap in-use, GC count, open file descriptors in `GET /admin/api/metrics`
   (additive fields; the header strip already polls it every 10 s and caches host stats 2 s, so no new poll).
   Show a small "Runtime" row in the Overview (tokens only, existing `Meter`/`StatCard` primitives).
   - Verify: `cd backend && go test ./internal/platform/metrics/... ./internal/transport/... -run Metrics -count=1`.
2. **Soak test (build-tagged, not in `go test ./...`).** `go test -tags soak ./internal/app/ -run TestSoak -timeout 20m`:
   boot the app with `MockManager` and a scripted fake LLM; run 200 short automation runs and 200 assistant turns
   with random cancels/preemptions/guardrail denials/SSE subscribe-unsubscribe; after settling, assert
   goroutines ≤ baseline + 5 and post-GC heap ≤ 1.25× baseline, printing the goroutine diff on failure. No new
   dependency (no goleak): diff `runtime.Stack(all)` grouped by top frame — about 40 lines.
   - Verify: `cd backend && go test -tags soak ./internal/app/ -run TestSoak -timeout 20m -count=1`.
3. **Disk-growth inventory** (docs only, then tickets): list every unbounded-on-disk writer and its reaper.
   Known: runs (reaper exists, `rundir.DefaultRunRetention`), logs (rotation, 2× max), recordings (under runs),
   `.sandbox` (size watch, best-effort). *Not found / to check:* sessions per workspace, `orchestrator.db` ledger
   rows, memory `session`-type rows, `meta/` state files. Output: a table in `docs/data-layout.md` with
   owner, growth driver, retention, and gap.
4. **Goroutine site census** (one-off): `grep -rn "go func\|safe.Go" backend/internal | wc -l`, then classify each
   as tethered to a context / bounded by a channel / fire-and-forget; file the fire-and-forget ones. Constitution
   §II.2 forbids untethered background work.
5. **Update the two July audit files' status** (table above) so nobody re-plans fixed items.
6. **Benchmarks for the suspects**, in the package that owns each: `BenchmarkWriteSession` (50 tool cycles × 20 kB
   results), `BenchmarkNormalizeHistory` (200 messages), `BenchmarkListSessions` (500 sessions), `BenchmarkStreamLines`
   (1 000 SSE lines). Record the numbers in `docs/audits/`.
   - Verify: `cd backend && go test -run ^$ -bench . -benchmem ./internal/platform/persistence/ ./internal/core/proxy/ ./internal/core/assistant/`.

Acceptance: soak passes on a clean tree (or fails and names a leak); the gauges show in the UI; the benchmark table exists;
the inventory names an owner for every growing path.

## Phase 1 — Session checkpoint cost (the one undecided product call)

**Evidence:** the observer rewrites the full session on every `tool_result`/`message` and re-walks all collected events
each time (`conversation_service.go:189-213`); total work per run is quadratic in tool cycles and the events slice is unbounded.

**Decision D-B1** (from the ops review, options A/B/C) with a recommendation:

| Option | Behaviour | Verdict |
|---|---|---|
| A | keep unconditional writes | fine only if the Phase 0.6 benchmark shows per-cycle cost < ~1 ms at realistic sizes |
| B | debounce ≤ 1 write / 500 ms, final write unconditional | simple; mid-run refresh can miss the last cycle |
| **C** | **B plus a trailing-edge flush**, and make `buildPartialHistory` incremental (append the new event to a running partial history instead of rebuilding from all events) | **recommended if A fails the benchmark** — bounded staleness (≤ 500 ms) with a guaranteed flush of the newest state |

Steps (only if A fails): (1) write the benchmark, (2) make `buildPartialHistory` incremental and cap retained
events to what `handle*Result` actually needs (`collectedEvents` is also the SSE replay source — check before trimming),
(3) add the trailing flush with a tethered timer, (4) amend the reload contract in
`docs/PLANS/cross-cutting/session-source-backend-driven.md` and change `TestBuildObserver_CheckpointsEveryToolResult`
to assert "latest state persisted within the window and on completion/cancel/error".

Verify: `cd backend && go test ./internal/core/assistant/ -run 'Observer|Checkpoint' -count=1 -race` (concurrency code changed → `-race`).

Acceptance: benchmark shows ≥ 5× fewer bytes written on a 50-cycle run; a page refresh mid-run shows state no older than
the window; the final persisted session is byte-identical to today's for completed, cancelled and failed runs.

## Phase 2 — List endpoints that scale with data, not with history

1. **`ListSessions`** (`workspace.go:662-694`): stop decoding whole sessions to get a snippet. Options: a small
   `sessions/index.json` per workspace updated on `WriteSession`/`DeleteSession` (atomic write, rebuilt from files
   if missing or stale — the files stay the source of truth), or cache briefs by `(path, mtime, size)`. Add
   `limit/offset` (the overhaul plan flagged this risk in 2026-06). Frontend list already groups by time and
   searches — keep the response shape, add paging fields additively.
2. **Workspace state reads**: give the dispatcher a per-request or short-TTL memo (mtime-invalidated, the
   `workspaceConfigCache` pattern already used for guardrail config) so one `GET /automations` or activity refresh
   reads each state file once.
3. **Do not add speculative caches** elsewhere (`AGENTS.md` Pre-Completion #5): every cache here is justified by a
   Phase 0.6 number.

Verify: `cd backend && go test ./internal/platform/persistence/ ./internal/core/automation/ -count=1` plus the benchmarks.

Acceptance: `BenchmarkListSessions` at 500 sessions improves ≥ 10×; a workspace with 500 sessions lists in < 50 ms;
index rebuild-from-files test passes after deleting the index.

## Phase 3 — Micro-costs (only if Phase 0.6 shows they matter)

| Item | Change | Gate |
|---|---|---|
| Timer per SSE line (`client.go:725`) | one `time.Timer`, `Reset` per line, stopped on exit | `BenchmarkStreamLines` alloc/op |
| `NormalizeHistory` copies | single pass, in-place on an owned copy; keep behaviour identical | existing 17+ parser/history tests unchanged |
| `processStream` O(n²) scans | incremental state for `FilterStreamingMarkup`/repetition window | **fuzz/property test first** across random chunk splits; skip entirely if a 10 kB stream costs < 1 ms |
| Flaky broadcast test | replace sleeps with channel synchronisation | `go test ./internal/core/automation/ -run Broadcast -count=200 -race` |

## Phase 4 — Frontend long-session checks (small)

Phase 6 of the redesign audited timers and bounded lists by reading. Add one optional Playwright soak (not in CI):
open the assistant page, drive a fake SSE stream for 10 minutes, sample `performance.memory.usedJSHeapSize` and
listener counts via CDP; assert growth < X%. Existing budgets (live events 1 000, run-notification ids 200) are the
expected ceilings.

Verify: `cd frontend && npm run test:soak` (new script; manual/nightly).

## Decisions for the user

| # | Question | Recommendation |
|---|---|---|
| D-B1 | Session checkpoint option A / B / C | decide after the Phase 0.6 benchmark; default to C if A fails |
| D-B2 | Is a build-tagged soak test acceptable in the repo (runs only on request, ~minutes)? | yes; same pattern as `recordreplay` |
| D-B3 | Expose goroutine/heap gauges in the UI or logs only? | UI row on Overview; they are cheap and make leaks visible to the operator |

## Risks

| Risk | Mitigation |
|---|---|
| Changing the reload contract surprises users mid-run | bound staleness to 500 ms; final save unconditional; documented |
| Session index drifts from files | files remain authoritative; rebuild on mismatch; test deletes the index |
| Metrics fields expand the polled payload | a few integers; measured against the existing 730-byte response |

## Remaining Work

Everything. First PR: Phase 0 items 1, 2, 5, 6 — observability and proof, no behaviour change.
