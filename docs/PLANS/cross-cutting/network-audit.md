# Network audit — guarded programs, an nmap scan tool, and a nightly auditor agent

**Status:** partial (2026-10-24, revised 2026-10-29). D1–D13 decided by the owner (§2); open facts in §11. Everything the plan
set out to build is merged and runs on vertex (PRs #137–#162, up to v0.23.0): the scan tool, checks, known-vulnerability
matching with the file import, guided fixes, expectations per device and the interactive deep check. What is left is in
Remaining Work at the end; the design as built moved to [`architecture/network-audit.md`](../../architecture/network-audit.md) on 2026-10-29.
**Related Specs:** SPEC-006 (Guardrails §II.6–II.7), SPEC-007 (Automation Dispatcher), SPEC-012 (Prompt Profiles),
SPEC-001 (Agent Loop), SPEC-003 (UI), SPEC-009 (Communication).
**Constitution refs:** I.1–I.3 (network guardrails), II.3 (sandbox isolation, env allow-list), II.10 (guardrail
decision flow), II.13 (prompt centralisation), III.3 (workspace jail), IV.4 (no dead code), V.1 (amend after a
security review), V.3 (no half-finished work).
**Depends on / defers to:** [`admin-api-authentication.md`](admin-api-authentication.md) — **no longer parked**: sign-in
has been compulsory since v0.22.0 (§9). **Builds on:** [`agent-os-sandboxing.md`](../ARCHIVE/cross-cutting/agent-os-sandboxing.md), [`sandbox-and-egress-residuals.md`](sandbox-and-egress-residuals.md),
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

## 3. Where things stand (at the start)

Moved on 2026-10-29 to [`audits/2026-10-29-network-audit-phase-log.md`](../../audits/2026-10-29-network-audit-phase-log.md) with §10 to §12 (risks, open points, review log).

## 4. to 6. Architecture, frontend, security design

Moved on 2026-10-29 to [`architecture/network-audit.md`](../../architecture/network-audit.md) with the section numbers kept (§4.0 to §4.10, §5, §6).

## 7. Phases

All phases are merged. The phase table and the Phase 0 findings moved on 2026-10-29 to [`audits/2026-10-29-network-audit-phase-log.md`](../../audits/2026-10-29-network-audit-phase-log.md).

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

## 9. Admin API authentication — was parked; sign-in is compulsory since v0.22.0

**Update 2026-10-29:** the text below is the position at the time. Admin sign-in landed (v0.19 to v0.22) and the operator-only parts of the audit are now controls, not conventions: the deep check and the vulnerability-data import are step-up routes (a token cannot call them). What was said then:

The owner accepts the current exposure (single operator) and wants it handled in the existing auth plan. That plan now
has a "features that depend on this plan" section: operator-only acknowledgements; the scope, mode and unconfined
settings; approval of intrusive modes; scripts and intrusive modes against the host's own addresses. Until it lands
these are conventions, not controls, and the Security UI copy says so. Nothing here edits routes, middleware or
binding. *(Note for the owner: the auth plan's own findings say the API can disable the host sandbox and wipe the data
root, and its Phase 1 Origin check is "a day of work"; that item may deserve its own slot even if login waits.)*

## 10. to 12. Risks, open points, review log

Moved on 2026-10-29 to the same audit document. The open facts that mattered are settled in Remaining Work below.

## 13. What is left (2026-10-26)

**Shipped:** scan scope and readiness; the guarded nmap runner and `network_scan` (discover, ports, services, checks); findings with
new / changed / resolved, accept and Undo; the code-written report and connector summary; the Security auditor and the Network Audit
playbook; the settings, report and form UI (Simple and Advanced); per-audit scan settings applied by the server (v0.12.0); plain-language
failures. Details are in the status notes above.

### 13.A and 13.B.2 as built

Moved on 2026-10-29 to [`architecture/network-audit.md`](../../architecture/network-audit.md) (section "13.A and 13.B.2 as built").

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

  **Result (2026-10-29, vertex 192.168.50.60, kernel 7.0.0-38-generic, nmap 7.94SVN; Ubuntu release not captured): steps 3 to 7 pass, with limits.** Confinement reads Landlock (file and network), `allow_unconfined` is false, and the nightly audit and a one-device rescan of 192.168.50.10 ran confined with service detection working. The tcpdump check found no connection by the scan to any host but the target (the one outside SYN was the system's own Ubuntu connectivity check, matched by its DNS lookup). Limits: one device; no script-based finding was produced, so it does not prove the scripts ran and printed; TCP SYN and UDP 53/5353/1900 only; nmap 7.94SVN here against 7.99 in the script review. Full detail in `docs/learnings/network-audit.md`. Still to do for the rule to be fully confirmed: a rescan of a device that triggers the web, TLS and database scripts.
- **Constitution text for guarded programs** (§8): approved by the owner 2026-10-27, security-reviewed against the code, and added to `CONSTITUTION.md` II.3 . The review found: the child's sockets are outside `NetworkTools` (`progexec.Run` always passes `NetworkOn: true`), so the text states the carve-out and its limits; `ftp-anon` is in nmap's `safe` category but does perform the well-known anonymous login, so the text allows exactly that one credential-bearing exchange; the `terminal.json` allow-list does not govern the guarded env, and the text says so.
- **Unverified, and now asserted by the law:** that none of the thirteen listed check scripts opens a connection to a host other than the target (redirects, referrals). The Constitution text makes this a rule for the list; confirm each script's source or behaviour in the Ubuntu hand check.
- **Admin API authentication:** was parked on 2026-10-27 (§9); it landed in v0.22.0, so the audit's operator-only parts are now controls (step-up routes).
- **Result of the hand check (2026-10-29, vertex):** the confinement and the no-other-host rule pass, with limits; see the runbook result above and `docs/learnings/network-audit.md`.

### 13.H, 13.G, 13.I, 13.J and Appendix A

The reviewed designs (assisted fixes, known-vulnerability matching, expectations per device, the deep check, and what packet capture would need) moved on 2026-10-29 to [`architecture/network-audit.md`](../../architecture/network-audit.md) with their numbers kept. Decisions taken since: Level 2 assisted fixes ("run it for me" over SSH) are **not to be built** (owner's recommendation accepted 2026-10-29: three candidate checks, Level 1 guides cover them, a stored SSH key and a new Constitution class are a high cost); the file import for the vulnerability data **was built** (2026-10-29).

### 13.E Suggested order
13.A (merged, #146) → pick the 13.B.1 data source → reviewed design for 13.B.1 → build 13.B.1 → pick the 13.C.2 targets → reviewed design for 13.C.2 → 13.B.2–4 → anything intrusive only after the 13.B conditions. Each of 13.B and 13.C needs its own reviewed design before code.

### 13.F Decisions of the owner (2026-10-27)
1. **Local vulnerability matching (13.B.1): yes.** Source: NVD for matching, CISA KEV to rank (design: §13.G).
2. **Assisted fixes (13.C.2): yes, as a design.** Targets: as scoped in 13.C.2 (hosts already configured for terminal or connector access; never embedded devices). Needs the guardrail flow; design written 2026-10-28 in §13.H (Level 1 guided fixes built; **Level 2 decided against on 2026-10-29**, see the note under 13.G to 13.J).
3. **Constitution text for guarded programs (13.D): approved.** Reviewed and added to `CONSTITUTION.md` II.3.
4. **Admin API authentication: no** on 2026-10-27 (§9). **Superseded:** it was built and is compulsory since v0.22.0.

## Remaining Work

**Built on 2026-10-29 (branch `netaudit-detection-plan`, uncommitted at the time of writing):**
- *Landlock hand check on vertex* (runbook in 13.D): passes with limits; result in 13.D and `docs/learnings/network-audit.md`.
- *File import for the vulnerability data* (13.B.1 / 13.G): `vulnfeed.Manager.Import`, `POST /admin/api/netaudit/vulndata/import?source=nvd&year=YYYY|kev` (raw body, capped at `vulnfeed.MaxFileBytes`, a step-up route), and "Import a file instead" in the data card (`VulnDataImport.vue`; Advanced always, Simple once a download stopped). Help text and tests included.
- *Busy state*: while a scan runs the report is a disabled `<fieldset>` with a banner ("A scan is running." or "Scanning <device>…", "Watch it live"); it goes busy at the click (`useFindings.starting`, released by the page's run state or after 20 s); clearing runs, the journal and deleting a run are off while it runs.
- *Report pills (Advanced)*: the five count tiles and the filter row, which repeated each other, became one row of pills (Problems, Devices, New, Changed, Resolved, Accepted, each with its count); the report opens on Problems when the checks found any, otherwise on Devices, and a part with nothing in it says so. `FindingsAdvanced.vue`.
- *Deep problems in the report*: a stored deep finding that no audit has carried into the report yet is listed with the other problems (Advanced and Simple), dated and read-only (it has no key to accept it by until the next audit records it); it is not listed once the report carries it, when its port is no longer seen open, or after 90 days (`deepProblemsOf`). The meta line says "N problems found by checks (M accepted)" so it agrees with the Problems pill, and accepted problems are quiet rows in Devices.
- *Assessment no longer claims a first scan*: root cause, the auditor prompt and the task text asked the model "what is new or changed" while the comparison is made by code and never given to the model, so it invented "first scan, no baseline". The profile (`prompts/profiles/security-auditor.md`) and the template now say the model is not given the comparison and must not say whether anything is new, changed or gone (`TestSecurityAuditorIsNotAskedForTheComparisonItDoesNotHave`). Caveat: an automation created before this keeps its own copy of the task file (templates are not refreshed), which still asks for it; its task file needs the same two lines changed by hand.
- *Known-flaws cards*: each place shows "29 known flaws · 1 exploited by attackers" and the "a match is not proof" caution is said once per card (`flawSummary`, `ProblemGroups.vue`); the full list stays one click away.
- *Audit page layout*: tabs Results / Live run / History / Setup on an audit automation (`AutomationDetails.vue`); "Last result" shows only the model's assessment (`assessmentOf`) because the scan data is the Results tab; a device header shows Rescan, More and Show ports, with expected ports and the deep check behind More.

**Decided on 2026-10-29:** Level 2 assisted fixes (§13.H) are **not built** and are not planned: only three checks qualify, Level 1 guides already cover them, and a stored SSH key plus a new Constitution class cost more than they return. Reopen only if the same fix is being repeated across many hosts.

**Open:**
1. **The deep check's live click: done and confirmed (2026-10-29).** Run on 192.168.50.60 at 18:18 UTC (`ssh_password_login`). A one-device rescan at 18:43 UTC carried it into the report as "The SSH server accepts password login (from the deep check of 2026-10-29)", medium, with the other seven devices kept (`devices_up` 8).
2. **Second Landlock capture** on a device that triggers the web, TLS and database scripts (for example 192.168.50.241, nginx), with the same two `tcpdump` filters, to complete the rule that no check script contacts another host. Also note vertex has nmap 7.94SVN while the script list was reviewed on 7.99.
3. **TLS hygiene (13.B.4): no check proposed.** A real run of the product's flags with `ssl-cert` against vertex.local (192.168.50.60, ports 22, 4001, 8081) found no TLS service (only `fingerprint-strings` produced output), so there is no real `ssl-cert` output to judge from, and fixtures are not evidence. Propose at most three once a TLS device is named and a real run has been captured (`nmap/testdata/deep_*.xml` is the pattern; addresses replaced).
4. **Archive decision:** the plan is "partial" only because of items 1 to 3. When they are settled, `git mv` it to `PLANS/ARCHIVE/cross-cutting/`, add its row to `ARCHIVE/README.md`, remove its row from `PLANS/README.md`, and keep what carries on in [`network-detection.md`](network-detection.md).
5. **Small UI follow-ups:** the audit tabs are a `SegmentedControl` (28 px high) and are below the 44 px touch size on a phone; a device with many ports still lists them all when unfolded.
