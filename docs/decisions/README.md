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
| [ADR-0015](ADR-0015-decoy-listeners-low-interaction-and-step-up.md) | Decoy services are low-interaction listeners, off by default, switched by a step-up route | proposed | 2026-10-30 | network-audit |
