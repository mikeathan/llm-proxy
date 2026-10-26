---
subsystem: agent-loop
specs: [SPEC-001, SPEC-002, SPEC-010]
---

# Agent loop, tool calling and the run pipeline

How a run executes: loop strategies, tool-call parsing, completion and recovery, stuck detection, the one run pipeline shared by chat, automation and helper sub-agents.

Start here, then open only what the task needs.

| | |
|---|---|
| **Contract (current behavior)** | [SPEC-001](../SPECS/agent-loop.md), [SPEC-002](../SPECS/tool-call-parser.md), [SPEC-010](../SPECS/agent-loop-strategies.md) |
| **Code** | `backend/internal/core/assistant/`, `backend/internal/core/run/`, `backend/internal/core/chat/`, `backend/internal/core/delegation/`, `backend/internal/core/proxy/tool_call_parser.go` |
| **Architecture** | [map](../architecture/map.md) · [contracts](../architecture/contracts.md) · [checklists](../architecture/checklists.md) · [pitfalls](../architecture/pitfalls.md) — search pitfalls for: sieve, stuck, completion, tool_call, history, prefill |
| **Decisions (why)** | [ADR-0003](../decisions/ADR-0003-natural-completion.md) Completion is a content-only final answer<br>[ADR-0006](../decisions/ADR-0006-cloud-helper-sub-agents.md) Helper sub-agents run in the cloud, depth 1, from their own pool |
| **Open plans (next)** | [`agent-loop/agent-improvements.md`](../PLANS/agent-loop/agent-improvements.md)<br>[`agent-loop/large-result-storage.md`](../PLANS/agent-loop/large-result-storage.md)<br>[`agent-loop/run-efficiency-and-convergence.md`](../PLANS/agent-loop/run-efficiency-and-convergence.md)<br>[`agent-loop/multi-agent-delegation.md`](../PLANS/agent-loop/multi-agent-delegation.md)<br>[`cross-cutting/tool-call-grammar-reenable.md`](../PLANS/cross-cutting/tool-call-grammar-reenable.md)<br>[`cross-cutting/assistant-liveness-heartbeat-package-split.md`](../PLANS/cross-cutting/assistant-liveness-heartbeat-package-split.md) |
| **Learnings (tried before)** | [`learnings/agent-loop.md`](../learnings/agent-loop.md) — read before proposing an approach |
| **Evidence (audits, evals)** | [`2026-06-26-stale-turn-bleed.md`](../audits/2026-06-26-stale-turn-bleed.md)<br>[`2026-07-06-assistant-debug-cycle.md`](../audits/2026-07-06-assistant-debug-cycle.md)<br>[`2026-08-30-llm-smoke-test-incomplete-run.md`](../audits/2026-08-30-llm-smoke-test-incomplete-run.md)<br>[`2026-10-22-architecture-review.md`](../audits/2026-10-22-architecture-review.md)<br>[`2026-10-22-multi-agent-research.md`](../audits/2026-10-22-multi-agent-research.md)<br>[`2026-10-23-delegation-eval.md`](../audits/2026-10-23-delegation-eval.md)<br>[`agent-stability-report.md`](../audits/agent-stability-report.md)<br>[`degenerate-stream-repetition-guard.md`](../audits/degenerate-stream-repetition-guard.md)<br>[`ephemeral-turn-context-failed-run.md`](../audits/ephemeral-turn-context-failed-run.md)<br>[`write-file-truncation-cycles.md`](../audits/write-file-truncation-cycles.md) |
| **Skills** | [`agent-loop`](../../.agents/skills/agent-loop/SKILL.md), [`debugging`](../../.agents/skills/debugging/SKILL.md), [`testing-guide`](../../.agents/skills/testing-guide/SKILL.md), [`event-streaming-patterns`](../../.agents/skills/event-streaming-patterns/SKILL.md), [`lifecycle-events`](../../.agents/skills/lifecycle-events/SKILL.md) |
| **Guides** | [`guides/loop-strategy.md`](../guides/loop-strategy.md) |
| **Archived plans** | [`ARCHIVE/agent-loop/`](../PLANS/ARCHIVE/agent-loop/); others by topic in [`ARCHIVE/README.md`](../PLANS/ARCHIVE/README.md) |
