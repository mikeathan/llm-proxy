---
subsystem: network-audit
specs: []
---

# Network audit

Guarded nmap scans, findings and reports, and the Security auditor agent.

Start here, then open only what the task needs.

| | |
|---|---|
| **Contract (current behavior)** | _no SPEC yet_ — the open plan below is the governing document |
| **Code** | `backend/internal/core/netaudit/` (known-vulnerability matching in `vulnindex/`, `vulnmatch/`, `vulnfeed/`), `backend/internal/platform/scanscope/`, `frontend/src/components/netaudit/` (the data card is `VulnDataCard.vue`, its state `composables/netaudit/useVulnData.ts`) |
| **Architecture** | [map](../architecture/map.md) · [contracts](../architecture/contracts.md) · [checklists](../architecture/checklists.md) · [pitfalls](../architecture/pitfalls.md) — search pitfalls for: netaudit, nmap, scan scope |
| **Decisions (why)** | [ADR-0007](../decisions/ADR-0007-no-nmap-in-ci.md) No nmap in CI; verify network scans by hand |
| **Open plans (next)** | [`cross-cutting/network-audit.md`](../PLANS/cross-cutting/network-audit.md)<br>[`cross-cutting/network-detection.md`](../PLANS/cross-cutting/network-detection.md) (proposed: new devices, identity drift, phoning home, decoys) |
| **Learnings (tried before)** | [`learnings/network-audit.md`](../learnings/network-audit.md) — read before proposing an approach |
| **Evidence (audits, evals)** | _none yet_ |
| **Skills** | — |
| **Guides** | — |
| **Archived plans** | others by topic in [`ARCHIVE/README.md`](../PLANS/ARCHIVE/README.md) |
