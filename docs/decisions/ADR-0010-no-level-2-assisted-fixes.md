---
id: ADR-0010
title: Assisted fixes are guided, not executed by the app
status: accepted
date: 2026-10-29
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0010: Assisted fixes are guided, not executed by the app

## Context
The plan (§13.C.2, §13.H) asked whether the app should run a fix for the operator on a host it can reach. Level 1 (a reviewed fix guide per check, copyable commands, an "I've done this" record, and the next scan deciding whether it worked) is built. Level 2 would add managed hosts, a stored SSH key and a guarded `ssh` program that runs a fixed command with a recorded undo. Admin sign-in, which Level 2 waited for, is compulsory since v0.22.0, so that blocker is gone.

## Decision
We will not build Level 2. The app tells the operator how to fix a finding and records that they did; it does not change another machine. Reopen only if the same fix is being repeated across many hosts.

## Consequences
Nothing new holds a secret that can log in to other machines, and no new Constitution class is needed. The operator keeps doing the change by hand, which costs time. Only three checks (`ssh_weak_algorithms`, `smb_signing`, `ftp_anonymous`) could ever have qualified; the rest stayed guided-only in the design.

## Alternatives considered
- **Level 2: run it over SSH on a managed host** — a stored key makes the app a credential holder; `ssh` sockets sit outside `NetworkTools` and a key login is a credential exchange that II.3 item 5 allows only as an anonymous login, so it needs a new amendment; the return is three checks that the guides already cover.
- **Automatic, unattended fixes** — out of scope (§13.C.3): a wrong fix on a router or NAS can cut the network off.

## Links
- Plan: [`network-audit.md`](../PLANS/ARCHIVE/cross-cutting/network-audit.md); as built: [`architecture/network-audit.md`](../architecture/network-audit.md).
