# Delegation eval (B7) — helpers vs no helpers on a 16k local model (2026-10-23)

**Status:** complete. Closes B7 of [`multi-agent-delegation.md`](../PLANS/agent-loop/multi-agent-delegation.md).
**Raw data:** [`2026-10-23-delegation-eval-results.json`](2026-10-23-delegation-eval-results.json) (all 48 runs, with the
full final answers). **Driver:** `scripts/delegation-eval.py`.

## Setup

- **Server:** vertex (the user's deployment, v0.11.x with #123), through its HTTP API, in a throwaway workspace
  (deleted afterwards; only `workspace-1` remains).
- **Parent model:** `Qwen3.6 35B A3B`, the local 16k model. **Profile:** Research. **Helpers:** `web-researcher` on
  `deepseek-v4.1-flash`.
- **Arms:** A = Research profile, no switch (what a user gets today). B = same message with `use_helpers: true`.
- **Prompts:** 12 — eight *breadth* prompts that compare three things and want citations (b1–b8) and four *simple*
  lookups (s1–s4). **Repeats:** 2. 12 × 2 × 2 = 48 runs, run one at a time (the local lane admits one), arm order
  alternated per prompt.
- **Judge:** me, blind. Each of the 24 pairs was shown with the arms hidden and shuffled (X/Y), against a fixed rubric:
  correct (internally consistent, no claim contradicted elsewhere in the pair, stated limits admitted rather than
  invented), complete (every item the prompt names answered), sourced (a primary source per claim). A pair is "tie"
  when neither side clearly wins on those three. The key was opened only after all 24 verdicts were written down.

## Results

### Objective

| | Breadth A | Breadth B (helpers) | Simple A | Simple B (helpers) |
|---|---|---|---|---|
| Runs | 16 | 16 | 8 | 8 |
| Errors / ended oddly | 0 / 0 | 0 / 0 | 0 / 0 | 0 / 0 |
| Answered | 16 | 16 | 8 | 8 |
| Parent prompt tokens, mean (median) | 37,325 (31,380) | **14,374 (11,031)** | 16,021 (6,283) | 9,847 (6,362) |
| Runs whose parent prompt tokens exceed 16k | 15 | **3** | 3 | 2 |
| Parent tool calls, mean | 5.8 | **1.9** | 1.6 | 1.1 |
| Wall time, mean (median) | **139 s (129 s)** | 190 s (183 s) | 56 s (34 s) | 35 s (30 s) |
| Runs that delegated | 2 | 16 | 0 | 0 |
| Helper runs failed | — | 1 of 48 | — | — |
| Bad `delegate_task` calls | 0 | 0 | 0 | 0 |

- "Parent prompt tokens" is the run record's total across all steps of the turn, so it is **not** the peak context of
  one request. It does measure how much the parent had to read.
- Arm A delegated by itself twice, both in repeat 2 (b3: 2 calls after 3 searches; b5: 1 call). In repeat 1 it never
  did.
- Neither arm delegated on a simple question: 0 of 16 runs with the switch on, so the note does not push silly
  delegation.
- Helper tokens are on top: the three helpers of one breadth run read a median of about 0.4M prompt tokens in total
  (range 64k–2.9M; each helper re-reads its own growing context on every step) and wrote 4k–27k. They run on the cloud
  model, so this is the cost of the feature.

### Quality (blind, 24 pairs)

| | helpers better | baseline better | tie |
|---|---|---|---|
| Breadth (16 pairs) | **13** | 0 | 3 |
| Simple (8 pairs) | 0 | 1 | 7 |

Per pair:

| Prompt | Repeat | Preferred |
|---|---|---|
| b1 | 1 | **helpers** |
| b1 | 2 | tie |
| b2 | 1 | **helpers** |
| b2 | 2 | **helpers** |
| b3 | 1 | **helpers** |
| b3 | 2 | tie |
| b4 | 1 | **helpers** |
| b4 | 2 | **helpers** |
| b5 | 1 | **helpers** |
| b5 | 2 | **helpers** |
| b6 | 1 | **helpers** |
| b6 | 2 | **helpers** |
| b7 | 1 | tie |
| b7 | 2 | **helpers** |
| b8 | 1 | **helpers** |
| b8 | 2 | **helpers** |
| s1 | 1 | tie |
| s1 | 2 | tie |
| s2 | 1 | tie |
| s2 | 2 | tie |
| s3 | 1 | tie |
| s3 | 2 | tie |
| s4 | 1 | tie |
| s4 | 2 | baseline (no switch) |

What decided the breadth pairs:

- **Coverage.** In several baseline runs the parent ran out of room and answered part of the question, or admitted it
  could not confirm one of the three items (b2 repeat 1: no Rust version, no date). The helper arm answered all three.
- **Freshness and sources.** The helper arm read the primary page (release notes, pricing pages, model cards) more
  often; baseline answers gave a stale llama.cpp version (b1 repeat 1: v0.6.0, where the helper arm found v0.7.0) and flags that
  do not exist (b6 repeat 2: `--kv-cache-type` and `--quantization kv_cache`, with generic one-line descriptions).
- **Where the baseline was not worse.** Simple questions (a single lookup): the two arms behave the same; the one
  baseline win (s4, repeat 2) is a source line.

## Ship bar

> Ship only if quality ≥ baseline and overflows go down. (`multi-agent-delegation.md`, B7)

- **Quality ≥ baseline:** met. 13 of 16 breadth pairs went to helpers, none to the baseline; simple questions are a tie.
- **Overflows go down:** **not measured directly.** The driver did not record an overflow event, and the vertex run
  recordings were not collected. What it shows instead is that 15 of 16 baseline breadth runs read more than 16k prompt
  tokens against 3 of 16 with helpers, with a third of the parent tool calls. No run failed in either arm, so any
  overflow was recovered by the existing one-step recovery. This is evidence in the right direction, not the count the
  bar asked for.
- **Verdict:** keep the feature on, as shipped (an explicit switch, off by default). Do not make it automatic.

## Limits of this eval

- One judge, which is me, with the arms hidden but not the answers' style (a helper answer reads more thoroughly
  sourced). Every pair is in the JSON, so any verdict can be checked.
- The world data is current-events, so "correct" was judged by internal consistency and primary sources, not against an
  independent truth. Two runs of the same arm sometimes disagreed on a number (Tailscale and Anthropic pricing), which is
  search noise that this design cannot separate from the arm.
- Two repeats, twelve prompts, one parent model, one helper model.
- Breadth runs are slower with helpers (+51 s mean, 190 s vs 139 s) because the parent waits for the helpers. Simple
  runs are not affected.
- Helper cost is not zero (see the token note above). It was not priced here.

## Follow-ups this suggests

- Record an overflow count in the run record so the next eval can read it instead of inferring it.
- Arm A delegated twice unprompted in repeat 2; worth watching whether that grows once Qwen has seen delegate results in
  its history (it was a fresh conversation each run, so it is not carry-over).
