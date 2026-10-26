# Reminders Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The assistant can store a one-off or recurring reminder from chat and deliver it verbatim at the right time to a chosen or default connector, with an in-app notice.

**Architecture:** New package `backend/internal/core/reminders` (SQLite store + single timer-loop scheduler + delivery), exposed to the agent as three tools in a new `reminders` category, started once from the app bootstrap layer. Delivery goes through `tools.CommunicationTools.NotifyAll`; no LLM at fire time.

**Tech Stack:** Go 1.26.2, `modernc.org/sqlite` via `internal/platform/db`, the existing cron parser (`core/automation/cron.go`), Vue 3 for the settings control.

**Spec:** `docs/PLANS/ARCHIVE/autonomy/reminders-design.md`

## Global Constraints

- Backend gate after each edit: `cd backend && go build ./...`; before finishing: `go test ./...` and `go run ./tools/check-complexity/` (≤12).
- Rule files are mandatory: `.agents/rules/go-staff-engineer.md`; `.agents/rules/frontend-vue-engineer.md` for UI.
- Background goroutine is tethered to the root context and exits on cancel (Constitution II.2/II.14); wired in one place only (AGENTS.md centralization).
- Vague-time table: morning 09:00, afternoon 14:00, evening 18:00, tonight 20:00 in the reminder's timezone. Late window: 12 h. Delivery attempts: 3. Horizon: 1 year. Open reminders: 50.
- One cron grammar only: reuse `core/automation/cron.go` (5 fields or descriptor, no seconds).
- Git: commits on `feat/assistant-reminders` only (user authorised commit/push/PR for this work); no other git mutations.
- Docs via `documentation-stewardship`; `./scripts/check-agent-harness.sh` green at the end.

## Review Focus

- Reminder text containing prompt-injection or markdown/HTML: delivered verbatim, never interpreted.
- Timezone string invalid (`"Mars/Base"`) or empty: tool errors with a clear message / falls back, never panics.
- Cron that never fires again (`until` in the past) or fires every minute: rejected / minimum gap enforced.
- Two reminders due the same instant, and a reminder created while the loop sleeps on a later one: both fire, order preserved.
- Connector send fails or hangs: bounded by context timeout, retried, then `failed`; never marked done.

---

### Task 1: Reminder store

**Files:**
- Create: `backend/internal/core/reminders/store.go`, `store_test.go`

**Interfaces:**
- Produces:
  - `type Status string` consts `StatusScheduled, StatusDone, StatusCancelled, StatusMissed, StatusFailed`
  - `type Reminder struct { ID int64; Message, Cron, Timezone, Connector, CreatedBy, LastError string; FireAt time.Time; Until *time.Time; Status Status; Attempts int; CreatedAt time.Time }`
  - `func NewStore(p db.Provider) (*Store, error)` (runs `CREATE TABLE IF NOT EXISTS reminders …` like `memory/store.go`)
  - `(*Store) Add(ctx, Reminder) (int64, error)`, `Get(ctx, id)`, `ListOpen(ctx) ([]Reminder, error)` (status scheduled, ordered by `FireAt`), `Due(ctx, now) ([]Reminder, error)`, `Update(ctx, Reminder) error`, `Cancel(ctx, id) error`, `CountOpen(ctx) (int, error)`

- [ ] **Step 1:** Write `TestStore_AddListOrderedByFireAt`, `TestStore_DueReturnsOnlyScheduledAtOrBeforeNow`, `TestStore_CancelExcludesFromOpen`, `TestStore_MessageStoredVerbatim` (message `"<b>x</b> '; DROP TABLE reminders;--"` round-trips unchanged) using an in-memory `db.Open`.
- [ ] **Step 2:** Run `cd backend && go test ./internal/core/reminders/ -run TestStore -v`; expect FAIL (package missing).
- [ ] **Step 3:** Implement the store with parameterised queries; times stored as UTC.
- [ ] **Step 4:** Re-run; expect PASS. Run `go build ./...`.
- [ ] **Step 5:** Commit `feat(reminders): sqlite reminder store`.

