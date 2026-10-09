---
status: active
date: 2026-09-30
last_reviewed: 2026-10-09
related_specs: [SPEC-001, SPEC-005, SPEC-007]
constitution_references: [II.5, IV.3, VI]
related_plans: [gpu-performance.md, cross-cutting/backend-hot-paths-and-leak-hardening.md, memory/small-context-memory.md, unattended-run-safety-hardening.md]
evidence: docs/audits/2026-09-30-platform-scan.md (P1, P9, P12)
---

# Inference Performance — Local llama.cpp and Cloud Providers

**Status:** active — Phase 0 started 2026-10-09: the cross-turn cache loss was measured, its cause proved (the chat template) and fixed (`preserve_thinking`), see [`../../audits/2026-10-09-local-prompt-cache.md`](../../audits/2026-10-09-local-prompt-cache.md). Items 0.1 (timings in run records), 0.2 (cloud TTFT) and 0.3 (prefix test) remain. **Phase 0 (measure) is mandatory and first.** The repo's own history (GPU plan,
"Round 6 lesson — measure first") is a run of well-meant changes that were later reverted or shown to be
noise. Nothing below ships on reasoning alone.

Backend-side costs that are *not* the model (session writes, history copies, list endpoints, leaks) live in
[`cross-cutting/backend-hot-paths-and-leak-hardening.md`](../cross-cutting/backend-hot-paths-and-leak-hardening.md).
Browser GPU/rendering stays in [`gpu-performance.md`](../gpu-performance.md).

## Ground truth (from the repo, not guesses)

| Fact | Source |
|---|---|
| Local reference hardware: Radeon 780M, Gemma-4 4B Q4_K_M, 8 192 ctx, `--parallel 1`: prefill ~500–650 tok/s cold, ~200–350 warm, generation ~20–22 tok/s, first-prompt eval 6–7 s cold → 0.3–0.6 s warm | `.agents/skills/llamacpp-setup/SKILL.md`; `memory-injection-investigation.md` §7 |
| "Slow assistant runs were provider TTFT, not our logic": the local loop is tight; the wait between `tool_result` and the next `"stream request sent"` log is upstream first-token time (NVIDIA free tier 17–40 s per call) | `docs/audits/known-performance-findings.md` §1 |
| Step count and reasoning budget are **not** the bottleneck and were deliberately left alone (user decision); `streamReasoningBudgetDivisor = 3` carries a "DO NOT CHANGE without the full smoke test" note | `known-performance-findings.md` §3; `core/assistant/stream.go:27-35` |
| A remote llama.cpp serving `.gguf` must use the local (10-min header) transport, not the 45 s cloud one; LLM chat is HTTP/1.1 only | architecture.md pitfalls 28, 33 |
| Lanes: `local_concurrency` 1, `cloud_concurrency` 3; a model swap is refused while a run uses it (residency gate) | SPEC-007 §V, §V.1 |
| `ChatRequest` has **no** `cache_prompt` or `id_slot` field; nothing in the backend reads llama.cpp's `timings` (`grep` of `internal/`) | `backend/models/llm_messages.go:57-74` (VERIFIED) |
| Sieve compresses mid-history messages in place and memory is inserted before the last user message — both change the prompt prefix | `core/assistant/sieve.go:54-56`, `stream.go:187-223` (VERIFIED) |
| Constitution §VI and SPEC-005's header refer to "slot persistence" defined in SPEC-005; the SPEC has no such section | `CONSTITUTION.md` §VI vs `docs/SPECS/orchestrator.md` (STALE) |

## What was tried before

| Attempt | Result | Lesson |
|---|---|---|
| Reasoning budget ÷2, ÷4; stuck threshold ÷2 | planning cut-offs, recompile loops, false stuck positives | leave the divisors alone |
| `context_budget` at `ctx × 2` chars/token | sieve fired at ~1/3 of the window | use the real 4 chars/token ratio (fixed) |
| Non-streaming fallback on the cloud transport | died at 45 s header timeout on remote llama | classify by endpoint **or** `.gguf` (fixed) |
| K-cache `f16 → q8_0` on the remote server | likely aggravated a repetition loop | change server flags one at a time, with a recorded baseline |
| Proxy-as-gateway for the remote llama | idle reaper now measures real use | keep; do not bypass the proxy |
| Sampling GPU per request | coupled to request volume | sample on a timer (done) |

