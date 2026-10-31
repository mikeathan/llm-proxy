---
id: ADR-0017
title: The audit scans on-link IPv6-only devices, keyed by hardware address, once the Constitution is amended
status: proposed
date: 2026-10-31
subsystems: [network-audit]
supersedes:
superseded_by:
---

# ADR-0017: The audit scans on-link IPv6-only devices, keyed by hardware address, once the Constitution is amended

## Context
The audit scans IPv4 only. Constitution II.3 item (3) says the scan scope can never contain public, shared (CGNAT), IPv6, loopback, link-local or multicast addresses, and I.7 says "the scan scope does not change" and no finding is keyed by an IPv6 address. Active IPv6 discovery (v0.28.0) now lists devices that exist only over IPv6, so the Devices card shows machines the audit never looks at. Every finding, acceptance, baseline diff and report row is keyed by `Finding.Device`, today an IPv4 address; IPv6 privacy addresses rotate daily, so keying by them would report every scan as all-new and make every acceptance stop matching.

## Decision
We will, in a later pull request that carries the Constitution change, let the nightly audit scan **on-link IPv6-only devices** and key their findings by the device's **MAC address**.

- **Scope.** A target is an IPv6 address from the host's neighbour table, on an interface that shares a segment with an in-scope IPv4 device, belonging to a device that has no IPv4 address in the inventory. The operator cannot type an IPv6 target. No routed or global address that is not in the neighbour table is ever a target; dual-stack devices stay IPv4-only, so nothing is reported twice. It is off until switched on, and switching on is step-up.
- **Key.** For these devices `Finding.Device` holds the MAC; it is "an IPv4 address, or a MAC for an IPv6-only device". The acceptance, diff, report and fix-guide code and the UI group-by-device view must treat both forms.
- **Proposed Constitution text** (not in `CONSTITUTION.md` yet): II.3 item (3) gains "except an on-link IPv6 address of an IPv6-only device taken from the host's neighbour table, scanned only when the operator has switched IPv6 scanning on", and I.7's "the scan scope does not change" bullet is rewritten to point to this exception. Link-local addresses stay refused for IPv4 and for any address the operator supplies; the exception covers the zone-qualified neighbour address only.

It is not built in the pull request that records this decision, for two reasons the owner agreed with: it widens the `Finding.Device` contract across the diff, acceptances, report and UI, and no `nmap -6` output from a real on-link device has been captured (the argument shape, the zone id in a link-local target and the XML address type are unverified).

## Consequences
When built: IPv6-only devices get port and service findings like any other; a small step-up switch; tests that today pin "the scope never contains IPv6" are changed on purpose, not deleted; the guarded-program rules (arguments from a fixed table, an explicit environment) are unchanged. Until then the Devices card keeps listing them without a scan, as now.

## Alternatives considered
- **Key by the IPv6 address.** Rotates daily; breaks baselines and acceptances.
- **Key by a stable link-local address.** Depends on the operating system deriving it stably; many do, not all, and it still needs a zone.
- **Operator-declared IPv6 prefixes.** Wider, needs rules for prefix size and public space, and nothing needs it for a home network.
- **Scan IPv6 for dual-stack devices too.** Reports every service twice and doubles the scan.

## Links
- Plan: [`network-followups.md`](../PLANS/cross-cutting/network-followups.md) §3.5.
- Related: [ADR-0016](ADR-0016-active-ipv6-discovery-one-echo-unprivileged.md); Constitution I.7, II.3.
