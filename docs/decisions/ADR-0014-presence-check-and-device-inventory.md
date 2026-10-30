---
id: ADR-0014
title: The presence check is a model-free background loop over one app-wide device inventory
status: accepted
date: 2026-10-29
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0014: The presence check is a model-free background loop over one app-wide device inventory

## Context
The nightly audit can find a stranger up to 24 hours late, and an unprivileged scan sees no hardware (MAC) addresses, so it cannot tell a new device from a known one or notice that the router's address changed (`PLANS/cross-cutting/network-detection.md`).

## Decision
We will keep one inventory of the devices seen on the network, in the application's data folder (`devices.json`, not a workspace: the network is the host's), fed by the host's own neighbour table (`/proc/net/arp`, read without starting a program). Both the nightly audit and a new frequent presence check go through one service (`netaudit.Inventory`) so a look is never reported twice. The presence check is a single loop started in `app.New` and tethered to the app context. Every cycle it sweeps the declared scan ranges through `Service.Scan` in `ModeDiscover`, so scope authorisation, the guarded dialer and the one-scan-at-a-time lock apply. It never starts a model. It is off until the operator switches it on. Its alert is counts and fixed words to the operator's connectors, and an alert is marked as told only after it was sent. Findings are derived from the inventory's state, not from events, so what the presence check noticed between two nights is still in the nightly report. The approved router MAC only moves when the operator confirms it (a step-up route).

## Consequences
New device, changed owner of an address, router MAC change, one MAC on several addresses, a named device gone for three days and a silent device are findings in the ordinary report, keyed by IP like every other, so accepting, fix marks and the baseline work unchanged. The sweep and a deep check share the scan lock, so a deep check can find the scanner briefly busy. The inventory sees only the host's own network segment and only Linux hosts; elsewhere the report says "could not be checked". A device that changes IP by DHCP appears as a new finding key (the device itself is still one entry, by MAC).

## Alternatives considered
- **Inventory per workspace** — the network is not a workspace's, and two audits would double-report.
- **Presence check as an automation** — runs a model per cycle; an alert every ten minutes would cost a model call each.
- **Event-based findings** — a finding the presence check raised could be consumed before the nightly run and never reported.
- **A packet-sniffing or ARP-watch daemon** — needs raw sockets and privileges; separate plan, not needed to see who is there.

## Links
- Plan: [`network-detection.md`](../PLANS/cross-cutting/network-detection.md); as built: [`architecture/network-audit.md`](../architecture/network-audit.md).
