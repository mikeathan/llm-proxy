# Request size estimator — one token-based measure of the prompt, corrected by the server

**Status:** proposed (2026-10-18) — approved in principle by the user; to be built on a branch off `main` after `profiles-followups` merges.
**Related:** SPEC-001 (Agent Loop: sieves, recovery), SPEC-005 / `orchestrator.md` (budget policy), SPEC-004 (memory shares of `ContextBudget`).
**Origin:** vertex, 2026-10-17 23:00: the `daily-llm-news` automation on Qwen3.6 35B (16,384-token window) was rejected twice with HTTP 400 `request (16401 tokens) exceeds the available context size (16384 tokens)` (and 16390), then hit the 30-minute run deadline. The two earlier midnight runs (2.4 and 5.3 minutes) succeeded. Reviewed by the Opus advisor.

## 1. Problem

Request size is estimated in several places, each in its own unit with its own blind spots, and none is checked against what the server counted.

| # | Finding | Where |
|---|---------|-------|
| 1 | `preparedOverContextBudget` summed message text only. Native tool schemas travel in the request's `tools` field (~20 KB of manifests, about 5k tokens) and are rendered into the prompt by the server's chat template, so they were never counted. | `assistant/sieve.go` |
| 2 | The physical sieve judged raw history against the whole budget, while the decision to run it used the prepared size. When the fixed overhead (system prompt, schemas, memory) tipped the request over, the sieve ran and pruned nothing. | `sieve.go` `physicalSieve.Sieve` |
| 3 | With few messages left, nothing could be dropped, so a handful of large fetched pages stayed in the request. | `sieve.go` |
| 4 | A 400 on the stream was treated as "streaming not supported" and the identical prompt was re-sent non-streaming (a stream 400 and a chat 400 one second apart in the log). | `stream.go` `computeNextResponse` |
| 5 | The budget converts tokens to characters with a fixed 4.0 chars/token; URLs, HTML and JSON tokenize denser. | `orchestrator/budget_policy.go` `localContextCharsPerToken` |
| 6 | `CloudBudgetPolicy` stores `publishedCtx - maxTokens` (tokens) in `ContextBudget` (characters), so cloud history budgets are about 4× too small. | `orchestrator/budget_policy.go` |
| 7 | `doPreflightCheck` measures raw `historyChars(history)`, a different number from the sieve's prepared size. | `stream.go` |
| 8 | Two marker lists decide "context overflow" and disagree. | `failures/failure.go` `isContextOverflowBody`, `failures/errors.go` `IsContextSizeError` |
| 9 | Why #87 made it appear: before it, a cold-start 503 on the tool probe dropped Qwen into XML mode (manual in the system prompt, which was counted); #87 waits the model out, so the run used native tools whose schemas were not counted. (Inferred from the #87 commit message and merge time; the XML/native mode of the earlier runs is not verified.) |  |

## 2. Done as a stopgap (uncommitted when this plan was written)

Schema counting in the prepared size (`toolSchemaChars`), the sieve target reduced by the request's overhead, `fitToBudget` (equal-share cut), and the no-non-stream-fallback on a context-size 400, with tests. These are kept except `fitToBudget`, which is reworked (§3.5).

## 3. Design

1. **One estimator** that takes the exact prepared request (messages, native `tools` JSON, tool-call arguments) and returns estimated prompt tokens. The sieve decision, the sieve target, preflight and the recovery all call it.
2. **Learn the ratio from the server.** After each response, ratio = characters sent ÷ reported `prompt_tokens`, kept per model in memory. Before the first measurement use a conservative default plus about 10% margin. Reported usage already reaches the tracker (`stream.go` `AddReportedUsage`). To confirm first: vertex's Qwen returns the total prompt tokens (with `stream_options.include_usage`), not only the uncached part (`timings.prompt_n` may exclude cached tokens).
3. **Typed overflow error at the provider edge**, like `providers.ParseOutputCapError`: `ContextOverflowError{PromptTokens, WindowTokens}`. llama.cpp supplies `n_prompt_tokens` and `n_ctx`; OpenAI- and Anthropic-style messages carry both numbers in text; with no numbers the type still applies. It replaces the two marker lists on this path. The error's window is the slot's real window and wins over the configured one (`--parallel` can make it smaller than `--ctx-size`).
4. **One-step recovery.** Target characters = sent characters × (window − output reserve − margin) ÷ reported prompt tokens; without numbers, about 70% of what was sent. Keep the retry cap. If the part that cannot be cut (system prompt, tools, task) is already over the window, fail the run at once and name the large parts instead of looping.
5. **Cut policy.** Cut oldest and largest first (content, reasoning and tool-call arguments); keep the locked head and the newest tool result whole unless it alone is over; fail fast as in (4) when nothing more can be cut. Replaces `fitToBudget`, whose 500-character floor, ignored arguments and equal share (it truncates the newest result and invites a re-fetch loop) do not guarantee a fit.
6. **Budget units.** `ContextBudget` becomes tokens (or the estimator converts at one place); fix `CloudBudgetPolicy` (finding 6) and the shares in SPEC-004 that derive from it (hot memory, recall, ledger).

## 4. Acceptance

- Unit tests: estimator counts schemas and arguments; typed error parsed from the llama.cpp, OpenAI and Anthropic bodies; recovery reaches a fit in one step; fail-fast when the fixed part alone is over; newest tool result kept whole.
- Real runs through the scratch backend: `llm-ai-release-brief.md` on vertex's Qwen at the 16k window and one deepseek case, logging estimated against reported tokens every turn. Pass: zero 400s and estimates within the margin.
- SPEC-001, `orchestrator.md` and SPEC-004 updated; `docs/PLANS/README.md` row moved to complete.

## 5. Not covered here — the 17 minutes

Nothing above explains why the failed run was still working 17 minutes after the model started (the log is silent between 23:00:03 and 23:17:47, then 9.5 minutes between the two 400s). The run's console (`20261017T230003Z_e9811b2e84c03eec`) is needed first. Facts verified in code that bound a silent wait: local streams are exempt from `streamFirstTokenTimeout`; the response-header timeout is 10 minutes (`proxy/client.go`); the in-request "model starting" wait is bounded only by the 10-minute turn timeout (`AgentTurnTimeout`), while the executor's own wait uses the 5-minute `ModelStartWaitTimeout`; repeats are logged at Debug. Stuck detection changes wait for that timeline.

## Remaining Work

Everything in §3 and §4; the stuck-detection question in §5.
