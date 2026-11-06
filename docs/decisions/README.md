# Decisions (ADRs)

Architecture Decision Records: **why** the system is the way it is. One short file per decision.

- **Immutable once `accepted`.** Do not rewrite an accepted ADR to change the decision — write a new ADR that
  `supersedes` it and set `superseded_by` on the old one. Fixing a typo or a dead link is fine.
- **Small.** Context, decision, consequences, alternatives, links — one screen. Detail stays in the plan, PR or
  audit the ADR links to.
- **When to write one:** a choice between real alternatives that a later contributor would otherwise re-open —
  an architecture or contract choice, a "we will not do X", a tooling or process rule. Plans record decisions
  while work is in flight; when the plan's phase merges, the decision moves here (see the harvest rule in
  [`../PLANS/README.md`](../PLANS/README.md)).
- **Naming:** `ADR-NNNN-<kebab-slug>.md`, next free number, from [`TEMPLATE.md`](TEMPLATE.md); add a row below.
  `subsystems:` uses the slugs in [`../subsystems/README.md`](../subsystems/README.md).

| ADR | Title | Status | Date | Subsystems |
|-----|-------|--------|------|------------|
| [ADR-0001](ADR-0001-harness-neutral-agent-instructions.md) | Harness-neutral agent instructions | accepted | 2026-10-26 | agent-harness |
| [ADR-0002](ADR-0002-docs-split-by-lifecycle.md) | Docs split by lifecycle, linked by subsystem | accepted | 2026-10-26 | agent-harness |
| [ADR-0003](ADR-0003-natural-completion.md) | Completion is a content-only final answer | accepted | 2026-07-21 | agent-loop |
| [ADR-0004](ADR-0004-explicit-primary-and-fallback-model.md) | No model auto-bootstrap; explicit primary and fallback | accepted | 2026-08-14 | orchestrator, admin-ui |
| [ADR-0005](ADR-0005-no-llm-rewrite-on-the-run-path.md) | No extra LLM call on the run path to apply memory | accepted | 2026-06-03 | memory |
| [ADR-0006](ADR-0006-cloud-helper-sub-agents.md) | Helper sub-agents run in the cloud, depth 1, from their own pool | accepted | 2026-10-22 | agent-loop |
| [ADR-0007](ADR-0007-no-nmap-in-ci.md) | No nmap in CI; verify network scans by hand | accepted | 2026-10-26 | network-audit |
| [ADR-0008](ADR-0008-terminal-blocks-a-curated-set.md) | The terminal guard blocks a curated sensitive set, not every dot-directory | accepted | 2026-08-20 | guardrails |
| [ADR-0009](ADR-0009-prompt-profiles-over-a-global-agents-layer.md) | Prompt profiles instead of a global AGENTS.md prompt layer | accepted | 2026-10-17 | prompt-profiles |
| [ADR-0010](ADR-0010-no-level-2-assisted-fixes.md) | Assisted fixes are guided, not executed by the app | accepted | 2026-10-29 | network-audit |
| [ADR-0011](ADR-0011-nvd-kev-local-vulnerability-data.md) | Known-vulnerability matching uses local NVD and KEV data, never an online lookup per scan | accepted | 2026-10-29 | network-audit |
| [ADR-0012](ADR-0012-intrusive-scripts-only-in-the-deep-check.md) | Intrusive scripts run only in an interactive, step-up deep check of one device | accepted | 2026-10-29 | network-audit |
| [ADR-0013](ADR-0013-vulnerability-data-import-is-step-up.md) | Importing vulnerability data is a step-up route | accepted | 2026-10-29 | network-audit |
| [ADR-0014](ADR-0014-presence-check-and-device-inventory.md) | The presence check is a model-free background loop over one app-wide device inventory | accepted | 2026-10-29 | network-audit |
| [ADR-0015](ADR-0015-decoy-listeners-low-interaction-and-step-up.md) | Decoy services are low-interaction listeners, off by default, switched by a step-up route | accepted | 2026-10-30 | network-audit |
| [ADR-0016](ADR-0016-active-ipv6-discovery-one-echo-unprivileged.md) | Active IPv6 discovery sends one echo through the unprivileged ICMP socket, off by default | proposed | 2026-10-30 | network-audit |
| [ADR-0017](ADR-0017-on-link-ipv6-scanning-keyed-by-mac.md) | The audit scans on-link IPv6-only devices, keyed by hardware address, once the Constitution is amended | proposed | 2026-10-31 | network-audit |
| [ADR-0018](ADR-0018-router-log-only-wakes-the-presence-check.md) | The router's log may only wake the presence check early; nothing in it is kept, shown or acted on | proposed | 2026-11-01 |
| [ADR-0019](ADR-0019-outside-lookup-is-opt-in-and-fails-open-to-unknown.md) | The outside lookup of open ports is opt-in, sends only the router's stated public address, and never reads a failure as "nothing is open" | accepted | 2026-11-01 | network-audit |
| [ADR-0020](ADR-0020-dns-check-only-records-and-never-reads-silence-as-agreement.md) | The DNS check only records, compares sets over DNS-over-HTTPS, and a missing answer is never read as agreement | accepted | 2026-11-01 | network-audit |
| [ADR-0021](ADR-0021-an-entered-public-address-stands-in-when-the-router-gives-none.md) | An entered public address stands in when the router gives none, and the router's own answer wins | accepted | 2026-11-01 | network-audit |
| [ADR-0022](ADR-0022-modules-is-the-navigation-group-for-feature-areas.md) | Modules is the navigation group for self-contained feature areas, and a page keeps its own settings | accepted | 2026-11-02 | admin-ui, network-audit |
| [ADR-0023](ADR-0023-network-ideas-rejected-or-already-built.md) | Network detection ideas that were weighed and rejected, parked, or found already built | accepted | 2026-11-02 | network-audit |
| [ADR-0024](ADR-0024-nearby-discovery-two-multicast-datagrams-bounded-replies.md) | Nearby discovery sends two fixed multicast datagrams and reads a bounded set of unicast replies, off by default (proposed, with Constitution I.9) | proposed | 2026-11-03 | network-audit |
| [ADR-0025](ADR-0025-network-timeline-reads-existing-records-summary-is-local-and-typed.md) | The Network timeline reads records the program already keeps, and its model summary is local-only and typed-fields-only | accepted | 2026-11-04 |
| [ADR-0026](ADR-0026-program-files-are-compared-with-the-checksums-dpkg-keeps.md) | Packaged program files are compared with the checksums dpkg keeps, and a pass that is cut short never resolves a finding | accepted | 2026-11-04 |
| [ADR-0027](ADR-0027-installed-packages-are-judged-against-canonicals-usn-oval.md) | Installed packages are judged against Canonical's per-release USN OVAL file, with Pro-only fixes left out | accepted | 2026-11-04 |
| [ADR-0028](ADR-0028-stack-module-registry.md) | Switchable capabilities are modules in stacks, described by one registry and switched in one place | accepted | 2026-11-05 |
| [ADR-0029](ADR-0029-the-service-keeps-memory-deny-write-execute-so-nuclei-is-removed.md) | The service keeps MemoryDenyWriteExecute, so the nuclei web checks are removed | accepted | 2026-11-06 | network-audit |
