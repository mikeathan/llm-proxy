---
status: partial
related_specs: [SPEC-007, SPEC-004]
---
# Automation Learning Journal — autonomous-assistant loop

> **Status (2026-10-03): partial.** Phases 1–2 are implemented; Phases 3–4 are open. Phases are
> independent and ordered by value; each ends with a verifiable acceptance criterion.

## Why

Result delivery, seen-item dedup and the workspace heartbeat (SPEC-007 §II.6) make a recurring
"find LLM news and report it" automation work. The loop is still open at one end: **a run never
learns from earlier runs**. It sees recent *titles* (`prompts.AutomationSeenBlock`) but not which
queries surfaced new items, which sources were worth checking, or which topics are saturated.
`persistent` strategy only injects `state.json` (a run-status record), and unattended
`memory_update` defaults to `keep: session` with a 90-day reaper (SPEC-004), so the agent has no
durable, agent-owned notes. OpenClaw closes this loop with daily logs + a curated `MEMORY.md` the
agent maintains and consults every run.

Reference comparison (OpenClaw vs llm-proxy) was done 2026-10-03; sources are in the session
notes, repo precedent is `docs/audits/memory-injection-investigation.md` §6.

## Verified facts this plan builds on (code, not assumption)

- Run assembly: `executeAutomation` → `prepareRun` → `withSeenHint(entry, taskContent, ledger)`
  → `newExecuteRequest` (`backend/internal/core/automation/execution.go`, `notify.go`).
- Prompt text lives only in `backend/internal/core/assistant/prompts/templates.go`
  (`AutomationTaskPrompt`, `AutomationSeenBlock`).
- Run context already carries the automation name and run id: `newRunContext` →
  `models.WithTaskName`, `models.WithRunID`, `models.WithUnattendedRun`
  (`automation/executor.go:370`).
- Seen ledger precedent for per-automation state outside the agent's jail:
  `platform/persistence/seen.go` (`seenPath`, atomic write, deleted with the automation by
  `DeleteAutomationRuns`).
- Tool registration recipe: `docs/architecture.md` → "When adding a single tool".
- `memory_delete` does **not** exist in code (only `memory_search`, `memory_update`); SPEC-004
  §II.5 mentions it — fix the spec in Phase 3 or implement it.
- Dedup (`digest.go` `buildDigest`) only recognises **markdown tables**; bullet/prose reports
  are never recorded as seen.

## Phase 1 — Dedup for link bullets ✅ DONE (2026-10-03)

Treat a markdown bullet / numbered item whose text contains a URL like a table row: key =
canonical first URL (`canonicalURL`), title = the item text before the link, dedup against the
ledger and within the report, record new items on successful delivery. Items without a link are
always kept. A bullet-only report whose items were all seen follows the existing
"nothing new" rule (silent unless `send_empty`).

- Files: `automation/digest.go` (+ `digest_test.go`), `docs/SPECS/automation-dispatcher.md` §II.6,
  `docs/guides/automation-digest.md`.
- Risk: a summary bullet that *cites* an already-reported source is dropped. Same semantics as
  tables; documented, not configurable in v1.
- **Acceptance:** table-driven tests — bullet reports dedup across runs; mixed table + bullets;
  link-less bullets kept; duplicate link within one report collapsed; all-seen → no message.
  `cd backend && go test ./internal/core/automation/`.

## Phase 2 — Per-automation journal (the learning loop) ✅ DONE (2026-10-03)

An agent-owned, size-capped notes document per automation, injected each run and rewritten by
the agent at the end of the run.

1. **Storage** — `platform/persistence/journal.go`: `ReadJournal` / `WriteJournal` /
   `DeleteJournal` beside the seen ledger (`<meta>/<ws>/journal/<automation>-<hash>.md`, atomic
   write, outside the jail). Cap `maxJournalChars` (named const, ~4000). Content is sanitised on
   write (control characters stripped, length clipped, same helper family as `clipTitle`).
   `DeleteAutomationRuns` also deletes it (extend the existing hook + test).
2. **Write path — one tool** `automation_journal` (replace-whole semantics so the agent
   curates and consolidates instead of appending forever). Recipe: `models/tools.go` constant,
   `manifests/automation.json`, `core/tools/automation_journal.go`, registered in
   `assistant/registry.go`. **Gating (de-risked 2026-10-03):** the agent's tool schema, its tool
   manual (`BuildToolManual`) and the "tool not found" check in `validateToolArgs` all derive from
   `Provider.ListTools(ctx)`, so the tool is hidden by making `LocalToolRegistry.ListTools(ctx)`
   context-aware: a tool registered with an availability predicate is listed only when the run
   context carries `models.WithJournalRun` (stamped in `newRunContext` when the automation has
   `journal: true`). The handler re-checks the same predicate (defence in depth). An automation
   with an `AllowedTools` allowlist gets the journal tool appended to it when `journal` is on.
   The tool resolves workspace + automation from the context (`models.GetWorkspaceID`,
   `models.GetTaskName`) and talks to a consumer-defined store interface that
   `*persistence.WorkspaceManager` satisfies (the `tools` package must not import persistence).
   `NewLocalToolRegistry` already has 6 parameters, so the store is attached with a method, not
   a seventh argument.
