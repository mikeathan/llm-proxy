---
id: ADR-0006
title: Helper sub-agents run in the cloud, depth 1, from their own pool
status: accepted
date: 2026-10-22
subsystems: [agent-loop]
supersedes:
superseded_by:
---

# ADR-0006: Helper sub-agents run in the cloud, depth 1, from their own pool

_Recorded 2026-10-26 from the plan and evidence below; the decision date is when it was taken._

## Context
A 16k local parent model overflows on breadth research. Running helpers on the same local GPU would split its context and queue behind the parent; sharing the cloud run lane with the parent can deadlock or cap fan-out.

## Decision
We will run helper sub-agents (`delegate_task`) in-run only, depth 1, each on its own cloud model, from a separate pool (global cap 3, 3 per turn), with at most 15 steps and a returned summary of ~1.5k tokens. Helpers get only tools that never need approval (web search and page fetch, no file access). A profile lists the helper types it may use and the parent model decides when; the `general` profile does not delegate.

## Consequences
The B7 eval kept helpers: on breadth tasks parent prompt tokens fell from 37k to 14k (mean) and runs over 16k from 15 to 3, at the cost of ~50 s more wall time and helper cloud tokens. Cloud requests in flight can exceed `cloud_concurrency` by the pool size. Local helpers (cloud → local) are a later opt-in.

## Alternatives considered
- **Shared cloud lane with a bounded wait** — turns deadlock into timeouts and caps fan-out.
- **Local helpers on the same GPU** — splits the parent's context window.

## Links
- [`audits/2026-10-22-multi-agent-research.md`](../audits/2026-10-22-multi-agent-research.md) §9 (D1–D7)
- [`audits/2026-10-23-delegation-eval.md`](../audits/2026-10-23-delegation-eval.md)
- [`PLANS/agent-loop/multi-agent-delegation.md`](../PLANS/agent-loop/multi-agent-delegation.md)