### Task 2: Time resolution and cron next-fire

**Files:**
- Create: `backend/internal/core/reminders/schedule.go`, `schedule_test.go`
- Read first: `backend/internal/core/automation/cron.go` (export or reuse its parser; if the parser is unexported, add a thin exported `ParseCron(spec string) (cron.Schedule, error)` there and use it from both packages rather than duplicating the grammar).

**Interfaces:**
- Produces:
  - `func ResolveZone(requested, ownerZone string) (*time.Location, error)` — order: `requested` → `ownerZone` → `time.Local`; invalid IANA name → error naming the bad value
  - `func NextFire(r Reminder, after time.Time) (time.Time, bool)` — one-off: `FireAt` if after `after`; recurring: next cron time in `r.Timezone`, `false` when past `Until`
  - `func ValidateNew(r Reminder, now time.Time) error` — rejects past `FireAt`, `FireAt` > now+1y, empty/whitespace message, cron with a gap under 15 minutes, `Until` ≤ now

- [ ] **Step 1:** Tests: `TestNextFire_CronAcrossDST` (`0 9 * * 1` in `Europe/London` across 2026-10-25 yields 09:00 local both weeks, i.e. different UTC hours), `TestNextFire_UntilExhausted`, `TestResolveZone_Order`, `TestResolveZone_Invalid`, `TestValidateNew_Rejects` (table: past time, 2y out, blank message, `* * * * *`).
- [ ] **Step 2:** Run; expect FAIL.
- [ ] **Step 3:** Implement using the shared cron parser.
- [ ] **Step 4:** Run; expect PASS; `go run ./tools/check-complexity/`.
- [ ] **Step 5:** Commit `feat(reminders): timezone resolution and next-fire`.

### Task 3: Scheduler and delivery

**Files:**
- Create: `backend/internal/core/reminders/service.go`, `service_test.go`