## Phase 0 — Measure (no behaviour change; ~1 day)

1. **Capture what the server already tells us.** llama-server returns a `timings` object
   (prompt tokens processed vs `cache_n` reused, `prompt_ms`, `predicted_ms`, tok/s) on the final response/chunk.
   Parse it if present in `core/proxy/client.go` (tolerate absence), attach it to the existing usage record and
   write it to `run-meta.json` / `events.jsonl`. *ASSUMPTION: the field is present on the user's llama.cpp build —
   step one is `curl` a streaming request and look.*
   - Verify: `cd backend && go test ./internal/core/proxy/ -run Timings -count=1` (table test with and without the object).
2. **Cloud:** record time-to-first-byte (request start → first SSE chunk) and total per call, per provider/model.
   The "stream request sent" log already marks headers; add the first-chunk timestamp, not a new timer.
3. **Prefix-stability test** (deterministic, no server needed). Record the `ChatRequest.Messages` a fake client
   receives across a 10-turn run and assert each request's serialized prefix equals the previous request's full
   content (append-only), except at a sieve firing. Report the first differing byte index.
   - Verify: `cd backend && go test ./internal/core/assistant/ -run TestPromptPrefixStable -count=1` — expected
     **red today** at the memory insertion and at in-place sieve compression; the failure message is the work list.
4. **Baseline table** in `docs/audits/` (three rows: local chat, local automation, cloud chat × prompt tokens,
   cached tokens, prefill ms, TTFT, generation tok/s, total). Fixed prompt, three runs, median.

Acceptance: the four items exist; the baseline is committed; Phase 1 items are re-ranked by the numbers
(if cache reuse is already >90% on turns 2+, skip 1.2–1.4 and say so here).

## Phase 1 — Local: make the KV cache work for us

Order by expected payoff; each item is independent and gated on its Phase 0 number.

0. **Done 2026-10-09 — keep the template append-only.** The local client sends
   `chat_template_kwargs.preserve_thinking: true`; turn-2 prefill on the reference server dropped from 4731 tokens /
   17.3 s to 31 tokens / 0.83 s. llama.cpp already reuses the prompt cache by default (`cache_n` > 0 without
   `cache_prompt`), so item 1 is only needed if a server turns it off.
1. **Make reuse explicit.** Add `cache_prompt` (and `id_slot` when the model is served with `--parallel > 1`) to
   `ChatRequest` for local workloads only; omit for cloud (strict providers reject unknown keys — same reason
   `FinishReason` is `json:"-"`). Cloud providers that support automatic prefix caching get it from stability
   alone, see 1.2.
2. **Fix the prefix breaks the Phase 0 test finds.**
   - Memory: block appended to the system message from a per-run snapshot (plan `memory/small-context-memory.md` Phase 1.1).
   - Sieve: make compression idempotent and do it once per prune, not on every call
     (`sieve.go:54-56`); after the first prune, compressed messages are never re-edited.
   - Anything date/time-varying in the system prompt moves to the *end* of the prompt or is day-granular.
3. **Try `--cache-reuse`** on the llama-server for the reference model. It lets the server reuse KV chunks after
   a shifted prefix, which is exactly what a sieve prune or memory change produces. *ASSUMPTION about its
   effect on this model/template — measure against the Phase 0 baseline; keep only if prefill ms drops.*
   This is an operator-side flag: document it in `llamacpp-setup`, not in code.
4. **Conversation switching cost.** With one slot, alternating between two chats re-prefills the whole history each
   time (a 6 K-token history at ~300 tok/s is ~20 s). Experiment: llama-server `--slot-save-path` and
   `POST /slots/{id}?action=save|restore` keyed by conversation id, restored on resume. Constraints:
   KV files are large (hundreds of MB at 8K on a 4B; scale with context), invalid after a model/context change, and
   must live under the data root with retention. **Decision D-P1** below. This also gives Constitution §VI's
   "slot persistence" sentence a real home — or removes it.
