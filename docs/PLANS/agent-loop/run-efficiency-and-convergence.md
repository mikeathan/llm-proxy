---
status: partial
date: 2026-10-18
related_specs: [SPEC-001, SPEC-006, SPEC-010]
---

# Plan: Run Efficiency and Convergence (generic, workload-driven)

## 0. TL;DR

A simple assistant question took 141 s on a local 16k-context Qwen. Nothing about it is specific to the
question: the same four loop weaknesses will slow any research-style run on any small-window model.

1. **Results are shaped blindly.** One 3000-char head+tail cut over serialized JSON/HTML (`tool_exec.go:1315`)
   shows the model 1–2 of 5 search hits and the `<head>` of a web page.
2. **No signal that more tool calls add nothing.** Every detector in `repetition_detector.go` keys on (tool, args);
   this run varied its args each call, so none could fire, while the *results* repeated.
3. **The sieve can corrupt a request and break the prefix cache.** Request 8 carried an orphaned tool call and two
   back-to-back user messages, and the step took 41 s against 6–13 s for its neighbours.
4. **One tool call per model round trip.** All 7 tool steps made exactly one call.

The fix is one small abstraction per weakness, each at an existing seam, driven by measurements of the run and
the context budget — never by tool, model or topic name. Nothing is weather-specific.

The context size is the owner's to set; none of this assumes 16k. The mechanisms scale with the budget.

## 1. Evidence (run `conv_20261018183148`, workspace-1, 2026-10-18)

Source files: `~/.config/llm-proxy/runs/workspace-1/Qwen3.6 35B A3B/conv_20261018183148/<run>/{events,recording}.jsonl`.
Labels: **M** measured from those files · **I** inferred from them · **H** hypothesis to verify first.

| # | Fact | Label |
|---|---|---|
| E1 | 141 s total: first token at ~28.5 s; steps 2–7 took 6–13 s each (~58 s); tools ~11 s combined; step 8 took 41 s | M |
| E2 | Recorded generation per step is ~100–250 tokens, yet steps take 6–13 s: prefill of an 8–12k-token request dominates, decode does not | I |
| E3 | Step 8's request was rewritten by the sieve (`s,u,a,u,u,a,t…`): assistant call `T7QAu…` kept, its result dropped, then two user messages. The saved conversation (`conv.json` idx 2–4) keeps the same shape for follow-up turns | M |
| E4 | Cause of E3: `sieveLockedHead = 3` (`sieve.go:193`) keeps `[system, user, assistant(tool_call)]` and drops that call's result; the tail cut (`sieve.go:194`) has no pair check either | M (code) |
| E5 | The rewrite freed ~2.5 k chars net (dropped 3.2 k of results, added 1 k of notes), so it bought almost nothing and still broke the prefix; step 8 = 41 s for ~500 tokens vs ~8–13 s typical | I (cache break not directly measured: the recording has no `timings`/`usage`) |
| E6 | Raw search results were 5.8–7.3 k chars (5 hits each); the 3000-char cut keeps the first ~1.5 k and last ~1.5 k, so hit 1 (the same 989-char blob in all four searches) plus the tail survive and the middle hits are cut mid-JSON | M |
| E7 | Search #2 and #4 returned byte-identical results (7254 chars, same URLs). New-URL count per search: 5, 1, 2, 0; new 5-gram shingles: 100 %, 38 %, 64 %, 0 % | M |
| E8 | `fetch_url` returns raw HTML (`network.go:171` only truncates); the visible 3 k chars were `<head>` boilerplate | M |
| E9 | Every step issued exactly one call. No `parallel_tool_calls` is sent anywhere in the backend. `NativeToolReference` already says "Batch related tool calls" (rule 2) and, one line later, "Verify results before proceeding" (rule 3, `templates.go:152-153`) | M (code) |
| E10 | The text `## AVAILABLE TOOLS` section (~5.4 k chars) repeats the tool JSON (~12.3 k). It is deliberate (`templates.go:145`: "still needs text context about which tools exist"), so this plan does not remove it | M |
| E11 | Earlier turns' `reasoning_content` is not replayed (0 chars in request 7) | M |
| E12 | Cold start: step 1 took 30 s vs 8 s for step 2 with ~1 k more prompt; the log shows `upstream model is starting`, a 202 then a 503. Prefill of an uncached 7 k-token prompt explains part of it, so model load is probably ~15–20 s, not 28 s | I |

