---
status: proposed
date: 2026-09-30
last_reviewed: 2026-09-30
related_specs: [SPEC-001, SPEC-004, SPEC-005, SPEC-007]
constitution_references: [II.2, II.12, IV.1, VI]
evidence: docs/audits/2026-09-30-platform-scan.md (M1–M9)
related_plans: [memory/memory-improvements-implementation-plan.md (Phase 2.1 session_search and Phase 3.1 skills stay there; this plan links, not copies), cross-cutting/agents-md-layering-guardrails.md]
---

# Memory for Small-Context Local Models — Make It Work, Keep It Cheap, Make It Usable

**Status:** proposed. Phase 0 is measurement and one correctness test; nothing else starts until it lands.

## The goal, stated as constraints

1. **Do not slow a local model down.** A 4B–9B model on an iGPU does ~21 tok/s generation and
   200–650 tok/s prompt processing with an 8K window. Every extra prompt token costs prefill time on *every*
   turn; every extra LLM call costs tens of seconds.
2. **Memory must change what the model sees, not ask it to behave.** Small models obey the most specific
   imperative in the prompt ("Step 5: run `npx tsc --version`") over general guidance ("check memory first").
   Proven across 8+ attempts (`docs/audits/memory-injection-investigation.md`).
3. **No LLM call on the run path.** The task rewriter was the only approach that reliably made the model skip
   known work, and it cost 77–89 s per run, so it was removed (`docs/audits/remove-memory-rewriter.md`).
   Anything that "summarises" or "consolidates" must be deterministic code, or run off-path while idle.
4. **Keep the prompt prefix byte-stable.** llama.cpp reuses its KV cache only for an identical prefix. Anything
   inserted or rewritten early in the prompt makes the server re-process everything after it.
5. **Budgets come from the context, not from constants.** The serving window is the source of truth
   (`context_budget = (ctx − max_tokens) × 4` chars, SPEC-005 §II.3). An 8K model and a 128K cloud model need
   different memory sizes.

## What exists today (verified 2026-09-30 — see audit §2)

| Piece | State |
|---|---|
| Store | SQLite + FTS5 (`orchestrator.db`), three-tier `scope/mode/keep`, Jaccard dedupe, substring replace, `user_profile` |
| Injection | hot entries only (`mode: always`), **2000-char fixed cap**, newest first, inserted once before the last user message on the *first* request (`stream.go:153-237`) |
| Assistant vs automation | assistant gets hot memory; **automations get none** (`conversation_service.go:232` only) |
| Forgetting | the physical sieve truncates, then deletes the middle and inserts a fixed note (`sieve.go:40-83`) |
| UI | list / filter / search / open / delete / clear (`MemoryPanel.vue`); no injection preview, no "hot" control |
| Designed, never built | `session_search`, skills (procedural memory) |
| Stale doc | plan says the usage meter is done; it is dead code (`WorkspaceCharCount` has no caller) |

## Problems, ranked by effect on small models

| # | Problem | Why it hurts | Phase |
|---|---|---|---|
| 1 | Hot memory probably reaches the model **only on request #1** (M1) | facts vanish after the first turn of a run | 0, 1 |
| 2 | The sieve deletes history with no record (M4) | the model redoes finished steps after a prune — the smoke-test "repeats step 3" failure | 2 |
| 3 | Automations have no memory at all (M3) | the audit's headline use case (skip known discovery) is unsolved | 3 |
| 4 | Fixed 2000-char cap, recency-only, silent drop (M2) | too big for 4K windows, too small for 128K; no relevance; model cannot know entries were dropped | 1 |
| 5 | Operator cannot see or steer what is injected (M9) | "why did it forget / why did it say that?" is unanswerable | 4 |
| 6 | No recall of older conversations; no reusable procedures (M6) | existing plan, unbuilt | 5 |

## Phase 0 — Measure and prove (no behaviour change)

