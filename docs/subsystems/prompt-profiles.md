---
subsystem: prompt-profiles
specs: [SPEC-012]
---

# Prompt profiles and prompt text

Selectable instruction bundles (profiles/agents) and the single home of prompt text.

Start here, then open only what the task needs.

| | |
|---|---|
| **Contract (current behavior)** | [SPEC-012](../SPECS/prompt-profiles.md) |
| **Code** | `backend/internal/core/assistant/prompts/`, `backend/internal/core/assistant/profile/` |
| **Architecture** | [map](../architecture/map.md) · [contracts](../architecture/contracts.md) · [checklists](../architecture/checklists.md) · [pitfalls](../architecture/pitfalls.md) — search pitfalls for: prompt, templates.go, profile |
| **Decisions (why)** | [ADR-0009](../decisions/ADR-0009-prompt-profiles-over-a-global-agents-layer.md) Prompt profiles instead of a global AGENTS.md prompt layer |
| **Open plans (next)** | [`cross-cutting/prompt-profiles.md`](../PLANS/cross-cutting/prompt-profiles.md)<br>[`cross-cutting/agents-md-layering-guardrails.md`](../PLANS/cross-cutting/agents-md-layering-guardrails.md) |
| **Learnings (tried before)** | [`learnings/prompt-profiles.md`](../learnings/prompt-profiles.md) — read before proposing an approach |
| **Evidence (audits, evals)** | [`2026-10-17-prompt-profiles-eval.md`](../audits/2026-10-17-prompt-profiles-eval.md) |
| **Skills** | [`engineering-practices`](../../.agents/skills/engineering-practices/SKILL.md) |
| **Guides** | — |
| **Archived plans** | others by topic in [`ARCHIVE/README.md`](../PLANS/ARCHIVE/README.md) |
