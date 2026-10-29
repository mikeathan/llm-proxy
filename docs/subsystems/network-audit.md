---
subsystem: network-audit
specs: []
---

# Network audit

Guarded nmap scans, findings and reports, and the Security auditor agent.

Start here, then open only what the task needs.

| | |
|---|---|
| **Contract (current behavior)** | _no SPEC yet_ — the as-built design is [`architecture/network-audit.md`](../architecture/network-audit.md); the open plan below holds what is left |
| **Code** | `backend/internal/core/netaudit/` (known-vulnerability matching in `vulnindex/`, `vulnmatch/`, `vulnfeed/`), `backend/internal/platform/scanscope/`, `frontend/src/components/netaudit/` (the data card is `VulnDataCard.vue`, its state `composables/netaudit/useVulnData.ts`) |
| **Architecture** | [map](../architecture/map.md) · [contracts](../architecture/contracts.md) · [checklists](../architecture/checklists.md) · [pitfalls](../architecture/pitfalls.md) — search pitfalls for: netaudit, nmap, scan scope |
| **Decisions (why)** | [ADR-0007](../decisions/ADR-0007-no-nmap-in-ci.md) No nmap in CI; verify network scans by hand<br>[ADR-0010](../decisions/ADR-0010-no-level-2-assisted-fixes.md) Assisted fixes are guided, not executed<br>[ADR-0011](../decisions/ADR-0011-nvd-kev-local-vulnerability-data.md) Local NVD + KEV data, no online lookup per scan<br>[ADR-0012](../decisions/ADR-0012-intrusive-scripts-only-in-the-deep-check.md) Intrusive scripts only in the deep check<br>[ADR-0013](../decisions/ADR-0013-vulnerability-data-import-is-step-up.md) Vulnerability data import and delete are step-up routes
| **Open plans (next)** | [`cross-cutting/network-audit.md`](../PLANS/cross-cutting/network-audit.md)<br>[`cross-cutting/network-detection.md`](../PLANS/cross-cutting/network-detection.md) (proposed: new devices, identity drift, phoning home, decoys) |
| **Learnings (tried before)** | [`learnings/network-audit.md`](../learnings/network-audit.md) — read before proposing an approach |
| **Evidence (audits, evals)** | [`audits/2026-10-29-network-audit-phase-log.md`](../audits/2026-10-29-network-audit-phase-log.md) — the phase table, Phase 0 findings and phase-by-phase log |
| **Skills** | — |
| **Guides** | — |
| **Archived plans** | others by topic in [`ARCHIVE/README.md`](../PLANS/ARCHIVE/README.md) |