5. **Tool-manual size in XML mode.** Measure tokens of the manual for the local tool set; if it is >10% of an 8K
   window, add a terse manifest description variant for local models (manifests are embedded JSON; tests exist).
6. **Warm start.** Measure model load time from cold; if material, add `warm_primary_on_start` (default off) to
   start the primary local model after the app root context is live (tethered, Constitution §II.2). Not needed if
   the idle timeout is `-1` as in the remote setup.

Verify for each: `cd backend && go test ./internal/core/proxy/... ./internal/core/assistant/... -count=1`
and the Phase 0 baseline re-run on the reference model (manual, recorded).

Acceptance: turn-2+ `prompt_n` (tokens actually processed) falls to roughly the new tokens only, and median
turn latency on the reference model improves ≥ 15% with no quality regression in the recorded smoke test
(`TestAgent_Execute_AgainstRecordings`). If an item misses its number, revert it and record the result in the
baseline doc — a negative result is a valid deliverable.

## Phase 2 — Cloud: measure, then remove waiting we control

Most cloud latency is provider TTFT (known finding). What we control:

1. **Fewer, fuller turns.** Count LLM calls per run by strategy; Constitution §II.1 already permits batched tool
   calls. If runs spend many calls on single `read_file`/`list_directory`, strengthen the tool manual's batching
   example (prompts live in `prompts/templates.go` only) and check `plan_execute` for read-heavy tasks. Measure
   calls/run before and after.
2. **Prefix caching via stability.** OpenAI-style and Gemini providers cache identical prefixes automatically;
   Phase 1.2 benefits them with no cloud-specific code. Record cached-token counts if the provider returns them.
3. **Say it is the provider.** The chat status strip should show "waiting for the model · 34 s" with the provider
   name when first-token time exceeds a threshold (data: Phase 0.2). No behaviour change, removes the "is it
   hung?" reports.
4. **No hedging or early fallback on slowness.** Constitution §IV.3: fallback only on terminal failures, not on
   transitional slowness. Do not add a timeout-based switch to the fallback model.
5. **Retry/backoff review** using the Phase 0 data: confirm `backoffForRetry` and the 45 s header timeout do not
   add dead time on the common 529/overloaded responses (NVIDIA incident).

Acceptance: a per-provider TTFT/total table exists; calls-per-run decreases on the read-heavy sample without a
completion-rate drop; the slow-provider hint appears and never appears on local runs.

## Phase 3 — Scheduling and estimation (small)

1. **ETA from history.** Show "usually ~N min" on a queued/running automation from the median of its last five
   completed runs (already in the activity ledger). SPEC-007 §V.1.1 notes "no ETA" for *inbound* callers because
   nothing can predict it; per-automation history can — scope the claim to automations.
2. **Lane policy check.** With the measured numbers, decide whether `cloud_concurrency: 3` and
   `local_concurrency: 1` are right defaults and whether scheduled runs on the cloud lane should never wait behind
   a local chat (they already use separate lanes; verify with a two-lane test, `internal/core/runlane`).

## Decisions for the user

| # | Question | Recommendation |
|---|---|---|
| D-P1 | Build per-conversation KV save/restore (Phase 1.4)? | Only if Phase 0 shows switch-back prefill > 10 s on your usual contexts; it adds disk churn and a file lifecycle. |
| D-P2 | Is a flag-only change (`--cache-reuse`, K-cache type) acceptable as the first win? | Yes — zero code, reversible. |
| D-P3 | Amend Constitution §VI/SPEC-005 to remove the "slot persistence" sentence if 1.4 is declined? | Yes; stale law is worse than none. |

## Non-goals

Reasoning-budget and step-count tuning; GPU/browser rendering (GPU plan); changing the HTTP/1.1-only chat
transport; new dependencies.

## Remaining Work

Everything. First PR: Phase 0.1 + 0.3 (timings capture, prefix-stability test) — they need no server and produce the ranked work list.
