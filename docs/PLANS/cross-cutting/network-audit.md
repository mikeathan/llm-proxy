# Network audit — guarded programs, an nmap scan tool, and a nightly auditor agent

**Status:** partial (2026-10-24, revised 2026-10-26). D1–D13 decided by the owner (§2); open facts in §11. Phases 0–6, the report
UI, checks mode and per-audit scan settings are merged (PRs #137–#144, v0.12.0). §13.A (the small finishes) is merged
(#146, v0.13.0) and runs on vertex. §13 lists what is left.
**Related Specs:** SPEC-006 (Guardrails §II.6–II.7), SPEC-007 (Automation Dispatcher), SPEC-012 (Prompt Profiles),
SPEC-001 (Agent Loop), SPEC-003 (UI), SPEC-009 (Communication).
**Constitution refs:** I.1–I.3 (network guardrails), II.3 (sandbox isolation, env allow-list), II.10 (guardrail
decision flow), II.13 (prompt centralisation), III.3 (workspace jail), IV.4 (no dead code), V.1 (amend after a
security review), V.3 (no half-finished work).
**Depends on / defers to:** [`admin-api-authentication.md`](admin-api-authentication.md) — **parked by the owner**
(§9). **Builds on:** [`agent-os-sandboxing.md`](../ARCHIVE/cross-cutting/agent-os-sandboxing.md), [`sandbox-and-egress-residuals.md`](sandbox-and-egress-residuals.md),
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
rescans; revisited as a decision in §13.C), internet CVE lookup (revisited in §13.B), any admin-API authentication work, a database.

## 2. Decisions

Owner, 2026-10-24:

| # | Decision |
|---|---|
| D1 | Admin API auth is **not** part of this work. A dependency note is in the auth plan (§9). Intrusive modes against the host's own addresses wait for it; quiet scans of the host are allowed. **Amended by the owner, 2026-10-26:** the safe checks (`checks` mode) may also run against the host's own addresses; until auth lands that remains a convention, not a control. |
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
| D13 | **No nmap in CI.** The sandbox-conformance job stays without nmap; the scripts are verified by hand on an Ubuntu host instead (§13.D). **Decided by the owner, 2026-10-26.** | Keeps CI small and fast; the cost is that the Landlock behaviour of nmap is only as verified as the last manual run. |
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
| **8 Close-out** | SPECs, `architecture.md` map + pitfalls, plan status, README tracker; ~~owner decision on adding nmap to the sandbox-conformance CI job~~ decided: no (D13) | `./scripts/check-agent-harness.sh` clean; Pre-Completion Review reported gate by gate |

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

- **Constitution (V.1, after a security review — approved and applied 2026-10-27, see 13.D; the applied text in `CONSTITUTION.md` II.3 supersedes this draft):** an addition to II.3 — *"A guarded program is a
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
| 3 (Opus advisor, 2026-10-27) | §13.G design and the Constitution step | Platform nodes (`vulnerable:false`) must not be indexed; `o:`/`h:` CPEs were wrongly excluded for a false reason; an acceptance could hide later KEV hits (ack now carries the CVE set); import endpoint added to the auth plan's dependents; coverage numbers recounted and the NVD configuration ceiling measured; the Constitution text shown to the owner in full, with what goes beyond the draft |

I re-checked in the code: the `registerScopedTool` behaviour, the Landlock nil-root return, the nil-Env inheritance, that
`ResolveOptions` acquires no client, that `useHostNetworkState` is not a singleton, the file counts, the `network.go`
lines, and the existence of `Disclosure.vue`, `Callout.vue` and `models.ErrToolUnavailable`. Review 2's other claims are
tagged "review 2 only" above and are re-verified in Phase 0.

## 13. What is left (2026-10-26)

**Shipped:** scan scope and readiness; the guarded nmap runner and `network_scan` (discover, ports, services, checks); findings with
new / changed / resolved, accept and Undo; the code-written report and connector summary; the Security auditor and the Network Audit
playbook; the settings, report and form UI (Simple and Advanced); per-audit scan settings applied by the server (v0.12.0); plain-language
failures. Details are in the status notes above.

### 13.A Finish what the plan already promised (small, no new risk) — merged as #146 (v0.13.0)
| Item | As built |
|---|---|
| "Scan it again" on the unscanned-device notice | `UnscannedNotice` button; one unscanned device → a rescan of that device, several → the whole audit runs again. Goes through the ordinary trigger (`useFindings.rescan`). |
| Readiness warning for an audit's targets | `POST /admin/api/netaudit/targets/check` → `Scope.CheckTargets` (the same `Policy.Authorize` a scan uses, one entry at a time); `useTargetCheck` debounces and `ScanSettingsFields` shows a warning naming each refused entry and why. A failed check shows nothing. |
| Library "Sets up:" line shows the scan settings | `TemplateLibrary.usesOf` appends `scanSummary(defaults.scan)`. |
| "Rescan this host" | `POST …/dispatcher/trigger/{ws}/{automation}?scan_target=<ip>` (`automation.RunOptions.ScanTarget`, one IP only, audit only, 400 otherwise); the run uses a copy of the entry with `Targets` = that IP, so depth, ports and checks stay the audit's own and nothing is saved. Button in the Advanced device header. `Trigger`/`admitRun` now take `RunOptions` instead of a bare `recordingRef`. |
| **Remediation proposals in the report** | `netaudit.RemediationFor(check)` (fixed table, `remediation.go`); the findings API returns it as `fix`, both views show "How to fix it", the Observed checks table has a "Suggested fix" column. Read-only text; the resolved-checks table carries the column too. |

Traps found while building 13.A: a rescan that is queued behind a running or queued run of the same audit is absorbed by the
lane's key dedupe and runs as the plain audit (the options of the absorbed trigger are dropped) — the response says "queued".
A one-device scan would have overwritten the baseline's `Unscanned` list and `DevicesUp` with its own one host; `carryOver`
(`runreporter.go`) keeps devices flagged earlier and counts devices that had findings and were not looked at this time.

### 13.B More ways to identify weaknesses (read-only; each needs its own reviewed design before it is built)
1. **Known-vulnerability matching.** Compare detected service versions with a vulnerability list stored locally and updated by the operator, so a scan never needs the internet. **Owner decision 2026-10-27: yes.** **Source picked by the owner 2026-10-27: NVD for matching plus the CISA KEV catalogue to rank.** The reviewed design is §13.G; nothing is built before it is accepted. Options considered:

   | Source | Offline? | Licence | Update path | Fit |
   |---|---|---|---|---|
   | NVD (NIST) CVE data with CPE applicability | Yes, once a snapshot is stored | US government data, no licence fee (verify the current terms before relying on this) | Operator imports a downloaded snapshot; or the app fetches it through `network.FetchURL` (an Internet destination, I.2/I.3) | Best match for nmap, which prints a CPE for each service it identifies; large, and some entries lack CPE data (verify) |
   | CISA Known Exploited Vulnerabilities catalogue | Yes | US government data | Operator imports one small JSON file | Few entries, high signal; names vendor and product, not CPE, so matching is looser |
   | Operator-written advisory file (product, version range, note) | Yes | The operator's own | Operator edits a file | Smallest and fully controlled; covers only what the operator writes |
   | OSV.dev / distribution trackers | Yes if mirrored | Varies by source | Mirror and import | Built for packages, not network device banners; distribution banners hide backported fixes, so version matching gives false alarms |
   | nmap `vulners` script | No: it asks vulners.com on every scan | Service terms | n/a | **Rejected**: leaks scan results off the network and is outside the reviewed script list |

   Every option shares one risk: a banner version does not prove a flaw (backported fixes), so a match is reported as "may be affected", never as a confirmed finding.
2. **More safe configuration checks** of the same kind as today's nine: one row in `netaudit/checks.go` and its test each, from nmap's `safe` category only. Web checks wait until the redirect behaviour of the scripts is proven to stay on the scanned device.
3. **Expectations per device.** The operator records what a device should offer ("the printer: only 631"), and code reports the difference. No new probing.
4. **Certificate and TLS hygiene** beyond expiry, as read-only reports.

**13.B.2 as built (2026-10-28, v0.15.0).** Eight more checks: Telnet open and an open Docker API (port rules in `portchecks.go`, no script, work in every scan mode; a name taken from the port table only is worded "normally Telnet" and ranked lower); weak certificate key and SHA-1/MD5 signature (read from the existing `ssl-cert` output); and four scripts added to the reviewed list, each checked against nmap 7.99: `ssh-hostkey` (weak DSA or short RSA host key), `memcached-info`, `mongodb-info` and `mqtt-subscribe`. All four are in nmap's `safe` category and talk only to the scanned host; NSE `dependencies` only order scripts and do not select `mongodb-brute` (a one-script run loaded exactly one). **Owner decision 2026-10-28:** `mqtt-subscribe`'s anonymous CONNECT counts as the "well-known anonymous login" II.3 allows; the evaluators read only whether it answered and never copy a topic or payload. **Rejected after checking the categories:** `ssl-enum-ciphers` (`discovery, intrusive`) and `ssh-auth-methods` (`auth, intrusive`, tries a random user), `dns-recursion` and `upnp-info` (UDP only, never run by `-sT --unprivileged`; `upnp-info` also follows a LOCATION URL to other hosts). **Owner decision 2026-10-28:** intrusive scripts are to be built later as a separate interactive "deep check" of one chosen device (not in a nightly audit), with its own Constitution amendment; weak-TLS-version detection and SSH password-login detection wait for it. Thorough scans now pass an explicit port list (`netaudit.ThoroughPorts`: nmap's 300 most common plus 2375, 11211, 27017), because nmap ignores `--top-ports` when `-p` is also given, so the extra ports could not be added to it; the quick list gained the same three. Self-signed certificates are not a check: nearly every router, NAS and media device has one, so it would only add noise.

**Report shape (same release).** The markdown report lists a first scan as one row per device and groups the same problem on several services into one row that names where, with each check's fix written once under the table (`writeChecks`, `writeInventory`). The findings API gained `first_scan` (a baseline saved by an older version reads as not the first) and `check_title`. The UI shows one card per kind of problem (`ProblemGroups.vue`), folds a device's ports unless something there needs a look, folds ports the scan could not identify into one line, and calls the first scan a baseline instead of marking every port new. The security-auditor profile now asks the model for a short assessment of what the code cannot say, and not to re-rank, restate or give a severity to anything the checks established.

Anything that sends more than ordinary connections, tries credentials, or could disturb a device is **intrusive** and is not specified here. It stays behind the existing Phase 7 rule (per-target operator grant, interactive only, never unattended) and needs, first: an owner decision, a Constitution review (§8), and admin API authentication (D1, §9).

### 13.C Remediation
1. **Proposals** (13.A): in scope, read-only.
2. **Assisted fixes**: the operator approves each action. **Owner decision 2026-10-27: yes, as a design.** **Targets confirmed by the owner 2026-10-27 (the proposal below).** The design is written (§13.H, awaiting acceptance); no code before it is accepted. The reminder of what the design must hold: It would need: a list of what the app is able to change and where (only on hosts or devices it can already reach through a configured connector), a recorded way back for every change, a log of each approval, and a Constitution review. Nothing is specified beyond that until the targets are chosen.

   **Scope (confirmed by the owner 2026-10-27):** *In scope*, only where the app can already act through a configured connector and the operator approves the single action in the guardrail flow (II.10): accepting a finding for a time (already built), and changes on hosts the operator has configured for terminal or connector access (for example closing a service with a command on a Linux host the operator already runs). *Never touched*: routers, switches, access points, NAS and any other embedded device; anything reached by a password the app would have to guess; anything not in the scan scope; any change in an unattended run (13.C.3). Each change needs a recorded way back and an approval log entry.
3. **Automatic, unattended fixes** are **out of scope**: a model must not change a device unsupervised, and a wrong fix on a router or a NAS can cut the network off.

### 13.D Verification gaps and governance
- **nmap under the Ubuntu Landlock grants** and the check scripts on real devices: verified by hand on an Ubuntu host, not in CI (D13). **Correction 2026-10-27:** the earlier wording named `TestChecks_RealNmap…` as the check, but that test starts nmap with plain `exec` (`checks_nmap_test.go`), so it never passes through Landlock; it verifies only the argument list and the parser. The Landlock check is one audit through the running app (steps in the 2026-10-27 hand-over: install nmap, start the backend, add a scope that holds only a test device the operator owns, run one checks audit, confirm the confinement shown in Settings → Security reads Landlock, the run reports devices rather than "failed to confine", and note the result and the date here). Not yet done: it needs an Ubuntu host and a device the operator owns, so it cannot be run from the development machine.

  **Runbook (written 2026-10-28; do it in this order, stop at the first surprise and record it):**
  1. On the Ubuntu host: confirm the kernel offers Landlock (`cat /sys/kernel/security/lsm` lists `landlock`) and install nmap (`sudo apt install nmap`).
  2. Start the backend as the user that will run it (`cd backend && go run main.go`, port 4001).
  3. Settings → Security: the file confinement must read **Landlock**, not none. If it reads none, stop: the check cannot be done on this host.
  4. Settings → Network scan → Advanced: allow only the one device you own (its address, not the whole network).
  5. Set up a network audit from its playbook (thorough, checks on, known vulnerabilities off for this check), then press Run.
  6. **Pass:** the run finishes, the report lists that device and its open services, and the checks that apply to it ran. **Fail:** the run says it failed to confine, nmap reports "Operation not permitted" or cannot read its data files (under `/usr/share/nmap`), or the report is empty for a device you know has open ports. Copy the exact message.
  7. Optional, for the unverified rule that no check script contacts another host: on an otherwise quiet host (no browser, no updates, and your own SSH session excluded with `not port 22`), run `sudo tcpdump -n -i any 'tcp[tcpflags] & (tcp-syn) != 0 and tcp[tcpflags] & (tcp-ack) == 0 and not dst host <the device> and not port 22'` in a second terminal during the run. It lists new outgoing connection attempts to anything but the device; the scan should add none.
  8. Record here: the date, Ubuntu and kernel versions, nmap version, pass or fail with the message, and add a line to `docs/learnings/network-audit.md`.
- **Constitution text for guarded programs** (§8): approved by the owner 2026-10-27, security-reviewed against the code, and added to `CONSTITUTION.md` II.3 . The review found: the child's sockets are outside `NetworkTools` (`progexec.Run` always passes `NetworkOn: true`), so the text states the carve-out and its limits; `ftp-anon` is in nmap's `safe` category but does perform the well-known anonymous login, so the text allows exactly that one credential-bearing exchange; the `terminal.json` allow-list does not govern the guarded env, and the text says so.
- **Unverified, and now asserted by the law:** that none of the thirteen listed check scripts opens a connection to a host other than the target (redirects, referrals). The Constitution text makes this a rule for the list; confirm each script's source or behaviour in the Ubuntu hand check.
- **Admin API authentication** stays parked by the owner's decision (2026-10-27, §9): the audit's operator-only parts are conventions until it lands.

### 13.H Reviewed design — assisted fixes (13.C.2)

**Status:** Level 1 accepted by the owner and built 2026-10-28 (see "Level 1 as built" below); Level 2 stays unbuilt until admin-API authentication exists (owner: leave authentication parked). Design proposed 2026-10-28. Self-reviewed against the code (below), not yet reviewed by anyone else. Scope confirmed by the owner 2026-10-27 (13.C.2): only hosts the app can already act on, each action approved by the operator in the guardrail flow, a way back and an approval log for every change, never embedded devices, never unattended.

**Facts checked in the code (2026-10-28)**
- **The app cannot run anything on another host today.** There is no SSH client or remote-execution tool. The `terminal` tool runs on the machine the app runs on, as the app's OS user, inside the workspace jail and the OS sandbox (Landlock or Seatbelt), so it cannot edit system configuration such as `/etc/ssh/sshd_config` and does not reach other devices. Communication connectors (Telegram and the like) send messages; they do not run commands. So the owner's target "hosts configured for terminal or connector access" currently matches **no host**. A fix that changes another machine needs a new kind of access that does not exist yet.
- **Approval is the guardrail flow (SPEC-006 §3):** the call is blocked, the operator approves or denies the exact tool and arguments through `POST /admin/api/conversation/guardrail-decision`, and the same call then passes for 30 minutes. Automation runs never wait for approval; a violation is denied at once. That decision endpoint, like the rest of the admin API, is unauthenticated (admin-API authentication is parked).
- **Fix text today** is fixed wording in `netaudit/remediation.go`, keyed by check. Nothing the device or a model wrote reaches it.
- **Findings, accept and rescan already exist:** `ScanAck` (accept for a time), "Scan it again" for one device, and `carryOver` in the run reporter.

**The design has two levels. Level 1 is proposed for building; Level 2 is only specified, because it cannot be built safely yet.**

**Level 1 — Guided fix (no execution; no new power).**
- Each check gets a reviewed *fix guide* in code: the steps in plain words, an optional command to copy for each common platform (never run by the app), what could go wrong, and **how to undo it**. This replaces the one-line suggested fix where a guide exists; the one-line text stays as the summary.
- The operator applies the fix themselves, then presses **"I've done this"** on the finding. That writes a **fix record** `{workspace, key, device, check, appliedAt, note}` (the approval log: who decided and when, here the operator's own statement) and offers **"Scan this device again"** (already built) to confirm.
- A new state follows from the record and the next scan: the finding **still present after the record's time** shows "Still showing after you marked it fixed. Check the steps again or accept it." A finding that is gone shows as fixed with the record's date. The report never says a fix worked unless a scan confirmed it.
- Storage and API follow acks: a per-workspace file through the `persistence` helpers (atomic, Constitution III.2), `GET/POST /admin/api/netaudit/fixes`, one entry per finding key, replaced by a later one. The note is bounded (200 characters, as for accepts) and shown only as text.
- UI: Simple shows the plain steps and one button; Advanced adds the copyable commands, the undo and the record list. Mobile, 44 px targets, Help part "Fixing a finding".
- **A fix record never hides a finding.** It does not change the diff, the accept state, the baseline or the connector notification; it only adds the "still showing after you marked it fixed" state. So an unauthenticated caller who marks things fixed cannot silence anything; the worst they can do is add noise to the log, and the next scan decides what the report says.
- Guides are fixed text the repo owns. No model writes them, so nothing untrusted reaches the screen, and the auditor model is unchanged.

**Level 1 as built (2026-10-28).** `netaudit/fixguide.go` (`GuideFor`: a fixed-text guide for each of the eleven checks, tested for completeness, no addresses and no placeholders); `models.FixRecord` and `persistence` `ReadFixes`/`WriteFixes` (`fixes.json` beside the accepted findings, outside the agent's jail); `netaudit/fixes.go` (`Fixes.Mark`/`Unmark`/`List`, `FixStateFor`, `ConfirmFixes`); `RunReporter.WithFixes` (a record whose finding the scan no longer shows is retired and its mark time kept in the baseline as `FixedAt`; trouble reading or saving records is stated in the report and never changes what is listed); `POST`/`DELETE /admin/api/netaudit/fixes`; findings carry `guide`, `fix_state`, `fix_marked_at`, `fix_note`, and resolved entries `fixed_at`. UI: `FixGuidePanel.vue` in the Simple and Advanced report (steps, what could go wrong and how to undo in both; commands to copy in Advanced; "I've done this", "Take back", "Scan this device again" when a scan since still shows it) and a Help part "Fixing a finding". A record never hides a finding, changes the diff, the accept state or the connector notification (tested). Known limit: a scan already running when the operator marks a finding is stamped with its end time, so the first report can read as "still showing"; the next scan settles it. The guides' commands are written for common Debian and Ubuntu systems, Samba, Windows 10 and later and OpenSSH and have not been run on real hosts; each should be tried on a test host before it is relied on.

**Level 2 — "Run it for me" on a managed host (specified, not to be built yet).**
- *What it needs that does not exist:* a **managed host** record (address inside the scan scope, user, an SSH key kept in the encrypted secrets store (III.6), the host key pinned on first approval), and a guarded `ssh` program started through `progexec`.
- *Rules, in the shape of the guarded-programs text:* the command comes from a **fixed table** keyed by check (typed parameters only, argv built in code, never a model-written or operator-typed string); every action shows the exact command, the **undo command** and the file it will back up, and runs only after approval through the II.10 flow; before changing a file it saves a copy on the host and records its path; after the change it validates the configuration where the program can (for example `sshd -t`), reloads rather than restarts, and runs the one-device rescan; any failure runs the undo and says so. Interactive only: never in an unattended run, never from a connector message.
- *Candidates (Linux hosts only):* `ssh_weak_algorithms` (a drop-in file, validated, reload), `smb_signing` and `ftp_anonymous` (one setting each). *Guided only, never run by the app:* `known_vulns` (a package upgrade can break a host), `redis_no_auth` (needs a secret chosen by the operator), `nfs_exports` and `vnc_no_auth` (can cut off the operator), the certificate checks (need certificate material), `rdp_no_nla` (Windows). Anything on a router, switch, access point, NAS, camera or phone is out, as scoped.
- *Why it must wait:* (1) **admin-API authentication.** Level 2 puts approval, managed-host records and stored keys behind an endpoint anyone on the network can call; a LAN caller could add a host and approve a command. The owner parked authentication, so Level 2 stays unbuilt until it lands (or an operator-only guard exists). (2) **A Constitution amendment after a security review (V.1):** `ssh` is a program whose sockets are outside `NetworkTools`, and a key-authenticated login is a credential-bearing exchange, which II.3 item 5 allows only as a service's well-known anonymous login. That is a new, intrusive class and needs its own text. (3) The owner's decision on managed hosts as a concept.

**Out of scope, as decided:** unattended or automatic fixes (13.C.3); routers and embedded devices; guessing or storing passwords for devices; anything outside the scan scope.

**Phases (TDD; Level 1 only)**
1. Fix guides: a table in `netaudit` (steps, per-platform commands, undo) with a test that every check with a one-line fix has a guide and that guides contain no placeholders or addresses.
2. Fix records: model, store, `GET/POST /fixes`, state derivation (`fixed`, `still_present`) in the report, with tests including a finding that returns after being marked.
3. UI: guide in the finding detail (Simple and Advanced), "I've done this", the still-present state, Help part, visual test.
4. Docs: SPEC/API reference, ledger.

**Self-review (what was checked, what was not)**
- Checked: that no remote-execution path exists (code search for SSH and remote tools); the approval flow and its 30-minute pass against SPEC-006; that Level 1 adds no new execution, network or secret use; that the only text shown is repo-owned or the operator's own bounded note.
- Not checked: the correctness of each platform command in the guides (each needs a test on the real platform before it is shipped); how an operator would want undo shown on a phone.
- Risks: a guide can go stale as software changes (mitigated: commands are copied by a person who sees them; each guide names the version it was written for); a record can be marked falsely (the next scan decides what the report says).

**Decisions needed from the owner**
1. Build Level 1 now? (Recommended: yes; it gives most of the value with no new power.)
2. Keep Level 2 unbuilt until admin-API authentication exists? (Recommended: yes.)
3. Is "managed hosts over SSH" the access you had in mind for Level 2, or something else (an MCP server, a different agent)?

### 13.E Suggested order
13.A (merged, #146) → pick the 13.B.1 data source → reviewed design for 13.B.1 → build 13.B.1 → pick the 13.C.2 targets → reviewed design for 13.C.2 → 13.B.2–4 → anything intrusive only after the 13.B conditions. Each of 13.B and 13.C needs its own reviewed design before code.

### 13.F Decisions of the owner (2026-10-27)
1. **Local vulnerability matching (13.B.1): yes.** Source: NVD for matching, CISA KEV to rank (design: §13.G).
2. **Assisted fixes (13.C.2): yes, as a design.** Targets: as scoped in 13.C.2 (hosts already configured for terminal or connector access; never embedded devices). Needs the guardrail flow; design written 2026-10-28 in §13.H (Level 1 guided fixes proposed; Level 2 execution waits for admin-API authentication).
3. **Constitution text for guarded programs (13.D): approved.** Reviewed and added to `CONSTITUTION.md` II.3.
4. **Admin API authentication: no.** It stays parked (§9).

### 13.G Reviewed design — known-vulnerability matching (13.B.1)

**Status:** proposed 2026-10-27, awaiting the owner's acceptance. Read-only: no new probing, no new process, no new network use.

**What it does.** After a scan, code compares each open port's identified software (nmap's CPE) with a local vulnerability index and
adds one finding per port: "this software version may be affected by N known flaws (M known to be exploited)". The scan itself is unchanged.

**Facts checked (2026-10-27; commands in the learnings ledger)**
- **nmap identification.** In nmap 7.99's `nmap-service-probes`, 6,814 of 11,970 `match` rules (57%; rules, not devices) carry a CPE. Counting rules whose CPE has a version slot (approximate): application CPEs 1,687 of 1,689; operating-system CPEs about 195 of 2,525; hardware CPEs about 20 of 1,223. So a service scan names application versions well, and almost never a firmware or hardware version; router and camera firmware flaws are therefore mostly out of reach until OS detection (`-O`) is used, which needs raw sockets and conflicts with `--unprivileged`. This is the real coverage ceiling and the UI must say it.
- The parser reads only `product` and `version` (`nmap/parse.go` `xmlService`), not the `<cpe>` elements. Phase 1.
- **KEV** (fetched): CC0 1.0 licence (`/sites/default/files/licenses/kev/license.txt`); 1.8 MB; 1,758 entries; fields `cveID`, `vendorProject`, `product`, `dateAdded` (names, not CPEs). The index keeps only the CVE id and date: KEV marks CVEs that NVD matching already found, so it needs no product matching of its own.
- **NVD** (fetched): the 2.0 API answers without a key (415,351 CVEs in total); **bulk yearly files still exist** at `https://nvd.nist.gov/feeds/json/cve/2.0/nvdcve-2.0-<year>.json.gz` (20 MB for 2023, 25 MB for 2024 and 2025, 42 MB for 2026, compressed). A search snippet saying the feeds were retired referred to the old 1.x files. **NVD's terms (2026-10-27, from search snippets of nvd.nist.gov, not the pages):** its legal disclaimer says the data is a public service from publicly available sources, provided as is; the CPE dictionary is stated to be free of US copyright, with attribution appreciated; no licence page for the CVE data itself was found. Cite NVD in the Help text. **Rate limits could not be read** (the pages return 403 to a plain `curl` and need JavaScript); they do not matter for v1, which imports the yearly files and makes no API request, and must be read before any later app-side fetch.
- **NVD coverage ceiling.** In the 2026 file, 35,448 of 80,783 CVEs (44%) have configuration data; 29,529 are "Deferred" and 8,223 "Awaiting Analysis". A CVE without configurations can never match. Platform nodes exist (6,559 non-vulnerable `o:` and 24,996 non-vulnerable `h:` matches in that file), so they must not be indexed (below).

**Data (the index).** One app-owned file in the config root, outside every workspace, written atomically through the `persistence` helpers (Constitution III.2) and replaced as a whole on import. It holds only what matching needs, so no text from any outside file is ever stored or shown:
`{cve, ranges: [{vendor, product, start, startIncl, end, endIncl}], exact: [{vendor, product, version}], cvss, kev, kevAdded}` plus a header `{nvdImportedAt, kevImportedAt, counts, sources}`. **NVD configurations are AND/OR trees. Only `cpeMatch` entries with `vulnerable: true` are indexed; "running on" platform nodes (`vulnerable: false`) are ignored**, otherwise every host on a listed platform would match. A rule that needs two vulnerable parts at once (an AND of two groups that each hold a vulnerable match) adds nothing from that rule and is counted in the import result (`needs_all`), not flattened; "vulnerable app AND running on platform" is not that case and is indexed from the app part (about 12% of 2026 CVEs are AND rules, so skipping them all would have cost coverage). Every CPE part (`a`, `o`, `h`) is indexed, and a port is matched on any of its CPEs that carries a version; one without a version cannot be compared and is counted as such. Descriptions are not stored.

**Getting the data (owner decision 2026-10-27: a one-off download from the UI, kept in the app's data folder, never in the repo).** A button in Settings → Security ("Download vulnerability data") makes the app fetch the files once and build the index; the index is the only thing stored (`vulnindex.json` in the app's data folder, outside the repo and every workspace). The raw files are streamed into the importer and not kept. Nothing downloads at start-up or on a timer; refreshing is another click, and a later monthly refresh is a separate decision. Importing a file the operator saved stays available as a fallback (same importer).
- **What it fetches:** exactly the KEV JSON and NVD's yearly `nvdcve-2.0-<year>.json.gz` files (the last five years by default, the operator may add older years), from fixed `https://` URLs written in code; the endpoint takes years, never a URL.
- **Constitution I.2 / I.3:** this is the first outbound request of this feature, so it goes through the guarded client (`NetworkTools.HTTPClient()`, which I.2 names) after the same checks `FetchURL` makes (network tools enabled, Internet access allowed, blocked domains). `FetchURL` itself cannot be used: it returns text cut at about 1 MiB and the files are 20 to 42 MB. If Internet access is off, the button says so and offers the file import.
- **Limits:** a size cap on the compressed download and the existing decompressed cap; a timeout; one download at a time; progress and a plain failure shown in the card; a failed or cut download changes nothing (the importer works on a copy and the store writes atomically).
- **Why a click and not automatic:** about 100 MB leaves the machine's network each time, and a surprise download at start-up is the wrong default.

**Matching (`netaudit/vulnmatch`, pure code, no I/O).** Parse the CPE (vendor, product, version); look up by vendor:product; compare the version with each range by a tolerant comparator (numbers compared as numbers, letter suffixes such as `8.9p1` ordered after the number). A CPE with no version never matches (the 2026-10-27 scan returned `go-ipfs` for the app's own API, a wrong identification that is harmless only because it has no version). A version that cannot be compared **does not match** and is counted as "could not compare", so a doubtful match is never reported as a fact. Multiple CPEs on one port: each is matched; duplicates by CVE id are merged.

**Finding.** One finding per port, `check = known_vulns`, so the existing key (`device|proto|port|check`), diff, accept/Undo and "changed from" all work unchanged. Evidence is written in code from fixed words, counts and validated CVE ids (`CVE-\d{4}-\d{4,}`), capped at five ids ordered KEV first then by CVSS; never text from the device or the files. Severity (proposal): any KEV match → `high`; CVSS ≥ 9 → `high`; ≥ 7 → `medium`; else `low`. When the port's `version` text carries a distribution build (for example `Ubuntu 3ubuntu13.20`), the evidence adds "this is a distribution build, so the flaw may already be fixed" and the severity drops one step, because the CPE keeps only the upstream number (`9.6p1`) while the distribution backports fixes. Wording is always "may be affected", never "is vulnerable": a banner version does not prove a flaw (a distribution can fix it without changing the version number). `RemediationFor("known_vulns")` says so: update the software, check the vendor's notice for the listed numbers, and accept the finding with a note if it is already fixed.

**False matches and new information.** Handled by the existing accept (with a note and an expiry), which already lapses when the version changes. That alone would hide a flaw added to KEV later for the same version, so for `known_vulns` the acceptance also records the matched CVE ids (`ScanAck.AcceptedCVEs`), and the finding comes back when the current set contains an id the operator did not accept or a KEV entry that was not known then. This is a small model change (Phase 4). A per-CVE ignore list is not built in v1; it is added if accepting a whole port proves too coarse in real use.

**Where it runs.** In the reporter (`runreporter.go`), after the scan result is turned into findings, so a newly imported index applies to the next scan with no engine change and a run without an index simply has no `known_vulns` findings. The index age is shown beside the report; older than 60 days shows a plain "data is old" notice.

**UI, progress, missing data and resume (owner request 2026-10-27; SPEC-003; Simple and Advanced; mobile).**
- **Where it is managed:** a "Vulnerability data" card in Settings → Network scan (`NetworkScanSettings.vue`, section `network-scan`), under the scan scope. Simple: one status line ("Not downloaded", "Up to date, 23 Oct", "Stopped after 3 of 6 files") and one button. Advanced: a row per source (KEV; NVD by year) with date and counts, the years to fetch (default the last five), the skipped counts, and Download / Resume / Cancel / Start again.
- **Where progress shows:** (1) the card, as the one full view: overall "file 2 of 6", the current file's megabytes, and a bar; (2) the readiness callout (`ReadinessBanner`, already shown in the audit setup, the report and Settings) shows one line, "Downloading vulnerability data, 2 of 6", with the link to the card while it runs. Nowhere else: no top-bar or global indicator, to keep the screen quiet.
- **How progress reaches the screen:** the download is a server-side job tethered to the app's lifecycle (Constitution II.2, II.14), so it keeps going if the page is closed and every tab sees the same state. The screen polls a status endpoint once a second only while a job runs (no streaming connection; the state survives a reload). Status: `{state: idle|running|stopped|done, file, filesDone, filesTotal, bytesDone, bytesTotal, reason}`; a failure carries plain words and the fix, never a raw error.
- **If the data is missing and an audit uses it:** matching is a per-audit scan setting ("Look for known vulnerabilities", with the other scan settings in `ScanSettingsFields`). When it is on and the index is empty, the readiness list gets a non-blocking requirement `vuln_data_missing` (a note, not a warning) with the existing "open settings" fix to the card, so `AuditSetupCheck`, the report and Settings all show it with the link. **Default proposed, owner to confirm:** the audit still runs (the enrichment is optional; failing a nightly audit for it would lose the port and check results too); the report and the connector summary say "Known-vulnerability matching was skipped: the data is not downloaded" with the link, and the old data is used when it is only stale.
- **Resume:** the index is saved after each file, so a stop (network down, restart, cancel) keeps every file already done. The saved index records which sources and years were wanted and which finished (`Years` on the NVD source), so the card can show "Stopped after 3 of 6" and Resume fetches only the rest. A file that is cut part-way restarts from its beginning (at most about 42 MB); byte-range resume is not built because the file is streamed into the importer and never kept. "Start again" re-fetches everything. After an app restart a job that was running shows as stopped, never as running.
- **Offline or Internet off:** if network guardrails refuse the Internet the card says so in plain words and offers the file import.

**Model-facing (corrected 2026-10-28).** The auditor model does **not** see `known_vulns` findings: the reporter adds them after the scan tool, so they appear in the report and, as counts, in the connector summary, but never in the model's prompt or tool results. The playbook therefore tells the model not to write about them, so that it cannot say "none were found". Nothing outside the repository's own text reaches a model.

**Constitution.** No amendment: no process, no secret. The download goes through `NetworkTools.HTTPClient()` as I.2 requires, to two fixed hosts (`nvd.nist.gov`, `www.cisa.gov`), operator-triggered. Security review of the download is part of Phase 5 (what it fetches, caps, failure, the blocked-domain and Internet-off cases).

**Phases (TDD; each its own PR)**
0. Verify by hand, with the owner's say-so for any scan. **Done 2026-10-27 (one Ubuntu host the owner named, one service scan with the app's flags):** `<cpe>` appears in nmap's XML inside `<service>`. Result: SSH → `cpe:/a:openbsd:openssh:9.6p1` (the distribution's `Ubuntu 3ubuntu13.20` suffix is dropped from the CPE but kept in `version`) and `cpe:/o:linux:linux_kernel` (no version); the admin API itself (Go `net/http`) → `cpe:/a:protocol_labs:go-ipfs` (a wrong identification, no version). So 1 of 2 services had a versioned CPE, and one CPE was simply wrong. **Whole home /24, same day (owner approved):** 6 devices up, 16 open services, **3 with a versioned CPE (19%)**, all application CPEs (OpenSSH twice, nginx once); the 2 operating-system CPEs had no version and there were no hardware CPEs. The 10 services with no CPE at all were web ports, DNS, streaming and unknown ports on what look like a router, a media device and a phone or TV; the router-class devices gave nothing usable. So expect matching to cover Linux servers and little else, and the report must say "looked at 3 of 16 services". **Still to do:** NVD's rate limits (read in a browser by the owner; not needed for v1); memory cost of the index at real size (all years is about 415,000 CVEs). Record in the learnings ledger.
1. Parse `<cpe>` into `Port.CPEs` (tests on real XML, hostile text, caps). **Built 2026-10-27:** `nmap/parse.go` `convertCPEs` keeps only the exact CPE 2.2 shape (`cpePattern`, at most 160 characters, 8 per service, deduplicated) and drops anything else whole; `Port.CPEs` is `json:"-"` like `Scripts`, so nothing serialises it yet. Tests: `parse_test.go` `TestParseXML_*CPE*`.
2. **Built 2026-10-27:** `models/vuln_index.go` (types), `core/netaudit/vulnindex` (`ImportNVD`, `ImportKEV`, `Key`; streaming, gzip sniffing, copy-on-write merge, a CVE re-imported replaces its old entries), `platform/persistence/vulnindex.go` (`VulnIndexStore`, compact atomic write, corrupt file quarantined). Real 2026 file: 493 MB read in 2.5 s with 57 MB heap, 33,993 CVEs, 177,604 ranges, 44,242 without configuration data, 1,093 rejected, a compact index of about 9 MB per recent year. Original scope: index types, streaming import, atomic replace, corrupt-file refusal; KEV and NVD fixtures, including one with a `vulnerable: false` platform node that must not be indexed, one CVE with no configurations, and one AND-tree that is skipped and counted.
3. `vulnmatch` with a table test for the version comparator (including `8.9p1`, `1.0.2k`, ranges with both ends open/closed). **Built 2026-10-28:** `core/netaudit/vulnmatch` — `Find(index, cpes)` returns matches (one per CVE, highest score kept, exploited first, then score, then id) and `Uncomparable` (distinct CVEs whose range could not be compared and that matched no other way). The comparator cuts a version into number and letter runs: numbers compare as numbers, a single letter after a number (`p1`, `k`) sorts after the plain release, `alpha`/`beta`/`rc`/`pre`/`dev` sort before it, and an unknown word, an empty piece, or a letter against a number (`1.0.2k` vs `1.0.2.1`) cannot be placed, so it never matches. No version, `-` or `*` never matches. Tests: `version_test.go` (table, with mirror check) and `match_test.go`.
4. Reporter: `known_vulns` finding, severity, fix text, diff/ack behaviour, summary wording. **Built 2026-10-28:** `netaudit/knownvulns.go` — `KnownVulnFindings(hosts, index)` makes one `known_vulns` finding per open port (severity: exploited or score ≥ 9 high, ≥ 7 medium, else low; a distribution build lowers it one step and says so; evidence is fixed words plus at most five validated CVE ids, the finding keeps all in `CVEs`/`Exploited`). `RunReporter.WithVulns(source)` adds them before the comparison, so diff, accept and the baseline work unchanged; `Diff` also counts a changed CVE set as a change, and `StateFor` returns `new_vulns` when an accepted finding gains a CVE or a newly exploited one (a smaller set stays accepted). The report states `compared N of M open services` (and how many could not be compared), or that matching was skipped when there is no data; the connector summary carries counts only. Not wired into the app yet: the source function and the per-audit on/off setting arrive in Phase 5. The auditor model does not see these findings (they are added in the reporter, after the scan tool).
5. API + the Settings card + Help; visual test. **Includes the one-click download, the status endpoint, progress, Resume/Cancel, the `vuln_data_missing` requirement and the per-audit setting** (fixed URLs, guarded client, caps, progress, failure text, its own security review and tests with a fake server) and the file-import fallback. **Backend built 2026-10-28 (the file-import fallback is not built; the UI is below):** `core/netaudit/vulnfeed` (`Manager`: `Start` fetches the files not yet held, which is also how it resumes, or all of them to refresh; `Cancel`; `Close` on shutdown; `Status` derived from what the index holds so a restart mid-download reads as stopped; the index is saved after each file; fixed https URLs on `www.cisa.gov` and `nvd.nist.gov` only, a 256 MiB cap per file, an answer from another host or scheme is refused, failures are one plain sentence that never repeats server text), `NetworkTools.Open` (the guarded GET that `FetchURL` now shares: tools on, internet allowed, blocked domains, address vetting), `GET /admin/api/netaudit/vulndata`, `POST .../download`, `POST .../cancel`, `ScanSettings.KnownVulns` (nil = on; the reporter reads it from the run's context), `VulnSource.Years`, `Readiness.Notices()` and `notices` in the readiness answer (kept apart from `Check` on purpose: the pre-flight refuses an audit on any unmet requirement, and an audit must still run without the data), and the wiring in `app/netaudit.go` (store `vulnindex.json` in the data root, reporter source, shutdown). **Security review of the download (done with the build):** it fetches only two fixed, non-configurable HTTPS URLs; it starts only from the operator's click and never at start-up or on a timer; it uses the guarded client, so the global network guardrails and blocked-domain list apply (a refusal shows as the stopped reason); neither real URL redirects today (checked with a HEAD request on 2026-10-28), and any answer from another host or scheme is refused; the body is read as a stream under a size cap and a gzip-bomb limit in the importer, so memory is bounded; nothing downloaded is executed or shown (only validated ids, versions, scores and dates are kept); the file is written atomically and a corrupt one is set aside; a second start while one runs is a `409`. Remaining risk: the admin API is unauthenticated, so a LAN caller could start a download (harmless: same fixed URLs) or cancel one; replacing the index with chosen data needs the not-yet-built import fallback, which must wait for the auth plan or an operator-only guard.

   **Phase 5 UI built 2026-10-28 (frontend only).** `components/netaudit/VulnDataCard.vue` (a "Vulnerability data" panel in Settings → Network scan, under the allowed networks): Simple is one status line and one button (Download / Resume / Cancel / Update), the attribution to NVD and CISA, the "may be affected, not proof" sentence, and the size of the download before it starts; while running it adds file N of M, the megabytes of the current file and a meter; a stopped download shows the server's plain reason; data older than 60 days is called old. Advanced adds a row per source (the exploited list; NVD with its years) with date, CVE count and what the last file left out. `composables/netaudit/useVulnData.ts` holds one shared status and polls once a second only while a download runs and a screen is mounted (a consumer count stops it at zero); a download that ends re-reads the readiness answer so the notice clears. `ReadinessBanner` lists the `notices` after the requirements as notes (never a warning), with the "Open settings" link (hidden on the page itself) and, while a download runs, the line "Downloading vulnerability data, file N of M"; it asks for the download's state only when that notice is present. `ScanSettingsFields` has "Look for known vulnerabilities" (default on, `scan.known_vulns`, also in the audit's summary line when off), and Help has a "Known vulnerabilities" part. Tests: `__TESTS__/utils/netaudit/vulnData.test.ts`, `composables/netaudit/useVulnData.test.ts`, the component file, and `e2e/netaudit-vulndata.visual.spec.ts` (four states, Simple and Advanced, both widths, no sideways scroll, 44 px button). **Added the same day after review:** "Start again" (Advanced, once something is held and nothing runs) calls `POST .../download?restart=1`, which fetches all files whatever is held (`Manager.Start(restart)`); and an out-of-date notice: `Readiness.Notices()` returns `vuln_data_old` when all files are held and the older source is more than 60 days old (`Status.Age`, `OldVulnDataAge`), and the banner offers **Update now**, which starts the update in place and then shows the file-by-file progress line (a missing or half-finished download still points to the card, where its size and progress are shown). Still no automatic download: the notice appears and one click starts it. Update is offered only when the data is old (more than 60 days); on held, recent data a plain start does nothing (`vulnfeed.ErrUpToDate`, `StaleAfter`) and only "Start again" refetches. A refresh counts its progress from 0 of N (`jobSize`/`jobDone`), not from the files held. When it runs, Update re-downloads every file and merges it by CVE id (a CVE in a file replaces its earlier entries; the exploited list is replaced whole); the window of five years moves with the calendar and a year that falls out of it stays in the index. A cheaper refresh (the exploited list and the last two years only) is not built. A source with no import date is hidden rather than listed as empty. **Delete data (2026-10-28):** `DELETE /admin/api/netaudit/vulndata` (`Manager.Clear`: stops a running download, deletes `vulnindex.json` and any `.corrupt` copy, empties the index in memory) and a "Delete data" button in the card (both views, only when something is held and nothing runs, after a confirmation that says what it costs). A factory reset also clears it, through `AppContext.AfterFactoryReset`; the reset still keeps `meta/`, so scan results, accepted findings and fix records survive it (`data-layout.md`). `vulnfeed.New` no longer takes a context (Constitution II.2, constructors allocate only): the application attaches its root context with `Manager.Attach`, and a start before that answers `ErrNotReady`. **Advanced detail added after the first real run (2026-10-28):** a `known_vulns` row in Advanced names the software that was compared (service and version, the text the match was made on) and folds a list of every flaw behind "All N known flaws": exploited first, then highest score, each id linking to its NVD page, with its score. The finding carries `cve_scores` for this (display only; the diff and accept still use the sorted id sets). Simple keeps the one line. **Defaults used, owner to confirm:** an audit still runs when the data is missing and the report says matching was skipped; progress is by polling; the last five NVD years; no automatic refresh. **Done afterwards (2026-10-28):** the playbook declares `**Known vulnerabilities:** on` and explains it (`TemplateStore` reads the field into `scan.known_vulns`); the 13.C.2 design is written (§13.H); the Ubuntu Landlock check has a runbook (§13.D) but has not been run. **Still not built:** the file-import fallback (waits for admin-API authentication or an operator-only guard).

**Admin API.** The download and import endpoints are a new operator-only write on the unauthenticated admin API: a LAN caller could import an empty or hand-made index and silence every `known_vulns` finding. The auth plan (§9, "features that depend on this plan") lists it, and the Security card says it is an operator action by convention until auth lands.

**Risks.** Version comparison edge cases (mitigated: doubtful = no match, counted); a very large index (streaming import, in-memory only the application CPEs, measured in Phase 0); false alarms on backported software (wording, accept flow); the operator never imports (matching is off and the card says so); an untrusted file (strict parser, size cap, nothing from it shown).

**Review.** Reviewed 2026-10-27 (row 3 of the review log in §12); the owner's acceptance is still needed before Phase 1.

## Appendix A — what packet capture (wireshark/tcpdump) would need

A **separate category** (e.g. `netcapture`), not part of `netaudit`: it needs elevated rights (conflicts with the
unprivileged service user), sees other parties' content including credentials, and on a switched LAN sees little beyond
the host. If ever built: off by default and operator-enabled, interface allow-list, time/size caps, capture file outside
the jail, the model sees only parsed summaries (never packets), and a `Program` value plus its own amendment. The
guarded-program runner is shaped so that adds a declaration, not plumbing; nothing for it is built now.

### 13.I Design for review — expectations per device (13.B.3)

*Status: draft for the owner's review; nothing is built. Read-only, no new probing, no Constitution change.*

**What it is.** The operator records what a device should offer ("the printer: only 631"). After each scan, code reports the difference. It reuses what the scan already finds; it sends nothing new.

**Data.** `Expectation{Device, Ports []int, Note, SetAt}`, one per device, stored per workspace in the same way acknowledgements are (`ack.go`, `ackservice.go`, the `persistence` JSON helpers: missing file = none, corrupt = quarantined). TCP only, like the scan. A device with no expectation behaves exactly as today.

**Findings.** Two new checks in `checks.go`'s fixed vocabulary, evaluated in `RunReporter` beside `applyAcks`, in fixed wording, never from device text:
- `unexpected_port` (medium): an open port that is not on the device's list. Key `device|tcp|port|unexpected_port`, so accepting it, "I've done this" and the baseline diff work unchanged.
- `expected_port_closed` (low): a listed port that did not answer, which usually means the service is down. It is skipped for a device in `Unscanned` (its ports are unknown, not none).
Both appear in `ProblemGroups` with a fix written once ("close it on the device, or add it to the expected list").

**API.** `GET/PUT/DELETE /admin/api/netaudit/expectations`. A device must be inside the declared scan scope (`scanscope`); at most 64 ports per device and a cap on devices; a refused entry is a 400 that names it, as the scope endpoint does.

**UI.** The Advanced device header gets "Set as expected": one click saves the device's current open ports, since typing a list is the slow way. An expectation shows as a short line on the device ("Expects 631") with Edit and Remove. Simple shows only the problems.

**Open questions for the owner.**
1. **Identity is the IP address.** `-sT --unprivileged` gives no MAC, so a DHCP change moves an expectation to the wrong device. Options: accept it and say so in the UI (recommended, same as acks), or require static leases for devices with an expectation.
2. **Should a device with no open port at all be allowed an empty list** ("nothing should answer")? Recommended: yes.
3. **Authentication.** Like acknowledgements, saving an expectation is an operator action behind the parked admin-API auth (D1); it stays a convention until that lands. An expectation can only hide or reveal findings, never widen the scan.

**Owner decisions 2026-10-28:** set with one click from the current open ports (then editable); a device is identified by its IP address and the UI says so; `unexpected_port` is medium and `expected_port_closed` is low; an empty list is allowed.

**Built 2026-10-28 (UI too: `ExpectedPortsDialog.vue`, "Set as expected" / "Edit expected" / "Remove expected" on the Advanced device header via `DeviceFindings.vue`, `useFindings.setExpected`, `utils/netaudit/expectations.ts`; the Help text says to set it again if a device gets a new address):** `models.Expectation`; `persistence/expectations.go` (`expectations.json` in the workspace's internal directory, outside the agent's jail, like the fix records); `netaudit/expectations.go` (`NewExpectation`, `ExpectationFindings`, the `Expectations` service, caps of 64 ports and 256 devices); `RunReporter.WithExpectations` adds the two checks to the scan's findings before the comparison, so they are diffed, accepted and kept like any other (a store that cannot be read is stated in the report, never hides the scan); a fix and a guide for each check; `GET/PUT/DELETE /admin/api/netaudit/expectations`. The deep check is §13.J.

**Phases.** (1) model, store, evaluator and tests; (2) endpoints; (3) UI and the report line; (4) docs. Each is small enough for one PR; (1) and (2) can ship together.

### 13.J Design for review — the interactive deep check (intrusive scripts)

*Status: draft for the owner's review; nothing is built, and the Constitution text below is a proposal, not applied law.*

**Goal.** Find what the nightly checks may not: SSL/TLS versions and ciphers a service still accepts (`ssl-enum-ciphers`, nmap categories `discovery, intrusive`) and whether SSH accepts passwords (`ssh-auth-methods`, `auth, intrusive`; it tries a random user name). Both talk only to the scanned host, but they send many handshakes or an authentication attempt, which II.3 does not cover ("anything that sends more than ordinary connections, tries credentials, or could disturb a device is intrusive ... needs its own amendment").

**Shape (owner decision 2026-10-28: a separate, interactive check of one device).**
- A new scan mode `deep` in the mode table (`nmap/argv.go`) with its own fixed list, `DeepScriptNames()`, kept apart from `CheckScriptNames()` so a nightly audit can never load it. The mode is refused unless the request carries the operator's grant.
- Started only from the UI for one named device already inside the scan scope: `POST /admin/api/netaudit/deep {device}`. The button states what will be sent. It is **not** an agent tool and has no catalog entry, so a model cannot start it, and automations cannot either.
- Limits in code: one device per request, only the ports the last scan found open on it, one run at a time, a cool-down per device, a hard run time limit, conservative timing.
- Output is parsed into typed findings with fixed wording (`tls_weak_protocol`, `ssh_password_login`), as II.3 item 6 requires. The last deep result per device is kept and carried into the next audit's report marked "from the deep check of <date>", because the nightly scan does not repeat it.

**Proposed Constitution amendment (for the security review, V.1).** Add to II.3's guarded-program text: *an intrusive program run is allowed only when (a) the operator asks for it in the interface for one named device in scope, (b) it never starts from an agent, an automation or any unattended path, (c) its scripts come from a separate exact reviewed list that never contains a script that guesses or brute-forces credentials, exploits a flaw, denies service, or can open connections to hosts other than the target, and (d) each run is rate-limited and time-limited. A change to that list is a security review.* Items (1) to (4) and (6) of the guarded-program text apply unchanged.

**Owner decision 2026-10-28: admin-API authentication stays parked, so option (a) below holds: the deep check is designed but not built until authentication lands, and D1 is unchanged.** Questions 2 and 3 still need answers before then.

**Open questions for the owner.**
1. **Authentication (decided: wait).** D1 says intrusive modes against the *host's own addresses* wait for admin-API auth; the note after §13.B.4 says intrusive modes need auth in general. Without it, anyone who can reach the API can trigger a deep check on a LAN device. Options: (a) wait for auth (nothing is built); (b) amend D1 so a deep check may run against other LAN devices, never the host's own addresses, until auth lands, with the risk written into the Security UI copy as for acks. I recommend (b) only if you accept that a LAN caller can start one probe of one device at a time, rate-limited; otherwise (a).
2. **Is `ssh-auth-methods` acceptable at all?** It sends an authentication attempt for a random user. The proposed wording allows it because it guesses no credential, but it is the closest item to II.3's line; dropping it leaves TLS only.
3. **Real-device safety.** Both scripts have only been read, not run here. Before the list is final, run each once against a device you do not mind disturbing and record what it sent (ADR-0007: fixtures are not evidence).

**Phases (after the amendment and the answers above).** (1) amendment text applied; (2) `deep` mode, list and refusal tests; (3) endpoint with grant, limits and cool-down; (4) findings, carry-over and report line; (5) UI button and copy.

## Remaining Work

**Current (2026-10-28):** v0.15.0 adds the Tier 1 checks and the grouped report (13.B.2 as built, above); not yet run against real devices (the four new scripts and the Docker/Telnet rules are covered by fixtures and the argument tests only, like ADR-0007 says). Next on the owner's list: the interactive deep check for intrusive scripts (design §13.J, on hold until admin-API authentication lands) and expectations per device (13.B.3, design §13.I; built 2026-10-28, all four phases). **Earlier (2026-10-27):** §13.A is merged (#146, v0.13.0, live on vertex; a live rescan there has not been tried yet). Left is everything in §13.B–F, which waits on the owner's
decisions (§13.F) or its own reviewed design; plus the hand verification of nmap under Landlock on Ubuntu (§13.D).
**Owner decisions 2026-10-27:** vulnerability matching yes (NVD + KEV; design §13.G); assisted fixes yes as a design (targets set in 13.C.2); guarded-programs Constitution text approved and applied; admin API authentication stays parked. Source (NVD + KEV) and fix targets confirmed the same day. Next: accept the §13.G design, then build 13.B.1 phase by phase; the 13.C.2 design follows. The notes
below are the phase-by-phase log and are due to be harvested into the learnings ledger and ADRs.

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
- **Speed, measured on a real /24 (192.168.50.0/24, 7–10 live hosts):** the first cut was slow — nmap `ports` took 136 s because it
  scanned every dead address (`-Pn`), nmap with its own host discovery took 23 s but found 6 hosts, not 8, and the Go prober took
  45 s. Retries on silent hosts dominate nmap's time (turning them off lost hosts). So **every scan sweeps first** with the Go
  prober (256 workers, 700 ms; a refusal counts as alive) and the engine looks at only the live hosts (`-Pn`). **Quick (default):**
  six liveness ports, then nmap `-T4` on the 24 common ports — discover 4 s, ports 11 s, services 29 s, 17 open ports.
  **Thorough (`thorough: true`, for audits):** a wider sweep (the quick ports plus every default port), then nmap `-T3`,
  `--max-rate 300`, the top **300** ports, 180 s per host — 2 m 04 s to 2 m 22 s, 10 hosts and 34–35 open ports (quick: 8 and 17).
  Top 1000 was tried and rejected: a host that drops probes takes about 205 s for 1000 ports against 62 s for 300, so the per-host
  limit abandoned every host. The first thorough design (no sweep, `-Pn` on all 254 addresses) took over 10 minutes, then 8
  minutes with a partial result, and reported every address as up.
- **Honesty about what a scan did not finish:** a scan that hits the engine's 8-minute limit returns the hosts it finished with a
  warning (`progexec` returns the partial stdout with the timeout; `ParsePartialXML`). A live host nmap abandoned (its per-host
  limit) is flagged `Unscanned` — "ports unknown", never "none open" — named in the report's Observed section, left out of the
  comparison, and its earlier findings are carried forward rather than called resolved. Devices rate-limit scanners: after
  minutes of heavy scanning one run saw 4 open ports and the next 0 where the same command later found 18, so a lower count
  than the last scan is a reason to look, not proof.
- Not done: ~~the Ubuntu run (Landlock grants for nmap),~~ persisted `allowed_tools` entries
  that still name `scan_local_network` (they now match nothing).

**Phase 6 backend done (branch `netaudit-guessed-services-and-netlink`):** accepted findings — `models.ScanAck`, `acks.json` per workspace
(the workspace-level path helper §4.5 was waiting for), `netaudit.NewAck/StateFor/ApplyAcks/HideAccepted`, `Acks` service and
`POST/GET/DELETE /admin/api/netaudit/acks`; `GET .../findings` marks accepted findings. Accepted findings are left out of what the
model sees (`scantool`) and counted in the Observed section, and stay in the baseline. The report UI (`FindingsReport`) waits on
the wireframes (F-0). Also from the vertex run of a real scheduled audit (Qwen3.6 local, thorough services, all of profile,
pre-flight, Observed, baseline and comparison worked): nmap's port-table names are marked as guesses (8081 had been read as
"blackice-icecap" and described by the model as an IDS), and where a service cannot open a netlink socket (vertex's systemd unit)
the interface list falls back to the default route's address on a /24, for both the scope suggestions and `get_network_info`.

**Phase 5 finished (branch `netaudit-summary-delivery-and-local-preflight`):** the connector message for an audit is a one-line
count summary written by code (`netaudit.SummaryOf`, `ExecuteResponse.Summary`, delivered instead of the report), and the
pre-flight refuses a scheduled audit whose model is not local before waking it (`model_not_local`).

**F-0 wireframes published for sign-off** (2026-10-25): https://claude.ai/artifact/BQPr4piiBVbmoVbDpgxHRp — six artboards: Settings →
Network scan at desktop and phone width, the readiness banner in every state with the confirm dialog, the findings report at
desktop and phone width (grouped by device, new/changed/accepted/resolved, the accept dialog and its bottom-sheet form, the
"not scanned to completion" notice), and the nightly-audit setup checklist. Four decisions to confirm are on the canvas. Nothing
in F-A (the UI build) starts before this is signed off.

**F-A built** (branch `netaudit-ui`, 2026-10-25): Settings → Safety → Network scan (Simple "Use my network" / Advanced ranges,
suggestions, typed entries, unconfined switch), `ReadinessBanner`, the Help panel, `FindingsReport` (Simple/Advanced, accept and
take back) on `security-auditor` automations, and `findingView.Key` on the findings API. Not in it: the nightly-audit setup
checklist (board 6; needs a create-automation path that sets profile `security-auditor`, `network_grant: lan`, journal off), and
per-host "not scanned to completion" (the findings API does not expose it yet).

**Setup made one path (2026-10-25):** no separate "create audit" endpoint. `network_audit.md` (was `nightly_network_audit.md`; `compliance_check_internal.md` and `network_recon_unprivileged.md` were removed — they are this audit with a `PORTS` line / `DEPTH: thorough`, and need `get_network_info`, which the auditor lacks) declares `**Agent:** security-auditor`,
`**Network:** lan`, `**Journal:** off` and has editable `TARGETS` / `DEPTH` / `PORTS` / extra-instruction sections; the ordinary new-automation
form applies the declared settings; playbook headers also carry `**Summary:**` and `**Schedule:**` (cron); Settings → Network scan has a "Set up a network audit" button (enabled once a scan could run) that opens that form from the playbook.
`network_scan` with no `targets` scans every allowed range (`Policy.Authorize`), so a model never guesses a network (an auditor run that
named no target had guessed 192.168.1.0/24). Also built: `AuditSetupCheck` (readiness + "this automation can't reach your network") in the form and on the page, `FailureExplanation` (plain failures with the fix, raw text behind "Show details"), playbook library descriptions and "Sets up:" line. Per-device time: `netaudit.device_timeout_minutes` (0 standard = 60 s quick / 180 s thorough, +120 s in checks mode because the scripts run inside it; 5 or 10), Advanced-only "Scan time" panel (anchor `#scan-time`, reached from the report's "Allow more time"); a device that times out is retried once with twice the time when at least 90 s of the 8-minute scan budget remain (`Service.retryUnscanned`). **Time budget (revised 2026-10-26 after a real run took 508 s of a 600 s cap and starved the retry):** `netaudit.ScanBudget` = 12 min for the scan, and audit runs get their own cap, `netaudit.AuditRunCap` = 20 min, through `automation.WithProfileTimeout` (wired in `BuildDispatcher`); every other automation keeps the 10-minute cap. nmap now scans all live devices in one group (`--min-hostgroup`, at most 64) so a scan takes as long as its slowest device, not the sum of groups of five. The report refetches when a run starts or finishes (`refreshKey`). The Network audit panel lists existing audits (automations on the Security auditor; `AuditList`) and offers "Set up" only when none exists ("Add another audit" in Advanced). Still to do: `checks` mode (nmap safe scripts); password testing needs its own decision and a Constitution review.

**Checks mode built (branch `netaudit-checks`, 2026-10-26; owner approved the list, `CHECKS: on` by default and the host's own addresses).**
`network_scan` mode `checks` = `services` plus nmap's safe scripts by exact name: `ftp-anon`, `nfs-showmount`, `rdp-enum-encryption`,
`redis-info`, `smb-protocols`, `smb2-security-mode`, `ssh2-enum-algos`, `ssl-cert`, `vnc-info` (`netaudit/checks.go` `checkTable`: one row per
script, and the nmap argv asks for exactly `CheckScriptNames()`; a test pins the list). Each was checked against nmap 7.99's own source: category
`safe`, no credential attempts, sockets only to the scanned address (`ftp.lua` PASV uses only the port the server names). Left out: `ssh-auth-methods`
and `ssl-enum-ciphers` (nmap classes them intrusive), every `brute` / `empty-password` / `default-accounts` / `vuln` script, and the `http-*`
scripts (they follow redirects). `ssl-cert` reports expired or soon-to-expire certificates only; self-signed is not a finding because nearly every home
device uses one. Findings come from code-written evaluators and carry the code's own wording as evidence: no text a device wrote reaches a finding, a
report or a chat message; script output is kept only for the reviewed scripts, cleaned and capped (`parse.go`). Check findings have their own
`Check` and `Severity` (low to critical), so they are compared, accepted ("This is fine") and shown like open ports; the report has a "Checks" table.
Verified end to end against real nmap by `TestChecks_RealNmapFindsAnExpiredCertificate` (skips where nmap is absent). **Not verified:** the scripts under
the Ubuntu Landlock grants (the sandbox-conformance job has no nmap), and each script's output on a real device other than the TLS fixture.

**False "not scanned to completion" fixed (2026-10-26, found on a real run):** nmap's `--open` leaves a device with no open ports out of its XML entirely, and the service read a device missing from the output as "ran out of time"; `--open` is gone (open ports are picked out by state). The converse is also fixed: a device that really hits `--host-timeout` is printed as `<host timedout="true">` with no ports, which used to read as "scanned, nothing open" (and could make its earlier findings look resolved); it is now `Unscanned` with no ports on record. Both shapes are real-nmap fixtures (`closed_host_all_ports_closed.xml`, `timed_out_host.xml`).

**Audit settings are real settings (v0.12.0, branch `netaudit-audit-settings`, 2026-10-26; replaces the `TARGETS` / `DEPTH` / `PORTS` / `CHECKS` task-text lines):** `models.Automation.Scan` (`ScanSettings`: targets, depth, ports, checks) is stored with the automation, validated on save (`netaudit.ValidateScanSettings`), carried `Register` → `ExecuteRequest.Scan` → the run context (`models.WithScanSettings`) and applied by the scan tool (`netaudit.RequestFromSettings`): **what the model passes for them is ignored**, which ends the class of bug where a model guessed a network or turned checks off. An audit with no settings of its own gets the defaults in code (`Automation.EffectiveScan`), so old audits are covered without reading task text. The playbook declares the starting values in its header (`**Depth:**`, `**Checks:**`, optional `**Ports:**` / `**Targets:**`); its body keeps only the optional extra instructions. UI: the automation form shows "What the audit scans" for the Security auditor (Simple: how thoroughly + checks; Advanced: networks + ports) and the details page a "Scans" line.

Check findings stay in the report until accepted (a problem found yesterday is not "new" today but is still a problem); they have a coloured edge and tint by severity (`SEVERITY_CARD_CLASS`), a "Problem · <severity>" tag, and a Problems tile/filter in Advanced. `ssl-cert` no longer reports "expiring soon" for a certificate issued for 14 days or less (devices such as Chromecasts renew those daily).

Not done from §4.1: a `validateCategory` case for the future `network_scan` (it now dispatches network tools by `Gate`, so a
new row with `GateNetwork` gets the L1 checks); the `validateNetwork` scan/info messages still name the two tools.

**Everything else (Phase 5 onwards; the UI waits for the wireframes, F-0):** Suggested first PR: Phase 0 (characterisation tests + the verifications) — it changes no behaviour and
settles the open facts in §11. All twelve decisions are made.
