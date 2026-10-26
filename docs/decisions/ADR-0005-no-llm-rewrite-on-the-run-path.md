---
id: ADR-0005
title: No extra LLM call on the run path to apply memory
status: accepted
date: 2026-06-03
subsystems: [memory]
supersedes:
superseded_by:
---

# ADR-0005: No extra LLM call on the run path to apply memory

_Recorded 2026-10-26 from the plan and evidence below; the decision date is when it was taken._

## Context
An LLM call rewrote each automation task with memory-check gates before the run. Its unique prompt never hit the llama.cpp cache, adding 77–89 s per run, and injected memories with prompt guidance already produced the wanted behavior.

## Decision
We will apply memory by injecting it into the prompt (and through the memory tools), never by an extra LLM rewrite or summarize step before or inside the run path.

## Consequences
Run start latency stays bounded by the run's own requests. Any future memory feature that needs an LLM pass runs offline (for example nightly consolidation with reviewable proposals), not in front of a run.

## Alternatives considered
- **Keep the rewriter but cache it** — still one extra generation per run for no measured gain.

## Links
- [`audits/remove-memory-rewriter.md`](../audits/remove-memory-rewriter.md)
- [`ARCHIVE/memory/small-context-memory.md`](../PLANS/ARCHIVE/memory/small-context-memory.md) ("What not to do")
- [`learnings/memory.md`](../learnings/memory.md)
