---
subsystem: prompt-profiles
last_updated: 2026-10-26
---

# Learnings — Prompt profiles and prompt text (SPEC-012)

What was tried and what happened. Read before proposing an approach in this area; add a line when a run,
eval, audit or merged phase teaches something (newest last within each section). Format and rules:
[`README.md`](README.md).

## Dead ends — do not retry without new evidence

- **Tried:** Negative instructions ("do NOT use this to submit results") to steer small models → **did not change the choice**. **Lesson:** positive instructions ("do Y") work better; fix the real cause first. Evidence: [memory-injection-investigation.md](../audits/memory-injection-investigation.md).
- **Tried:** A global AGENTS.md prompt layer for the product → **superseded**. **Lesson:** prompt profiles (SPEC-012) carry the same guidance per use. Evidence: [agents-md-layering-guardrails.md](../PLANS/cross-cutting/agents-md-layering-guardrails.md).

## Root causes and fixes that held

- **Tried:** Profiles with a generic default (eval on three local models) → **a vague one-liner no longer scans the workspace (3–6 tool calls → 0); 21/21 cases pass; one run per cell**. **Lesson:** repeat runs before treating a cell as a rate. Evidence: [2026-10-17-prompt-profiles-eval.md](../audits/2026-10-17-prompt-profiles-eval.md).
