# Local prompt cache: why every chat turn re-processed the conversation (2026-10-09)

Phase 0 finding for [`../PLANS/orchestrator/local-and-cloud-inference-performance.md`](../PLANS/orchestrator/local-and-cloud-inference-performance.md).
Server: vertex, llama.cpp build `b10721-8e53fcefd`, `Qwen3.6 35B A3B`, `--ctx-size 16384`, `--parallel 1`,
prompt processing ≈ 270 tokens/s. All numbers come from llama-server's own `timings` (`prompt_n` = tokens processed,
`cache_n` = tokens reused from the cache, `prompt_ms`).

## Symptom

Within one turn, each agent step reused the previous step's prompt (step 3: 2175 reused, 1375 new). Across turns,
only the system prompt and the first user message (~1445 tokens) were reused: turn 2 of a chat re-processed every
earlier tool call and tool result. In a two-turn test with one 3k-token tool result, turn 2 processed 4731 tokens
in 17.3 s.

## Cause (proved, not inferred)

Rendering the same messages with the server's template (`POST /apply-template`) shows the prompt changing mid-way
between turns. Qwen's template puts a `<think>…</think>` block on assistant turns after the last user message — empty
if no reasoning is sent back — and drops it from those turns once a newer user message exists:

```
turn 1: …<|im_start|>assistant\n<think>\n\n</think>\n\n<tool_call>…
turn 2: …<|im_start|>assistant\n<tool_call>…            ← first difference, 34% into turn 1
```

Everything after the first difference is outside llama.cpp's prefix cache, so each new turn re-processes the previous
turn's assistant messages and tool results. Sending reasoning back or not makes no difference; within a turn the
prompt stays append-only, which matches the step measurements. This is the template, not our message handling.

## Fix

The template reads `chat_template_kwargs.preserve_thinking`. With it set, earlier turns keep their `<think>` block and
the prompt only grows. The local llama.cpp client now sends `preserve_thinking: true` on every request
(`proxy.LLMClient.withTemplateKwargs`, merged into whatever the reasoning resolver set; cloud requests never carry it;
templates that do not use it ignore it). `ChatTemplateKwargs.EnableThinking` became optional so the kwargs object can
carry `preserve_thinking` alone without also sending `enable_thinking: false`.

## Measurements

Direct to llama-server, fixed prompt, two repeats each (turn 1 = system + user + tool call + 3k-token tool result;
turn 2 = turn 1 + answer + new question):

| | turn 2 processed | turn 2 reused | turn 2 prompt time |
|---|---|---|---|
| default template | 4731 | 1445 | 17.3 s |
| `preserve_thinking: true` | 31 | 6153 | 0.83 s |

End to end through the app on v0.8.1 (before the fix), two-turn chat where turn 1 reads a 400-line file, two runs:
turn 1 23.1 s / 23.9 s, turn 2 **8.1 s / 8.0 s**. The same benchmark is re-run after the fix is deployed; the result is
recorded here.

## Not covered

- Other local model families: the switch is harmless where unused; a template that breaks the prefix another way needs
  its own `/apply-template` check.
- Conversation switching (one slot, alternating chats) and sieve compression still re-process; plan Phase 1 items 2–4.
- Cloud time-to-first-token and timings capture in run records: plan Phase 0 items 1–2, still open.