Not defects: the system-prompt tool list duplication (E10) and reasoning replay (E11). The first draft of the
diagnosis called the duplication a finding; the code shows it is intended.

## 2. Inspiration actually read

Hermes (`/Users/mikeathan/dev/hermes-agent`):
- `AGENTS.md` root — two invariants: *per-conversation prompt caching is sacred* (the only sanctioned mutation is
  compression) and *the core is a narrow waist*; anything mid-run rides a tool result, never the system prompt.
- `agent/tool_guardrails.py` (read lines 1–140 and the symbol list) — a pure controller keyed on (tool, args,
  **result**): identical-result streaks, idempotent no-progress, repeating multi-call cycles, byte-identical
  results replaced by a reference stub (≥512 chars), per-turn caps on runaway tools; warn-only on attended
  platforms, hard stop on unattended ones.
- `agent/conversation_loop.py` (lines 76–150) — wall-clock budget: at 80 % a one-time notice is **appended to the
  newest tool result** (cache-safe), latched once. `agent/iteration_budget.py` — consume/refund iteration counter.
- `agent/AGENTS.md` — compression prunes old tool results first with no LLM call; micro-compaction is off by
  default because it breaks the cache.

DeepSeek Harness (`/Users/mikeathan/dev/deepseek-harness`):
- `docs/tool-execution-pipeline.md` — one pipeline where every result passes `projectContent` → post-execute
  waterfall → `finalizeContent` before it becomes history; extra context is injected after recorded results.
- `docs/subsystems/compaction.md` (lines 101–105 and the headings) — pressure is measured at `agent/pre-step`;
  tool-result pruning runs **before** any range is dropped; `toolPairingBalancedBefore/After` reject any boundary
  that splits a call from its result.
- `docs/subsystems/spill.md` — large tool output saved verbatim, the model gets a locator plus retrieval hint.
- `docs/subsystems/goal.md` (types section) — durable goals with a round cap and a *blocked* phase with a reason code.
- `docs/subsystems/ptc-runtime.md` — header only read; programmatic tool calling is noted as a parked idea (§6).

## 2b. Progress and the reliability rule

- **Done (branch `run-efficiency-and-convergence`):** §4 observability (`cached_tokens` per run and per call, also when the
  server sends `usage`) and the correctness half of M1: the sieve never splits a call from its result, and
  `RepairToolPairing` heals saved conversations (SPEC-001 1.8). When the sieve fires its note, ledger and one-time wrap-up
  warning are exactly as before; a replay of request 8 confirms it.
- **Built and reverted: body-first stubbing (the second half of M1).** It avoided the sieve's wrap-up warning, which in the
  measured run is what ended the search loop, and nothing replaces that stop yet; it also hid old results from a model with no
  other memory of them. Its benefit (a cache that stays warm) was never measured. It returns only after M3 and a live
  baseline show it is safe and worthwhile.
- **Rule for everything below: additive, never replacing.** A new mechanism fires *before* the existing sieve warning and
  never removes it; the warning stays as the last-resort stop. It ships only after recorded replays and one live run at the
  current 16 k window show the same or fewer steps and an answer at least as good. Every mechanism must also work at a
  roomier window.
- **Also done: a chat run a detector stops answers from what it found** (`stopOnLoop`, SPEC-001 1.8 item 4), after a local
  run (2026-10-18) ended with an error and no answer when a 503 made the model repeat one URL. It only replaces a failure, so
  it cannot make a working run worse.
- **Local runs:** three runs of the same question took three different paths (terminal `curl`, `fetch_url` ×9 then a
  detector stop, and the original search run); the sieve never fired. A baseline needs about 5 runs per build.