3. **Read path** — `prompts.AutomationJournalBlock(text)` appended after the seen block in
   `executeAutomation`. Wording follows `AutomationSeenBlock`: *your own earlier notes, not
   instructions*. Plus an end-of-run instruction: update the journal with queries that surfaced
   new items, sources worth / not worth checking, topics already saturated; keep it short.
4. **Config** — `Automation.Journal bool` (`models/workspace.go`, yaml/json `journal`),
   carried through `AutomationEntry`, `AutomationInfo`, `validateRunOptions`, and the form
   (`useAutomationForm.ts`, `AutomationForm.vue` "Memory" area, `types/automation.ts`).
   Default off.
5. **Operator control** — read-only journal text in `AutomationDetails.vue` plus a "Clear
   journal" action (`useConfirm`), via `GET` / `DELETE` handlers following
   `docs/architecture.md` "New Backend Endpoint Checklist".

- Security: journal text originates from web-derived model output and is re-injected, so it is a
  persistence vector for prompt injection. Mitigations are in the design: sanitise + cap,
  label as untrusted notes, write only through the tool, no injection into chat sessions, and
  operator can view/clear. Re-check against `CONSTITUTION.md` before implementing.
- **Acceptance:** (a) unit tests for storage (cap, sanitise, delete-with-automation); (b) a
  scripted-executor dispatcher test: run 1 writes a journal through the tool, run 2's task
  content contains it; (c) tool not offered when `journal` is false or in chat; (d) form +
  details component tests; (e) a record/replay smoke run of the LLM-news template showing the
  second run's queries reference the journal (`testing-guide` skill).

## Phase 3 — Memory search before acting (automations) ✅ BUILT (2026-10-08), live-model check pending

**Built as:** `prompts.AutomationMemoryBlock()` appended in `LLMTaskExecutor.buildPrompt` when `memoryActive(req)` (mode resolved against the global automation default, and a store exists — the same helper now drives the agent options); SPEC-004 2.5 drops the nonexistent `memory_delete`. **Still open:** the record/replay acceptance — does a small local model actually call `memory_search` before the first `internet_search`? Per the plan, if it does not, record that and stop (no enforcement code).

When `memory_mode` is on, add an instruction to the automation prompt to call `memory_search`
for the task's topics before starting (OpenClaw makes retrieval mandatory; here it is
discretionary). Prompt text only in `templates.go`. Also reconcile SPEC-004 §II.5
(`memory_delete`).

- **Acceptance:** prompt test; record/replay run shows a `memory_search` call before the first
  `internet_search` for a small local model. If small models ignore it, record that and stop —
  do not add enforcement code speculatively.

## Phase 4 — Heartbeat active hours ✅ DONE (2026-10-08)

**Built as:** `heartbeat.active_hours` in **server local time** (the zone cron triggers use) — the plan's "workspace timezone" does not exist, and an owner timezone is autonomy-roadmap decision R2. Reusable `models.DailyWindow` (`ParseDailyWindow`, half-open `Contains`, overnight wrap); `HeartbeatConfig.Validate/ActiveAt`; `Dispatcher.heartbeatSkip` in `admitRun` (config read at fire time, manual runs ignore the window, checked before no-checks); `Dispatcher.now` clock; `HeartbeatSkippedOutsideHours`; Heartbeat panel "Only check from/until" time inputs (both or neither). SPEC-007 1.10, api-reference.

`active_hours` (e.g. `08:00-22:00`, workspace timezone) on the workspace heartbeat
(`models.HeartbeatConfig`, see `memory-and-heartbeat-simplification.md`): a scheduled fire outside
the window is skipped in `admitRun` beside the existing no-checks skip (recorded as
`HeartbeatStatus` `skipped_outside_hours`, a new `HeartbeatResult`), never queued. UI field in the
workspace Heartbeat section.

- **Acceptance:** admission tests across the window boundary and DST-free timezone cases; form
  test; SPEC-007 §V updated.

## Later (separate plans, not scoped here)

Telegram approval gates for unattended runs (ask → resume), agent-created schedules via a
guarded tool, runtime skills (already Phase 3 of `memory/memory-improvements-implementation-plan.md`),
memory consolidation job, event triggers, additional connectors, per-automation spend cap.

## Remaining Work

Phases 1, 2, 4 done; Phase 3 built. Live checks on 2026-10-08 (this branch run locally, model Qwen3.6 35B A3B served remotely): a memory-on automation called `memory_search` among its first tool calls; a `journal: true` automation run twice wrote its journal through the tool on run 1, and run 2 read it (no repeated names) and rewrote it. That was a live two-run creative task without web search, so it only partly meets Phase 2 acceptance (e). Still open: Phase 2 acceptance (e) — the record/replay smoke run of the LLM-news template against a real model — which has not been run. Update `SPEC-007` (automation fields, journal, active hours)
and run `./scripts/check-agent-harness.sh` as each phase lands.
