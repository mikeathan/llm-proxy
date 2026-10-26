# Agent Loop — Execution, Sieve, Stuck Detection & Fallback — Stuck and Repetition Detection

Reference for the [`agent-loop`](../SKILL.md) skill, loaded on demand. A run loops, repeats a tool call, or is wrongly declared stuck.

## Stuck Detection

**Reasoning stuck (length ceiling):** When reasoning content exceeds `maxTokens * 2` chars (= `max_tokens / 2` tokens at 4 chars/token; floor 2000), the stream is aborted. A `lifecycle` event with phase `stuck_detected` is emitted. The same ceiling applies to every model: there is no budget-based early cut (the old `reasoningBudget == 0` branch compared characters with a token count and aborted thinking Qwen at ~2K tokens, wasting ~80 s per abort).

**Reasoning-loop guard (`reasoning_repetition.go`):** while the stream is only thinking, once per 1 KB of growth the last 4 KB (whitespace collapsed, digits as written) are scanned: if ≥ half of the 64-byte windows already occurred earlier in the tail, abort into `[stuck]` recovery (`stuck_detected`, `reason: reasoning_repetition`). Judges content, not length, so loops stop in ~3K chars; on real thinking the top score was 0.06 vs the 0.5 threshold. Stays on when `SkipStuckCheck` is set. Paraphrased, count-up or long-period loops fall back to the ceiling / duration cap; digits are not normalised so drafted CSV / tables / checklists are not flagged.

Note: workloads classified local (a llama.cpp server run directly, or one behind an OpenAI-style URL identified by its listing fingerprint, SPEC-005) auto-derive a think-token budget from `max_tokens` (`resolveReasoningSpec(provider, workload, …)` → `DefaultReasoningBudget`, `max_tokens/3`); the derivation is from workload and context size, never the model name.

**Detector stop in chat answers instead of failing:** when `rd.Check`, `CheckAlternating`, `CheckSequenceRepeat` or `CheckSameTarget` fires on the assistant channel and the run has a tool result, `stopOnLoop` runs one tools-off finalization turn (`prompts.LoopStopFinalizePrompt`) and returns the model's answer; automations, runs with no tool work and a model that will not answer keep the `LoopError`.

**Progress monitor (`progress/`, chat only):** judges read-only results by how much of them is new to the run (word 4-grams of values), stricter for a call that rewords an earlier one of the same tool; streaks are per tool; any non-read-only call resets. Two stale results of one tool append one factual `[progress note: ...]` line to the newest tool result, `StrongAfter` (MaxSteps/6, clamped 4..6) a second; each once per streak. Never replaces the sieve warning; automations are not watched. Wired in `Agent.progressNote` (`tool_exec.go`).

**Empty tool_call spiral:** When pure-reasoning stream (no content, no native tool deltas) has ≥`emptyToolCallSpiralLimit` (3) closed empty `<tool_call></tool_call>` blocks, stuck fires immediately — same lifecycle + nag recovery as char-threshold stuck. Does **not** kill the run. Dangling open tags (still forming a real call) are not counted. Catches Qwen 3.5 empty-tag loops in ~1s instead of waiting for the char threshold (~19s). See `countEmptyClosedToolCalls()`.

**Content-level repetition guard:** Catches degenerate loops that write visible content with no tool calls and no progress (e.g. a model echoing a malformed tool-call dialect as ~190 repeated closing tags — the deepseek-v4-flash workspace-health-test incident). When `Content` is dominated by verbatim repeats (a single 60+ char window or repeated line covering ≥50% of the text, minimum fragment 400 chars, fail-open) **and** zero native tool calls are parsed, the stream aborts into the same `[stuck]` recovery as char-threshold stuck. Model/provider-agnostic — keys purely off the streamed bytes, independent of grammar/tool format. Real tool calls are never discarded (guard requires zero parsed calls). See `isRepetitionDominated()` (Hermes Agent `repetition_guard` port).

**Per-stream duration cap:** A stream producing no native tool calls and no natural completion beyond `streamMaxDuration` (default 5 min, test-shortenable) is terminated; the clock starts at the **first token**, not the request (prompt processing on a busy local server can take a minute); the wait for the first token of a cloud stream is bounded separately (`streamFirstTokenTimeout`, 3 min; local workloads exempt). Content is preserved (not cleared) so the partial turn is evaluated/salvaged.

**Local thinking budget and recovery:** a local workload sends `thinking_budget_tokens` (`max_tokens/3` unless `reasoning_budget` is set) together with `reasoning_budget_message` (`prompts.ThinkBudgetWrapUp`) so the server closes the thought cleanly at the budget instead of letting the reasoning spill into the visible reply. The server appends it on ANY turn that hits the budget, so it must never say the answer is next — a run skipped its playbook's `memory_update` that way (2026-10-07). After a stream is aborted as stuck (ceiling or loop guard), `Agent.noteStuckThinking` makes the next request ask for less reasoning once (`reasoning.StepDown`, for every workload): a local model answers without thinking (`enable_thinking=false`), an effort model drops one level and never to none, an object model lowers its effort, and an on/off model is left alone — a cloud model's reasoning is never switched off. It fires only where the guard reads reasoning text (`reasoning_content`): local models and effort-mode endpoints that stream it; OpenRouter's `reasoning` field and OpenAI/Gemini's opaque reasoning are not read. The think budget is derived from the output cap, so it is only as good as that cap: a stale cap (the GGUF training window) gave a 29k-token budget and the 2026-10-21 runaway loop; a context overflow that reports the window re-derives it (`lowerThinkBudgetToOutputCap`, never raising it, never touching a configured budget).

**Progressive sieve recovery:**
- 1st stuck → reactive sieve (first 2 + last 6 messages) + nag prompt
- 2nd consecutive stuck → aggressive sieve (first 2 + last 3 messages) + stronger nag
- 3rd consecutive stuck → agent fails with clear error

**Stuck detection is skipped on XML fallback retries** (the `SkipStuckCheck` config flag).

## Repetition/Spiral Detector

`repetitionDetector.check()` in `agent.go`:

1. **Exact duplicate args** — streak ≥ 3 → aborts with "infinite loop"
2. **Single-tool spiral** — 12+ consecutive calls to the same tool that **recycle ≤4 distinct argument values** → aborts with "spiral detected". A burst of varied-argument calls to one tool (e.g. an audit automation running a sequence of distinct `execute_terminal_command` calls) is legitimate batching (Constitution II.1), not a spiral — identical-argument repeats are caught earlier by the duplicate detector.
3. Streak < 3 → injects AutomationDuplicateNagPrompt and continues

**Sequence-repeat (n-gram) detector** — `checkSequenceRepeat()`: catches
repeating tool-call cycles of length 3–5 in the last 30 calls. Keys are
**name + args** (full `toolKey`), so a run that legitimately uses one tool
with varying arguments (e.g. a series of distinct `execute_terminal_command`
calls in a storage audit) is never flagged as a cycle; true degenerate cycles
recur with identical arguments and are caught after 3 repeats. Single-tool
stagnation with varied args is still bounded by the 12-consecutive spiral
detector. `checkAlternating()` catches 2-tool oscillation (≤30% unique
(tool, args) calls over 20 calls for 15+ turns), and `checkSameTarget()` catches
many-tools-one-path oscillation within a turn.
