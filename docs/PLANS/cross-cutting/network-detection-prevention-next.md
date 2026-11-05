---
status: partial
date: 2026-11-04
last_reviewed: 2026-11-04
related_specs: [SPEC-003]
constitution_references: [I.5, I.6, I.8, II.2, II.3, V.1]
evidence: A review of the Network module at v0.45.0 (origin/main 71620cc4, 2026-11-04) on branch `network-audit-review-fixes`, which fixed eight defects (ledger 2026-11-04). The ideas below were checked against ADR-0023, the open network plans and the code; none is built, measured or run on vertex. Each "verify first" item is a fact this plan assumes and has not checked.
---

# Network: the next detection, monitoring and prevention work

**Status:** partial (2026-11-04). The owner asked for every phase to be built on branch `network-audit-review-fixes` (v0.46.0) and
for the open decisions to be taken on the conservative side; §2a says what was decided. All seven phases are built and tested; none
has run on vertex yet, and the "verify first" facts below are still open unless §2a says otherwise. No Constitution amendment was
needed.
**Related:** [`network-security-roadmap.md`](network-security-roadmap.md) (Phases 5 and 6 still open there; nothing here
repeats them), [ADR-0023](../../decisions/ADR-0023-network-ideas-rejected-or-already-built.md) (read before adding an idea),
[`network-attempts-integrity-timeline.md`](network-attempts-integrity-timeline.md) (the Timeline and host integrity this
builds on).
**Origin:** the owner asked (2026-11-04) for a review of the network audit and security work, fixes on a fresh branch, and
"new features that will bring high value to the network security, detection, monitoring and prevention" in a new plan.

## 1. Goal and non-goals

**Goal.** Turn what the module already collects into fewer, sharper signals, close the blind spots an unprivileged service
can still close on Linux and macOS, and help the operator *prevent* the problems the audit keeps finding.

**Non-goals (unchanged from the roadmap and ADR-0023):** executing fixes (ADR-0010), packet capture, a DNS server, threat
lists without a source of destinations, `adm`-group log reading, Wi-Fi scanning, UPnP traffic counters, UDP/OS scans.

## 2. Phases, highest value first

### P1. Watch the watchers: say when a monitor that is switched on has gone quiet
**Why.** Every collector here already follows "silence is never absence" for devices, but not for itself: if the nightly
audit stops running, the router log stops arriving, the decoys cannot bind after an update, or the presence check fails
for a day, the page simply shows older data. That is also what an intruder who silences monitoring would cause.
**What.** One check over the module's own status, run by a small loop of its own in `netauditLoops` (every few minutes, no
probe), because the presence check is one of the parts it watches and cannot be the trigger: a switched-on part
whose last success is older than a fixed limit raises `monitor_stalled` with fixed words naming the part. Sources already
exist: `Presence.Status` (`LastCheck`, `Problem`), `routerlog.Status.LastHeard`, `DNSCollector.Status().LastRun`,
`decoy.Status.Problem`, the audit baseline's `ScannedAt`, `nearby` `LastLook`. Alert through `AlertLog` as a new kind,
once per stall, cleared by the next success.
**Needs.** Nothing new. **Verify first:** a quiet router log is normal at night on some firmware; measure `LastHeard` gaps
on vertex for a week before choosing its limit (24 h is a guess).
*Accept:* a stopped collector in a test raises one alert and one finding; a recovery clears both; a part that is
switched off never alerts.

