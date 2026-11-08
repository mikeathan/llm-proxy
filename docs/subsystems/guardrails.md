---
subsystem: guardrails
specs: [SPEC-006]
---

# Guardrails, sandbox and egress

What the agent may do: the guardrail engine and approvals, the OS sandbox, egress control, control-plane security.

Start here, then open only what the task needs.

| | |
|---|---|
| **Contract (current behavior)** | [SPEC-006](../SPECS/guardrails.md) |
| **Code** | `backend/internal/core/assistant/guardrails/`, `backend/internal/platform/sandbox/`, `backend/internal/platform/isolation/` (the one door to the sandbox and guarded programs, with its conformance suite `isolationtest`), `backend/internal/core/egress/` |
| **Architecture** | [map](../architecture/map.md) · [contracts](../architecture/contracts.md) · [checklists](../architecture/checklists.md) · [pitfalls](../architecture/pitfalls.md) — search pitfalls for: guardrail, sandbox, Landlock, egress, approval |
| **Decisions (why)** | [ADR-0008](../decisions/ADR-0008-terminal-blocks-a-curated-set.md) The terminal guard blocks a curated sensitive set, not every dot-directory<br>[ADR-0031](../decisions/ADR-0031-the-wifi-look-is-a-guarded-nmcli-with-no-network-and-only-unix-sockets.md) Jailed children are refused Unix sockets (the jail did not control a path-based socket), and the Wi-Fi look is the one program allowed them, with no network (proposed)<br>[ADR-0033](../decisions/ADR-0033-the-sandbox-is-core-and-stacks-gain-no-way-around-it.md) The sandbox is core and cannot be swapped or configured off, and a stack gains no way around it (proposed)<br>[ADR-0034](../decisions/ADR-0034-the-privileged-helper-is-core-not-a-stack-and-decides-who-is-calling.md) The privileged helper is core, not a stack, and it decides who is calling (proposed) |
| **Open plans (next)** | [`cross-cutting/sandbox-and-egress-residuals.md`](../PLANS/cross-cutting/sandbox-and-egress-residuals.md)<br>[`cross-cutting/opensandbox-evaluation.md`](../PLANS/cross-cutting/opensandbox-evaluation.md)<br>[`cross-cutting/admin-api-authentication.md`](../PLANS/cross-cutting/admin-api-authentication.md)<br>[`cross-cutting/cloud-privacy-firewall.md`](../PLANS/cross-cutting/cloud-privacy-firewall.md)<br>[`cross-cutting/agents-md-layering-guardrails.md`](../PLANS/cross-cutting/agents-md-layering-guardrails.md)<br>[`cross-cutting/stacks-as-packages-and-network-stack.md`](../PLANS/cross-cutting/stacks-as-packages-and-network-stack.md) (proposed 2026-11-06: a stack as a package with a manifest, kept apart by Constitution IV.6 tests, Network first, the isolation library extracted; running a stack as its own process is out of scope) |
| **Learnings (tried before)** | [`learnings/guardrails.md`](../learnings/guardrails.md) — read before proposing an approach |
| **Evidence (audits, evals)** | [`hermes-write-file-guardrail.md`](../audits/hermes-write-file-guardrail.md)<br>[`terminal-vs-filesystem-guardrail-asymmetry.md`](../audits/terminal-vs-filesystem-guardrail-asymmetry.md) |
| **Skills** | [`engineering-practices`](../../.agents/skills/engineering-practices/SKILL.md) |
| **Guides** | — |
| **Archived plans** | others by topic in [`ARCHIVE/README.md`](../PLANS/ARCHIVE/README.md) |
