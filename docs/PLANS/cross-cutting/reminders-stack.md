---
status: partial
date: 2026-11-19
last_reviewed: 2026-11-19
related_specs: [SPEC-011, SPEC-003]
constitution_references: [I.1, IV.6, V.1]
evidence: Built 2026-11-19 on branch stack-sensors-agents-and-next-stack from origin/main a767a9e5 (v0.110.1), uncommitted. Read before building - stacks/sdk (manifest.go, ports.go), stacks/agents and stacks/radio (the patterns), backend/internal/app/stacks.go and stackhost.go, core/stackport, archtest/boundary_test.go, core/reminders (all of it), handlers/reminder_handlers.go, core/tools/reminders.go, core/commands/remind.go, ADR-0028/0032/0035/0036/0051, Constitution IV.6. Run - go build, go vet, go test ./... (see Checks), check-complexity, each stack with GOWORK=off -race, frontend vitest and build. Not run on vertex or in a real browser.
---

# The Reminders stack

**Status:** partial (2026-11-19). Built and tested; waits on the owner's merge, a check on vertex and the removal of the old table.
**Decisions:** [ADR-0068](../../decisions/ADR-0068-reminders-are-a-stack-with-its-own-store-a-notify-port-and-a-schedule-port.md) (proposed).
**Owner decisions (2026-11-19, asked once, all the recommended option):** a narrow delivery port for the owner's own text (amends ADR-0036); Reminders gets a module
switch and moves under Stacks; the rows move to the stack's own database with a one-time copy.

## What moved, what stayed

| Piece | Now |
|---|---|
| Store (rows, revision guard, events), schedule, delivery loop, late/missed/retry policy, channel policy, `ParseCommand`, the eight routes, the tool logic | `stacks/reminders` |
| The cron grammar | the core (`automation.ParseCron`), reached through the new `schedule` port |
| Sending the text | the core (`stackport.Notify` over `CommunicationTools.NotifyAllPlain`), reached through the new `notify` port |
| The assistant's tool schemas (`tools/manifests/reminders.json`), `tools/reminders.go` (now a thin adapter), `commands/remind.go` (thin), `models.ChatZone` | core |
| The old `core/reminders`, `handlers/reminder_handlers.go`, `app/reminders.go` service wiring | removed |
| The old `reminders` table in the main database | **left in place**, no longer read after the copy |

## Phases (all built in one change set)

1. SDK: `PortNotify` + `Notify`, `PortSchedule` + `Schedule`, `MaxNotifyBytes`. *Accept:* SDK tests, `TestStackModulesDeclareOnlyTheSDK`. Done.
2. The stack module with its tests moved (service, store, transition, `when`, channel, schedule) and new tests through the routes and tools. *Accept:* `GOWORK=off go test -race`. Done.
3. Core: `stackport.Notify`, `stackport.Schedule`, registry module and switch, the host, the adapters (tools, `/remind`), the legacy copy, removal of the old code. *Accept:* `go test ./...`, the architecture tests, complexity. Done.
4. UI: Reminders in the Stacks group, a Modules tab, the card follows the stack. *Accept:* vitest, build, visual. Done except the visual run noted below.

## Checks run (2026-11-19)