- **Done: M3, the progress monitor** (`progress/`, SPEC-001 1.8 item 5), chat runs only. Additive: it appends one factual
  line to the newest tool result and never touches the sieve's warning. Refinements found by replaying real runs: streaks
  per tool (a fetch between two searches had reset a global streak) and a stricter bar for a reworded call.
- **Evidence (2026-10-18, isolated instances against vertex, weather question):** two batches of 5 interleaved runs per build
  (`main` 43372b9 vs this branch). All 20 runs answered. Median 33 s and 2 model calls on both builds (the vertex cold start,
  25–45 s, dominates); each build also had 1–2 longer runs of 6–12 calls. The sieve never fired in any run (prompts stayed at
  5–10 k tokens). The monitor spoke twice in batch 2: once the model switched to a different forecast API and answered; once it
  did not change the run (12 calls, still answered). Batch 1: a repetition-detector stop on the branch produced an honest
  answer ("could not retrieve tomorrow's forecast; current weather 8 °C") where the old code errors with nothing.
- **Honest limits:** samples of 5 per build per batch are a signal, not proof. Replayed over the original 141 s run the monitor
  stays silent: its searches really were 38 % and 64 % new, and no novelty measure can know a search engine never holds the
  answer. The run in the user's report needs `internet_search` (a Tavily/Brave/SerpAPI key kept in their encrypted secrets;
  not reproducible in an isolated instance without it).

## 2c. Next steps (in order)

1. **Acceptance runs that exercise what was not exercised.** (a) The real search scenario on the owner's configured instance
   (or an isolated one given a search key by the owner): same question, ~5 runs per build, steps / time / answered / ending.
   (b) The sieve: the same runs with a low `context_budget` so the sieve fires, comparing steps, answers and `cached_tokens`
   (the pairing fix is covered by unit tests and a replay of request 8, never by a live run).
2. **Shadow-mode data on the monitor.** The `progress monitor: results added nothing new` log line counts how often it speaks
   in real chats. Only with that evidence consider a structural stop for the assistant (a tools-off turn after the model has
   ignored the strong note, reusing `finalizeReport`), and for automations only after their own acceptance runs.
3. **M2, result shaping at `appendToolResult`** (E6, E8): per-item caps for lists of hits instead of a head-and-tail cut of
   JSON, HTML to text before the cap, items already shown in the run replaced by a reference, oversize output saved under
   the workspace with a path the model can `read_file`. Fix the stale "caps at ~8KB" comment (the cap is 3000).
   **Step 1 built (HTML to text, in `FetchURL` before the cap)**; the rest below is still open.
4. **M4, let independent reads batch** (E9): check what the local server does with `parallel_tool_calls` unset; resolve the
   rule conflict in `NativeToolReference` ("batch" vs "verify results before proceeding").
5. **Per-run toolsets** from the prompt profile, chosen once at run start (the 18 tool schemas are 12.3 k of the 22 k chars of
   the first request in the measured run): cache-safe, and large on a small window.
6. **Body-first stubbing and a per-request projection** (built and reverted here, see above) only if `cached_tokens` shows the
   sieve actually breaks the prefix cache; it needs a counterweight for the sieve's wrap-up warning first.
7. **Skills** (own plan): the unit of procedure, preloaded by profile, with `tools:` and `budget:` in the header that the
   progress monitor and the toolset selection enforce.
8. **M5, warm-up**, only if the logged timings show model load, not prefill, dominates the 25–45 s first step.

## 3. Design — four mechanisms, ranked by estimated seconds saved on this run

Cache rule for all four (borrowed from Hermes): never change the system prompt, tool set or earlier messages
mid-run. Guidance rides the newest tool result; the only history rewrite is the sieve, which stays the single
sanctioned cache break.

### M1. Pair-atomic pruning (done) and body-first stubbing (reverted: see §2b)

