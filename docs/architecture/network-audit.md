# Network audit — how it is built

The as-built design of the network audit: the tool catalog, guarded programs, the netaudit domain, persistence,
readiness, the report and the UI, the security design, and the features built on it (known-vulnerability matching,
guided fixes, expectations per device, the deep check). **Harvested on 2026-10-29 from the plan
[`PLANS/ARCHIVE/cross-cutting/network-audit.md`](../PLANS/ARCHIVE/cross-cutting/network-audit.md)** with its section numbers kept, so
a reference such as "§4.3" or "§13.J" still finds its text. The text was moved, not rewritten; read it as the design at
the time it was merged. Where it says admin-API authentication is parked or that a feature waits for it, that is
history: sign-in has been compulsory since v0.22.0 (see
[`admin-api-authentication.md`](../PLANS/cross-cutting/admin-api-authentication.md)), the deep check (§13.J) is built,
and the file import for the vulnerability data (§13.G) is built (`POST /admin/api/netaudit/vulndata/import`, a step-up
route). Level 2 assisted fixes (§13.H) were decided against on 2026-10-29 and are not built.

Laws it must keep: [`CONSTITUTION.md`](../../CONSTITUTION.md) I.1–I.3, II.3 (guarded programs and "Intrusive runs"), II.10.

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

   **Phase 5 UI built 2026-10-28 (frontend only).** `components/netaudit/VulnDataCard.vue` (a "Vulnerability data" panel in Settings → Network scan, under the allowed networks): Simple is one status line and one button (Download / Resume / Cancel / Update), the attribution to NVD and CISA, the "may be affected, not proof" sentence, and the size of the download before it starts; while running it adds file N of M, the megabytes of the current file and a meter; a stopped download shows the server's plain reason; data older than 60 days is called old. Advanced adds a row per source (the exploited list; NVD with its years) with date, CVE count and what the last file left out. `composables/netaudit/useVulnData.ts` holds one shared status and polls once a second only while a download runs and a screen is mounted (a consumer count stops it at zero); a download that ends re-reads the readiness answer so the notice clears. `ReadinessBanner` lists the `notices` after the requirements as notes (never a warning), with the "Open settings" link (hidden on the page itself) and, while a download runs, the line "Downloading vulnerability data, file N of M"; it asks for the download's state only when that notice is present. `ScanSettingsFields` has "Look for known vulnerabilities" (default on, `scan.known_vulns`, also in the audit's summary line when off), and Help has a "Known vulnerabilities" part. Tests: `__TESTS__/utils/netaudit/vulnData.test.ts`, `composables/netaudit/useVulnData.test.ts`, the component file, and `e2e/netaudit-vulndata.visual.spec.ts` (four states, Simple and Advanced, both widths, no sideways scroll, 44 px button). **Added the same day after review:** "Start again" (Advanced, once something is held and nothing runs) calls `POST .../download?restart=1`, which fetches all files whatever is held (`Manager.Start(restart)`); and an out-of-date notice: `Readiness.Notices()` returns `vuln_data_old` when all files are held and the older source is more than 60 days old (`Status.Age`, `OldVulnDataAge`), and the banner offers **Update now**, which starts the update in place and then shows the file-by-file progress line (a missing or half-finished download still points to the card, where its size and progress are shown). Still no automatic download: the notice appears and one click starts it. Update is offered only when the data is old (more than 60 days); on held, recent data a plain start does nothing (`vulnfeed.ErrUpToDate`, `StaleAfter`) and only "Start again" refetches. A refresh counts its progress from 0 of N (`jobSize`/`jobDone`), not from the files held. When it runs, Update re-downloads every file and merges it by CVE id (a CVE in a file replaces its earlier entries; the exploited list is replaced whole); the window of five years moves with the calendar and a year that falls out of it stays in the index. A cheaper refresh (the exploited list and the last two years only) is not built. A source with no import date is hidden rather than listed as empty. **Delete data (2026-10-28):** `DELETE /admin/api/netaudit/vulndata` (`Manager.Clear`: stops a running download, deletes `vulnindex.json` and any `.corrupt` copy, empties the index in memory) and a "Delete data" button in the card (both views, only when something is held and nothing runs, after a confirmation that says what it costs). A factory reset also clears it, through `AppContext.AfterFactoryReset`; the reset still keeps `meta/`, so scan results, accepted findings and fix records survive it (`data-layout.md`). `vulnfeed.New` no longer takes a context (Constitution II.2, constructors allocate only): the application attaches its root context with `Manager.Attach`, and a start before that answers `ErrNotReady`. **Advanced detail added after the first real run (2026-10-28):** a `known_vulns` row in Advanced names the software that was compared (service and version, the text the match was made on) and folds a list of every flaw behind "All N known flaws": exploited first, then highest score, each id linking to its NVD page, with its score. The finding carries `cve_scores` for this (display only; the diff and accept still use the sorted id sets). Simple keeps the one line. **Defaults used, owner to confirm:** an audit still runs when the data is missing and the report says matching was skipped; progress is by polling; the last five NVD years; no automatic refresh. **Done afterwards (2026-10-28):** the playbook declares `**Known vulnerabilities:** on` and explains it (`TemplateStore` reads the field into `scan.known_vulns`); the 13.C.2 design is written (§13.H); the Ubuntu Landlock check has a runbook (§13.D) but has not been run. **File import (built 2026-10-29):** `vulnfeed.Manager.Import`, `POST /admin/api/netaudit/vulndata/import` (step-up), "Import a file instead" in the data card; see ADR-0013.

**Admin API.** The download and import endpoints are a new operator-only write on the unauthenticated admin API: a LAN caller could import an empty or hand-made index and silence every `known_vulns` finding. The auth plan (§9, "features that depend on this plan") lists it, and the Security card says it is an operator action by convention until auth lands.

**Risks.** Version comparison edge cases (mitigated: doubtful = no match, counted); a very large index (streaming import, in-memory only the application CPEs, measured in Phase 0); false alarms on backported software (wording, accept flow); the operator never imports (matching is off and the card says so); an untrusted file (strict parser, size cap, nothing from it shown).

**Review.** Reviewed 2026-10-27 (row 3 of the review log in §12); the owner's acceptance is still needed before Phase 1.


## Device inventory and the presence check (as built 2026-10-29, v0.25.0)

Harvested from [`PLANS/cross-cutting/network-detection.md`](../PLANS/cross-cutting/network-detection.md) §5; the reasons are in [ADR-0014](../decisions/ADR-0014-presence-check-and-device-inventory.md).

- **Source.** `platform/neighbors` parses `/proc/net/arp` (complete, unicast entries only; an entry the kernel asked about and nobody answered has flags `0x0` and a zero MAC and is skipped) and the default route in `/proc/net/route`. Off Linux it returns `ErrUnavailable`. `app/netaudit.go` adapts it to the domain's `NeighborSource`.
- **Inventory.** `core/netaudit/inventory.go`: `Observe` is pure; `Inventory` holds the mutex, the store (`persistence.DeviceInventoryStore`, `devices.json` in the data root, deleted by a factory reset), the scan scope as an `inScope` predicate and the operator's actions (`Name` approves, `Approve`, `Forget`, `ConfirmGateway`). A failed read of the table changes nothing. The first look that finds anything is the baseline.
- **Findings.** `InventoryFindings(inv, now)` and `HiddenDeviceFindings(current, hosts)`; checks `new_device`, `mac_changed`, `gateway_mac_changed`, `duplicate_mac`, `device_missing`, `hidden_device`, each with title, fix and guide (completeness tests). `RunReporter.WithInventory` calls `Refresh` after the scan and keeps only findings of devices that scan looked at, so a rescan of one device does not report the rest as new.
- **Presence check.** `core/netaudit/presence.go`; settings in `NetauditConfig` (`presence_check`, `presence_interval_minutes`) through `PresenceConfig`; started by `AppServices.StartNetauditPresence` from `app.New`. Pending alerts are derived from the inventory (`Inventory.Pending`) and marked after a successful send (`MarkNotified`). The message is `PresenceMessage`: counts, no names.
- **API.** `GET/PUT /admin/api/netaudit/presence`, `GET /admin/api/netaudit/devices`, `PUT`/`DELETE /admin/api/netaudit/devices/{mac}`, `POST /admin/api/netaudit/devices/gateway/confirm` (step-up).
- **UI.** `components/netaudit/DevicesCard.vue`, `composables/netaudit/useDevices.ts`, `utils/netaudit/devices.ts`.
- **Identity drift.** `core/netaudit/identity.go` (`ObservedIdentities`, the pure `TrackIdentities`, `IdentityFindings`, the `Identities` service) reads fingerprints from the `ssh-hostkey` and `ssl-cert` output, stores them in `identities.json` (`persistence.DeviceIdentityStore`, deleted by a factory reset), and `RunReporter.WithIdentities` adds `hostkey_changed` and `cert_changed` for the devices that scan looked at.
- **Router UPnP.** `core/netaudit/upnp` (`Client.Probe`, `ParseSSDP`, `ParseDescription`) behind `netaudit.RouterCheck`, wired in `app/netaudit.go` (`newRouterCheck`) with `NetworkTools.DialContext()` and the guarded HTTP client; `RunReporter.WithRouter` adds `upnp_enabled` and one `upnp_forward` per enabled port mapping (`upnp.Client.Mappings`). Nightly audit only. A router whose list could not be read returns its other findings with an error, stated in the report.
- **IPv6.** `neighbors6*.go` (netlink on Linux, `ErrUnavailable` elsewhere); `Observe` attaches addresses to a device by MAC (`attachIPv6`, `scoped6`); no finding is keyed by an IPv6 address. **Active discovery** (Constitution I.7, ADR-0016): `neighbors.Discover6` (`discover6*.go`; Linux sends one ICMPv6 echo to `ff02::1` per interface through the unprivileged ICMP socket, `ErrNotPermitted` when `net.ipv4.ping_group_range` forbids it) is called by `Inventory.discover` (`netaudit/discovery6.go`) inside `Refresh`, on the links of in-scope IPv4 devices, then the table is read again (`NeighborTable.Discovered6`); the first such look sets `DeviceInventory.Baselined6` and approves the IPv6-only devices it finds. Switch and problem code: `Discovery6Config`, `GET/PUT /admin/api/netaudit/ipv6-discovery`; the app adapter is `echoDiscoverer`.
- **Decoys.** `core/netaudit/decoy` (`Manager`, `Specs`), `persistence.DecoyTouchStore` (`decoys.json`), `netaudit.DecoyFindings`/`DecoyMessage`/`DecoyConfig`, `NetauditDecoyHandlers`; the alert lists the sources' addresses (`Deps.Notify` takes them, `DecoyMessage` words them); started by `AppServices.StartNetauditDecoys` in `app.New`; Constitution I.6, ADR-0015.
- **Port checks.** `portchecks.go` holds one `exposures` table (check, ports, a confirming test, and the words for "confirmed" and "port only"); `DefaultPorts` and `ThoroughPorts` carry the ports.

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

**(Historical: on 2026-10-28 the owner kept admin-API authentication parked and the deep check was designed but not built. Both changed: sign-in has been compulsory since v0.22.0 and the deep check is built, v0.23.0.)** Questions 2 and 3 still need answers before then.

**Open questions for the owner.**
1. **Authentication (decided: wait).** D1 says intrusive modes against the *host's own addresses* wait for admin-API auth; the note after §13.B.4 says intrusive modes need auth in general. Without it, anyone who can reach the API can trigger a deep check on a LAN device. Options: (a) wait for auth (nothing is built); (b) amend D1 so a deep check may run against other LAN devices, never the host's own addresses, until auth lands, with the risk written into the Security UI copy as for acks. I recommend (b) only if you accept that a LAN caller can start one probe of one device at a time, rate-limited; otherwise (a).
2. **Is `ssh-auth-methods` acceptable at all?** It sends an authentication attempt for a random user. The proposed wording allows it because it guesses no credential, but it is the closest item to II.3's line; dropping it leaves TLS only.
3. **Real-device safety.** Both scripts have only been read, not run here. Before the list is final, run each once against a device you do not mind disturbing and record what it sent (ADR-0007: fixtures are not evidence).

**Owner decisions 2026-10-30:** the amendment is approved and applied (Constitution II.3, "Intrusive runs"); admin authentication has landed (0.22.0, sign-in compulsory), so question 1 is answered by (a): the deep check needs nothing more; `ssh-auth-methods` stays; the real-device test ran (below).

**Real-device evidence 2026-10-30 (nmap 7.99, the product's flags: `-sT -sV --version-intensity 2 -Pn -T2 --max-retries 1 --script-timeout 30s --unprivileged`, one port at a time).**
- `ssh-auth-methods` on `192.168.50.60:22` (vertex, OpenSSH 9.6p1 Ubuntu): **2 TCP connections, about 0.6 s**; a normal SSH handshake, then a list of the methods the server accepts, asked for a random five-letter user name (the script source: `rand.random_alpha(5)`; nmap's own description says the abandoned attempt "will likely be logged"). It guessed no password. Result: `publickey`, `password`, so **vertex's SSH accepts password login** (a real finding).
- `ssl-enum-ciphers` against a throwaway OpenSSL server on this machine (`127.0.0.1:9443`; no device on the network offers TLS): **58 TCP connections (one handshake each) in 28 s** for one port; it lists the protocol versions and ciphers with a grade and its least strength. That is the load a fragile device would see, which is why the deep check takes one port at a time, `-T2`, and the 30 s script limit.
- The script only runs on a port nmap already knows is TLS: with plain port detection RDP printed nothing; `-sV` is needed (the checks mode already has it). A device that closes the connection at once (RDP on `192.168.50.10:3389`, `tcpwrapped`) or answers plain HTTP (Plex on 32400) gives the script nothing, and the deep check must say "no TLS service answered", not "no problem".

**Phase 1 built 2026-10-30 (branch `netaudit-deep-mode`, backend only, nothing reachable yet):** `ModeDeep` (`Intrusive`, needs nmap) in `mode.go`; `Modes()` now lists only what an ordinary scan accepts, so deep is valid but never offered; `Service.Scan` refuses an intrusive mode with `ErrIntrusiveRefused` before the authorizer or the engine is touched (the agent tool and the nightly audit both come through it); `deep.go`: `DeepScriptNames()` (`ssh-auth-methods`, `ssl-enum-ciphers`, a list of its own that never mixes with `CheckScriptNames()`), `deepTable` evaluators with fixed wording (`tls_weak_protocol`: SSL 2.0/3.0 high, TLS 1.0/1.1 medium; `tls_weak_cipher`: nmap grade C medium, D to F high; `ssh_password_login`: `password` or `keyboard-interactive`, medium) and `DeepFindingsOf` (deep checks of open ports only, no open-port findings); a fix line, a title and a guide for each of the three (`remediation.go`, `fixguide.go`); `nmap/argv.go`: the `deep` profile (`-T2`, `--max-rate 50`), exactly one target and explicit ports required, `-Pn`, the deep list only; `parse.go` keeps both reviewed lists' output. Tests use real nmap 7.99 XML captured against vertex (SSH) and a local OpenSSL server (TLS), addresses replaced (`nmap/testdata/deep_*.xml`). **Not built:** the service method and endpoint with the limits (phase 3), persistence and the report line (phase 4), the UI (phase 5).

**Phases 3 to 5 built 2026-10-30 (same branch, v0.23.0).** *Phase 3:* `Service.DeepScan` (nmap only, no sweep, one device, the given ports, a six-minute deadline, the shared one-scan-at-a-time lock, refuses an unattended run and a device outside the scope right now); `DeepChecks.Run` (reads the device's open TCP ports from the audit's latest scan, at most 16, refuses a device that scan did not see or ran out of time on, ten-minute cool-down judged from the stored time, stores the result only when the run succeeded); `POST/GET /admin/api/netaudit/deep` and the step-up route (a session with a freshly confirmed passphrase, never a token; a test pins the agent tool's mode list to `netaudit.Modes()`). *Phase 4:* `persistence/deepchecks.go` (`deepchecks.json`, outside the agent's jail, one result per device, newest 256), `RunReporter.WithDeepChecks` carries the findings into each audit (only while the device is up and finished and the port still open, only deep-check findings, 90 days, dated in the evidence, stable between audits), and a fix mark on a deep finding is judged against the deep check's own time. *Phase 5:* the Deep check button, the question that states what is sent, the result with its fixes and steps, the cool-down, Help. **Not done:** the connector summary says counts only, so it needs no wording; nothing drives the endpoint from a script (by design a token cannot); a real click on vertex after deploy is the last check.

**Phases (after the amendment and the answers above).** (1) amendment text applied; (2) `deep` mode, list and refusal tests; (3) endpoint with grant, limits and cool-down; (4) findings, carry-over and report line; (5) UI button and copy.

## 13.A and 13.B.2 as built (moved from the plan on 2026-10-29)

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

**13.B.2 as built (2026-10-28, v0.15.0).** Eight more checks: Telnet open and an open Docker API (port rules in `portchecks.go`, no script, work in every scan mode; a name taken from the port table only is worded "normally Telnet" and ranked lower); weak certificate key and SHA-1/MD5 signature (read from the existing `ssl-cert` output); and four scripts added to the reviewed list, each checked against nmap 7.99: `ssh-hostkey` (weak DSA or short RSA host key), `memcached-info`, `mongodb-info` and `mqtt-subscribe`. All four are in nmap's `safe` category and talk only to the scanned host; NSE `dependencies` only order scripts and do not select `mongodb-brute` (a one-script run loaded exactly one). **Owner decision 2026-10-28:** `mqtt-subscribe`'s anonymous CONNECT counts as the "well-known anonymous login" II.3 allows; the evaluators read only whether it answered and never copy a topic or payload. **Rejected after checking the categories:** `ssl-enum-ciphers` (`discovery, intrusive`) and `ssh-auth-methods` (`auth, intrusive`, tries a random user), `dns-recursion` and `upnp-info` (UDP only, never run by `-sT --unprivileged`; `upnp-info` also follows a LOCATION URL to other hosts). **Owner decision 2026-10-28:** intrusive scripts are to be built later as a separate interactive "deep check" of one chosen device (not in a nightly audit), with its own Constitution amendment; weak-TLS-version detection and SSH password-login detection wait for it. Thorough scans now pass an explicit port list (`netaudit.ThoroughPorts`: nmap's 300 most common plus 2375, 11211, 27017), because nmap ignores `--top-ports` when `-p` is also given, so the extra ports could not be added to it; the quick list gained the same three. Self-signed certificates are not a check: nearly every router, NAS and media device has one, so it would only add noise.

**Report shape (same release).** The markdown report lists a first scan as one row per device and groups the same problem on several services into one row that names where, with each check's fix written once under the table (`writeChecks`, `writeInventory`). The findings API gained `first_scan` (a baseline saved by an older version reads as not the first) and `check_title`. The UI shows one card per kind of problem (`ProblemGroups.vue`), folds a device's ports unless something there needs a look, folds ports the scan could not identify into one line, and calls the first scan a baseline instead of marking every port new. The security-auditor profile now asks the model for a short assessment of what the code cannot say, and not to re-rank, restate or give a severity to anything the checks established.

Anything that sends more than ordinary connections, tries credentials, or could disturb a device is **intrusive** and is not specified here. It stays behind the existing Phase 7 rule (per-target operator grant, interactive only, never unattended) and needs, first: an owner decision, a Constitution review (§8), and admin API authentication (D1, §9; the authentication has since landed, the owner decision and Constitution review were done for the deep check as II.3 "Intrusive runs").

## End-of-life software finding (network-module plan, Phase 5)

`end_of_life` (`netaudit/eol.go`) is a pure check added by the reporter beside `known_vulns`, so it is diffed, accepted and kept like any finding and the auditor model never sees it. It reads the CPEs nmap printed for each open port (application CPEs only: `cpe:/a:vendor:product:version`), looks the vendor and product up in a short reviewed table (`endOfLifeSeries`: Apache httpd 2.0 and 2.2, PHP up to 8.1, OpenSSL 1.0.2 to 1.1.1, MySQL 5.5 to 5.7, PostgreSQL 9.6 to 13; dates from endoflife.date, read 2026-10-31) and reports a series whose support date has passed. A version continues a series with a dot, a dash or a letter, never a digit (PHP 7.40 is not 7.4). The evidence is one fixed sentence; the software is in the finding's own `service` and `version` fields. A distribution build is rated low instead of medium, and series that distributions routinely keep patching (OpenSSL 3.0) are left out. It makes no network request and needs no data download, so it is on whenever a scan identifies services. **Cost of the choice:** the table is data that ages; adding a series needs a date from the makers and a test.