1. **Test M1.** Add a test in `internal/core/assistant` that runs a 3-turn fake-LLM session with one hot
   memory and records the `Messages` of each `ChatRequest`. Assert whether `<memory>` is present on turns 2 and 3.
   - Verify: `cd backend && go test ./internal/core/assistant/ -run TestHotMemory_PresentEveryTurn -count=1`
     → expected **FAIL today** if M1 is real (then it is the Phase 1 acceptance test); if it passes, correct
     the audit and delete Phase 1 item 1.
2. **A memory scoreboard** from data we already record (`events.jsonl`, `run-meta.json`, recordings; do not add
   timers): per run — memory tokens injected per request, sieve firings, **repeated `(tool,args)` pairs**, turn-1
   latency, total steps. Script `scripts/memory-scoreboard.sh` (or a Go test helper) that reads a run directory
   and prints the row. Baseline three runs: the smoke-test automation (local 4B), an assistant chat (local),
   the same chat on a cloud model.
   - Verify: `scripts/memory-scoreboard.sh backend/data/runs/<ws>/<model>/<run>` prints the six columns.
3. **Replay harness.** Use the existing record-replay tooling (`docs/architecture.md` "Record-Replay Testing",
   `FixtureClient`) so each later phase can be compared offline: `go test -tags recordreplay ./internal/core/assistant/ -run TestAgent_Execute_AgainstRecordings`.

Acceptance: baseline numbers committed in the audit file; M1 confirmed or refuted by a named test.

## Phase 1 — Correctness and sizing (small, safe)

1. **Persist the injection for the whole run** (if M1 confirmed). Choose the position by the prefix rule:
   append the memory block to the **system message** at the point the run's first request is built, and
   rebuild it identically on every turn from a per-run snapshot (`runSession`, not a re-query), so the prefix
   is byte-identical turn to turn and across sessions while memory is unchanged. The one-shot flag
   (`session.go:133`) becomes "snapshot taken". SPEC-004 §II.4 is amended (spec-first: Constitution §V.2).
2. **Size from the window.** `maxHotInjectionChars` (`stream.go:39`) becomes
   `clamp(contextBudgetChars × MemoryShare, minChars, maxChars)`; `MemoryShare` default 8% (an 8K/21 848-char
   budget → ~1 700 chars; a 128K cloud budget hits the max). All three numbers are named constants in one place,
   tunable via `model_overrides` like the other agent tuning fields (Constitution §III.5) — never in `registry.json`.
3. **Say what was dropped.** When the cap cuts entries, end the block with `(+N more saved facts — use
   memory_search)`. Costs ~15 tokens, gives the model a reason and a way to fetch.
4. **Rank, then cut.** Order hot entries by `pinned desc, updated_at desc` today; add an optional
   `priority` (0–2) column (additive migration, default 1) so the operator can protect the facts that must
   survive a small window. No relevance scoring yet (Phase 3).
5. **Use the caller's context** in `SearchHot` (`stream.go:197`, Constitution §IV.1).
6. **Resolve the stale meter.** Either wire `WorkspaceCharCount` into the Memory panel as "used vs budget"
   (preferred — it is UI, not prompt tokens) or delete `WorkspaceCharCount`/`SoftMemoryCharLimit` and fix the
   plan to say so. Do not put it in the prompt: it is per-turn tokens for zero behavioural effect on a small model.
7. Update `memory-improvements-implementation-plan.md` Task 1.2 to match reality.

Verify: `cd backend && go build ./... && go test ./internal/core/assistant/ ./internal/platform/memory/ -count=1 && go run ./tools/check-complexity/`.

Acceptance: the Phase 0 test passes; with an 8K-window fixture the memory block is ≤ 8% of the budget and ends with
the overflow hint when entries are cut; scoreboard shows prefill tokens on turns 2+ ≤ baseline + memory block.

## Phase 2 — A deterministic progress ledger instead of a blind sieve

**Idea.** When the sieve must drop the middle of the history, replace the fixed note with a compact,
code-generated **ledger of what was already done**, built from the events/tool calls we already hold. No LLM call,
no extra latency, and it removes the trigger for "model repeats finished steps".

