---
id: ADR-0015
title: Decoy services are low-interaction listeners, off by default, switched by a step-up route
status: accepted
date: 2026-10-30
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0015: Decoy services are low-interaction listeners, off by default, switched by a step-up route

## Context
A device probing the network for services is the first sign of an intruder or of malware, and a scan of the other devices cannot see it. A fake service that nothing legitimate uses can: any connection to it is worth a look. It means this program opens listening sockets, which Constitution I.4 allows only for inbound webhooks.

## Decision
We will run three fixed decoy listeners (`core/netaudit/decoy`) under Constitution I.6: off until the operator switches them on through a step-up route, fixed unprivileged ports that are in neither scan list, a read of at most 64 bytes that is discarded, a fixed reply string only, nothing but source, port, count and times kept, only sources inside the declared scan networks and never this computer's own addresses, bounded concurrency and rate, alerts in counts, fixed words and the source addresses (amended 2026-10-30: the first version was counts only and could not tell the operator which device it meant; the address is the connection's remote address as the kernel reports it, never bytes the peer sent). They bind all IPv4 interfaces and filter by source, because the host is a laptop on DHCP.

## Consequences
A host firewall can hide the decoys; the screen says so. Any device can open connections up to the limit, which is the defence against exhausting them. A scanner app on the operator's own phone will touch a decoy, so the finding can be accepted.

## Alternatives considered
- **No decoys** — leaves a probing device invisible to the audit.
- **Interactive honeypots (real SSH or SMB emulation)** — a much larger attack surface for little more signal.
- **Binding only to the host's current LAN address** — breaks when DHCP changes it.

## Links
- Plan: [`network-detection.md`](../PLANS/cross-cutting/network-detection.md) Phase 5; Constitution I.6.
