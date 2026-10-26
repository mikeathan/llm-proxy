---
status: complete
date: 2026-10-11
last_reviewed: 2026-10-11
related_specs: [SPEC-007, SPEC-003]
constitution_references: [III.3, V.2, V.3]
related_plans: [chat-to-automation.md, playbook-library-management.md, ../assistant-ui/assistant-automation-workbench-layout.md, ../autonomy/event-driven-triggers.md]
---

# Automation Authoring UX — set up an automation without knowing cron

**Status:** complete (2026-10-11). Phases 0–5 built. Phase 0 was folded into the build rather than run as a separate step.

## Why this plan exists

Creating an automation today assumes the user already knows four things the product could handle for them:

1. **Cron syntax.** The schedule editor opens on "Custom expression" with a raw `0 7 * * *` field.
2. **Go duration syntax** for the interval trigger (`5m`, `1h`).
3. **That a task file must already exist.** The form can only *pick* a file, so a new user has to leave the form,
   create a file in the workspace, and come back.
4. **Which timezone a schedule is in.** The scheduler is created with no location (`dispatcher.go:97`), so
   cron runs in the server's local time, and nothing in the UI says so.

The goal is one outcome: *a person who has never heard of cron can create, read and trust a scheduled
automation.* Everything below serves that; nothing here changes how a run executes.

| User benefit | Service benefit |
|---|---|
| Pick "Every weekday at 7:00" instead of writing `0 7 * * 1-5`. | Fewer wrong schedules (a `*/5` typo that fires every five minutes all day). |
| See the next three run times before saving, in a stated timezone. | Wrong-timezone surprises are caught at creation, not the morning after. |
| Write or pick the task inside the form. | The first automation takes one screen, not a file-manager detour. |
| Test run right after save. | Failures surface while the user is still looking at the form. |

## Ground truth (verified in code, 2026-10-11)

| Fact | Source |
|---|---|
| The schedule editor defaults to `custom` and offers only "every N minutes/hours/days" as the simple mode | `frontend/src/components/AgentIde/automation/CronEditor.vue:21,34-38` |
| Trigger choice is a three-way control `Cron / Interval / Manual`; interval is a free-text Go duration | `AutomationForm.vue` `TRIGGER_OPTIONS`, Schedule panel |
| Human description of a stored schedule comes from `cronstrue` in the browser | `frontend/src/utils/automation/automationDisplay.ts` `triggerLabel` |
| The server parses cron with `SecondOptional|Minute|Hour|Dom|Month|Dow|Descriptor`, built twice (in `cron.go` and in `dispatcher.go`) — seconds are accepted today and will be removed (D4) | `backend/internal/core/automation/cron.go:21`, `dispatcher.go:97-101` |
| Scheduler has **no** `cron.WithLocation` → server local time; heartbeat `active_hours` is also server local time | `dispatcher.go:97`; memory note on `models.DailyWindow` |
| Every trigger already implements `NextRun(now)` | `backend/internal/core/automation/trigger.go:13`, `cron.go:36`, `interval.go:33` |
| `AgentState.NextRunAt` exists but nothing sets it, so the UI cannot show a next run | `backend/models/workspace.go:173` (only reference) |
| The task is a workspace file chosen from `workspaceFiles` | `AutomationForm.vue` Basics panel, `taskFiles` |
| Playbooks are embedded, read-only, listed at `GET /admin/api/templates`; "New file" writes `<id>.md` | `playbook-library-management.md` ground truth; `useTemplates.ts` |
| Workspace file write: `PUT …/workspaces/{ws}/files/{file...}` | `docs/api-reference.md` |

## Design

### A. One source of truth for schedule meaning: the server

A browser-side cron implementation would diverge from `robfig/cron` (seconds, descriptors like `@daily`).
So the server answers both questions, with the **same parser the scheduler uses**:

`POST /admin/api/schedule/preview` `{ trigger: {type, value}, count?: 3 }` →
`{ valid, error?, timezone, next_runs: [RFC3339…] }`.

It is stateless and writes nothing. Go has no cron describer, so the server answers validity, the error, the
timezone and the next runs; the plain-English reading is built client-side from the preset (`describeSchedule`) or,
for a custom cron, `cronstrue`. `triggerLabel` now delegates to `describeSchedule`, so the form, the review and the
list use one wording.

### B. A schedule model the UI owns, cron as its serialisation

A small pure module (`utils/automation/schedule.ts`, types in `src/types/`) with a discriminated union:

```
Schedule = { kind: 'manual' }
         | { kind: 'every';   n; unit: 'minutes'|'hours' }
         | { kind: 'daily';   time }
         | { kind: 'weekdays'; time }
         | { kind: 'weekly';  days[]; time }
         | { kind: 'monthly'; day; time }
         | { kind: 'custom';  expression }
```

`toTrigger(schedule) → {type, value}` and `fromTrigger({type, value}) → Schedule` (anything that does not match a
preset exactly becomes `custom`, never a lossy rewrite). Round-trip is table-tested. `interval` triggers map to
`every`; the stored trigger type does not change (no migration, product is new).

