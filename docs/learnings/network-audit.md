---
subsystem: network-audit
last_updated: 2026-10-27
---

# Learnings — Network audit (no SPEC yet; plan `cross-cutting/network-audit.md`)

What was tried and what happened. Read before proposing an approach in this area; add a line when a run,
eval, audit or merged phase teaches something (newest last within each section). Format and rules:
[`README.md`](README.md).

## Dead ends — do not retry without new evidence

_None recorded yet._

## Root causes and fixes that held

- **Tried:** Letting the model choose scan targets → **an auditor run guessed `192.168.1.0/24`**. **Lesson:** `network_scan` with no targets scans every allowed range. Evidence: [network-audit.md](../PLANS/cross-cutting/network-audit.md).
- **Tried:** Trusting nmap's port-table service names → **8081 was read as "blackice-icecap" and described as an IDS**. **Lesson:** mark table names as guesses. Evidence: [network-audit.md](../PLANS/cross-cutting/network-audit.md).
- **Tried:** Reading interfaces via netlink under a systemd unit → **no netlink socket on the deployment**. **Lesson:** fall back to the default route's address on a /24. Evidence: [network-audit.md](../PLANS/cross-cutting/network-audit.md).
- **Tried:** Running nmap in CI → **decided against (D13)**. **Lesson:** verify nmap under Landlock by hand on Ubuntu; keep CI small. Evidence: [network-audit.md](../PLANS/cross-cutting/network-audit.md).
- **Tried:** A one-device rescan through the ordinary audit run → **its baseline write replaced `Unscanned` and `DevicesUp` with the single host's, while findings were already merged per device**. **Lesson:** a run that covers some devices must carry over the rest (`carryOver` in `runreporter.go`). A rescan queued behind the same audit is absorbed by the lane's key dedupe and runs as the full audit. Evidence: [network-audit.md](../PLANS/cross-cutting/network-audit.md) §13.A.
