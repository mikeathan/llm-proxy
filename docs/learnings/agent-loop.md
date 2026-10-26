---
subsystem: agent-loop
last_updated: 2026-10-26
---

# Learnings — Agent loop, tool calling and the run pipeline (SPEC-001, SPEC-002, SPEC-010)

What was tried and what happened. Read before proposing an approach in this area; add a line when a run,
eval, audit or merged phase teaches something (newest last within each section). Format and rules:
[`README.md`](README.md).

## Dead ends — do not retry without new evidence

- **Tried:** Ephemeral per-turn context message (`[Turn N | Tools used …] Proceed directly.`, as user then as system role) → **reverted: smoke test 370 s → 555 s, no gain in either role**. **Lesson:** the model's recap happens in hidden reasoning, so the injected text is pure overhead (~30 tokens/turn). Evidence: [ephemeral-turn-context-failed-run.md](../audits/ephemeral-turn-context-failed-run.md), [ephemeral-turn-context.md](../PLANS/ARCHIVE/agent-loop/ephemeral-turn-context.md).
- **Tried:** GBNF grammar in the same llama.cpp request as `tools` → **HTTP 400 "Cannot use custom grammar constraints with tools"**. **Lesson:** llama.cpp does not combine them; grammar is only usable on the XML text path. Evidence: [2026-07-06-assistant-debug-cycle.md](../audits/2026-07-06-assistant-debug-cycle.md), [tool-call-grammar-reenable.md](../PLANS/cross-cutting/tool-call-grammar-reenable.md).
- **Tried:** Content-length thresholds (500 / 1000 / 2000 / 2730 chars) to decide a turn is final → **rejected as fragile**. **Lesson:** the `no_tool_content_cap` at `maxTokens` chars is the least-bad bound. Evidence: [2026-07-06-assistant-debug-cycle.md](../audits/2026-07-06-assistant-debug-cycle.md).
- **Tried:** A 30 s time limit (heartbeat termination) on a turn → **rejected**. **Lesson:** real tasks (network scans, long reports) take longer; detect lack of progress instead. Evidence: [2026-07-06-assistant-debug-cycle.md](../audits/2026-07-06-assistant-debug-cycle.md).
- **Tried:** Removing the empty-tools-list check in `handleNoToolCalls` → **rejected**. **Lesson:** the agent would accept incomplete mid-task text as the final answer. Evidence: [2026-07-06-assistant-debug-cycle.md](../audits/2026-07-06-assistant-debug-cycle.md).
- **Tried:** Halving or quartering the reasoning budget, halving the stuck threshold → **planning cut-offs, recompile loops, false stuck positives**. **Lesson:** leave the divisors alone. Evidence: [local-and-cloud-inference-performance.md](../PLANS/orchestrator/local-and-cloud-inference-performance.md).

## Root causes and fixes that held

- **Tried:** Running the agent on `context.Background()` so a run survives the HTTP request → **caused stale-turn bleed: the orphan run kept writing into the next turn**. **Lesson:** a detached run needs an explicit cancel path; frontend `AbortController` does not stop backend work. Evidence: [2026-06-26-stale-turn-bleed.md](../audits/2026-06-26-stale-turn-bleed.md).
- **Tried:** Collapsing whitespace in terminal commands (`strings.Fields`) → **batched multi-line commands ran as one broken line, and the per-command whitelist could be bypassed**. **Lesson:** normalize per line, treat `\n` as a separator in both validation and execution. Evidence: [2026-08-30-llm-smoke-test-incomplete-run.md](../audits/2026-08-30-llm-smoke-test-incomplete-run.md).
- **Tried:** Accepting any ≥20-char content turn after tool use as final → **a truncated `Thought: … Action:` scaffold finished the run**. **Lesson:** reject a turn whose last line is a bare `Action:` marker, at every completion surface and in every loop strategy. Evidence: [2026-08-30-llm-smoke-test-incomplete-run.md](../audits/2026-08-30-llm-smoke-test-incomplete-run.md).
- **Tried:** A local model without `tool_call_format: native` → **fell back to XML text mode; months of parser patches treated the symptom**. **Lesson:** check the model's tool-call mode first when a local model misbehaves. Evidence: [2026-08-30-llm-smoke-test-incomplete-run.md](../audits/2026-08-30-llm-smoke-test-incomplete-run.md).
- **Tried:** Trusting the stream when a model invents a tool dialect → **`</konjll>` repeated ~189 times for ~49 s**. **Lesson:** repetition-dominance guard + 90 s per-stream cap that keeps content (provider-agnostic). Evidence: [degenerate-stream-repetition-guard.md](../audits/degenerate-stream-repetition-guard.md).
- **Tried:** `maxLength: 800` on `write_file` content in the tool schema → **llama.cpp/vLLM enforce it as grammar: silent truncation, endless rewrite loops**. **Lesson:** never put `maxLength` on free-text tool arguments. Evidence: [write-file-truncation-cycles.md](../audits/write-file-truncation-cycles.md).
- **Tried:** Fixing frontend `messageBuilder`/`turnGrouper` for bad chat output → **every frontend fix was reverted**. **Lesson:** the bugs were backend (history, SSE URL, duplicate append) — find the producing side first. Evidence: [2026-07-06-assistant-debug-cycle.md](../audits/2026-07-06-assistant-debug-cycle.md).
- **Tried:** Cloud helper sub-agents for a 16k local parent (B7 eval, 48 runs) → **breadth tasks: parent prompt tokens 37k → 14k mean, runs over 16k 15 → 3; wall time 139 s → 190 s; simple questions never delegated**. **Lesson:** kept; helpers cost ~0.4M cloud prompt tokens per breadth run. Evidence: [2026-10-23-delegation-eval.md](../audits/2026-10-23-delegation-eval.md).
- **Tried:** Each entry point (chat, automation, memory review) building its own run → **10 duplicated pieces; memory review did not wait for a cold model**. **Lesson:** one run pipeline (`run.ResolveOptions`, `core/chat`) before adding sub-agents. Evidence: [2026-10-22-architecture-review.md](../audits/2026-10-22-architecture-review.md), [run-pipeline-and-agent-commands.md](../PLANS/ARCHIVE/agent-loop/run-pipeline-and-agent-commands.md).
