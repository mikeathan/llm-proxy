---
id: ADR-0041
title: testssl.sh and osquery are guarded looks whose results are typed, and each fact has one home
status: proposed
date: 2026-11-09
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0041: testssl.sh and osquery are guarded looks whose results are typed, and each fact has one home

## Context

The owner asked (2026-11-09) to add a penetration, detection or monitoring tool that works on macOS and Linux, and picked two: `testssl.sh` (what
encryption a device's service offers) and osquery (this computer's own state). Reading the code first (`platform/progexec`, `platform/isolation`,
`core/netaudit/hostwatch.go`, `platform/hostwatch/`, `core/netaudit/nmap/`, ADR-0012, ADR-0036, ADR-0039) showed four constraints:

1. **The host watch already exists, on Linux only.** `HostSource` (`core/netaudit/hostwatch.go:248`) lists open ports from `/proc/net`, hashes startup
   files (systemd, cron, sudoers, authorized_keys) and compares packages and accounts. On macOS `Listeners()` reads the kernel's socket tables with sysctl (added 2026-11-10); on any other OS it returns `ErrUnavailable`, which `Look` reports as "ports not checked". A second set of "host findings" from osquery would show the same fact twice.
2. **The host watch deliberately does not name the program behind a port** (`platform/hostwatch/listeners.go`: "its name is text any account can set").
3. **A program that opens its own sockets stays in the core** (ADR-0039, programs rule (b)): `testssl.sh` is such a program, so the scan scope is checked
   in the core before it runs, as for nmap and `portprobe`.
4. **osquery needs more than the service has.** On Linux, a socket's owning process is readable only for the service's own account; on macOS many tables
   need root or Full Disk Access; and on macOS a guarded program does not start unless the operator allows running unconfined (`progexec.ErrUnconfined`).
   Not yet measured: neither program is installed on the development Mac or checked on vertex.

## Decision

We will:

1. **Give each fact one home.** A finding is where severity lives, in the existing Findings tab. A detail view shows only what a finding cannot hold,
   and links to the finding. A fact is never shown on a second page in a second form.
2. **Run two lanes from one collector.** A *manual* look (a step-up button) shows the rich result in its one home. An *audit* look (scheduled, a module that
   is off by default) offers findings by id (ADR-0036) and typed counts to the agent. Both lanes run the same collector and parser.
3. **Keep program-written text away from findings, alerts, prompts and model summaries.** Text a program prints (process names and paths, certificate
   subjects) may be shown to the person as text on the view that asked for it, and never goes further. Results that are a fixed vocabulary (a protocol
   version, a cipher category, a yes or no) are typed enums, so they may feed findings and the agent.
4. **`testssl.sh` is a guarded program in the core, and a closer look inside the existing deep check.** Its arguments are built in code from a fixed list
   (protocols and standard cipher categories). It is **not** a nightly module: the deep check already makes `tls_weak_protocol` and `tls_weak_cipher`
   with nmap, and a look of hundreds of connections is the class of probe ADR-0012 keeps for the operator's own request (amended 2026-11-09 after reading
   `deep.go`). Where it judges a port it replaces nmap's finding for that port under the same check id, so the key is the same. Targets are authorised
   against the scan scope before the run. It makes no certificate finding: the certificate checks exist in `stacks/network/webprobe/tls.go`.
5. **osquery is a second reader behind `HostSource`, not a second watch.** It supplies what the Linux readers supply, on macOS too, through a fixed
   allow-list of queries (no query text from a model or a request). It runs `osqueryi --json` one-shot with no network. Its first use is the listening-ports
   reader that macOS lacks; process attribution is a view-only extra. Whether root-only tables need a new helper action is decided after the
   measurement (plan phase O0), not before.

## Consequences

- macOS gets the same host findings as Linux without a parallel set, if the measurement shows osquery can read the tables unprivileged. If it cannot, the
  macOS look stays "not checked" (never "clean") and the helper action is a separate decision.
- No new screen: the deep check's result is where a person reads it; the host watch gains no new page.
- Two new external programs are optional installs (`brew install osquery testssl`, `apt install testssl.sh`); the app works without them and says so.
- The query allow-list and the `testssl.sh` argument list are code, pinned by tests, so a change to either is a reviewed change.

## Alternatives considered

- **osquery as its own findings and its own page** — duplicates the host watch on Linux and shows one fact twice.
- **Let the agent run any osquery query** — turns a read-only look into an open query surface fed by model output (Constitution I.8, I.11).
- **Built-in macOS readers (`netstat`, launchd plist folders) instead of osquery** — no install, but one more parser per OS and no process attribution.
  Kept as the fallback if the measurement shows osquery cannot read the ports unprivileged.
- **Suricata or Zeek** — heavy on macOS, and they see only this computer's link.

## Links

`docs/PLANS/cross-cutting/network-host-and-tls-looks.md`, ADR-0012, ADR-0036, ADR-0038, ADR-0039, ADR-0040.