### P2. Host privilege and persistence indicators the watch does not see yet
**Why.** The host watch covers listeners, startup files, sshd, sudoers and packaged program files. Classic persistence
and privilege-escalation marks are outside it, and all of them are files any account can read.
**What.** Typed findings in the existing "This computer" group, baseline-and-accept like `new_listener`:
- `/etc/ld.so.preload` exists or changed (a rootkit's favourite file; normally absent);
- `/etc/passwd`: a second account with UID 0, or a new account with a login shell;
- `/etc/group`: an account added to `sudo`, `adm`, `docker` or `lxd` (each is root-equivalent or close to it);
- `/proc/modules`: a kernel module loaded that was not in the baseline;
- SUID/SGID files outside what `dpkg` lists, in a bounded walk of `/usr/local`, `/opt` and `/srv` under a budget and the
  run's context (the integrity pass already knows the packaged set).
Fixed wording and names only (an account or module name is a plain token, vetted like the integrity paths); never file
contents.
**Needs.** Nothing new on Linux. macOS has none of these files: the finding family is Linux-only and the Mac says
"not checked", never "fine". **Verify first** on vertex: `/proc/modules` readable by `llm-proxy`; how many SUID files the
walk finds and how long it takes; whether Docker or snap add groups or modules on their own (noise).
*Accept:* fixtures from vertex's real files; a first look reports nothing; one added UID-0 line yields one finding.

### P3. One device, one story: correlate what the module already knows
**Why.** Today a device can be new (inventory), touch a decoy (decoys), look like a camera with an admin port open
(`class_mismatch`) and open a new port (port record), and each shows as a separate low or medium line. Together, within a
day, they are the most likely sign of a compromised or hostile device on the network.
**What.** A pure function over the stored records (no probe): a device with a decoy touch plus any of {first seen in the
last 24 h, `class_mismatch`, a port that opened since the last audit, `mac_changed`} raises `device_incident` (high), whose
fixed evidence lists which signals joined. The Timeline groups those rows under the device. The decoy alert gains one fixed
phrase when the source is such a device.
**Needs.** Nothing new; ADR-0025's rule that the Timeline reads existing records holds. **Owner decision:** which
combinations are high enough to alert at once rather than wait for the nightly report.
*Accept:* table-driven tests of the combinations; a decoy touch alone stays what it is today.

### P4. Prevention: a guided firewall plan for this computer
**Why.** vertex reports `host_firewall_off` (ufw `ENABLED=no`, ledger 2026-11-02) and the guide today says only to turn it
on. Turning a firewall on blind can cut off the services the owner relies on, which is why it stays off.
**What.** A fix guide built from facts the module holds: the host's real listeners (`hostwatch.Proc.Listeners`) and the
declared scan networks. It shows a ufw rule set (default deny incoming; allow each listening port in use from the declared
networks only; keep SSH first) as copyable commands with one line of why per rule, and the command to undo it. Nothing is
run (ADR-0010). The guide refuses to suggest a rule for a port it cannot attribute and says so.
**Needs.** Nothing new. **Verify first:** that ufw is the firewall on the owner's Ubuntu (it is per `ufw.conf`), and that
the decoy ports and the router-log port 5514 are listed as "allow from the router / the LAN" when those switches are on.
*Accept:* a test from vertex's real listener capture produces the expected rules, in order, with SSH first.

### P5. Sign-in attempts on the control plane (*owner decision*, security review of I.5's wording)
**Why.** The admin API can run code as the service user (I.5). The sign-in service already locks out after wrong
passphrases (`auth.LockedError`), but nobody is told.
**What.** An alert (new kind) when a lockout happens, and when a sign-in succeeds from an address that has never signed in
before; fixed words with the request's remote address as the kernel reports it (the same rule as the decoy alert). At most
one alert per five minutes.
**Needs.** A change in the auth subsystem, so the owner decides; the address in a message is the same class of data I.6
already allows for decoys, but I.5 does not say so and should, after review.
*Accept:* a test locks an account and sees one alert; a correct passphrase from a known address sends nothing.

### P6. A light follow-up scan of a device seen for the first time (*owner decision*)
**Why.** A new device is reported in minutes, but what it offers (Telnet, RDP, an open camera stream) is known only at the
next nightly audit.
**What.** When the presence check marks a device new, run one ordinary default scan of that one address directly through
`Service.Scan` (one scan at a time, the declared scope, the checks list; never the deep check) and derive its findings in
code, with no model: the presence check is a model-free loop (ADR-0014), and the "Rescan this host" button is not usable
here because it is an audit automation run, which wakes the auditor model (ledger, §13.A). At most one scan per device per
day and a few per hour; its findings join the device's alert as counts and wait for the nightly report to be compared.
**Needs.** No amendment (an ordinary scan of an in-scope address, unattended like the nightly audit; intrusive runs stay
operator-only per II.3). **Verify first:** how the result is kept until the nightly audit without a second baseline. **Owner decision:** it adds scan traffic during the day and a phone that rotates its address
looks new each time (ledger 2026-10-30), so it needs the private-address rule applied first.
*Accept:* a new device produces one queued scan; a repeat within a day does not; a scan already running defers it.

### P7. Small honesty and policy items
- **Erasing evidence is a step-up action (*owner decision*).** `DELETE /admin/api/netaudit/decoys/touches` clears the record
  of who probed the decoys with an ordinary session, while deleting the vulnerability data is step-up because it "silences".
  Clearing intrusion evidence is the same kind of silencing.
- **The Internet view says it looks at IPv4 only.** The outside lookup asks about the router's IPv4 address; a device with a
  global IPv6 address is not covered. Say so on the card, and count such devices if the inventory keeps global addresses
  (**verify first:** whether `scoped6` keeps them).

## 2a. As built (2026-11-04, v0.46.0) and the decisions taken

The architecture doc holds the design (`docs/architecture/network-audit.md` → "Detection and prevention additions"); SPEC-003 2.31 the
behaviour. Decisions where the plan left a choice, all on the conservative side:

- **P1:** the presence part reads `PresenceStatus.LastLook` (a sweep and refresh that worked), not `LastCheck`, which an undelivered
  alert holds back. The nightly audit is watched through the daily history (an audited day in the last 30 means "in use"; stalled after 48 h),
  because the baseline is per workspace and automation. Nearby is not watched: it looks only when the audit asks. **Data collected
  (vertex API, 2026-11-04 22:26):** the router log was heard 3 minutes before the read and the day already held 1,363 refused attempts, so
  the 24 h quiet limit is very loose for this router; presence every 10 min, DNS every 60 min, all three decoys listening.
- **P2:** a loader preload and a second UID-0 account are reported from the first look (a normal system has neither); the other four marks
  compare with the first look. Kernel modules are `low`: plugging in hardware loads modules. Linux only; macOS notes "not checked".
  A list that cannot be read keeps its last finding (`HostPrivilege.Reported`); a closed folder below a set-id root counts as unreadable
  and does not stop the list from being learned.
- **P3:** the incident is a finding in the nightly report and a fixed sentence in the decoy alert; no separate alert fires at once. It
  is looser than "within a day": the touch is any still in the report (30 days) and `new_device` means unapproved and active now; the
  evidence says so. Tightening it to 24 h needs the touch's time in the reporter.
- **P4:** a listener the audit cannot name gets no rule and is listed (the plan's "refuses and says so"); no declared network means no
  allow rule at all.
- **P5:** lockout and new-address alerts carry **no address**, so I.5 needed no new wording; the Security page lists the addresses.
  An existing record is seeded from its sessions and sign-in events, so the first deploy does not call the owner's other devices new.
- **P6:** off by default, step-up to switch on, private hardware addresses skipped, at most once a day per device and three an hour;
  the result is its own alert kind (`follow_up`), so the Timeline does not fold it into the new-device row; it is told as counts and **not stored** (the nightly audit compares). Uses `ModeServices`, never the deep check. Scans run
  one at a time, so a nightly audit that starts during a follow-up scan gets `ErrScanBusy` from the scan tool, as it can during a
  presence sweep; a follow-up of one device is longer than a sweep. Not measured; the switch's details say so.
- **P7:** clearing the decoy touches is step-up; the page's request already asks for the passphrase and resends. The Internet view's
  details say it covers the IPv4 address only; counting devices with a global IPv6 address was not built (see Remaining Work).

## 3. Follow-ups found in the 2026-11-04 review and not fixed

| Item | Why it was left | Where |
|---|---|---|
| `Exposure` keeps "what the last look did" for `Carry` on the shared instance (mutex-guarded, but two audits reporting at once could mix their states) | the same refactor as `ServiceResult`; rarer, and it touches the router check's contract | `core/netaudit/exposure.go` `Begin`/`Carry` |
| A failed write at the end of a day leaves those counts to be written under the next day | needs a per-day pending table, more than a fix; the window is one write per day | `core/netaudit/blocks.go` `Add` |
| The router log's read loop retries at once on any read error that is not "closed" | no such error was seen on Linux for an unconnected UDP socket; a pause is cheap if one ever is | `routerlog/manager.go` `serve` |
| The decoys' pending alert is sent inside `Reconcile`, so a slow connector delays a switch change (Poke) by that one send | bounded by the connector's own timeout; moving the send to its own step is a small change if it is ever noticed | `decoy/decoy.go` `Reconcile` |
| `cachedGateway` reads the neighbour table under `context.Background()` | a synchronous `/proc` read with no network; the signature change touches three callers | `app/netaudit_alerts.go` |
| Not read closely in the review | the review prioritised loops, alert paths and lifecycle | `vulnfeed`, `ubuntuoval`, `nmap/argv.go`, `scantool`, `fixguide.go`, `upnp`, `dnswire`, `names`, most Vue components |

## 4. Strain and safety

P1 to P4 add no socket, no probe and no new privilege; they read records and files the service already reads or can read.
P5 adds an alert in the control plane. P6 adds ordinary scans inside the declared scope, within the existing one-at-a-time
rule. Every finding uses fixed wording; names that appear (accounts, modules) are vetted plain tokens, and nothing a device,
log or file wrote reaches a message or a model verbatim.

## Remaining Work

- **Next (owner):** deploy v0.46.0 on vertex and look at: the first `monitor` alert after a restart (none is expected), the "This
  computer" group after the first look (no privilege finding is expected unless vertex really has a preload or a second root), the firewall
  guide on the `host_firewall_off` finding (check the commands against `sudo ss -lntup`), one sign-in from a new address (one `sign_in`
  alert), and, if wanted, the scan-on-arrival switch.
- **Verify on vertex:** `/proc/modules` readable by `llm-proxy`; the set-id walk's size and time over `/usr/local`, `/opt`, `/srv` (model
  folders under `/opt` count towards the 200,000-entry limit); whether Docker or snap add group members or modules on their own (noise);
  whether `Proc.Listeners` leaves out this program's own sockets as the firewall plan assumes (it should: matched by inode).
- **Not built:** counting devices with a global IPv6 address on the Internet view (needs to know whether `scoped6` keeps them); a
  `monitor` status line per part on the overview (today the alert and the finding say it).
- **Follow-ups of the review** in §3 are still open.