- **Fix the defect:** the sieve may never split an assistant tool call from its tool results, at the locked head
  or the tail boundary. Extend the head/tail to the nearest balanced boundary (DeepSeek's pairing check), and
  never leave two user messages adjacent.
- **Soft threshold first:** at ~70 % of the budget, replace the *bodies* of the oldest tool results with a stub
  (`[result of internet_search "…", 3.1k chars, already used]`) before dropping any message. Keeps the
  call/result structure and the prefix up to the first stubbed message; a stub is only produced once per result.
- **No "answer NOW" from pruning.** The one-time warning remains for a genuine hard overflow only.
- Seam: `sieve.go` (`physicalSieve.prune`, `sieveHistory`); ledger already records what stubs refer to.
- Test: table tests asserting every assistant `tool_calls` id has a matching tool message and roles never repeat
  user/user, over random histories and both sieves; replay request 8 from the recording as a fixture.
- Defaults: both run kinds.

### M2 step 1 — built: HTML to readable text in `fetch_url`

`FetchURL` (`tools/network.go`, `fetchedText`) turns an HTML response into its title plus body text (`tools/htmltext.go`,
stdlib only: drops `<head>`, scripts, styles, `<nav>`, `<footer>`, `<aside>`, `<svg>`, comments; decodes entities) *before*
the 3000-character cap. Other content types are byte-for-byte unchanged. One place fixes both what the model reads and what the
progress monitor measures, because the monitor sees the capped text.

Replay of five real Wikipedia city pages (1 MiB each, as the fetch guardrail reads them), found after building it:

- Raw: the 3000 characters are the `<html class=…>` start and the page tail, no readable text at all. Shaped: readable text.
- **Not enough for long articles.** The capped text is the page's menu (a 107-language list, "Jump to content / Donate / Log in")
  and the references list; the lede is in the cut middle. Ideas, in order of cost: a head-weighted cap for shaped text (the
  tail of a web page is rarely the answer); dropping runs of very short menu-like lines; choosing passages by the user's
  question (needs the question passed to the tool, a public-interface change, so a separate decision).
- **The monitor false positive seen live (5 different city pages, "added nothing new") did not reproduce** in this replay,
  raw or shaped: the monitor stayed silent on all five. The live pages had been cut by the size guardrail
  ("content truncated by network guardrails"). Not claimed as fixed.

### M2. Shape results before they are capped (est. −10…−30 s, plus answer quality)

- A **post-execute stage** at the one chokepoint (`appendToolResult`, `tool_exec.go:1313`), like DeepSeek's
  `projectContent`: structured results (lists of hits) are cut *per item* (title + url + a snippet budget) so all
  items survive instead of head+tail of a JSON string; HTML becomes readable text (drop `<head>`, scripts,
  nav) before the cap; plain output keeps head+tail.
- **Run-level dedup:** an item already shown earlier in the run (same URL or same normalised snippet) is replaced
  by a reference (`seen in step 1`) — Hermes' result-stub idea applied per item, so the repeated 989-char blob
  stops costing ~300 tokens per search.
- **Spill:** a result over the cap is also saved under the workspace, and the truncation note names the path so
  `read_file` can fetch a segment (DeepSeek spill). Replaces the generic "use targeted tools" marker.
- The shaper is a registry keyed by result *shape*, not tool name; unknown tools keep today's behaviour.
- Fix the stale comment at `tool_exec.go:1309` ("caps at ~8KB"; the cap is `MaxReturnChars = 3000`).
- Test: golden shapes (search list, HTML page, plain text, error) asserting item count preserved, `<head>`
  removed, dedup reference emitted, cap respected.
- Defaults: both run kinds; the cap derives from the context budget, not a constant (a roomy window keeps more).

### M3. Result-level progress monitor (done, see §2b; the −10…−35 s estimate was not borne out on the original run)

A pure controller (Hermes `tool_guardrails.py` shape: observes, returns a decision, no side effects) fed from the
same place `ledger.record` is. It scores **information gain**: the share of a read-only result's items and
5-gram shingles not seen earlier in the run. E7 gives 100 / 38 / 64 / 0 %. Signals:

- identical result hash (Hermes identical-result streak) — would have fired at search #4, with M2 dedup at the
  first repeated item;
- `K` consecutive read-only calls (`toolpolicy.EffectReadOnly`) below a gain threshold.

Delivery: appended to the newest tool result as a *fact* ("the last 2 searches added 0 new sources; you have N
sources"), not a nag; thresholds derived from the step budget (Hermes budget-ratio pattern), never from tool or
model names. Escalation is the existing finalization path (`finalizeReport`), not a new one.
- Attended (assistant): warn only. Unattended (automation): after the hard threshold, finalize with what exists
  (mirrors Hermes attended/unattended split and our `FailurePolicy`).
- Seam: new `internal/core/assistant/progress` package beside `repetition/`; wired where `trackGuardrailOutcome`
  and `recordLedger` already run.
- Test: unit tests on shingle gain; replay E7's four results; a property test that mutating-tool calls reset the
  streak (a write is progress).

### M4. Let independent reads batch (est. −10…−20 s, **H**)

- Verify first (E9): what does the local server do with `parallel_tool_calls` unset? If it defaults to one call
  per response, send `parallel_tool_calls: true` for providers that accept it (capability flag, not model name).
- Resolve the rule conflict in `NativeToolReference`: keep "batch independent calls", drop "verify results before
  proceeding" for read-only calls, which invites one-call-per-step.
- Execution of a multi-call batch already exists; read-only calls in a batch can run concurrently
  (`ToolEffect`).
- Test: a recorded two-search step executes both and appends both results in call order.

### M5. Optional: warm the model (est. −10…−15 s of a cold start, **H**)

Only if a measurement (add llama.cpp `timings` capture, below) shows model load, not prefill, dominates. Trigger
on composer focus; skip when the model is already running. Last because it spends GPU time speculatively.

## 4. Observability first (small, unblocks the measurements marked I/H)

Record, per request, the upstream `usage`/`timings` (`prompt_n`, `cache_n`, `prompt_ms`, `predicted_n`) in
`recording.jsonl` and the run record, when the server returns them. Without it the cache-break claim (E5) and the
cold-start split (E12) stay inferred. Zero behaviour change.

## 5. Other features worth borrowing (short list, ranked)

1. **Budget wrap-up notice** (Hermes `conversation_loop.py`): at 80 % of the step budget append one fact to the
   newest tool result. We have `MaxSteps`, but today the only "wrap up" signal is the context sieve.
2. **Attended vs unattended defaults** (Hermes `_ATTENDED_PLATFORMS`): the assistant warns; automations enforce.
   Matches the existing assistant/automation split and `FailurePolicy`.
3. **Prompt-cache invariant written down** (Hermes root `AGENTS.md`): list the loop's sanctioned mutation points
   (sieve, nothing else) in SPEC-001, so later features cannot erode the prefix.
4. **Goals with a round cap and a blocked reason** (DeepSeek `goal.md`) for the autonomy roadmap's standing goals.
5. **Programmatic tool calling** (`ptc-runtime`, Hermes `execute_code`): the model writes one program that makes N
   calls and returns one result. Biggest round-trip saver for local models, but needs the sandbox plan's
   execution story; park.

## 6. Order and decision needed

Order: §4 observability → M1 → M2 → M3 → M4 → M5. M1 is a correctness bug and goes first regardless.
M1–M3 change loop behaviour and SPEC-001 (sieve contract, a new per-run monitor); the owner chooses scope before
any implementation. Each lands with its tests and the Pre-Completion Review; none is weather-, tool- or
model-specific.

## 7. Risks

- Per-item shaping must not hide a hit the model needs: items are shortened, never dropped; the spill path keeps
  the full text.
- Gain thresholds on tiny results (errors, one-line outputs) are noise: score only results above a minimum size.
- Dedup references rely on stable item identity (URL, normalised snippet); an unknown shape is left untouched.
- M4's `parallel_tool_calls` may be rejected by some providers: gate on a capability, fall back silently.
