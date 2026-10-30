---
subsystem: network-audit
specs: []
---

# Network audit

Guarded nmap scans, findings and reports, and the Security auditor agent.

Start here, then open only what the task needs.

| | |
|---|---|
| **Contract (current behavior)** | _no SPEC yet_ — the as-built design is [`architecture/network-audit.md`](../architecture/network-audit.md); the finished plan ([`ARCHIVE/cross-cutting/network-audit.md`](../PLANS/ARCHIVE/cross-cutting/network-audit.md), archived 2026-10-29) holds the postponed follow-ups |
| **Code** | `backend/internal/core/netaudit/` (known-vulnerability matching in `vulnindex/`, `vulnmatch/`, `vulnfeed/`), `backend/internal/platform/scanscope/`, `backend/internal/platform/neighbors/` (the host's IPv4 and IPv6 neighbour tables), `core/netaudit/upnp/` and `core/netaudit/decoy/`, the device inventory (`inventory.go`, `presence.go`), `frontend/src/components/netaudit/` (the data card is `VulnDataCard.vue`, its state `composables/netaudit/useVulnData.ts`) |
| **Architecture** | [map](../architecture/map.md) · [contracts](../architecture/contracts.md) · [checklists](../architecture/checklists.md) · [pitfalls](../architecture/pitfalls.md) — search pitfalls for: netaudit, nmap, scan scope |
| **Decisions (why)** | [ADR-0007](../decisions/ADR-0007-no-nmap-in-ci.md) No nmap in CI; verify network scans by hand<br>[ADR-0010](../decisions/ADR-0010-no-level-2-assisted-fixes.md) Assisted fixes are guided, not executed<br>[ADR-0011](../decisions/ADR-0011-nvd-kev-local-vulnerability-data.md) Local NVD + KEV data, no online lookup per scan<br>[ADR-0012](../decisions/ADR-0012-intrusive-scripts-only-in-the-deep-check.md) Intrusive scripts only in the deep check<br>[ADR-0013](../decisions/ADR-0013-vulnerability-data-import-is-step-up.md) Vulnerability data import and delete are step-up routes<br>[ADR-0014](../decisions/ADR-0014-presence-check-and-device-inventory.md) The presence check is a model-free background loop over one app-wide device inventory<br>[ADR-0015](../decisions/ADR-0015-decoy-listeners-low-interaction-and-step-up.md) Decoy services are low-interaction listeners (proposed with Constitution I.6)
| **Open plans (next)** | [`cross-cutting/network-detection.md`](../PLANS/cross-cutting/network-detection.md) (partial: device inventory, identity drift, the presence check, port checks, the router's UPnP, passive IPv6 and the decoy services are built; the UPnP port-mapping list, phoning home and active IPv6 are not) |
| **Learnings (tried before)** | [`learnings/network-audit.md`](../learnings/network-audit.md) — read before proposing an approach |
| **Evidence (audits, evals)** | [`audits/2026-10-29-network-audit-phase-log.md`](../audits/2026-10-29-network-audit-phase-log.md) — the phase table, Phase 0 findings and phase-by-phase log |
| **Skills** | — |
| **Guides** | — |
| **Archived plans** | others by topic in [`ARCHIVE/README.md`](../PLANS/ARCHIVE/README.md) |
