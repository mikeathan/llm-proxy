---
status: complete
related_specs: [SPEC-004, SPEC-007, SPEC-003]
---
# Memory Defaults + Workspace Heartbeat — Implementation Plan

> **Status (2026-10-04): complete.** Part A (memory defaults) and Part B (workspace heartbeat) are built,
> tested and documented; see *As built* at the end for where the implementation differs from this plan. **No legacy support** (decided 2026-10-04): the
> product is new, so old values, old starters and old config paths are deleted, not migrated.
> Done already: dead `MemoryConfig.Enabled` removed (`models/infrastructure.go`, `paths_test.go`).
> **For agentic workers:** use `superpowers:executing-plans` (or subagent-driven) task by task. Steps use
> checkbox syntax. Repo rules apply: TDD, `.agents/rules/go-staff-engineer.md`,
> `.agents/rules/frontend-vue-engineer.md`, Pre-Completion Review (AGENTS.md).

**Goal:** (A) one global memory default per surface with explicit per-assistant and per-automation
overrides; (B) heartbeat as one simple per-workspace setting instead of a preset spread over the
automation form.

**Architecture:** both reuse the *inherit* pattern: a value is `inherit | on | off`; effective value =
override if set, else the global default. Heartbeat is a small per-workspace config that the dispatcher
compiles into an ordinary automation entry, so scheduler, lanes, history, delivery and dedup are unchanged.

**Tech Stack:** Go 1.26 (`backend/`), Vue 3 + TS + Tailwind tokens (`frontend/`), settings in `settings.yml`.

## Verified facts this plan builds on (code, not assumption)

- Assistant hot memory is hardcoded on: `WithHotMemory(true)` at `core/assistant/conversation_service.go:262`.
- Automation hot memory is per automation: `models.Automation.MemoryMode` (`models/workspace.go:117`),
  values `""`/`off`/`hot` (`models/memory_mode.go`), validated `handlers/dispatcher_handlers.go:107`, applied
  `automation/executor.go:429` via `req.MemoryMode.HotEnabled()`, request built at `automation/execution.go:419`.
- `MemoryConfig.Enabled` was dead (nothing read it) and is **already removed**; only `SessionRetention()` is used
  (`app/app.go:121`). An old `settings.yml` still carrying `enabled:` loads fine (YAML decoding is not strict).
- Heartbeat today is a convention: `heartbeat.md` (`models.HeartbeatFilename`), `HEARTBEAT_OK` quiet marker
  (`automation/executor.go:628-650`), per-automation `skip_if_busy` and `model`, a preset panel in
  `AutomationForm.vue` (`utils/automation/heartbeat.ts`). No empty-file check exists. Starter text:
  `prompts.DefaultHeartbeat` (`templates.go:209`), written at workspace creation
  (`handlers/dispatcher_handlers.go:475`).
- Lanes: `LaneKeyFor(model)` → cloud lane for cloud models, local lane otherwise; empty model resolves the
  registry primary (`app/inbound_gate.go:117`). Cloud runs never wait on the GPU.
- `registerWorkspaceAutomations` (`automation/scheduling.go:164`) is the one place automations are
  registered, including a legacy `default` automation from `cron_schedule`.

## Global Constraints

