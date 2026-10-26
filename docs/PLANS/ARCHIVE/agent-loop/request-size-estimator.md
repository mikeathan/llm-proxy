# Request size estimator — one measure of the prompt, scaled by what the server counts

**Status:** complete (2026-10-18) — built on `request-size-estimator`; not yet re-run on vertex's cold-start automation.
**Related:** SPEC-001 1.7 (Agent Loop: sieves, recovery), SPEC-005 1.4 (`TokenRatios`).
**Origin:** vertex, 2026-10-17 23:00: the `daily-llm-news` automation on Qwen3.6 35B (16,384-token window) was rejected twice with HTTP 400 `request (16401 tokens) exceeds the available context size (16384 tokens)` (and 16390), then hit the 30-minute run deadline. The two earlier midnight runs (2.4 and 5.3 minutes) had succeeded. Reviewed by the Opus advisor (twice).

## 1. What was wrong (verified)

| # | Finding | Where |
|---|---------|-------|
| 1 | The size check summed message text only. The native tool schemas travel in the request's `tools` field and the server renders them into the prompt; for the twelve built-in tools that is **6,316 characters = 1,758 tokens** on Qwen3.6 (measured by a real call). Vertex's own set, with search and MCP tools, is somewhat larger. (An earlier note here said "about 20 KB, 5k tokens": that was the raw manifest files, which also hold guardrail settings that are never sent.) | `assistant/sieve.go` before this change |
| 2 | The budget turns the window into characters at a fixed 4.0 chars/token. Measured on Qwen3.6: prose 4.1, but JSON search results **2.76**, raw HTML **2.68 to 3.36**, URL-heavy text 3.2. A history of tool results at 2.8 puts 43,692 characters (the 16k budget) at about 15.6k tokens before any overhead. This is the dominant cause. | `orchestrator/budget_policy.go` |
| 3 | The sieve decided on the prepared size but pruned against the whole budget, so when overhead tipped the request over it ran and pruned nothing. With few messages left nothing could be dropped even when each was large. | `sieve.go` |
| 4 | A 400 on the stream was treated as "streaming not supported" and the identical prompt re-sent non-streaming (a stream 400 and a chat 400 one second apart in the log). | `stream.go` |
| 5 | Recovery kept a fixed tail (6, then 3 messages) and retried up to three times, with no knowledge of how far over the request was; the nag it adds talks about a reasoning loop. | `session.go` |
| 6 | Two marker lists decided "context overflow" and disagreed. | `failures/failure.go`, `failures/errors.go` |
| 7 | Local runs recorded no prompt tokens: llama.cpp puts counts in `timings` on its final chunk, and in `usage` only when asked; the agent read `usage` only. | `models/llm_messages.go` |
| 8 | Why #87 made it appear: before it, a cold-start 503 on the tool probe dropped Qwen into XML mode, where the tool manual sits in the system prompt (counted). #87 waits the model out, so the run used native tools whose schemas were not counted. Inferred from the #87 commit message and merge time; the XML/native mode of the earlier runs was not verified. | #87 |

Not a cause, left alone by decision: `CloudBudgetPolicy` stores `publishedCtx - maxTokens` in `ContextBudget`. On cloud, `ContextBudget` is a history allowance chosen for cost (SPEC-005 §3 tier rows, 20 to 50K characters), not a window; making it 4× larger would change cost and latency. **Open decision for the owner.**

## 2. Design (built)

- **One measure.** `measureRequest` sizes the prepared messages plus, for a local native-tools request, the tool schemas. Cloud is measured by messages alone, the basis its allowance is stated on.
- **Local budget scaled by the learned ratio.** Limit = `ContextBudget` × (learned chars per token) ÷ 4.0. `orchestrator.TokenRatios` (on `Orchestrator.Ratios`) learns the ratio per model from the server's own prompt count (`timings.cache_n + prompt_n` on llama.cpp, `usage` elsewhere), default 3.0 before the first observation, blended 50/50, clamped 1.5 to 6.0, at most 256 models, nil-safe. No persisted value, API or UI changed unit. Cloud is not scaled and not learned.
- **Typed overflow error** `providers.ContextOverflowError{PromptTokens, WindowTokens}` at the HTTP edge (llama.cpp's `n_prompt_tokens`/`n_ctx`; OpenAI, Anthropic, Gemini prose), still an `LLMHTTPError`, never retried, one marker list (`providers.IsContextOverflowBody`).
- **One-step recovery** for a local model: target = sent characters × (window − output reserve) ÷ reported prompt tokens, less the request's overhead; the rejection also teaches the ratio. When the part that cannot be cut fills the window the run ends with `models.ErrPromptTooLarge`. Without counts, or on cloud, the old fixed-tail sieves apply.
- **Cut policy** `shrinkHistory`: oldest first to a 256-character floor; never the locked head, the sieve's own messages or tool-call arguments (kept valid JSON, still counted); the newest message last.

## 3. Not done, on purpose

- No change to `ContextBudget`'s unit or to `CloudBudgetPolicy` (§1, last paragraph).
- No `stream_options.include_usage`: llama.cpp's `timings` already carry the counts, and cloud is not calibrated.
- The request is still prepared twice per turn (once to measure, once to send). Measured: sizing costs 22 µs and 64 KB per turn (`BenchmarkMeasureRequest`) against LLM calls of seconds, so it was not restructured.
- Cloud typed overflows use the old recovery; the counts exist on the error if a cloud window is ever wanted.
- The 17-minute question (§4).

## 4. The 17 minutes — still open

The log is silent between 23:00:03 and 23:17:47, then 9.5 minutes between the two 400s. Prefill is not it (about 287 tokens a second, so a full 16k prompt is about a minute). Facts verified in code that bound a silent wait: local streams are exempt from `streamFirstTokenTimeout`; the response-header timeout is 10 minutes (`proxy/client.go`); the in-request "model starting" wait is bounded only by the 10-minute turn timeout (`AgentTurnTimeout`) while the executor's own wait uses the 5-minute `ModelStartWaitTimeout`; repeats are logged at Debug. The failed run's console (`20261017T230003Z_e9811b2e84c03eec`) is needed before changing any timeout or progress detection.

## Verification

Unit and integration tests in `request_size_test.go`, `token_ratios_test.go`, `context_overflow_error_test.go`, `client_test.go`, `errors_test.go`, `llm_messages_test.go`; `-race` on the orchestrator. Live: tool schemas, token ratios and the `timings` fields were measured against vertex's Qwen3.6 (2026-10-18). The manual `daily-llm-news` run on vertex completed in 3m19s on the earlier stopgap; tonight's cold-start run is the real test of this build.

## Remaining Work

Re-run vertex's cold-start automation on this build; the cloud `ContextBudget` decision; the 17-minute question.
