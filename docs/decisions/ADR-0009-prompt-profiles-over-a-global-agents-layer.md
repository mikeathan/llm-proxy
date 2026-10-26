---
id: ADR-0009
title: Prompt profiles instead of a global AGENTS.md prompt layer
status: accepted
date: 2026-10-17
subsystems: [prompt-profiles]
supersedes:
superseded_by:
---

# ADR-0009: Prompt profiles instead of a global AGENTS.md prompt layer

_Recorded 2026-10-26 from the plan and evidence below; the decision date is when it was taken._

## Context
Scope and safety guidance for the product's agent lived in the embedded default workspace `AGENTS.md`, which a workspace's own file replaced entirely. A global AGENTS.md layer was designed to fix that.

## Decision
We will carry behavioral guidance in selectable prompt profiles (SPEC-012) with a generic default, rebuilt per turn. The global AGENTS.md layer is not built; the workspace `AGENTS.md` stays for workspace-specific notes. The write guardrail from the same plan was built separately (SPEC-006 §II.8).

## Consequences
Guidance is chosen per use (Research, Operator, Builder, …) instead of stacked text layers. Profile evaluations decide changes (see `learnings/prompt-profiles.md`).

## Alternatives considered
- **Global + workspace AGENTS.md layers** — stacked text with unclear override order.

## Links
- [`PLANS/cross-cutting/agents-md-layering-guardrails.md`](../PLANS/cross-cutting/agents-md-layering-guardrails.md) (status line)
- [`PLANS/cross-cutting/prompt-profiles.md`](../PLANS/cross-cutting/prompt-profiles.md)
- [`SPECS/prompt-profiles.md`](../SPECS/prompt-profiles.md)
- [`audits/2026-10-17-prompt-profiles-eval.md`](../audits/2026-10-17-prompt-profiles-eval.md)