1. `assistant/ledger.go`: `type Ledger` fed from `executeSingleToolStep` results (tool name, key argument,
   outcome class, ≤ 80-char result head). Entry format: `✓ write_file smoke/hello.txt` /
   `✓ terminal "uname -a" → Darwin …` / `✗ terminal "npx tsc" → exit 1: TS2353`.
2. **Bounded:** ≤ 1 200 chars (scaled like Phase 1.2), newest kept, identical consecutive entries collapsed with `×N`.
3. `physicalSieve`/`reactiveSieve`/`aggressiveSieve` emit the ledger in place of `SieveSystemNote`
   (`prompts.SieveSystemNote` stays as the header; ledger text lives in `prompts/templates.go` — prompts have one home).
4. **Prefix discipline:** the ledger is written when a sieve fires and left unchanged until the next sieve, so the
   server re-processes the prefix once per prune, not once per turn. Stop truncating mid-history messages in place
   on every call once the first prune has run (make compression idempotent — compressed messages stay compressed).
5. Persist the ledger into `run-meta.json` so the operator sees what the agent believed it had done.

Verify: `cd backend && go test ./internal/core/assistant/ -run 'Ledger|Sieve' -count=1`; replay the smoke-test
recording: repeated `(tool,args)` pairs after the first sieve must be 0 (scoreboard column).

Acceptance: on the recorded 8K smoke test, repeated calls after a sieve drop to zero; no added request latency
(ledger build < 1 ms per tool result, asserted by a benchmark); complexity gate green.

## Phase 3 — Memory for automations: step-aware, rule-based, no LLM

The audit's three missing pieces (`memory-injection-investigation.md` "What would fix…"):

1. **Step-aware query:** derive the query from the *current* tool call or the next unchecked task line, not the
   entire task file. Deterministic: tokenise, stopword-filter (the FTS5 list already exists), BM25 top-K with a
   **score floor** so weak matches inject nothing (the 46-annotation failure was noise, not the idea).
2. **Delivery at the point of need:** attach the match to the **tool result** it relates to
   (`Note: memory "tool_versions" (saved 2026-06-03): TypeScript 6.0.3`) — 1 line, at the end of the prompt, no
   prefix change. This is the audit's Option D made cheap: it cannot prevent the first redundant call, but it
   teaches the run inside the same context and costs ~30 tokens. Cap at 3 notes per run and 1 per memory entry.
3. **Early, static placement** for the always-hot set (Phase 1 block) so automations receive the same stable
   prefix as chats. `EnableHotMemory` becomes `MemoryMode: off | hot | hot+hints`, default `hot` for local,
   `hot+hints` opt-in per automation until the scoreboard shows a win.
4. **Do not re-enable broad task-wide injection** — the rewriter, annotation and prefill experiments all lost to
   the instruction hierarchy or to noise.
5. **Write discipline for unattended runs:** an automation's `memory_update` defaults to `keep: session` with
   `source = run id`, and a reaper deletes `session`-type entries older than N days (check whether one exists
   first; `grep -rn "memory_type = 'session'" backend/internal`). Stops unattended runs filling the hot set.

Verify: `cd backend && go test ./internal/core/assistant/ ./internal/core/automation/ -run 'MemoryHint|StepAware' -count=1`;
scoreboard on the smoke test with `hot+hints`: redundant discovery calls down, sieve firings not up, turn-1 latency unchanged (±5%).

Acceptance: a measured reduction in redundant calls on the recorded smoke test **and** no rise in sieve firings;
if either fails, ship only the `off|hot` switch and record the negative result in the audit (that is a valid outcome).

## Phase 4 — Usability (the operator's side)

All UI follows SPEC-003 (tokens only, no shadows, `ConfirmDialog` for destructive actions, `utils/format/`).

1. **Injection preview:** a panel in Workspace → Memory showing exactly the block the model will receive,
   its char/token estimate and % of the selected model's budget, and which entries were cut. Backed by a
   read-only endpoint that runs the same builder as the agent (single code path, so the preview cannot lie).