- `cd backend && go build ./... && go vet ./...`: clean. `go test ./... -count=1`: all pass (`progexec`'s `TestRun_KillsTheWholeProcessGroupOnTimeout` failed once under full load and passed alone; it does not touch this change).
- `go run ./tools/check-complexity/`: no new violations (`api.go: item` was 14 and was refactored to a table).
- Each stack module: `GOWORK=off go test -race` (reminders: pass).
- Frontend: `npm test` (343 files, 3304 tests) and `npm run build`: pass. `npm run test:visual`: 409 passed and one failed (`dashboard.visual` "a tall card leaves no hole", because the e2e mock's stack list lacked `reminders`, which now gates the Reminders card); the mock was fixed, a Reminders page test added, and the two affected specs passed (18 passed, 2 skipped by design). The whole suite was not re-run after that fix.
- `go test -race` on `internal/app`, `internal/core/stackport`, `internal/archtest`: pass (a race in a test double was found and fixed).
- **Not run:** anything on vertex; the copy against the owner's real database; the Reminders page in a real browser with a real stack status.

## Remaining Work

0. **First file to open:** this note, then `stacks/reminders/reminders.go` (`Start`, `StageLegacy`) and `backend/internal/app/stacks.go` (`newRemindersHost`, `remindersPort`). First command: `git status -sb && ./scripts/check-agent-harness.sh`.

1. **Owner:** approve a commit and a PR for the branch (it also carries the Radio Sensors and Agents record moves, v0.111.0); read ADR-0068 and accept or change it.
2. **Owner, on vertex after a deploy:** check the Reminders page and a reminder set before the deploy (it must still be there, with its id and time); `scripts/verify-deployment.py`
   (Reminders stack 1 of 1 module, the routes answer); set one for two minutes ahead and check it arrives word for word; switch the module off on Stacks → Reminders → Modules, check
   nothing is sent and the page says so, then on again.
3. **Done 2026-11-19:** `reminders` added to the stack test matrix in `.github/workflows/ci.yml` (a parallel job, so no slower); a factory reset erases reminders; the reminder tools are not listed while the module is off.
3c. **Later, owner-approved:** drop the old `reminders` table from the main database once the copy has been seen working (nothing in the code reads it; it is the rollback copy).
4. A `moves-next` list for the same pattern (each its own plan): `memorycapture` review and `delegation` need a run port first; the agent profile catalogue (ADR-0009) is the next without one.

## Traps found

- **The cron grammar cannot live in the SDK.** First built as `stacks/sdk/cron` over `robfig/cron`; `TestStackModulesDeclareOnlyTheSDK` and `TestStacksImportOnlyTheSDK` failed (the SDK requires nothing; core may not import an SDK sub-package). The port keeps the one parser in the core.
- **A timer for an overdue reminder spins the loop while the module is off.** `processDue` returns early when off, but `armTimer` armed a zero-duration timer for the overdue row and the loop ran a query in a circle. `armTimer` now returns no timer while off; `TestSwitchedOff_ArmsNoTimerForAnOverdueReminder` fails without the fix (the fake clock only fires on `Advance`, so the earlier switch test could not see it).
- **The copy of the old rows must run before the stack serves anything.** First built as a call after `Run`: a reminder set from a queued message or the screen at boot took id 1 and the copy then failed on the primary key at every later start. The rows are staged before `Start` and copied inside it; a taken id keeps the old reminder under a new id. Pinned by `TestStageLegacy_*` and `TestImportLegacy_ATakenId...`.
- **A factory reset keeps `orchestrator.db`, where reminders lived.** Removing `reminders.db` would have left an open handle and, with the marker gone, brought the old rows back. The reset hook empties the stack's rows in place (`Stack.EraseAll`, marker kept) and the old table (`clearLegacyReminders`).
- **A database created at start leaves files behind.** `stackHost.Run` stops the stack from `context.AfterFunc`, which is asynchronous, so a test that cancels and cleans its temp folder races the stack closing the file and the app boot tests failed with "directory not empty". The store now creates its file on the first write and reads a missing file as empty; fresh boots write nothing.
- **A settings form that saves a whole block resets a flag inside it.** `reminders.disabled` lives in the block the Settings form saves whole; `applySettingsFields` keeps the stored value, pinned by `TestRemindersSwitch_IsAnOffFlagThatTheSettingsFormNeverChanges`.
- **A bad cron with no start time reads "recurrence never fires again"**, not the grammar's message (the old code did the same); with a start time (the screen) the grammar's own words show. Both are pinned in `internal/app`.
- `internal/app` tests may not import a stack either; they reach it through `remindersStack()` and `importLegacyReminders` in `stacks.go`.