### C. The form

- Replace the `Cron / Interval / Manual` control with one **"Runs"** choice (a select, one step fewer than a
  segmented control plus presets): *Manual only · Every few minutes or hours · Every day · Every weekday · Certain
  days of the week · Once a month · Custom (cron)*. A new automation starts as Manual.
- Time input + weekday chips; always show **"Runs at 07:00 Europe/Berlin (server time)"** and the next three runs
  from §A. Invalid input shows the server's error, not a silent empty state.
- "Skip a run if the model is busy" stays where it is.
- **Task step:** beside "Task file", add **"Write it here"** (a textarea that creates `<name>.md`, asking before
  replacing a different file — reuse `useTemplates`' existing-content/confirm logic) and **"Start from a playbook"**
  (lists `GET /admin/api/templates`). Choosing an existing file still works.
- **Review** panel keeps its plain-language rows; "Runs" reads from the same description string.
- After create: toast with **"Run it now to test"** (uses the existing manual trigger — no new run mode; this is
  D3 of `chat-to-automation.md`).

### D. List and detail

Show **next run** and **last result** on each automation row/detail, from a new `next_run_at` per automation on
the existing list response (computed from `Trigger.NextRun`; nothing persisted). This also closes the
"next-run/ETA fields" item the workbench plan parked (`assistant-automation-workbench-layout.md:144`).

### E. Hand-off to chat-to-automation

`chat-to-automation.md` Phase 4 (heuristic trigger suggestion "every morning → `0 7 * * *`") should emit a
`Schedule`, not a cron string, and its draft lands in this same form. Build this plan's form first so that plan's
UI phase is only "pre-fill and open".

## Decisions (settled 2026-10-11)

| # | Decision | Outcome |
|---|---|---|
| D1 | Timezone | Keep server-local and **show it**. A stored per-automation zone is a later SPEC-007 change. |
| D2 | `interval` trigger | Stays a stored trigger type; the UI presents it as "Every N". No backend change, no migration. |
| D3 | Task authoring in the form | In scope (Phase 3). |
| D4 | Seconds-level cron | **Not supported anywhere** — not even under Advanced. The one shared parser drops `SecondOptional` (5 fields + `@descriptors`), so the scheduler, the preview endpoint and the picker cannot disagree. A 6-field expression is rejected with a clear error. |

## Phases

### Phase 0 — Characterise (no behaviour change) — **partly done 2026-10-11**
- Pin current behaviour with tests: `triggerLabel` for cron/interval/manual/invalid; `CronEditor` modes; the
  Go parser's accepted forms (descriptors, 6-field). Confirm what `AutomationInfo` returns today and where
  `next_run_at` could be added.
- **Acceptance:** `cd frontend && npm test` and `cd backend && go test ./internal/core/automation/ -count=1`
  green; findings recorded in this plan.
- **Status:** not done as a separate step — the new tests were written against the new behaviour instead (the
  `triggerLabel` and form tests were updated, not first pinned). Still open and moved to Phase 4: what the automation
  list response carries and where `next_run_at` goes. Findings so far: `trigger_value` is the stored trigger's
  `Value()`, which for an interval is a Go `Duration.String()` (`90m` comes back as `1h30m0s`) — the picker reads
  every whole number of minutes back as a preset for that reason.

### Phase 1 — Schedule model + preview endpoint — **done 2026-10-11**
- Backend: one shared cron parser (5 fields + descriptors, no seconds) used by `NewCronTrigger`, the dispatcher and the preview handler — delete the duplicate construction. `schedule/preview` handler on it (TDD: 6-field rejected, valid, invalid, descriptor,
  `count` bound, never registers a job). Frontend: `schedule.ts` + `toTrigger`/`fromTrigger` with a round-trip
  table test (every preset ↔ its cron/interval; non-matching → `custom`).
- **Acceptance:** `go test ./internal/transport/http/handlers/ -count=1`; `go run ./tools/check-complexity/`;
  `npm test` — the round-trip table passes.

### Phase 2 — Schedule picker UI — **done 2026-10-11**
- New `SchedulePicker.vue` replaces `CronEditor.vue` and the interval field; timezone label, next-3 preview,
  Advanced cron. Remove the dead trigger copies (`TRIGGER_OPTIONS`, interval field) rather than leaving both.
- **Acceptance:** component tests (preset → payload, edit existing cron → correct preset or Advanced, invalid
  shows server error); `npm test && npm run build && npm run test:visual`.

### Phase 3 — Task authoring in the form — **done 2026-10-11**
- "Write it here" + "Start from a playbook", reusing `useTemplates` logic (extract the shared piece, do not copy).
- **Acceptance:** component tests (creates file, asks before replacing, playbook fills the textarea);
  `npm test && npm run build`.

### Phase 4 — List/detail next run + test run — **done 2026-10-11**
- `next_run_at` on the automation list response; row/detail display; post-create "Run it now to test".
- **Acceptance:** handler test for `next_run_at` (cron, interval, manual → empty); component tests;
  `go test ./... && npm test`.

### Phase 5 — Docs and hand-off — **done 2026-10-11**
- SPEC-007 (preview endpoint, `next_run_at`, server-local timezone statement), SPEC-003 (picker, task step),
  `docs/api-reference.md`, `automation` skill, `docs/guides/automation-digest.md` schedule wording; update
  `chat-to-automation.md` Phase 4 to emit a `Schedule`.
- **Acceptance:** `./scripts/check-agent-harness.sh` green.

## Risks and non-goals
- **Preset ↔ cron drift:** mitigated by one pure module and the round-trip table; unknown expressions stay `custom`.
- **Timezone confusion:** the plan states the zone but does not change it (D1).
- **Non-goals:** new trigger types (`file`/`watch`/`after` live in `../autonomy/event-driven-triggers.md`);
  the master-detail/one-renderer rework (`../assistant-ui/assistant-automation-workbench-layout.md`); editing
  automations from chat.

## Built so far (2026-10-11)

- Backend: one `cronParser` (5 fields + descriptors) in `core/automation/cron.go`, used by `NewCronTrigger` and the
  dispatcher's cron; `automation.PreviewSchedule` + `POST /admin/api/dispatcher/schedule/preview`
  (`handlers/dispatcher_handlers.go`); `models.SchedulePreview`.
- **Found while verifying against a real server:** a trigger the scheduler could not read (six-field cron, garbage)
  was saved before `Register` failed (500), and the re-register loop in `scheduling.go` then stopped at that entry,
  so every automation after it in the workspace stopped being scheduled. Two changes, both needed because D4 makes
  some already-stored triggers invalid: (1) `registerWorkspaceAutomations` now registers every automation it can and
  returns the failures joined (`errors.Join`), so one unreadable trigger skips only itself; (2) create and update
  refuse an unreadable trigger with a 400 (`validateSchedulable`, the scheduler's own factory) before anything is
  written. The whole-document config PUT does **not** apply (2): it carries every stored automation back, and an
  older unreadable trigger must not block an unrelated save.
- **Behaviour change beyond the approved scope (named for review):** create/update with a bad or missing trigger went
  from "saved, then 500" to a 400 with nothing written. A request with no `trigger` at all is now refused too.
- **Not yet verified:** the vertex host runs an older build and was unreachable on 2026-10-11, so its stored triggers
  are unchecked (the local data root holds only `manual` triggers). Before deploying, list its automations
  (`GET /admin/api/dispatcher/automations`) and look for six-field crons.
- Frontend: `types/schedule.ts`, `utils/automation/schedule.ts` (+ round-trip table tests), `runTime.ts`,
  `useSchedulePreview`, `SchedulePicker.vue` (replaces `CronEditor.vue`); `triggerLabel` reads through
  `describeSchedule`.
- Live check: a scratch build on :4001 returned the right weekday runs in BST, rejected `*/10 * * * * *` with
  "expected exactly 5 fields", and refused it on create (400) with no re-register errors.

- Phase 3: `TaskField.vue` (Existing file / New task, playbook picker, text box), `useTaskAuthoring`, and the shared
  `useWorkspaceFileWriter` extracted from `useTemplates` (one confirm-before-replace rule for the playbook library
  and the form). The file is named after the automation (`<name>.md`). Checked in a real browser against a scratch
  backend: picked a playbook, edited it, saved; the file and the automation (`30 7 * * 1-5`) both existed afterwards.
  Review fixes: the form gets every automation and refuses a taken name / another automation's task file before
  writing; New task cannot outlive a switch to edit (the view reuses one form instance); a second press while saving
  is ignored. A create that fails for any other reason (network) can still leave `<name>.md` behind.

- Phase 4: `Dispatcher.NextRuns()` (read from the scheduler's entries, so an interval reports the tick it will really
  fire) → `next_run_at` on the automation list; `nextRunLabel`, a Next run column in the list and a Next run row on
  the detail page; after a create the view opens the automation and offers **Run now / Not now** (the existing run
  path, no new run mode). **Beyond the original plan:** a **Create automation** action on each playbook in the
  library (`/automations/new?workspace=&playbook=`), which opens the form set up from that playbook.
- Also: the form's Create button is held back while the server says the schedule is not valid (the picker reports it
  through `update:error`); a playbook card hides **Append** when there is no open file to append to.
- Phase 5: SPEC-007 1.13, SPEC-003 2.8, `docs/api-reference.md`, the `automation` skill, the digest guide wording and
  `chat-to-automation.md` (its trigger suggestion now emits a `Schedule`, not a cron string).

## Remaining Work
None for this plan. Known small gaps, not blocking: a create that fails for a non-conflict reason (network) can leave `<name>.md`
behind; the vertex host's stored triggers are unchecked for six-field crons before a deploy.
