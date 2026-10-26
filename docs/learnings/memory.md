---
subsystem: memory
last_updated: 2026-10-26
---

# Learnings — Memory system (SPEC-004)

What was tried and what happened. Read before proposing an approach in this area; add a line when a run,
eval, audit or merged phase teaches something (newest last within each section). Format and rules:
[`README.md`](README.md).

## Dead ends — do not retry without new evidence

- **Tried:** An LLM step that rewrites the task with memory gates before the run → **77–89 s added to every automation run (its prompt never hits the cache)**. **Lesson:** injected memories plus prompt guidance already worked; removed. Evidence: [remove-memory-rewriter.md](../audits/remove-memory-rewriter.md).
- **Tried:** Annotating every task line with matching memories → **46 annotations → sieve fired on turn 1 → repeated steps**. **Lesson:** cap at 5 and require a shared non-stop-word. Evidence: [memory-injection-investigation.md](../audits/memory-injection-investigation.md).
- **Tried:** Prefill or "check memory first" nags to make a model skip an explicit step → **does not beat an explicit instruction**. **Lesson:** change the instruction, not the nag. Evidence: [small-context-memory.md](../PLANS/ARCHIVE/memory/small-context-memory.md).
- **Tried:** Counters and meters in the prompt → **cost tokens, no behavior gain**. **Lesson:** show them in the UI. Evidence: [small-context-memory.md](../PLANS/ARCHIVE/memory/small-context-memory.md).

## Root causes and fixes that held

- **Tried:** `context_budget` at 2 chars/token → **the sieve pruned history at ~1/3 of the real window**. **Lesson:** use 4 chars/token, scaled by the server's own token counts. Evidence: [memory-injection-investigation.md](../audits/memory-injection-investigation.md), [request-size-estimator.md](../PLANS/ARCHIVE/agent-loop/request-size-estimator.md).
- **Tried:** Hot memory injected once, or at a moving position each turn → **later turns lost it, or the KV cache was invalidated every turn**. **Lesson:** per-run snapshot in the head system message. Evidence: [2026-09-30-platform-scan.md](../audits/2026-09-30-platform-scan.md), [small-context-memory.md](../PLANS/ARCHIVE/memory/small-context-memory.md).
- **Tried:** Passing the model's search text straight to FTS5 → **operator words (`OR`) caused syntax errors**. **Lesson:** sanitize FTS5 operator keywords. Evidence: [remove-memory-rewriter.md](../audits/remove-memory-rewriter.md).
- **Tried:** The `memory_update` contract (2026-10-15 review) → **scope/mode silently reset when `keep` is omitted; overlapping facts concatenated; no way to forget**. **Lesson:** tracked as the memory tool contract work. Evidence: [2026-10-15-next-work-analysis.md](../audits/2026-10-15-next-work-analysis.md), [memory-gaps-plan.md](../PLANS/memory/memory-gaps-plan.md).
