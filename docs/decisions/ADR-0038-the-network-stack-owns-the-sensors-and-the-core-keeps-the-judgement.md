---
id: ADR-0038
title: The Network stack owns the sensors and the core keeps the judgement
status: accepted
date: 2026-11-09
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0038: The Network stack owns the sensors and the core keeps the judgement

## Context

`core/netaudit` is 77 files and 13,703 lines, with 16 sub-packages, 100 files that use `models` types and 21 files in `app` and the handlers that import
it (`docs/PLANS/cross-cutting/network-stack-s6-plan.md`). Its one `migrationDebt` entry was retired on 2026-11-09, so the rule that the core never imports a
stack already holds. What remained was a choice about the end state. The owner chose option B of the S6 plan on 2026-11-09.

The root's inventory, baseline, ack, fix, history and alert types are read by the core's report, alerts, bell, handlers and automation. A stack cannot
import `models`. ADR-0036 already lets a stack offer findings by id, worded and placed by the core, without any of those types crossing a port.

## Decision

We will move the **sensors** into the Network stack and keep the **judgement** in the core.

- **Sensors (move, one pull request each):** the programs (`nmap`, `portprobe`, `scantool`, `wifilook`), the ingest-fed watchers (`routerlog`,
  `phonehome`, `knownbad`), `upnp`, `webprobe`, `udpchecks`, `nearby`, `decoy`. The DNS check has moved already.
- **Judgement (stays):** the report, baseline and its diff, history, acknowledgements, fixes, alert delivery, the finding wording tables, and the
  `models` types. A sensor reports typed facts; the core words, rates, keys and places them (ADR-0036).
- **No `models` type crosses a port** and no sensor gets a way to word a finding.

### The three rules every sensor move must meet

1. **Findings stay byte-identical.** Acknowledgements, fixes and the baseline diff are keyed on `Finding.Key()`. A finding that comes back through the
   stack has the same `Device`, `Port`, `Proto`, `Service`, `Check`, `Severity` and `Evidence` as today's, so on the first run after deploy nothing
   reads as resolved and no acknowledgement is lost. A parity test written **before** the move feeds the same answers through both paths and compares
   the findings field by field. The nightly UDP findings are pinned now by `TestUDPChecks_TheFindingsOfAnAnswerAreByteIdentical`.
2. **A stack never probes wider than the core does.** Today a probe first checks the operator's declared scan scope, then the guarded dial
   (`newUDPChecks`, `app/netaudit.go`). The stack's `Guard` port is the guarded dial only (`stackport.Guard` over `NetworkTools.DialContext`); it does **not** apply
   the declared scan scope, so a stack that dialled through it could reach LAN addresses the core refuses. Before any sensor that sends to devices
   moves, the SDK gets a **scoped dial**: a port whose dial first applies the declared scan scope and answers a refusal as `CodeDenied`, built by the core
   from the same two checks. It passes the second-stack test: any stack that probes local devices needs "what the operator declared may be scanned".
3. **Sensors that need input get their own method.** `Stack.Findings` (ADR-0036) reads only the log a check already keeps and sends nothing. A
   sensor that sends datagrams during the nightly report is a different thing, so it is a separate, optional method taking plain data: the scanned live
   addresses the core already filters, a bound on how many, and the module id; it returns typed answers plus the pairs that were asked and stayed
   silent. The core keeps the judgement around it: the carry rule (silence is not a fix), the replacement of a helper-kept finding by a newer nightly
   answer, and the switch (it stays typed in the core's settings until the settings flip). The exact shape is fixed when the first sensor is built and
   must be checked against `webprobe` and `upnp`, which need the same input-in, typed-answers-out form, so it is designed once for all three.
   `StackFinding` has no port field today; the first sensor adds a typed one (or encodes the port in the id) and the core maps it back to today's `Finding`.

### Built 2026-11-09 (v0.64.0): the contract and the first sensor, `udpchecks`

- **SDK:** `PortScopedGuard` (`Host.ScopedGuard`), `Manifest.Looks`, `LookRequest`, `LookResult`, `Pair`, `MaxLookAddrs` and the optional `Looker` interface; the manifest check requires the scoped dial of a stack that lists a sensor.
- **Core:** `stackport.ScopedGuard` (the declared scan scope, checked afresh for each datagram, then the guarded dial; a refusal reads as `CodeDenied`); `stackHost.Look` (module and bound checked before the stack is called, three-minute limit); `stackUDPAsker`; `netaudit.UDPChecks` now takes typed pairs and, when the sensor cannot look at all, keeps every earlier finding.
- **Stack:** `stacks/network/udpchecks` moved with its tests unchanged; `Stack.Look` answers for `udp_probes`.
- **Gates that exist:** the byte-identical findings test, the same through the whole stack path, an outside-scope address never dialled (shown to fail when the scope check is removed), the plain `Guard` never used by the sensor, and the stack's `Look` refusing another module or too many addresses.
- **Shape check for `webprobe` and `upnp` (not built):** both fit "addresses in, typed answers out" for the probe itself, but `webprobe` returns richer facts (a certificate's fields, a page's headers) than `Pair`; before it moves, `LookResult` gets a second typed list or the module gets its own result type, decided from `webfindings.go` and `tlsfindings.go`.

## Consequences

- **Order.** The scoped dial and the contract of rule 3, with the parity test, come first with `udpchecks` as their first user (built, see above); then (no dependency on the service, 121 lines plus
  the SNMP parser), then `webprobe` and `upnp`, then the ingest port (S4) for the watchers, then the programs (S5). The lookup port of ADR-0037 is added
  with the matching code that first needs it, optional in the manifest.
- **What improves:** the Network-specific, port-hungry code leaves the core one reviewable pull request at a time, the `models` types stay put, and an
  extracted stack would carry no `models` dependency.
- **What it costs:** two new SDK surfaces (the scoped dial, the input-bearing method) before the first sensor can move, and a wording table in the core for
  every sensor (the core keeps wording on purpose: ADR-0025).
- **What must stay true:** the core never imports a stack except in `internal/app/stacks.go`; a stack imports only the SDK; the parity tests pass before and after
  each move; a settings key is not moved until the flip at the end, and the round-trip test on a real `settings.yml` guards it.
- **Security:** no new egress; a probe is refused outside the declared scope exactly as now (rule 2); sensor inputs are untrusted text and are bounded and
  validated at the port; stacks are in-process, so these are architecture-test guarantees, not isolation.

## Alternatives considered

- **A. Move the whole audit engine** — about 13,700 lines, needs the `models` types moved or ported, 4 to 6 ports and the settings flip; the reward is
  architectural only.
- **C. Stop at the empty debt list** — cheapest, and still available after any step here; the owner chose B.
- **Dial through the stack's existing `Guard` and check the scope in the stack** — leaves the declared scope as a convention in stack code; rejected, the scope is data
  the core checks.

## Links

- `docs/PLANS/cross-cutting/network-stack-s6-plan.md` (the numbers and the order).
- ADR-0036 (findings by id), ADR-0037 (the feeds stay in the core), ADR-0035 (ports are plain data), ADR-0025 (finding text from typed fields only).
- `backend/internal/core/netaudit/udpfindings.go`, `backend/internal/app/netaudit.go` (`newUDPChecks`), `backend/internal/core/stackport/host.go` (`Guard`).
