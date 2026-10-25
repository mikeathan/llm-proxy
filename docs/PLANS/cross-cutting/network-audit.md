# Network audit — guarded programs, an nmap scan tool, and a nightly auditor agent

**Status:** proposed (2026-10-24, revised 2026-10-25 after a plan-vs-code review). D1–D12 decided by the owner
(§2); open facts in §11. Phases 0–3 merged (PR #137); Phase 4 built on branch `network-audit-engine` (2026-10-25).
**Related Specs:** SPEC-006 (Guardrails §II.6–II.7), SPEC-007 (Automation Dispatcher), SPEC-012 (Prompt Profiles),
SPEC-001 (Agent Loop), SPEC-003 (UI), SPEC-009 (Communication).
**Constitution refs:** I.1–I.3 (network guardrails), II.3 (sandbox isolation, env allow-list), II.10 (guardrail
decision flow), II.13 (prompt centralisation), III.3 (workspace jail), IV.4 (no dead code), V.1 (amend after a
security review), V.3 (no half-finished work).
**Depends on / defers to:** [`admin-api-authentication.md`](admin-api-authentication.md) — **parked by the owner**
(§9). **Builds on:** [`agent-os-sandboxing.md`](agent-os-sandboxing.md), [`sandbox-and-egress-residuals.md`](sandbox-and-egress-residuals.md),
the R0 category table (`models/tools.go` `toolsByCategory`).
**Origin:** the owner's request (2026-10-24): a *generic* agent able to run nmap-class network analysis, with a nightly
vulnerability audit as the first case study, safe against leaking sensitive information. Two independent model reviews
(Opus) against the code; corrections are folded in and tagged *(review 1)* / *(review 2)*. §12 is the review log.

## 1. Goal, and what this is not

**Goal.** An agent can run a powerful network tool (nmap first) on a schedule, in a chat, or by hand, against ranges the
operator has declared, and report what it finds — without the model choosing flags, reaching targets outside the
declared scope, or seeing data it should not.

**The audit is a use of the capability, not the capability.** The tool knows nothing about "nightly" or "vulnerability
audit". The *Security auditor* agent (a profile file), an automation template and the report format give it that job.

**Not in this plan:** packet capture (Appendix A), agent-applied fixes (v1 proposes; the operator applies, then
rescans), internet CVE lookup, any admin-API authentication work, a database.

## 2. Decisions

Owner, 2026-10-24:

| # | Decision |
|---|---|
| D1 | Admin API auth is **not** part of this work. A dependency note is in the auth plan (§9). Scripts and intrusive modes against the host's own addresses wait for it; quiet scans of the host are allowed. |
| D2 | A **new tool category** (`netaudit`), not `network`. The scattered network-tool facts are **merged into one catalog** first (§4.1). |
| D3 | **nmap-class power is the goal.** nmap is the primary engine; the Go prober is a labelled, limited fallback. The design stays open to other programs (§4.3, Appendix A). |
| D4 | nmap is **detected, never installed**. Missing → a notification with the OS-specific command (macOS `brew`, Linux `apt`/`dnf`). |
| D5 | Missing nmap in an **unattended audit stops the run** with a clear reason (fail fast). An ad-hoc chat may fall back to the limited engine, **labelled**. A fallback never runs silently. |
| D6 | No OS file jail (macOS dev): **fail closed**, with an in-place "allow unconfined scans (development only)" shortcut. Production is Ubuntu (Landlock). |
| D7 | The scan-scope list is **host-wide**, empty = scanning off, with a one-click "add my network". |
| D8 | Acknowledgements: a **JSON file per workspace**, operator-only, expiring. The previous-scan baseline: **latest only**. |
| D9 | The auditor is available in the operator's **own web chats and in automations**, not in connector chats. |

Proposed by the review, **need the owner's yes**:

| # | Change from the first draft | Why |
|---|---|---|
| D10 | **Nothing new goes into `assistant`** (owner rule, 2026-10-25: "assistant is too big, period"). The tool registry (`registry.go`, `tool_provider.go`, ~825 lines) **moves out** into its own package first (Phase 1b), and the module seam (§4.2) is built there; `assistant` ends smaller. **Decided by the owner.** | Review 2 showed the seam is not self-contained inside `assistant`; the registry has few outside users, so the move looks small (§4.2). |
| D11 | **A tool whose prerequisites are unmet is hidden**, as SPEC-006 §II.6 and the search-provider precedent do; the reason is shown by the readiness UI and, if the tool is somehow called, by the existing `models.ErrToolUnavailable`. **Accepted by the owner, 2026-10-25.** | The first draft contradicted the spec, and `registerScopedTool` can only hide (§4.6). |
| D12 | **The auditor profile lists only `netaudit`.** No `filesystem` category (it cannot be read-only and includes `write_file`, which could edit past reports); the report is written by the executor, not the model. **Decided by the owner, 2026-10-25** (option A: no file tools; the auditor needs none). | Profiles select whole categories (`profile.go`). |

## 3. Where things stand (verified)

*Verified by me against the code unless tagged "review 2 only".*

- `scan_local_network` is an in-process TCP connect + banner scan (`core/tools/network.go:390`), capped at 1,024
  addresses per range (`:220`), with 12 tests (`network_scan_test.go`). The scanner dials with a **bare**
  `net.Dialer` at `:400`; the dialer at `:41` is wrapped by the guarded `DialContext`.
- `queueHost` checks a host name but queues the **name**, not the resolved IP (`:251-261`); workers resolve it again at
  dial time (rebinding gap). At `internet` scope `validateIP` allows public IPs and `scopeForbiddenTools` forbids
  nothing. The cap check is at `:231` and lets 1,025 addresses through; `ValidateScanTargets` has an empty branch
  (`:651-655`).
- `terminal` is not in `hostNetworkGatedTools` (`guardrails.go:36`). *Nuance (review 2):* `nmap` is not in the default
  `allowed_commands` (`terminal.json`), but `python3` and `bash` are, so the shell stays an open path in general; at
  scope `none` Landlock ABI ≥ 4 denies TCP to shells (SPEC-006 §II.7.5). The dedicated tool is still right, because
  with network on, the shell's targets are uncontrolled.
- `sandbox.New` returns a **no-op provider** where no mechanism exists (macOS, or `sandboxing.filesystem` off on any
  OS) (`sandbox.go:97-116`). Landlock `Wrap` returns nil — **no jail** — when `RootPath == ""`
  (`landlock_provider_linux.go:158-160`). `appendChildEnv` inherits the full `os.Environ()` when `cmd.Env` is nil
  (`runner.go:108-112`). `networkOn` is all-or-nothing, so target control is the Go pre-check on every OS.
- Network facts are in more than seven places: `models/tools.go:73-75,126`, `assistant/registry.go:624-640`,
  `guardrails.go:36,224,249,379-388,425-433,458-464,500`, `toolpolicy/policy.go:24-26,60-66`, `profile/profile.go:37,197`,
  `profile/profile_schema.go:45`, `guardrails/remember.go:77`, `delegation/runner.go:35`, frontend
  `utils/assistant/toolSteps.ts:26`, templates (`compliance_check_internal.md:16`, `sandbox-conformance-probe.md`),
  SPEC-006 text, and the JSON manifest *(review 2 only for the non-Go sites)*. The manifest-sync test already checks
  category↔manifest (`TestToolCategoriesMatchManifests`, `manifest_sync_test.go:72-80`).
- `assistant` has 28 non-test files and 8,964 non-test lines (25.5k with tests); `core/tools` has 17 non-test files, over
  the ~15-file cap.
- `registerScopedTool` takes a tool **out of `ListTools`** (schema, manual, argument validation) and its handler returns
  `ErrToolNotInternal` for other contexts (`registry.go:651-665`).
- `run.ResolveOptions` (`core/run/options.go`) returns an options struct, takes the model as input and acquires no client.
  Clients are acquired elsewhere *(review 2)*: `executor.go:283-291`, `chat/service.go:291-293`, the memory-review
  lease (`chat/service.go:108-128`, `modelFor` can pick the fallback), and `delegation/runner.go:142`.
- An automation with no model is pinned to the primary and does not reach the failover (`executor.go:283-287`);
  **failover is reachable from chat** (`chat/service.go:291`) *(review 2; my first draft said otherwise)*.
- Reports live **inside** the workspace jail on purpose, are pruned by `MaxSavedReports`, and a later chat in that
  workspace — even on a cloud model — can `read_file` them (`persistence/reports.go`). The final report is written to
  four places: the run dir, `state.json` `Output` (unredacted), `reports/` (redacted, `executor.go:519`) and
  `resp.Report` *(review 2)*.
- Failure handling: a prepare failure is recorded in history (`execution.go:86-90`) but only an executor failure calls
  `notifyFailure`; the limiter is per automation, 1-hour cooldown, reset on success (`notify.go:24-66`) *(review 2)*.
- Always-on profile categories are `system`, `security` (`security_guardrails`) and `automation` (`profile.go:37`).
- `TestAuditNoOutOfBandEgress` (`transport_test.go:107`) flags `http.DefaultClient`, `http.Get/Post…`,
  `os.StartProcess` and clients without a Transport; it does **not** flag `net.Dialer`/`net.Dial` — which is how the bare
  scanner dialer passes *(review 2)*.
- The shipped template `backend/data/templates/network_recon_unprivileged.md` calls `scan_local_network` with
  `fast`/`deep` (lines 16, 20).
- Frontend: `common/layout/Disclosure.vue` and `common/feedback/Callout.vue` exist; `useHostNetworkState` creates fresh
  state per call (it is **not** a singleton).

## 4. Architecture

### 4.0 Rules this design follows (`clean-code`, `engineering-practices`, the Go and Vue rules)

- **One source of truth; less code for the same logic.** Facts about a tool are declared once and *derived*.
- **Do not grow `assistant` — owner rule.** New logic lives in its own packages; the tool registry moves out of
  `assistant` first (D10, Phase 1b).
- **Ports and adapters.** Domain packages own small named interfaces; adapters at the edge; the composition root wires.
- **Strategy tables over switches**; typed string enums with `Valid()`; options structs past four parameters;
  constants over magic values; `ctx` first; `%w`.
- **Rule of two/three before abstracting.** A seam is built when this plan has the users stated for it.
- Packages stay under ~15 files; a family with several implementations becomes a subpackage; tests mirror source 1:1;
  no `init()`; functions ≤ 80 lines, complexity ≤ 12.

### 4.1 One tool catalog (the merge) — `models/tool_catalog.go`

Extend R0's `toolsByCategory` into one table of `ToolSpec{Name, Category, Effect, Reach, Gate}` and derive the rest.
`ToolEffect` must **move into `models`** (`toolpolicy` imports `models`, not the reverse) *(review 2)*.

- `Reach` ∈ {`none`, `lan`, `internet`, `any`} is the scope rule. Today's behaviour (truth table, `guardrails.go`):

  | Scope | Forbidden today |
  |---|---|
  | `none` | fetch, scan, info, search, notify |
  | `lan` | search, notify |
  | `internet_only` | scan, info |
  | `internet` | nothing |

  It is reproduced exactly by `fetch_url`=`any`, `scan_local_network`/`get_network_info`=`lan`,
  `internet_search`/`notify_user`=`internet`, **every other tool (terminal, delegate, MCP, …) = `none`**. That last
  value means "not governed by the scope table", not "has no network"; the field is documented so.
- `Gate` names the config switch the schema-hide loop reads (`guardrails.go:425-428` maps each tool to
  `Network.Enabled`, `Communication.Enabled`, `searchDisabled`), so the `add()` lines become a loop over rows.
- Tool-specific *validation* (`guardrails.go:379-388`) stays but looks the tool up through the catalog. A new
  `network_scan` **must get a `validateCategory` case**: today an unknown category returns nil (`guardrails.go:239-263`),
  which would skip the L1 checks (`Network.Enabled`, `AllowLanAccess`, `BlockedIPs`) *(review 2)*.
- The manifest stays the schema source; the manifest-sync test already ties category to manifest.
- **Behaviour-preserving; done first and alone** (Phase 1), behind the Phase 0 table test.

### 4.2 Registration — D10 (decided: move the registry out, then add the seam)

`registry.go` needs a struct field, `init…Tools`, `register…Tools` and two call sites per category. A `ToolModule`
would remove that, but the first draft's version was not self-contained *(review 2)*: `ToolHandler` is
`assistant.ToolHandler`, manifests load only from the `core/tools/manifests` embed, `Specs()` duplicated the catalog,
and profile validation reads the catalog before any module registers.

**Refactor first (Phase 1b), then the seam:**
- **Move** `registry.go` (705 lines), `tool_provider.go` (120) and the types they own (`LocalToolRegistry`, `ToolHandler`,
  `AgentStackDeps`/`AgentStackContext`, `InitializeAgentStack`, `registerTool`/`registerScopedTool`) into a new package
  `internal/core/toolregistry`. `assistant` keeps only the small interfaces its agent consumes (`ToolProvider`, `Engine`,
  consumer-owned, per the Go rule), so `assistant` shrinks by ~825 lines and gains nothing.
- **Outside users (measured):** `internal/app/bootstrap.go`, `automation_notifier.go`, `reminders.go` and
  `registry_test.go`; `egress.go` and `memory_tools.go` mention it (likely comments). **Unverified:** whether other
  `assistant` files use `ErrToolNotInternal` or `ToolHandler`, and the exact import direction after the move
  (`toolregistry` may import `assistant`'s interfaces, never the reverse). Phase 0 maps this before the move.
- **Then** `toolregistry.Use(Module)` with `Module{Handlers, Ready}`; specs stay in the catalog (§4.1); the manifest loader
  is a function taking an `fs.FS`, so a family embeds its own manifest. `netaudit` is the first module; existing
  categories move onto it only when touched.
- The move is behaviour-preserving and lands alone, behind the existing `registry_test.go`.

### 4.3 Guarded programs — `platform/progexec`

A small runner for "spawn a known program safely", with no knowledge of nmap or networks:

- `Program{Name, Binary, VersionArgs, InstallHint map[GOOS]string}`; `Detect(Program) (Version, error)` with typed
  `ErrNotInstalled` — the single source of the install message.
- `Run(ctx, Program, Invocation{Args, Env, Root, Timeout, MaxOutput}) (Output, error)`: spawns through
  `process.AgentCommand` wrapped by `sandbox.Provider`; **requires** a non-empty `Root` and an explicit `Env` (never a
  nil Env, which inherits the host's) *(review 2)*; timeout, output cap, kills the process group on cancel; returns stdout
  only.
- **Fail closed on the actual mechanism**, not on `GOOS`: refuse to run unless
  `Effective().Filesystem.Mechanism != none`, or the operator opted in (D6). Landlock-off hosts count as unconfined.
- **The sandbox API needs per-program read grants** (nmap's install dir, `nselib`, `/etc/services`): `DefaultProfile`
  does not grant them and `WorkspaceView` cannot add read paths *(review 2)*. This is a SPEC-006 §II.7 change (§8).
- **HOME:** `BuildSandboxEnv` puts HOME inside the workspace `.sandbox`. The runner passes its own empty, per-run
  directory (not inside the agent's workspace) — settle where in Phase 0.
- **Targets are policy in the domain**, injected as a `TargetGuard`; `platform` imports nothing from `core`.
- One program exists, so the runner carries no unused fields; a second program adds a `Program` value.

### 4.4 The domain — `core/netaudit`

```
core/netaudit/
  mode.go          Mode enum (typed, Valid) + table: Mode → {Intrusive, NeedsNmap, argv builder}
  scan.go          ScanRequest, Engine port, Service (chooses the engine; NeedsNmap modes never fall back)
  finding.go       Finding {Device, Port, Proto, Service, Version, Check, Severity, Evidence}; Key()
  diff.go          pure: Diff(prev, cur) Delta
  ack.go           Ack, Apply(findings, acks, now) — pure; expiry and stale rules
  readiness.go     Ready(ctx) []Requirement — the one source (§4.6)
  report.go        RenderObserved(Delta, ackCount) — deterministic, no model
  nmap/            Engine: argv builder (whitelist), XML parser (testdata), curated script names
  portprobe/       the rebuilt Go prober, the labelled fallback
platform/scanscope/ Policy: operator input (IP, range, name, CIDR) → CIDR set; Authorize; pure, no I/O
```

- **Scope authorisation is one function** (`scanscope.Policy.Authorize`): resolve names **in process** and vet **every
  resolved address** (all must pass), normalise IPv4-mapped IPv6 with `To4()`, reject public, **CGNAT (`100.64/10`)**
  and IPv6 ULA (`IsLAN` counts `fc00::/7`), apply `network.CheckAlwaysBlocked`, cap hosts, and hand **IPs only** to the
  engine (`-n`). IPv4 only in v1. Resolving a name sends a DNS query that may leave the LAN; prefer IP input and say
  so in the UI *(review 2)*.
- **Interaction with existing network policy:** a scan runs only if the host-wide scope **and** the run's network grant
  **and** the workspace's `AllowLanAccess`/`BlockedIPs` all allow it (an intersection; the narrowest wins).
- **nmap argv** comes from the mode table: fixed flags per mode, `-sT`, `-oX -`, rate/host/time caps, scripts by
  **exact name**. A test refuses `-iL`, `--script-args`, `--datadir`, `--proxies`, any `-o*` file option and any flag
  not in the table. Data dir pinned. Constitution I.3 ("destination checked before the first byte") cannot hold for a
  connection an NSE script opens itself, so only scripts verified to stay on the scanned address are allowed (§11).
- **The Go prober is rebuilt**: IP-only jobs, a guarded dialer, the scope check, the off-by-one fixed, the empty branch
  gone. `scan_local_network` is **deleted** (IV.4, V.3, no compat shims); the template is rewritten as the nightly audit
  template. Deleting it also breaks persisted `allowed_tools` entries (`workspace.go:124`), recordings used for replay
  and user template copies — Phase 0 lists them and Phase 4 fixes or removes each.
- **Tool surface:** category `netaudit`, tool `network_scan(targets, mode, ports?)`, effect not read-only, `Reach: lan`.
  Scanning is **one at a time** (a lock; a second request waits or is refused) and has stated rate limits (values set
  in Phase 4 from a measured run).
- **Findings are structured by code**, parsed, truncated and `RedactSecrets`-scrubbed. Version and banner fields are
  returned inside a fenced data block and labelled untrusted, not as free prose *(prompt injection)*.

### 4.5 Persistence

`seen.go` has read / quarantine-corrupt / atomic write / delete. Acknowledgements and the baseline are the second and
third users of that shape, so the common helper is extracted *inside* `persistence` (unexported) — a modest gain
(the journal is markdown with no quarantine and stays separate) *(review 2)*.

- `acks.json` — one per **workspace**; entries `{key, accepted_version, note, accepted_at, expires_at, last_seen}`;
  only an operator endpoint writes it. `automationFilePath` is per automation, so a workspace-level path helper is
  added, and the workspace-delete path removes the file.
- `baseline.json` — **per automation** (two scan automations would clobber one workspace baseline), latest scan only,
  outside the jail; **added to `DeleteAutomationRuns`**, which today removes only seen and journal (`workspace.go:496`)
  *(review 2)*.
- **Device key:** an operator-assigned name (or an ARP-table lookup in code); `-n` and unprivileged scans give no
  hostname or MAC.
- Both live in `InternalDir`, outside the agent's reach.

### 4.6 Readiness, hiding, and preflight

`netaudit.Ready(ctx)` returns typed `models.Requirement{Code, Message, Fix}` (`nmap_missing`, `scope_empty`,
`no_sandbox`, `host_network_off`, `cloud_model_blocked`); `Fix` is `open_settings{section}` | `copy{text}` |
`confirm{action}`. Consumers:

1. **Tool schema (D11):** when `Ready` is non-empty the tool is hidden by the same availability path as the search tool;
   it is not wrapped in `registerScopedTool` with I/O (that predicate runs every turn and can only hide) *(review 2)*.
   `registerScopedTool` is used for run source only (D9).
2. **If called anyway:** `models.ErrToolUnavailable` with a reason — the existing typed, non-retried "operator action
   required" error. No new error type and no new prompt line.
3. **UI:** `GET /admin/api/netaudit/readiness`; the banner on the chat (when the auditor is chosen) and the automation
   page shows each requirement with its fix button.
4. **Automation preflight:** at the start of `executeAutomation`, **before `executor.Execute`** (a cold model is woken
   by `getLLMClient`); an unmet requirement ends the run `failed` with the reason (recorded through the existing
   prepare-failure path, `execution.go:86-90`); no new run state. The notification needs its own limiter key
   (`automation + reason`): the existing limiter is per automation with a 1-hour cooldown and would notify every night.

### 4.7 Local-only — enforce where a client is acquired

`ResolveOptions` returns no error and acquires no client, so a gate there does nothing *(review 2)*. Profile
front-matter `local_only: true` (SPEC-012) sets a context flag (`models.WithLocalOnly`) in `ResolveOptions`; the check
runs **at client acquisition** — the proxy provider's `GetClient`/`GetClientForModel` — against the model actually
chosen, covering all four acquisition sites (executor, chat, memory-review lease, delegation) and the chat failover.
Phase 0 reads the provider to confirm the chosen model is available there (`GetClient` does not report it today).

### 4.8 Report and delivery

- The report is written to four places (§3). Code renders **Observed** (`report.go`) and appends it through one hook in
  the executor's report path (`executor.go:445-461`); the model writes only **Suggested**. Connector delivery sends a
  code-built summary (`notify.go:197` change).
- **Auditor profile (D12):** `tools: [netaudit]` only (plus the always-on `system`, `security`, `automation`
  categories); no `delegates`; `memory_save: off`; `local_only: true`; the automation has `journal: false` (an
  automation field, off by default `workspace.go:143-146`). **Open:** `security_guardrails` is always on — Phase 0
  confirms what it can do in an unattended run, because a prompt-injected auditor must not be able to change guardrails.
- Reports go to a **dedicated auditor workspace**: the jail does not hide them from other agents.
- Findings need an API: `GET …/netaudit/findings` returns the structured latest findings and delta (the UI's
  `FindingsReport` cannot be built from a markdown report).

### 4.9 Prompt and message text

Text addressed to the model goes in `assistant/prompts/templates.go` (Constitution II.13); the auditor's instructions
are the embedded profile file. Operator-facing strings (readiness messages, install hints) are constants in `netaudit`.

### 4.10 Duplication this removes or avoids

| Removed / avoided | By |
|---|---|
| Network-gating facts in 10+ places | §4.1 catalog |
| Two scanners and two target guards | one `scanscope` + engines behind one tool |
| A `seen.go` copy for acks and baseline | §4.5 shared helper |
| Bespoke "is it set up?" checks per screen | §4.6 `Ready` |
| A new error type and prompt line for "not ready" | the existing `ErrToolUnavailable` |
| A new collapsible and banner primitive | `Disclosure.vue`, `Callout.vue`, `CopyButton.vue` |

## 5. Frontend

Layering stays Views → Feature components → Composables → Services → HTTP client; no `fetch` in components; types
only in `src/types/`; tokens only; `usePersistedState`, `useConfirm`, `usePolling`, typed route builders.

```
types/netaudit.ts                       Requirement, FixAction, ScanScope, NmapStatus, Finding, Ack, Delta (unions)
constants/api.ts                        endpoint constants
services/netaudit/netauditService.ts    readiness, scope, interfaces, findings, acks
composables/netaudit/
  useNetauditReadiness.ts               NEW module-level singleton (useHostNetworkState is per-call and is not the model)
  useScanScope.ts                       draft / dirty / save (SettingsActions + dirty-change)
  useAcknowledgements.ts
components/common/feedback/ReadinessBanner.vue   built on Callout.vue (actions slot) + CopyButton.vue; generic
components/settings/NetworkScanSettings.vue      ScanScopePanel, NmapStatus, ModeToggles
components/AgentIde/netaudit/FindingsReport.vue  grouped by device with Disclosure.vue; Acknowledge; Acknowledged list
```

- **Settings section checklist** (`architecture.md` → Adding a Frontend Settings Tab): add to the `SettingsTab` union
  (`types/admin.ts`), the label, the `FIXED_SECTIONS` exclusion (otherwise it is treated as a cloud provider),
  `SETTINGS_GROUP_SECTIONS.safety`, a `v-show` block in `SettingsView.vue`, and `local_only` in `AGENT_FORM_SECTIONS`.
  Link with `toSettings('network-scan')`. Do **not** add to `GlobalSettings.vue` (26 KB) or `SecuritySettings.vue`.
- **One panel, many entry points:** Settings, the banner, the auditor checklist and the report all open the same
  component or route.
- **Scope panel:** suggested networks from the backend (every private interface; the operator picks — never "first
  non-loopback"), plain inputs converted server-side to CIDR with a device count, large-range warning, public/CGNAT
  rejected with a reason.
- **Unconfined shortcut:** a `confirm` fix → `useConfirm` stating the consequence.
- **Mobile first:** single-column stacks, tables become cards, ≥ 44 px targets, banners wrap, no horizontal scroll;
  Playwright at a phone viewport (`npm run test:visual`); the built `index.html` is committed (owner rule).
- **Create-from-template:** `useTemplates` only writes a task file today *(review 2)*; whether the playbook flow can
  create an automation is verified in Phase 0 before the plan relies on it.
- **Wireframes first** (Phase F-0): phone and desktop for the scope panel, banner, checklist and report, reviewed by the
  owner before any component is built (precedent: `autonomy/autonomy-ux-design.md`).

## 6. Security design (summary)

- **Targets:** in-process resolution → every address vetted → IP-only → allow-list ∩ run grant ∩ workspace guardrails →
  always-blocked → caps. Empty list = off. IPv4 only in v1.
- **Process:** code-built argv from a mode table, exact-name scripts, no model flags, explicit env, empty HOME, pinned
  data dir, `-sT`, no privilege, timeouts, output cap, stdout XML only, one scan at a time.
- **Sandbox honesty:** fail closed on the actual `Effective().Filesystem.Mechanism`; opt-in for unconfined (stricter than
  the terminal, which runs under the no-op provider without one — intended, and stated in the amendment); per-program
  read grants added to the sandbox API.
- **Data:** structured, fenced, scrubbed findings; the auditor has only `netaudit`; reports in a dedicated workspace;
  connector summary only; `local_only` enforced at client acquisition; run-dir and `state.json` copies follow the same
  retention (Phase 0 measures what is stored).
- **Honest limit:** until auth lands, "operator-only" is a UI convention, not a boundary (§9).

## 7. Phases

Each phase ends green on `cd backend && go build ./... && go test ./... && go run ./tools/check-complexity/`; frontend
phases also `cd frontend && npm test && npm run build` (+ `npm run test:visual`). TDD per `tdd-guide`. No test uses the
real network: nmap is exercised through XML fixtures and a fake-binary test of `progexec`.

| Phase | Work | Acceptance |
|---|---|---|
| **0 Characterise & verify** | Table test of today's gating (scope × tool, incl. the §4.1 truth table). Read: `executeAutomation` failure/notify/report path; the proxy provider (can `GetClient` see the chosen model?); `security_guardrails`; `AllowedTools` effect on the schema (`tool_availability.go:51-90`); the notification API; run source in context; where recordings/`state.json` store scan text and for how long; whether the playbook flow can create an automation; every user of `scan_local_network` (templates, `allowed_tools`, replay fixtures); nmap under the Landlock grants on Ubuntu; candidate scripts' redirect/own-connection behaviour; `NetworkTools.configProvider` scope and sub-agent scope inheritance | §11 closed or relabelled; tests green on `main`; a short findings note appended to this plan |
| **F-0 Wireframes** | Phone + desktop wireframes for scope panel, banner, checklist, report; owner review | Owner sign-off recorded here |
| **1 Merge the catalog** | §4.1: move `ToolEffect` into `models`; `ToolSpec{…,Reach,Gate}`; derive gated lists, scope sets, effects, descriptions, the `add()` loop; add the `validateCategory` seam; delete the originals | Phase 0 table test unchanged; adding a tool = one row + one manifest |
| **1b Move the tool registry out of `assistant`** | Phase 0 maps the dependencies; move `registry.go`, `tool_provider.go` and their types to `core/toolregistry`; `assistant` keeps consumer-owned `ToolProvider`/`Engine`; then add `Use(Module)` and the `fs.FS` manifest loader | No behaviour change (`registry_test.go` unchanged); `assistant` has fewer lines and files; a stub module registers with no edit to `assistant` |
| **2 Persistence helper** | Shared unexported read/quarantine/write/delete in `persistence`; `seen.go` moves onto it; workspace-level path helper | `seen` tests unchanged |
| **3 Safety gates before any scan** | Constitution + SPEC-006 text (§8); `platform/scanscope` + `netaudit:` settings block (per `config-persistence`) + endpoints; `local_only` flag + enforcement at client acquisition; readiness endpoint; per-program read grants in the sandbox API | A scan cannot start without a scope; a `local_only` run cannot reach a cloud client through any of the four sites or the chat failover |
| **4 Engines + tool** | `progexec` (fail-closed rules); `netaudit/nmap` (`discover`, `ports`, `services`); rebuilt `portprobe`; `Service`; `network_scan` registered as a `toolregistry` module; delete `scan_local_network` and fix its users; rewrite the template; a test listing every `AgentCommand` caller; rate-limit values from a measured run | Argv refuses every forbidden flag; fixtures parse; fallback labelled; a hostile banner stays inside its data block; two scans cannot overlap |
| **F-A UI for 3–4** | Scope panel, nmap status, mode toggles, `ReadinessBanner` | Phone-width checks; visual baselines updated |
| **5 Auditor + nightly run** | Profile `security-auditor`; findings + diff + per-automation baseline; code-rendered Observed via the executor hook; findings endpoint; summary delivery; preflight before `Execute` with a per-reason limiter; template; setup checklist; **runtime limits**: the tool's own timeout and the run cap (`agent.go:49` 2 min, `dispatcher.go:22` 10 min — *review 2 only*) are checked against a real /24 scan, and long scans chunked or limits set per automation; behaviour when an interactive chat preempts the run (recorded as skipped, `execution.go:236`) | A blocked night fails fast with a reason; the model cannot alter the Observed section; a full /24 audit fits its run |
| **5b Model evaluation** | Run the auditor on a local model against a fixture network, as the profile and delegation evals did (`docs/audits/2026-10-17-prompt-profiles-eval.md`, `2026-10-23-delegation-eval.md`); record tool-use correctness and report quality | Result recorded in `docs/audits/`; the supported model list stated |
| **6 Acknowledgements + report UI** | `acks.json`, operator endpoints, expiry/stale rules; `FindingsReport` with `Disclosure.vue` | Acked findings never reach the model; an expired or version-changed ack resurfaces |
| **7 Deeper modes** | `vuln_check` with exact-name scripts; per-target operator grant for intrusive modes (interactive only); remediation proposals in the report; "rescan this host" | Intrusive modes cannot run unattended |
| **8 Close-out** | SPECs, `architecture.md` map + pitfalls, plan status, README tracker; owner decision on adding nmap to the sandbox-conformance CI job (`ci.yml:134`) | `./scripts/check-agent-harness.sh` clean; Pre-Completion Review reported gate by gate |

## Phase 0 findings (2026-10-25, in progress)

Written by me against the code; the plan's tags are unchanged until the whole phase closes.

- **Truth table pinned.** `TestRunScopeTruthTable` (`guardrails_test.go`) asserts, for each run scope × {fetch, scan, info,
  search, notify, terminal}, both the schema hide and the hard `ErrNetworkDisabled` denial. It passed against unchanged
  code, so the §4.1 table is exact. Phase 1 must leave it green.
- **Registry users (§4.2).** Outside `assistant`: `internal/app/bootstrap.go:104` (`InitializeAgentStack`, `AgentStackDeps`),
  `automation_notifier.go:19` and `reminders.go:87,93` (type-assert `*LocalToolRegistry`); `egress/egress.go:26` and
  `tools/memory_tools.go:3` are comments only. Inside `assistant`, only `registry.go`, `tool_provider.go` and their two tests
  mention `LocalToolRegistry`/`ErrToolNotInternal`/`ToolHandler`, so the move is self-contained. Still open: import direction
  after the move.
- **§4.7 client acquisition.** `RuntimeClientProvider.GetClient` (`proxy/provider.go:50`) picks primary/fallback and calls
  `GetClientForModel` (`:69`), which has the instance and `inst.Local` (`ensureClient`, `:84`). So one check inside
  `GetClientForModel` sees the chosen model and whether it is local, and covers the chat failover. Open: that the other
  acquisition sites all go through this provider.
- **`security_guardrails` (§4.8).** It is a manifest (`manifests/security.json`) of `block_secrets` and blocked patterns in
  category `global`; no handler is registered for it in `registry.go`. It cannot change guardrails, so a prompt-injected
  auditor cannot use it. Closed.
- **`scan_local_network` users.** Code/fixtures: `models/tools.go`, `manifests/network.json`, `assistant/stream_test.go`,
  `agent_test.go`, `repetition_detector_test.go`, frontend `utils/assistant/toolSteps.ts` + its test. Templates:
  `network_recon_unprivileged.md`, `compliance_check_internal.md`, `sandbox-conformance-probe.md`. Docs: `docs/SPECS/guardrails.md`,
  the agent-loop skill and four plans. Persisted `allowed_tools` and recordings are not yet searched.
- **Run scope reaches the tools (closed).** The `NetworkTools` provider built in `registry.go:283-299` applies
  `RunNetworkScopeFrom(ctx)` via `WithRunScope`, the same mapping the validator uses. (`bootstrap.go:533` builds a second
  `NetworkTools` for infrastructure from global guardrails; it is not the agent's.) Sub-agents inherit: `delegation/runner.go:153,160`
  copies the parent's scope. `terminal.go:568` also reads it.
- **Stored scan text (partly closed).** `state.json` keeps each run's `Output` (`executor.go:575`, written by `AppendRun`);
  reports are pruned by count (`MaxSavedReports`, `persistence/reports.go:50`). In the files I read I found no age-based
  expiry for `state.json` outputs or run dirs; deletion is by user action (`workspace.go:~495,558`). Recordings
  (`internal/recordings`) are referenced per automation by `RecordingRef`. Not yet read: whether run logging stores tool
  results, so whether scan findings land in recordings.
- **Playbook flow (closed).** `useTaskAuthoring.ts` only puts the template text into a task file (`choosePlaybook`,
  `saveTask`); the automation itself comes from the automation form. A template cannot carry profile, `network_grant`,
  `journal` or `allowed_tools`, so the nightly template needs those set in the form, or a new create path (a Phase 5 item).
- **Persisted `allowed_tools`.** The field is `Workspace` automation YAML (`models/workspace.go:124`); no data fixtures in
  the repo name `scan_local_network` besides the three templates, but a user's own automation files may. Phase 4 must
  handle that at load (drop unknown names), per the no-compat-shims rule.
- **`AllowedTools` effect (closed).** It lives in `assistant/toolpolicy/tool_availability.go:40-64`: `ResolveForScope` adds the
  guardrail-disabled names to `excluded`, and `applyFilter` wraps the provider as allow-set minus exclude-set. That is the
  single place a D11 "hidden because not ready" exclusion plugs in (the readiness names join `excluded`).
- **Recordings (closed).** `internal/recordings/types.go:21,34` stores request `Messages`, which carry earlier tool results,
  so with recording on, scan findings land in recordings. The auditor automation must have recording off (Phase 5 checks it).
- **Not yet verified (need an Ubuntu host with nmap):** nmap under the Landlock grants; NSE script behaviour (readable from
  the local nmap install).

## 8. Documentation and law

- **Constitution (V.1, after a security review — draft for the owner):** an addition to II.3 — *"A guarded program is a
  known binary an agent tool starts with code-built arguments, a cleaned explicit environment, targets validated
  in-process against an operator-declared scope, and the sandbox spawn path. The model supplies values for a typed
  schema, never flags. Where the host has no OS file confinement, the program does not run unless the operator opts
  in."* Notes for the security review: II.3 says the env allow-list is defined via `terminal.json` — the guarded
  program has its own explicit env and the text must say which wins; I.3 ("before the first byte") holds for the
  program's targets as passed, not for connections a script opens itself — hence the exact-name script rule; the I.2
  carve-out has no precedent for a child's sockets and must be written as one (the shell's have none either).
- **SPEC-006** §II.7 (guarded programs, per-program read grants, honest macOS note, `Effective`); **SPEC-007**
  (preflight, per-reason notification, local-only, summary delivery, template); **SPEC-012** (`local_only`, the
  `netaudit` category, the auditor); **SPEC-003** (Settings section, readiness banner, report view);
  **`architecture.md`** (directory map, guarded-program checklist, pitfall "the catalog is the only place a tool is
  declared"); **`admin-api-authentication.md`** (dependency note, done).

## 9. Admin API authentication — parked, with its dependencies recorded

The owner accepts the current exposure (single operator) and wants it handled in the existing auth plan. That plan now
has a "features that depend on this plan" section: operator-only acknowledgements; the scope, mode and unconfined
settings; approval of intrusive modes; scripts and intrusive modes against the host's own addresses. Until it lands
these are conventions, not controls, and the Security UI copy says so. Nothing here edits routes, middleware or
binding. *(Note for the owner: the auth plan's own findings say the API can disable the host sandbox and wipe the data
root, and its Phase 1 Origin check is "a day of work"; that item may deserve its own slot even if login waits.)*

## 10. Risks

| Risk | Mitigation |
|---|---|
| A flag or script reaches an unintended target or file | Mode table + refusal tests; exact-name scripts; IP-only; explicit env; pinned datadir/HOME; Phase 0 checks the scripts |
| Running with no jail (Landlock off, empty root, nil Env) | Fail closed on `Effective()`; `progexec` requires root + env; tests for each |
| macOS dev runs nmap unconfined | Fail closed; explicit opt-in; `Effective` shows it |
| False assurance from a weaker engine | `NeedsNmap` modes never fall back; audits stop; fallback results labelled |
| Wrong subnet suggested | Operator picks from the list of interfaces |
| Catalog merge changes gating by accident | Phase 0 table test written first; Phase 1 must leave it unchanged |
| A local-only run reaches a cloud model through a side path | Check at client acquisition; test all four sites and the chat failover |
| Findings leak via reports, recordings, state.json, connectors, memory | Dedicated workspace; auditor has only `netaudit`; summary delivery; memory save off; retention measured in Phase 0 |
| A long scan hits the run or tool timeout, or is preempted | Phase 5 measures a /24; chunking or per-automation limits; preemption behaviour recorded |
| Fragile devices disturbed by a scan | Conservative timing and host caps; intrusive modes interactive only |
| Scope creep into capture / fixes | Non-goals; Appendix A is a separate plan |

## 11. Open points and unverified facts

- nmap NSE: whether candidate scripts follow redirects or open their own connections; data-dir and `~/.nmap` search order;
  the exact Landlock paths nmap needs on Ubuntu (Phase 0).
- Whether a LAN web page can reach `:4001` (CSRF/CORS) — relevant to the parked auth plan.
- What `security_guardrails` can do in an unattended run.
- Where recordings and `state.json` keep scan text, and for how long; whether the chat memory extractor could capture it.
- Whether `NetworkTools.configProvider` reflects the run scope, and whether sub-agents inherit it.
- Whether the playbook flow can create an automation.
- Whether `GetClient` can see the model it chose (for §4.7).
- Where the HOME directory for the child should live.
- Tool name `network_scan` (free once `scan_local_network` is deleted) — owner to confirm.
- Where device names are assigned in the UI.
- Whether a profile with only `netaudit` plus the always-on categories runs end to end, including the executor saving the report (Phase 0).

*Answered by review 2 (not re-run by me):* `AllowedTools` hides tools from the schema (`tool_availability.go:51-90`) and
argument validation rejects unlisted calls (`tool_exec.go:573-600`).

## 12. Review log

| Review | Scope | Outcome |
|---|---|---|
| 1 (Opus, 2026-10-24) | The ten decisions against the code | Admin API exposure; Go scanner defects; allow-list gaps; macOS sandbox no-op; per-workspace acks; drop agent-applied fixes; category gating; local-only at admission |
| 2 (Opus, 2026-10-25) | This document against the code | Corrected counts and references; local-only gate relocated to client acquisition; sandbox fail-open paths; `ToolModule` dropped for v1; hidden-vs-visible contradiction resolved; catalog needs `Gate` and `ToolEffect` in `models`; preflight/notification/report-hook details; per-automation baseline; auditor profile reduced to `netaudit`; existing frontend primitives reused; model evaluation, wireframes, runtime limits, concurrency, rate limits, test and CI strategy added |

I re-checked in the code: the `registerScopedTool` behaviour, the Landlock nil-root return, the nil-Env inheritance, that
`ResolveOptions` acquires no client, that `useHostNetworkState` is not a singleton, the file counts, the `network.go`
lines, and the existence of `Disclosure.vue`, `Callout.vue` and `models.ErrToolUnavailable`. Review 2's other claims are
tagged "review 2 only" above and are re-verified in Phase 0.

## Appendix A — what packet capture (wireshark/tcpdump) would need

A **separate category** (e.g. `netcapture`), not part of `netaudit`: it needs elevated rights (conflicts with the
unprivileged service user), sees other parties' content including credentials, and on a switched LAN sees little beyond
the host. If ever built: off by default and operator-enabled, interface allow-list, time/size caps, capture file outside
the jail, the model sees only parsed summaries (never packets), and a `Program` value plus its own amendment. The
guarded-program runner is shaped so that adds a declaration, not plumbing; nothing for it is built now.

## Remaining Work

**Done on branch `network-audit-catalog`:** Phase 0 characterisation (`TestRunScopeTruthTable`) and Phase 1 — `models/tool_catalog.go`
holds `ToolSpec{Name, Category, Effect, Reach, Gate}`; the category table, `toolpolicy` effects, `hostNetworkGatedTools`,
`scopeForbiddenTools` and the schema-hide loop in `DisabledToolNames` are derived from it. The truth table passed unchanged.
**Phase 1b done (same branch):** `registry.go`, `tool_provider.go` and their tests moved to `internal/core/toolregistry`;
`assistant` keeps only the consumer-owned `ToolProvider`/`Engine` in `ports.go` (non-test lines in `assistant` fell by about 800).
`toolregistry.Module{Manifests fs.FS, Handlers, Available}` + `LocalToolRegistry.Use` registers a self-contained family
(tested with a stub module and a `fstest.MapFS` manifest); `tools.LoadManifestFromFS` is the loader. The app layer's two
"find the local registry in the provider chain" helpers are now one.

**Phase 2 done (same branch):** `persistence/jsonfile.go` holds the generic read (missing = zero value, corrupt =
quarantined as `*.corrupt`) / atomic write / remove helpers; `seen.go` is its first user and its tests pass unchanged.
The workspace-level path helper (§4.5) is **not** added yet: nothing uses it before the acknowledgements in Phase 6, and
an unused helper would be dead code (Constitution IV.4).

**Phase 3 in progress (same branch):**
- `platform/scanscope` built and tested: `Parse` (address, CIDR, `first-last` range or name → prefixes; public, CGNAT, IPv6,
  loopback and link-local refused) and `Policy.Authorize` (empty list = `ErrScanningOff`; every resolved address vetted;
  out-of-scope refuses the whole request; the host cap is `>`, not `>=`; network and broadcast addresses of a block left out).
- `local_only` built, but enforced in **`Agent.Execute`**, not in `GetClientForModel` as §4.7 drafted. Reason: the chat
  acquires its client before the session's profile is resolved, so a check at acquisition has no profile to read without
  reordering the chat; `Execute` is the one place the client and the profile are both known, for chat, automation,
  delegation and the chat failover. `proxy.Locality`/`IsLocalClient` (a client that does not say is not local),
  `models.ErrLocalOnly`, profile field + schema entry. **Gap:** the memory-review `Lease` has no profile, so it is not
  covered; the auditor has `memory_save: off`.
- Scan scope settings built: `models.NetauditConfig` (`netaudit:` in `settings.yml`: `scan_ranges` as canonical CIDR strings,
  `allow_unconfined`) with its own `storage` view (not part of `UserSettings`, so the ordinary settings save cannot
  overwrite it; tested across reload); `core/netaudit.Scope` (turns typed entries into CIDR blocks, one bad entry refuses the
  whole save, lists the machine's private IPv4 networks as suggestions with a /24 floor); `GET/PUT /admin/api/netaudit/scope`
  (`entries` and `allow_unconfined` are independent; a refused entry is a 400 that names it).
- Still to do in Phase 3, and it moves to Phase 4 because it depends on it: the readiness endpoint (needs nmap detection from
  `progexec`) and the per-program read grants in the sandbox API (needs the `progexec` design). The Constitution/SPEC-006/
  SPEC-012 text still needs the owner's security review.

**Phase 4 done (branch `network-audit-engine`):**
- `platform/progexec`: `Runner.Run` (explicit env, run dir, timeout, capped stdout, process-group kill, fails closed on
  `Effective().Filesystem.Mechanism == none` unless `netaudit.allow_unconfined`), `Detect` with the install hint, `CleanEnv`;
  `sandbox.WorkspaceView.ReadPaths` is the per-program read grant (Landlock only; **not yet verified on Ubuntu**).
  A test pins the files allowed to call `process.AgentCommand` (shell, one-shot terminal, progexec).
- `core/netaudit`: `Mode` table (discover, ports, services; `services` needs nmap), `Finding`, `Service` (authorise → one scan
  at a time → nmap, or the labelled limited engine for an attended run; an unattended run or a nmap-only mode never falls back),
  `Readiness` (`Blocking()` is settings-only and drives the tool's visibility; `Check()` adds nmap and sandbox), `ports.go`.
- `netaudit/nmap`: argv from a mode table (a test pins the allowed flag set and refuses `-iL`, `--script*`, `--datadir`,
  `--proxies`, `-o*` files…), XML parser with fixtures captured from real nmap 7.99, hostile-banner cleaning (control chars,
  secret redaction, length cut), NMAPDIR pinned. `netaudit/portprobe`: the Go connect prober (guard re-check before every dial,
  64 workers, 50k probe cap). `netaudit/scantool`: the `network_scan` tool module (untrusted-data fence + reading rule).
- `scan_local_network` is deleted (code, tests, manifest entry); `network_scan` is category `netaudit`, reach lan, mutating.
  The templates, frontend step label, SPEC-006 (§II.7.6) and the agent-loop skill follow. `GET /admin/api/netaudit/readiness`.
- Checked live on macOS against this machine's own LAN address: real nmap through `progexec` in all three modes, the fallback
  with nmap marked unavailable, and loopback / public / documentation addresses refused.
- **Speed, measured on a real /24 (192.168.50.0/24, 8 live hosts, 24 ports):** the first cut was slow — nmap `ports` took 136 s
  because it scanned every dead address (`-Pn`), and with host discovery it took 23 s but found 6 hosts, not 8; the Go prober took
  45 s. Retries on silent hosts dominate nmap's time (max-retries 0 brought discovery to 7 s but lost hosts), so liveness is now
  the Go sweep's job. **Quick (default):** the limited engine sweeps the targets on six shared liveness ports (22, 80, 443, 445,
  5000, 8080; a refusal counts as alive; 256 workers, 700 ms), then the engine looks at only the live hosts (`-Pn`, `-T4`,
  common ports). Through the real service: discover 4.2 s, ports 8.9 s with nmap (same 8 up / 17 open as the Go prober),
  services 25.5 s. **Thorough (`thorough: true`, for audits):** no sweep, every address, `-Pn`, `-T3`, `--max-rate 100`,
  top 100 ports — unchanged from the first cut, and the slow one by design.
- Not done: the Ubuntu run (Landlock grants for nmap), persisted `allowed_tools` entries
  that still name `scan_local_network` (they now match nothing).

Not done from §4.1: a `validateCategory` case for the future `network_scan` (it now dispatches network tools by `Gate`, so a
new row with `GateNetwork` gets the L1 checks); the `validateNetwork` scan/info messages still name the two tools.

**Everything else (Phase 5 onwards; the UI waits for the wireframes, F-0):** Suggested first PR: Phase 0 (characterisation tests + the verifications) — it changes no behaviour and
settles the open facts in §11. All twelve decisions are made.
