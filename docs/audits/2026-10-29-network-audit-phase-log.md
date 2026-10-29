---
subsystem: network-audit
tags: [plan-history, phases, evidence]
result: reference
status: complete
date: 2026-10-29
---

# Network audit — phase table, Phase 0 findings and phase-by-phase log

Moved unchanged on 2026-10-29 from the plan [`network-audit.md`](../PLANS/cross-cutting/network-audit.md) when it was
harvested: what was done and found while building, as raw evidence, not instructions. The design as built is in
[`architecture/network-audit.md`](../architecture/network-audit.md); the distilled lessons are in
[`learnings/network-audit.md`](../learnings/network-audit.md).

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


### Phase-by-phase log (the plan's Remaining Work, up to 2026-10-28)

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

**F-0 wireframes published for sign-off** (2026-10-25): (a hosted wireframe page, link not kept in this file) — six artboards: Settings →
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

## Moved from the plan on 2026-10-29: where things stood at the start (§3), risks, open points and the review log (§10 to §12)

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
