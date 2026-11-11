---
id: ADR-0048
title: The router watch keeps a history of companies and countries, and the audit's own code reads the router once a night
status: proposed
date: 2026-11-12
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0048: The router watch keeps a history of companies and countries, and the audit's own code reads the router once a night

## Context

ADR-0047 reads the router's connection table on the operator's press and keeps the last read in memory only. A snapshot cannot say what is new: a device
that starts talking to a new company at 3 a.m. is gone by the time anyone presses. The Traffic page (plan `network-traffic-by-device-redesign`) wanted a
"New today" mark, and the audit wanted findings for a device that reaches a new company or country. ADR-0047 had rejected "reading on a schedule" because a
scheduled sign-in is a standing credential use, and kept no history. On 2026-11-12 the owner approved both (stored history, and a nightly read inside the
audit) on one condition: no agent may be able to branch the security layer. Other code-driven watches (phone-home, login watch, router log) already work
this way: a switch, a bounded store, and the report's own code turning facts into findings.

## Decision

We will add one module, **Router connections** (a step-up switch, off by default, ready only when the router's setup is finished), and make two changes
under it, both amending Constitution I.15.

1. **A bounded history on disk** (`core/netaudit/routerhistory`, `router-history.json`): per device, by hardware address else its address, the holder's AS
   number and name and the country, with when each was first and last seen. No address, port, state or connection. At most 128 devices, 64 companies and
   32 countries each; the least recently seen goes first. A destination is **new** when it was first seen more than a day after the device's first read and
   within the last day; the first day only learns. The mark is stateless (derived from the first-seen times), so a restart changes nothing.
2. **A nightly read by the report's own code.** `RunReporter.addRouterConn` asks `netaudit.RouterConnSource.Refresh` for one read while the module is on,
   then turns the history's signals into findings `router_new_company` and `router_new_country` (low severity, one of each per device, evidence in fixed words
   and the holders' names from the public tables). A failed read is said in the report and shown as stopped by the monitor; a night with nothing to read
   (setup unfinished, router just read) is neither.

**The agent cannot reach any of it.** The read is not a tool: it is a call in the reporter, wired in `internal/app`. No tool, prompt, task text or setting an
agent writes names the router, the user or the key, starts or changes a read, switches the module, or reads the history. The test
`TestOnlyTheServiceCodeReachesTheRouter` (`internal/archtest`, Constitution IV.6) fails the build if a package other than `internal/app`, the netaudit core, its
`routerconn` and `routerhistory`, `platform/routerssh` and the operator's handlers imports them.

## Consequences

- "New today" on the Traffic page, new-company and new-country findings, and a monitor entry for the nightly read.
- The program signs in to the router every audit night while the module is on: a standing credential use, bounded to one fixed read-only command as before.
  The module's switch is step-up and off by default, and the Modules tab says what it never does.
- A longer record of what each device uses is now on disk. It holds names and dates only, is removed by a factory reset, and should be switched off on a
  computer others can read.
- A company seen only briefly may be missed (the table is a snapshot, once a night), and a CDN's servers sit under many names: findings are hints.
- Matching destinations against the abuse.ch list by **address** is not possible: its host file maps names to loopback, so it holds no addresses.
  The earlier proposal for that is dropped, not deferred.

## Alternatives considered

- **Keep history on every press-read even with the module off.** Stores data the operator did not ask for; a switch is the repo's way to say yes.
- **Store addresses and ports.** Larger, more sensitive, and nothing here needs them; companies and countries answer "what is new".
- **A background scheduler in the program.** A second place that holds the credential and runs unattended; the audit run already exists and is the one
  place the other watches report from.
- **A tool the audit agent calls.** Exactly the path the owner ruled out: a model would hold a way to the router.

## Links

- Plan: `docs/PLANS/cross-cutting/network-traffic-by-device-redesign.md` (Phase 2); Constitution I.15; ADR-0047 (which this amends).
- Code: `backend/internal/core/netaudit/routerhistory/`, `backend/internal/core/netaudit/routerconnfindings.go`, `backend/internal/app/netaudit_routerconn.go`,
  `backend/internal/archtest/boundary_test.go`.
