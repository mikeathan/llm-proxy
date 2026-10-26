# Learnings

**What we tried and what happened** — the repo's experience, kept so nobody (human or agent) repeats a dead
end. One ledger per subsystem (same slugs as [`../subsystems/`](../subsystems/README.md)); the raw evidence
(audits, evals, run analyses, post-mortems) stays in [`../audits/`](../audits/README.md) and in plans.

**Before proposing an approach** in an area, read its ledger. **After** a run, eval, audit or merged phase
teaches something, add one line.

## Entry format

```
- **Tried:** <what was done or proposed> → **<what happened, with numbers>**. **Lesson:** <the rule to keep>. Evidence: [<file>](<link>).
```

- Put it under **Dead ends** (rejected, reverted, made things worse — do not retry without new evidence) or
  **Root causes and fixes that held**.
- One line, numbers over adjectives, always an evidence link. If the lesson is a recurring code trap, also
  add a numbered pitfall in [`../architecture/pitfalls.md`](../architecture/pitfalls.md); if it settled a
  choice between alternatives, write an ADR in [`../decisions/`](../decisions/README.md).
- Never delete a line. If later evidence overturns it, add a new line that says so and links both.

## Evidence documents (audits)

New audits and eval reports go in `docs/audits/` with this frontmatter (and a row in its README):

```yaml
---
subsystem: <slug>          # primary subsystem; others in tags
tags: [<free-form>]
result: worked | failed | inconclusive | reference   # reference = analysis or research, no single outcome
status: complete | active | reference
date: YYYY-MM-DD
---
```

## Ledgers

| Subsystem | Ledger |
|-----------|--------|
| Agent loop, tool calling, run pipeline | [`agent-loop.md`](agent-loop.md) |
| Admin UI and chat rendering | [`admin-ui.md`](admin-ui.md) |
| Memory | [`memory.md`](memory.md) |
| Inference, budgets, providers, performance | [`orchestrator.md`](orchestrator.md) |
| Guardrails, sandbox, egress | [`guardrails.md`](guardrails.md) |
| Automation and heartbeat | [`automation.md`](automation.md) |
| MCP | [`mcp.md`](mcp.md) |
| Communication connectors | [`communication.md`](communication.md) |
| Autonomy and reminders | [`autonomy.md`](autonomy.md) |
| Prompt profiles | [`prompt-profiles.md`](prompt-profiles.md) |
| Network audit | [`network-audit.md`](network-audit.md) |
| Tool catalog and registry | [`tool-registry.md`](tool-registry.md) |
| Storage, config, events, logging, backend performance | [`platform.md`](platform.md) |
| Docs and the coding-agent harness | [`agent-harness.md`](agent-harness.md) |