2. **Hot/priority controls** on each entry (toggle `always`, priority 0–2) and "used N times / last used" counters
   (incremented by `memory_search` hits and injections; additive columns).
3. **Quick add / edit** with the three-tier choice explained in plain words (scope, always vs on-demand, permanent
   vs this-conversation). Today only the agent can create entries.
4. **Operator-authored `MEMORY.md`** (decision D-M1 below): a file in the workspace tree loaded as the top of the hot
   block, editable in the existing editor. It is diffable, version-controllable, and matches what every other agent
   product does (Hermes, OpenClaw, AGENTS.md — audit §6). Coordinate with
   `cross-cutting/agents-md-layering-guardrails.md` (proposed) so there is one loading story for instruction files.
5. **Import / export** memory as markdown (no new dependency).
6. A one-line "why was this injected?" on each entry in the preview (hot + priority, or hint match + score).

Verify: `cd frontend && npm test && npm run lint && npm run build && npm run test:visual`;
`cd backend && go test ./internal/transport/... ./internal/platform/memory/ -count=1`.

Acceptance: an operator can, without reading logs, (a) see what the model received, (b) protect a fact from being
cut, (c) add a fact in under 15 seconds, (d) see which facts are never used.

## Phase 5 — Recall and procedures (existing plan, re-scoped)

Keep the designs in `memory-improvements-implementation-plan.md` (Task 2.1 `session_search`, Task 3.1 skills) —
do not copy them here. Changes from this analysis:

- **`session_search` must be local-model safe:** return ≤ 3 snippets of ≤ 300 chars, never whole messages;
  index after the turn completes off the hot path (goroutine tethered to the app root, Constitution §II.2);
  it reuses the session checkpoint already written, so it adds no second write. Sequence it **after** the
  session-checkpoint decision in `backend-hot-paths-and-leak-hardening.md`.
- **Skills** are injected as a name+description list only when the workspace has any, capped by the same
  Phase 1 budget. Skill *bodies* load on demand by tool call.

## Phase 6 — Experiments, only if the scoreboard justifies them (need your approval)

| Experiment | Risk | Gate |
|---|---|---|
| **Fact cache for read-only commands**: a memory entry may declare `cache: {tool, args, ttl}`; an exact `(tool,args)` match within TTL returns the stored result without executing. Gives the rewriter's win at zero latency. | returning a stale result silently | allow-list read-only tools only (`uname`, `--version`…), TTL ≤ 24 h, result labelled `cached`, off by default |
| Off-path consolidation: when the local lane is idle, a background job merges near-duplicate entries with a *small* model | steals GPU from a waiting run | only when `runlane` reports idle and queue empty; cancels instantly on admission |
| Per-model memory settings (share, hints) | configuration sprawl | only after Phase 3 results differ by model |

## Decisions for the user

| # | Question | Recommendation |
|---|---|---|
| D-M1 | Add operator-authored `MEMORY.md` alongside SQLite? | Yes — cheap, transparent, and solves "I want to pin this" without UI work; SQLite stays for agent-written facts. |
| D-M2 | Default `MemoryShare` (8%) and whether cloud models use the same default | 8% local / 5% cloud; revisit with the scoreboard. |
| D-M3 | Turn on `hot+hints` for automations by default once it wins | Yes for local lane automations; opt-in for cloud (cost). |
| D-M4 | Phase 6 fact cache | Decide after Phase 3; do not start earlier. |

## What not to do (learned the hard way)

- Do not add an LLM rewrite/summarise step to the run path (77–89 s).
- Do not annotate every task line (46 annotations → sieve on turn 1 → repeated steps).
- Do not rely on prefill or "check memory first" nags to beat an explicit step.
- Do not put counters or meters in the prompt; show them in the UI.
- Do not inject a memory block at a moving position every turn (defeats the KV cache).

## Remaining Work

Everything. First PR: Phase 0 (M1 test + scoreboard script). It is read-only and decides whether Phase 1.1 is needed.
