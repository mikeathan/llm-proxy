---
id: ADR-0003
title: Completion is a content-only final answer
status: accepted
date: 2026-07-21
subsystems: [agent-loop]
supersedes:
superseded_by:
---

# ADR-0003: Completion is a content-only final answer

_Recorded 2026-10-26 from the plan and evidence below; the decision date is when it was taken._

## Context
The loop used a synthetic `submit_final_answer` tool as the completion signal. Models forgot it, batched it with other tools or mangled its arguments; ~80 lines of backend code and three frontend special cases existed only to cope, and no other production agent works this way.

## Decision
We will treat a turn that returns content and no tool call as the final answer. There is no completion tool. Truncated or markup-only content is rejected by the completion gates (for example a bare `Action:` last line) and goes through the recovery ladder instead.

## Consequences
Completion behaves the same for every model and loop strategy, and the frontend renders the answer as a normal message. The gates that tell a real answer from a truncated one are now critical code; they live at every completion surface (see `learnings/agent-loop.md`).

## Alternatives considered
- **Keep the synthetic tool and harden its parsing** — keeps a convention models are not trained on.

## Links
- [`ARCHIVE/cross-cutting/universal-agent-completion.md`](../PLANS/ARCHIVE/cross-cutting/universal-agent-completion.md)
- SPEC-001 §5 ([`agent-loop.md`](../SPECS/agent-loop.md))
- [`audits/2026-08-30-llm-smoke-test-incomplete-run.md`](../audits/2026-08-30-llm-smoke-test-incomplete-run.md)
