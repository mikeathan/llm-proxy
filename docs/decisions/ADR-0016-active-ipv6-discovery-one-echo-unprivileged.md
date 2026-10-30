---
id: ADR-0016
title: Active IPv6 discovery sends one echo through the unprivileged ICMP socket, off by default
status: proposed
date: 2026-10-30
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0016: Active IPv6 discovery sends one echo through the unprivileged ICMP socket, off by default

## Context
The host's IPv6 neighbour table lists only devices it has already talked to, so passive reading (v0.26.0) finds few of them. On the owner's server one echo to `ff02::1` took the table from 4 entries (3 devices) to 11 entries, 7 hardware addresses that had not been there. Sending that echo needs an ICMPv6 socket, which Constitution I.1 to I.3 do not provide (`NetworkTools` has no ICMP) and II.3 item (3) forbids as a scan target (no IPv6, no multicast).

## Decision
We will send one fixed ICMPv6 echo request to `ff02::1` per interface per look, through the kernel's unprivileged ICMP datagram socket (`golang.org/x/sys/unix`, already a dependency), under a new Constitution item I.7. Replies are never read; the effect is only the kernel's table, which the existing netlink reader lists. It is off until the operator switches it on. The host decides whether the socket may be opened (`net.ipv4.ping_group_range`); the owner's server has it at `1 0`, so the operator changes that setting once, and the screen shows the command when the host refuses. The first look that runs discovery is a baseline for IPv6-only devices, so switching it on does not alert for devices that were always there. The scan scope stays IPv4 and no finding is keyed by an IPv6 address.

Verified on a real Linux kernel in two containers on an IPv6 network: with the default setting the peer appears in the table after the echo and was absent before; with `ping_group_range` set to `1 0` the call returns the typed refusal.

## Consequences
The operator makes one host change (a sysctl) for the feature to work, and the broader range lets other local accounts ping. A device that never answers multicast stays invisible. Findings are unchanged; the new devices appear on the devices card and in the new-device alert.

## Alternatives considered
- **Run `ping` through the guarded launcher** — needs no sysctl but widens II.3 (a program allowed to touch IPv6 and multicast), plus sandbox and path handling, for the same packet.
- **`golang.org/x/net/icmp`** — less code, a new dependency, and the same socket underneath.
- **A raw socket or a capability for the service** — more privilege than one packet needs.
- **Scan IPv6 addresses too** — an IPv6 scan scope and a change to II.3 item (3); not needed to list devices.
- **mDNS or other UDP multicast** — needs no privilege but only finds devices that answer that protocol, and a reply does not fill the table.

## Links
- Plan: [`network-detection.md`](../PLANS/cross-cutting/network-detection.md); Constitution I.7.
- Related: [ADR-0014](ADR-0014-presence-check-and-device-inventory.md), [ADR-0015](ADR-0015-decoy-listeners-low-interaction-and-step-up.md).