**Interfaces:**
- Consumes: Task 1 `Store`, Task 2 `NextFire`
- Produces:
  - `type Deliverer interface { Deliver(ctx context.Context, connector, message string) error }` — `connector` is a connector *type* or `""`
  - `type Notice interface { Notify(ctx context.Context, r Reminder, event string) }` — in-app sink; `event` ∈ `due|late|missed|failed`
  - `type Clock interface { Now() time.Time; NewTimer(d time.Duration) Timer }` (+ `realClock`); tests use a fake
  - `type Config struct { DefaultConnector func() string }`
  - `func NewService(s *Store, d Deliverer, n Notice, c Clock, cfg Config) *Service`
  - `(*Service) Create(ctx, Reminder) (Reminder, error)` — validates, checks `CountOpen < 50`, checks connector exists (via `Deliverer`'s optional `HasConnector(type) bool`, see Task 5), inserts, wakes the loop
  - `(*Service) Cancel(ctx, id) error`, `(*Service) List(ctx) ([]Reminder, error)`
  - `(*Service) Run(ctx)` — blocks until ctx cancelled; on start applies the missed policy (due < 12 h ago → deliver with `"(late) "` prefix; older → `StatusMissed` + notice, recurring ones advance to the next future fire), then sleeps to the earliest `FireAt`, resets on a wake channel, delivers, marks `done` (one-off) or advances `FireAt` (recurring). Failure: `Attempts++`, retry delay 1 min then 5 min; after 3 → `StatusFailed`, `LastError` set, notice `failed`.
  - Unnamed connector resolves to `cfg.DefaultConnector()`; a named one is used as given. A notice is always emitted on delivery, whether or not a connector exists.

- [ ] **Step 1:** Fake-clock tests: `TestRun_FiresAtDueTime`, `TestRun_EarlierInsertWakesLoop`, `TestRun_TwoDueSameInstantBothFire`, `TestRun_CancelBeforeFireNeverDelivers`, `TestRun_RestartDeliversLateUnder12h`, `TestRun_RestartMarksMissedOver12h`, `TestRun_RecurringAdvances`, `TestRun_RetryThenFailed` (Deliverer errors thrice → `failed`, never `done`), `TestRun_ExitsOnContextCancel` (no goroutine left; use `-race`), `TestCreate_UnconfiguredNamedConnectorErrors`, `TestCreate_LimitOf50`.
- [ ] **Step 2:** `go test -race ./internal/core/reminders/`; expect FAIL.
- [ ] **Step 3:** Implement. Per-delivery context timeout 30 s. Do not log message bodies.
- [ ] **Step 4:** Run with `-race`; expect PASS; complexity check.
- [ ] **Step 5:** Commit `feat(reminders): scheduler, missed policy and delivery`.

### Task 4: Agent tools

**Files:**
- Modify: `backend/models/tools.go` (consts `ToolScheduleReminder = "schedule_reminder"`, `ToolListReminders = "list_reminders"`, `ToolCancelReminder = "cancel_reminder"`; `CategoryReminders = "reminders"`), `backend/internal/core/tools/manifest_sync_test.go` (add rows)
- Create: `backend/internal/core/tools/manifests/reminders.json` (three tools; check `manifests.go` for how multi-tool files are loaded, mirror `memory`'s), `backend/internal/core/tools/reminders.go`, `reminders_test.go`
- Modify: `backend/internal/core/assistant/registry.go` — field `Reminders *tools.ReminderTools`, `registerReminderTools()`, call from `registerAll()`, `init` helper and wiring in `InitializeAgentStack` (architecture.md "new tool category" checklist)

**Interfaces:**
- Consumes: Task 3 `Service` via a local interface `ReminderService { Create; Cancel; List }` declared in `tools/reminders.go` (keeps `tools` free of the `reminders` import; adapter lives in the app layer, Task 5)
- Produces: `type ScheduleReminderArgs struct { Message string "json:message"; When string "json:when"; Cron string "json:cron"; Until string "json:until"; Timezone string "json:timezone"; Connector string "json:connector" }`; `when` is RFC 3339. Result string states the absolute local time, IANA zone, channel and id, e.g. `Reminder 7 set for Mon 12 Oct 2026 09:00 Europe/London via telegram`. Manifest description tells the model: compute `when` from the current time and `Timezone` in its system prompt, pass the IANA zone, map "morning/afternoon/evening/tonight" per the table, repeat the confirmation to the user verbatim, and never claim success if the tool errors.

- [ ] **Step 1:** Tests: `TestScheduleReminder_EchoesAbsoluteTimeZoneChannel`, `TestScheduleReminder_PastTimeError`, `TestScheduleReminder_BadTimezoneError`, `TestScheduleReminder_RequiresWhenOrCron`, `TestListReminders_FormatsOpenOnly`, `TestCancelReminder_UnknownID`; extend `TestToolManifestConsistency`; add an agent-loop style test in `assistant` (mock LLM calls `schedule_reminder` → a row with the resolved UTC time exists) following an existing registry test.
- [ ] **Step 2:** Run `go test ./internal/core/tools/ ./internal/core/assistant/ -run 'Reminder|Manifest' -v`; expect FAIL.
- [ ] **Step 3:** Implement tools and registration.
- [ ] **Step 4:** Run; expect PASS; `go build ./...`.
- [ ] **Step 5:** Commit `feat(reminders): schedule/list/cancel assistant tools`.

### Task 5: App wiring, config and in-app notice

**Files:**
- Modify: `backend/internal/app/app_context_runtime_data.go` (create the store from `dbProvider` next to `memStore`; accessor `ReminderStore()`), `backend/internal/app/app.go` (start `Service.Run` tethered to the root ctx beside `startMemoryJobs`), `backend/internal/app/automation_notifier.go` (add the `Deliverer` adapter over `CommunicationTools.NotifyAll`, plus `HasConnector(type)`), `backend/models/config.go` (settings: `DefaultReminderConnector` under `CommunicationConfig`, `OwnerTimezone` as a top-level optional setting — confirm exact placement while following the `config-persistence` skill: settings → registry → API/UI mapping → reload)
- Modify: `backend/internal/transport/http/handlers/webhook_handlers.go:181` — stop hard-coding `"UTC"` as the *reminder* zone: connector-originated sessions must resolve through `ResolveZone("", ownerZone)`.
- Discover first (read-only, record in the commit message): the lightest existing mechanism for an in-app notice (event bus type in `core/eventbus` / `assistant` `AgentEventType` vs a REST list). If none fits, add `GET /admin/api/reminders` (list open + recent done/missed/failed) and publish nothing else.

**Interfaces:**
- Consumes: Tasks 3–4. Produces: `Notice` implementation; `GET /admin/api/reminders` if chosen.

- [ ] **Step 1:** Tests: adapter `TestDeliverer_NamedTypeMissingErrors`; `TestApp_StartsReminderLoopAndStopsOnCancel` (mirror `bootstrap_test.go` style); config round-trip test per `config-persistence` (default connector and owner timezone persist, reload, and invalid timezone is rejected on save).
- [ ] **Step 2:** Run affected packages; expect FAIL.
- [ ] **Step 3:** Implement wiring, config fields, webhook fallback, notice sink/endpoint; update `docs/api-reference.md` if an endpoint is added.
- [ ] **Step 4:** `go build ./... && go test ./...`; expect PASS.
- [ ] **Step 5:** Commit `feat(reminders): app wiring, settings and in-app notice`.

### Task 6: Settings control and notice UI

**Files:**
- Modify: existing settings component where communication connectors are edited (`CommunicationSettings.vue`) for default reminder channel; owner timezone field in the general settings tab; one notice surface (Overview card or Activity row — pick the one the Task 5 discovery supports; follow SPEC-003 and the retro design tokens, no new tab).
- Test: matching Vitest files beside the components.

- [ ] **Step 1:** Write component tests: default-channel select lists only configured connector types and saves; owner timezone rejects invalid names; notice renders due/late/missed/failed states.
- [ ] **Step 2:** `cd frontend && npm test`; expect FAIL.
- [ ] **Step 3:** Implement; clean, simple UI per the user's standing UI bar.
- [ ] **Step 4:** `npm test && npm run build && npm run test:visual`; expect PASS (update snapshots only for the intended change).
- [ ] **Step 5:** Commit `feat(reminders): settings control and notice UI`.

### Task 7: Docs, review gate, PR

**Files:**
- Modify/Create per `documentation-stewardship`: new SPEC entry for reminders and `docs/SPECS/README.md` row, SPEC-009 note on the default channel, `docs/architecture.md` (package map, pitfall: reminders are verbatim, loop wiring location), `docs/PLANS/autonomy/agenda-reminders-and-followups.md` (mark Phase 2 and the reminder part of Phase 3 delivered), `docs/PLANS/README.md`, `docs/INDEX.md` if a catalog row is needed.

- [ ] **Step 1:** Update docs; run `./scripts/check-agent-harness.sh`; expect green.
- [ ] **Step 2:** Pre-Completion Review per AGENTS.md: `go test ./...`, complexity, `go test -race ./internal/core/reminders/`, `npm test`, `npm run build`; review `git diff HEAD` line by line; check goroutine/timer leaks and error paths. Dispatch the `reviewer` agent on the branch diff.
- [ ] **Step 3:** Commit remaining work; `git push -u origin feat/assistant-reminders`; open the PR with `gh pr create` using the `pr-commit-message` skill (attribution line from the session reminder). Report each gate pass/fail/clean to the user.