- Defaults (chosen from the repo's own evidence, see *Default rationale*): assistant memory **on**,
  automation memory **off**, heartbeat **off** and heartbeat runs **never** inject memory.
- `memory_mode` wire values are the clean set `""` (inherit), `"on"`, `"off"`. The old `"hot"` is **not**
  accepted: it fails validation on save and reads as inherit.
- Heartbeat is **one per workspace**, reserved automation name `heartbeat`. `cron_schedule` and its hidden
  `default` automation are deleted.
- Never send an empty task to the model. A skipped tick makes no LLM call.
- UI: semantic tokens only; `Panel`, `FormField`, `ToggleField`, `BaseButton`; plain-English copy; every
  surface has loading, empty, error and saved states; keyboard and screen-reader complete; numbers via
  `utils/format/`; navigation via typed route builders; unsaved edits via `useUnsavedChangesGuard`.

## Review Focus

- `memory_mode: ""` resolves to the **global automation default**; a stale `"hot"` or `"bogus"` 400s on save and
  reads as inherit, never panics.
- `heartbeat.md` containing only comments or whitespace is "no checks" → skipped, no model call.
- Saving an automation named `heartbeat` while the workspace heartbeat exists is rejected with a clear message.
- Heartbeat with an empty model on a local primary must warn that every tick wakes the local model.
- A workspace file that still has `cron_schedule:` must load (field ignored), not fail.

---

# Part A — Memory defaults and overrides

### Task A1: Tri-state value + resolver (models)

**Files:** Modify `backend/models/memory_mode.go`, `backend/models/infrastructure.go`,
`backend/models/workspace.go`; Test `backend/models/memory_mode_test.go`.

**Interfaces — Produces:**
- `MemoryMode` values: `MemoryInherit = ""`, `MemoryOn = "on"`, `MemoryOff = "off"` (drop `MemoryModeHot`).
- `func (m MemoryMode) Effective(globalDefault bool) bool` — `on`→true, `off`→false, `""`→globalDefault;
  replaces `HotEnabled()` (delete it; update callers).
- `MemoryConfig.AssistantHot *bool` (`yaml:"assistant_hot,omitempty"`), `AutomationHot *bool`
  (`yaml:"automation_hot,omitempty"`); accessors `func (c MemoryConfig) AssistantHotDefault() bool` (nil→true)
  and `AutomationHotDefault() bool` (nil→false), nil-safe on a nil `*MemoryConfig` caller side as
  `SessionRetention` is.
- `WorkspaceConfig.AssistantMemory MemoryMode` (`yaml:"assistant_memory,omitempty" json:"assistant_memory,omitempty"`).

- [ ] **Step 1:** Write `TestMemoryModeEffective` (table: each mode × default true/false → expected) and
  `TestMemoryConfigDefaults` (nil → assistant true, automation false; explicit false/true honoured).
- [ ] **Step 2:** Run `cd backend && go test ./models/ -run 'MemoryMode|MemoryConfig'` → FAIL (undefined).
- [ ] **Step 3:** Implement the three items above; `Valid()` unchanged.
- [ ] **Step 4:** Same command → PASS; `go build ./...` fails only at `HotEnabled` callers (fixed in A2).

### Task A2: Wire assistant + automation, validate, expose (backend)

**Files:** Modify `core/assistant/conversation_service.go:262`, `core/automation/execution.go:419`,
`core/automation/executor.go:67,429`, `transport/http/handlers/dispatcher_handlers.go` (workspace-config and
automation validation), settings handler (locate: the handler behind `GET/PUT` of `settings.yml` memory);
Tests beside each.

**Interfaces — Consumes:** A1. **Produces:** `ExecuteRequest.HotMemory bool` (resolved) replacing
`ExecuteRequest.MemoryMode`; conversation service reads the workspace's `assistant_memory` and the global
`AssistantHotDefault()`.

- [ ] **Step 1:** Tests: `TestConversationBuilder_HotMemoryFollowsOverride` (workspace override off beats
  global on; inherit follows global; override on beats global off), `TestExecuteRequest_HotMemoryResolved`
  (`""` + automation default false → false; `""` + default true → true; `off` + default true → false;
  `"hot"` → rejected),
  `TestWorkspaceConfigRejectsBadAssistantMemory` (400 with the valid values listed, same wording as
  `dispatcher_handlers.go:108`), `TestSettingsMemoryDefaultsRoundTrip`.
- [ ] **Step 2:** Run them → FAIL.
- [ ] **Step 3:** Implement. Resolve at request construction (one place per surface). Settings are read live
  through closures wired at startup (precedent: the retention closure at `app/app.go:121`, reading
  `dataMgr.Settings().Get()`); add `AutomationHotDefault func() bool` to the dispatcher/executor deps and
  `AssistantHotDefault func() bool` to the conversation service deps, both set in `app/bootstrap.go` from
  `Settings().Get().Memory`. A settings change then applies to the next run without a restart. Do not add a
  7th `NewLocalToolRegistry` param.
- [ ] **Step 4:** `go build ./... && go test ./...` → PASS.

### Task A3: Memory UI (frontend)

**Files:** Modify `components/settings/GlobalSettings.vue` (new "Memory" `Panel`),
`components/AgentIde/automation/AutomationForm.vue:78-84,263-270` (Memory field),
`components/AgentIde/memory/MemoryPanel.vue` (assistant override), `AutomationDetails.vue` (effective tag),
`types/automation.ts` (`MemoryMode`), settings + workspace services/types; Tests under `__TESTS__/`.

UI spec (one pattern, three places):
- **Settings → Memory panel:** two `ToggleField`s — "Assistant remembers" (on) and "Automations remember"
  (off). Hint each in one sentence: what is injected and that it uses part of the context window.
- **Automation form:** replace the select with a three-option control: *Use default (currently off)*, *On*,
  *Off*. The first label shows the live global value, so nobody has to leave the form to learn it.
- **Workspace → Memory panel header:** a compact control "Assistant in this workspace" with the same three
  options and live default.
- **Automation details:** show the effective value with a quiet "(default)" suffix when inherited.

- [ ] **Step 1:** Component tests: `InheritField` renders the live default in its first option and
  emits `''|'on'|'off'`; Settings panel saves both toggles; automation form round-trips `memory_mode`.
- [ ] **Step 2:** Run `cd frontend && npx vitest run src/__TESTS__/components` → FAIL.
- [ ] **Step 3:** Build one small `components/common/forms/InheritField.vue` (props: `modelValue`,
  `defaultOn: boolean`, `label`, `hint`) and use it in the automation form and Memory panel; the Settings
  panel uses `ToggleField`. Reuse `FormField` for labels/hints.
- [ ] **Step 4:** `npm test && npm run build && npm run test:visual` → PASS (theme tokens unchanged; update
  snapshots only for the intended panels, and review them).

---

# Part B — Workspace heartbeat

### Task B1: Config, starter, body extraction, compile to automation (backend)

**Files:** Modify `models/workspace.go` (`HeartbeatConfig`, `WorkspaceConfig.Heartbeat`), create
`models/heartbeat.go` + `models/heartbeat_test.go`, modify `core/assistant/prompts/templates.go:209`,
`core/automation/scheduling.go:164`, `handlers/dispatcher_handlers.go` (reserved-name validation).

**Interfaces — Produces:**
- `type HeartbeatConfig struct { Enabled bool; Every string; Model string; Notify *NotifyConfig }`
  (yaml `heartbeat,omitempty`; `Every` is an interval trigger value, default `30m`).
- Delete `WorkspaceConfig.CronSchedule`, the hidden `default` automation in `registerWorkspaceAutomations`,
  and the cron field in `WorkspaceSettings.vue` with its tests (`WorkspaceSettings.component.test.ts`,
  `models/config_test.go:48`); unknown YAML keys stay ignored so old files still load.
- `const HeartbeatAutomationName = "heartbeat"`.
- `func HeartbeatBody(content string) string` — strips `<!-- … -->` comments (multi-line), trims; returns `""`
  for blank or comments-only content.
- `func (h HeartbeatConfig) Automation() *Automation` — `Trigger{interval, Every}`, `TaskFile heartbeat.md`,
  `Strategy isolated`, `SkipIfBusy` (see *Skip-if-busy*), `Model`, `Notify`, `MemoryMode MemoryOff` (explicit, so a global
  "automations remember" switch can never make a 48-ticks-a-day check pay the prefill cost).

- [ ] **Step 1:** Tests: `TestHeartbeatBody` (blank; comments only; comment + one line → that line; text containing `<!--`
  inside a code fence is kept — document the limit), `TestHeartbeatAutomation` (fields above),
  `TestRegisterWorkspaceAutomations_Heartbeat` (enabled → one registered entry named `heartbeat`; disabled →
  none; a user automation named `heartbeat` is rejected at save with a clear message),
  `TestWorkspaceConfig_IgnoresLegacyCronSchedule` (file with `cron_schedule: "0 * * * *"` loads, registers nothing).
- [ ] **Step 2:** Run `go test ./models/ ./internal/core/automation/` → FAIL.
- [ ] **Step 3:** Implement; new starter is comments only (what to watch, importance bar, the `HEARTBEAT_OK`
  rule) — no placeholder bullet. `registerWorkspaceAutomations` appends `Heartbeat.Automation()` when enabled.
- [ ] **Step 4:** Tests → PASS.

### Task B2: Skip on no checks, send the body, record status, expose API (backend)

**Files:** Modify `core/automation/execution.go` (task read in `prepareRun`), `core/automation/executor.go`,
`platform/persistence/` state (`State.Heartbeat *HeartbeatStatus`), `handlers/dispatcher_handlers.go`,
`app/routes.go`; Tests beside each.

**Interfaces — Produces:**
- `type HeartbeatStatus struct { At time.Time; Result string }` — `Result` ∈ `quiet`, `alert`,
  `skipped_no_checks`, `skipped_busy`, `error`.
- `GET /admin/api/dispatcher/workspaces/{ws}/heartbeat` → `{config, status, lane: "local"|"cloud",
  wakes_local_model: bool, has_checks: bool}`; `PUT` same path saves `config` (validates `every` against the
  interval grammar the trigger already uses; `model` must resolve).

- [ ] **Step 1:** Tests: `TestHeartbeatTick_NoChecksSkipsWithoutModelCall` (stub client asserts zero calls;
  status `skipped_no_checks`), `TestHeartbeatTick_SendsBodyWithoutComments`, `TestHeartbeatStatus_QuietAndAlert`,
  `TestHeartbeatStatus_SkippedBusy`, `TestHeartbeatTick_SkipIfBusyOnlyOnLocalLane` (local model + busy lane →
  skipped; cloud model + busy cloud lane → queued, not skipped; lane re-resolved when the primary changes), `TestHeartbeatAPI_WakesLocalModel` (empty model + local primary → true;
  pinned cloud model → false, lane `cloud`), `TestHeartbeatAPI_RejectsBadInterval`.
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3:** Mark heartbeat entries (`AutomationEntry.Heartbeat bool` set when compiled from config or
  name reserved). In `prepareRun` apply `HeartbeatBody` before building the task; empty → record status and
  return a skipped outcome (no run record, no lane submission). Record `quiet`/`alert` where
  `ApplyPulseLogic` runs; `skipped_busy` where the lane reports `DispositionSkipped`.
- [ ] **Step 4:** `go build ./... && go test ./... && go run ./tools/check-complexity/` → PASS. Run `-race`
  (lifecycle code touched).

### Task B3: Heartbeat section UI, remove the old panel (frontend)

**Files:** Create `components/AgentIde/heartbeat/HeartbeatPanel.vue`, `composables/automation/useHeartbeat.ts`,
`services/automation/heartbeatService.ts`; modify `views/WorkspacesView.vue` + `router/routes.ts` +
`types/routes.ts` (new `heartbeat` section with a typed builder), `AutomationForm.vue` (delete the Heartbeat
`Panel` and `applyHeartbeat`), `utils/automation/heartbeat.ts` (keep marker/file constants and
`modelWakeNotice`; delete `heartbeatPreset`); Tests under `__TESTS__/`.

UI spec — one `Panel` titled "Heartbeat", top to bottom:
1. Header row: title, one-line purpose ("A frequent check that only speaks up when something needs attention."),
   `ToggleField` On/Off (default off).
2. Fields (disabled while off): **Check every** (select: 5m, 15m, 30m, 1h, 2h, 6h; an existing other value is
   shown and preserved), **Model** (the same connection picker the automation form uses), **Send alerts to**
   (the same connector select).
3. Notices, only when relevant: warning when `wakes_local_model` ("Every check starts the local model. Pick
   a cloud model to keep it asleep."); neutral empty state when `!has_checks` ("No checks yet — add what to
   watch in heartbeat.md. Until then every check is skipped at no cost.") with an "Edit heartbeat.md" link
   via `toWorkspaceFile`.
4. Status line: "Last check 14:30 — quiet" using `formatRelativeTime`/`formatAbsoluteTime`; states quiet,
   alert, skipped (no checks / busy), error (cause + next step).
5. Footer: standard save/discard via the shared settings actions pattern and `useUnsavedChangesGuard`.
6. Skip-if-busy is not shown: it is always on for heartbeat (see *Skip-if-busy*).

- [ ] **Step 1:** Tests: toggle off by default; saving sends `{enabled, every, model, notify}`; local-model
  warning appears only when the API says so; empty-checks notice; status line renders each state;
  automation form no longer renders a Heartbeat panel; workspace settings no longer shows a cron field.
- [ ] **Step 2:** Run `npx vitest run` → FAIL.
- [ ] **Step 3:** Implement; reuse `Panel`, `FormField`, `ToggleField`, `BaseButton`, existing pickers; no
  new colours or shadows; layout stacks cleanly at phone width.
- [ ] **Step 4:** `npm test && npm run build && npm run test:visual` → PASS; screenshot the panel in each
  state at desktop and phone width and review before declaring done.

---

### Task C1: Docs and harness

**Files:** Modify `docs/SPECS/memory.md` (§ automation opt-in → modes and defaults), `docs/SPECS/automation-dispatcher.md`
(heartbeat section, reserved name, skip semantics, status), `docs/SPECS/discovery-panel.md` (new panels),
`docs/api-reference.md` (heartbeat endpoint, memory defaults), `docs/guides/automation-digest.md` (heartbeat
setup), `docs/PLANS/README.md` (row for this plan).

- [ ] **Step 1:** Update each doc to match shipped behaviour only.
- [ ] **Step 2:** `./scripts/check-agent-harness.sh` → `PASS: agent harness is consistent.`
- [ ] **Step 3:** Run the AGENTS.md Pre-Completion Review and report each gate.

## Default rationale (evidence from the repo)

- Hot memory injects only the facts the operator tagged hot, capped at 8% of a local model's window (5%
  cloud, clamped 400–6000 chars) — `docs/SPECS/memory.md`. Nothing is injected when none are tagged, so the
  cost is zero until the operator opts facts in.
- The cost is paid in prefill on **every** run (200–650 tok/s on an 8K local window —
  `docs/PLANS/memory/small-context-memory.md`), and whether hot memory *improves* a small model's results is
  recorded there as **unproven**.
- So: **assistant on** (interactive, operator-curated, already shipped behaviour, user feels the benefit
  immediately); **automations off** (unattended, deterministic playbooks gain nothing and pay per run; opt in
  per automation where an interest profile helps, e.g. the news brief); **heartbeat memory always off**
  (cost × frequency). Revisit the automation default only after the memory scoreboard shows a win.

## Skip-if-busy

Derived, not a setting: a heartbeat tick uses skip-if-busy **only when its model runs on the local lane**. The
local lane runs one thing at a time, so a tick that fires while a chat or another run holds the GPU is dropped
instead of queueing (and a tick preempted by a chat is dropped, not restarted). A cloud heartbeat never waits
on the GPU, so it queues normally; the scheduler already allows at most one queued run per automation
(`ErrAlreadyQueued`), so a slow cloud tick cannot pile up. The lane is resolved **at fire time** with
`LaneKeyFor(model)` (an empty model follows the registry primary), so changing the primary or the heartbeat's
model takes effect on the next tick. The Heartbeat panel does not show the option; regular automations keep
their own `skip_if_busy` toggle (same mechanism).

## As built (2026-10-04)

Deviations from the plan above, so the plan is not read as the contract:

- **Heartbeat identity**: the compiled automation is recognised by its reserved name
  (`isHeartbeat(entry)`), not by an `AutomationEntry.Heartbeat` flag.
- **Admission, not `prepareRun`, skips an empty heartbeat**: `admitRun` checks `heartbeat.md` before the lane
  is touched; `prepareRun` still applies `prompts.HeartbeatTask` and treats an emptied file as a missing task.
- **Status file**: the last check is stored in `meta/<ws>/heartbeat-status.json`, not in `state.json`
  (a read-modify-write of `state.json` outside the workspace lock would race a running automation).
- **Dispatcher interface**: `HeartbeatState(ws)` lives on `handlers.Dispatcher`; handlers are
  `GetHeartbeat`/`PutHeartbeat` in `heartbeat_handlers.go`. `validateRunOptions` validates the compiled
  automation's `notify`, so the heartbeat accepts exactly what an automation accepts.
- **Memory settings plumbing**: `MemorySettings() *models.MemoryConfig` was added to `LLMServiceProvider`,
  `ConversationDeps` and `handlers.AssistantService` (implemented by `AppServices`) instead of closures wired in
  `bootstrap.go`; settings are still read live. The admin config view always carries the resolved
  `memory: {assistant_hot, automation_hot}`; `PUT /admin/api/config` accepts the same pair.
- **UI**: the Heartbeat panel uses its own Save/Discard footer (not `SettingsActions`) and `UnsavedTag`; the
  model picker is one grouped select (Local / Cloud models) rather than the automation form's two-step
  connection → model. The automations list and detail header send the compiled heartbeat to its settings
  instead of offering Edit/Delete. `useConnectorOptions` was extracted from `useAutomationForm` so both
  forms share it.
- **Cleanups made on the way**: dead `MemoryConfig.Enabled` removed; the earlier search-time-range tests were
  merged into the file that mirrors their source (`search_registry_test.go`).

## Out of scope

Heartbeat active hours; per-automation "last quiet check" beyond the workspace status line; a global
cross-workspace heartbeat.
