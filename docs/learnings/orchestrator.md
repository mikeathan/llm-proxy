---
subsystem: orchestrator
last_updated: 2026-10-26
---

# Learnings — Inference, budgets, providers and performance (SPEC-005)

What was tried and what happened. Read before proposing an approach in this area; add a line when a run,
eval, audit or merged phase teaches something (newest last within each section). Format and rules:
[`README.md`](README.md).

## Dead ends — do not retry without new evidence

- **Tried:** Non-streaming fallback on the cloud transport for a remote llama-server → **died at the 45 s header timeout**. **Lesson:** classify local vs cloud by endpoint or `.gguf`, not by transport. Evidence: [local-and-cloud-inference-performance.md](../PLANS/orchestrator/local-and-cloud-inference-performance.md).
- **Tried:** K-cache `f16 → q8_0` on the remote server → **likely aggravated a repetition loop**. **Lesson:** change server flags one at a time against a recorded baseline. Evidence: [local-and-cloud-inference-performance.md](../PLANS/orchestrator/local-and-cloud-inference-performance.md).

## Root causes and fixes that held

- **Tried:** Qwen chat template without `preserve_thinking` → **earlier `<think>` blocks were dropped, so every turn re-processed the conversation**. **Lesson:** send `preserve_thinking: true` to local llama.cpp: turn-2 prefill 17.3 s → 0.8 s. Evidence: [2026-10-09-local-prompt-cache.md](../audits/2026-10-09-local-prompt-cache.md).
- **Tried:** Routing the remote llama through the proxy (gateway) → **the idle reaper measures real use**. **Lesson:** keep; do not bypass the proxy. Evidence: [local-and-cloud-inference-performance.md](../PLANS/orchestrator/local-and-cloud-inference-performance.md).
- **Tried:** Reading SSE one byte per syscall → **streaming bottleneck**. **Lesson:** `bufio` + `ReadBytes('\n')`, then strip `\r\n` (do not regress: the equality check on `[DONE]`). Evidence: [known-performance-findings.md](../audits/known-performance-findings.md).
- **Tried:** A run whose upstream model was stopped by the idle reaper → **retries burned, truncated `<tool_call>` salvaged as the answer, UI stuck**. **Lesson:** treat salvaged markup as a failure and always give the UI a terminal state. Evidence: [2026-08-28-ops-performance-review.md](../audits/2026-08-28-ops-performance-review.md).
