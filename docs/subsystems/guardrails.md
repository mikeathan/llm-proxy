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
| **Code** | `backend/internal/core/assistant/guardrails/`, `backend/internal/platform/sandbox/`, `backend/internal/core/egress/` |
| **Architecture** | [map](../architecture/map.md) · [contracts](../architecture/contracts.md) · [checklists](../architecture/checklists.md) · [pitfalls](../architecture/pitfalls.md) — search pitfalls for: guardrail, sandbox, Landlock, egress, approval |
| **Decisions (why)** | [ADR-0008](../decisions/ADR-0008-terminal-blocks-a-curated-set.md) The terminal guard blocks a curated sensitive set, not every dot-directory |
| **Open plans (next)** | [`cross-cutting/sandbox-and-egress-residuals.md`](../PLANS/cross-cutting/sandbox-and-egress-residuals.md)<br>[`cross-cutting/admin-api-authentication.md`](../PLANS/cross-cutting/admin-api-authentication.md)<br>[`cross-cutting/cloud-privacy-firewall.md`](../PLANS/cross-cutting/cloud-privacy-firewall.md)<br>[`cross-cutting/agents-md-layering-guardrails.md`](../PLANS/cross-cutting/agents-md-layering-guardrails.md) |
| **Learnings (tried before)** | [`learnings/guardrails.md`](../learnings/guardrails.md) — read before proposing an approach |
| **Evidence (audits, evals)** | [`hermes-write-file-guardrail.md`](../audits/hermes-write-file-guardrail.md)<br>[`terminal-vs-filesystem-guardrail-asymmetry.md`](../audits/terminal-vs-filesystem-guardrail-asymmetry.md) |
| **Skills** | [`engineering-practices`](../../.agents/skills/engineering-practices/SKILL.md) |
| **Guides** | — |
| **Archived plans** | others by topic in [`ARCHIVE/README.md`](../PLANS/ARCHIVE/README.md) |
