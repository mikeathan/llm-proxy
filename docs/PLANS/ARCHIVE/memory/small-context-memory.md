---
status: complete
date: 2026-09-30
last_reviewed: 2026-10-01
related_specs: [SPEC-001, SPEC-004, SPEC-005, SPEC-007]
constitution_references: [II.2, II.12, IV.1, VI]
evidence: docs/audits/2026-09-30-platform-scan.md (M1–M9)
related_plans: [memory/memory-improvements-implementation-plan.md (Phase 2.1 session_search and Phase 3.1 skills stay there; this plan links, not copies), cross-cutting/agents-md-layering-guardrails.md]
---

# Memory for Small-Context Local Models — Make It Work, Keep It Cheap, Make It Usable

**Status:** complete (2026-10-01) for Phases 0–4: hot memory reaches the model, is sized from the served window, survives pruning via the
progress ledger, is opt-in for automations, and has an operator UI. Phase 5 stays in its own plan and Phase 6 was never started (see
*Closed out*). Whether hot memory *improves* a small model's results is **unproven** (see *Closed out*).

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

## Progress log

**2026-10-01 — M1 confirmed and fixed (Phase 0.1 + Phase 1.1/1.2/1.3/1.5).**
- `TestHotMemory_PresentEveryTurn` failed on the streaming path (requests 2+ had no memory); it passed on the
  default mock only because the non-streaming fallback reset the one-shot flag and re-injected at a moving
  position. Lesson: memory tests need a real `StreamFunc` (`streamingToolRun` in `hot_memory_test.go`).
- Fix (`assistant/hot_memory.go`): one snapshot per run (`runSession.run`, run context — Constitution IV.1),
  appended to the **head system message** of every request, byte-identical across turns. The
  `setMemoryInjected(false)` reset and the one-shot flag are gone.
- Size = share of the resolved `ContextBudget` (local 8%, cloud 5%, clamp 400–6000 chars, 2000 if unresolved).
  `ContextBudget` is already derived from the probed serving window (SPEC-005 §II.3), so 8K/16K/32K models
  scale automatically (`TestHotMemory_SizedFromServingContext`, `TestHotMemory_BlockScalesWithContextWindow`).
- Overflow hint `prompts.HotMemoryOverflowHint`; newest fact always kept.
- `preparedOverContextBudget` now measures the block (it used to switch hot memory off for the measurement).
- Constitution II.12, SPEC-004 §4 and the memory-system skill amended (user-approved 2026-10-01, D-M2 share
  accepted as 8% local / 5% cloud with the window taken from model metadata, not a constant).
- **Deferred from Phase 1:** 1.4 `priority` column and 1.6 usage meter (`WorkspaceCharCount` /
  `SoftMemoryCharLimit` stay, to be wired into the Memory panel in Phase 4 — not removed); 1.7 update of
  `memory-improvements-implementation-plan.md` Task 1.2.
- **Phase 0.2 done (2026-10-01):** `scripts/memory-scoreboard.sh` / `backend/tools/memory-scoreboard/` (tested).
  Existing runs are a *no-memory* baseline (see audit "Memory scoreboard baseline"); sieve is derived because no
  sieve event is recorded (adding one is a candidate follow-up, not done).
- **Phase 2 implemented (2026-10-01):** `assistant/ledger.go` + sieves (`sieve.go`). Deviations from the plan text, all
  deliberate: the ledger is its **own message** after the byte-exact note (the note is matched by equality in
  `isAgentControlMessage`; the ledger has an owned prefix registered there); wording is **facts only** because the
  physical sieve already appends `ContextSieveWarning` ("deliver your final answer NOW"); it is recorded in
  `appendToolResult` (all result paths incl. guardrail denials); real tool names (`execute_terminal_command`/`command`).
  In-place compression idempotency (plan item 4) needed no change — verified by `TestTruncateLongContent_IsIdempotent`
  (head/tail are sliced from the ends, so re-truncation is byte-identical).
  **Not measured:** "repeats after a sieve drop to 0" — replay cannot show it (recorded responses ignore the prompt) and
  no existing run pruned. Needs a live 8K run with the scoreboard. Deferred: persisting the ledger to `run-meta.json`
  (written only by `core/automation/executor.go`; assistant chats have no run-meta).
  **Observation:** with an 8K-class budget the physical sieve can fire every turn once the 10-message tail alone exceeds the
  budget, and each firing appends another `ContextSieveWarning` (they pile up in history). Not changed here (outside
  Phase 2); worth a follow-up.
- **Phase 3 steps 1+3.5 implemented (2026-10-01, user-approved: default off, reaper included):**
  per-automation `memory_mode: off|hot` (`models.MemoryMode`; backend → registry → ExecuteRequest → agent options;
  validated 400; UI select in AutomationForm, shown in AutomationDetails — needed so saving from the UI does not
  erase it). Unattended runs: bare `memory_update` → `keep: session`, `source = run:<id>`. `memory.SessionReaper`
  wired in `app.New` (tethered, store and retention re-resolved each tick; only `session` type is deleted).
  Constitution II.12 + SPEC memory §4 amended. **Found:** `retention_days` existed but nothing read it and
  `DeleteOlderThan` had no caller. **Side effect to know:** a hot-mode automation now also gets the pre-sieve
  memory-flush nudge (it keys on store + hot memory). **Not done:** Phase 3 steps 1 (step-aware query) and 2
  (point-of-need hints) — `hot+hints` is intentionally not a value; they need the scoreboard to show a win first.
- **Phase 4 partly implemented (2026-10-01):** DONE — injection preview (4.1; backend `GET …/injection-preview`, UI
  "Show what the model receives", model picker, size vs the model's own window, what was cut), hot switch
  (4.2 first half) and hot filter, quick add in plain words (4.3), Session and User (global) filters, Always tag.
  Fixed on the way: editing a fact **wiped its tags** (a hot fact silently became on-demand on any edit; the UI
  never sent tags); user-wide facts (workspace `global`) were injected everywhere but invisible/uneditable in the
  panel; a missing entry returned 500 not 404. Plan item 1.6 settled by **deleting** the dead usage meter
  (`WorkspaceCharCount`, `SoftMemoryCharLimit`) — the preview supersedes it.
  **Acceptance (plan Phase 4):** (a) see what the model received — YES; (c) add a fact in under 15 s — YES;
  (b) protect a fact from being cut — **NO** (needs the 1.4 `priority` column; today order is newest-first and a hot
  switch only decides membership); (d) see which facts are never used — **NO** (needs usage counters; they must not add
  a DB write per run on the hot path, so record off-path/batched). **Not built:** 4.4 operator-authored `MEMORY.md`
  (decision D-M1 still unanswered — ask before building), 4.5 import/export, 4.6 per-entry "why injected" (every
  injected fact is hot; the preview shows order and what was cut).
- **Phase 4.4 operator-authored MEMORY.md implemented (2026-10-01, D-M1 answered yes by the user):** user chose
  "outside the agent's reach" and "goes first, never cut". Deviation from the plan text (plan said a file in the
  workspace tree): the plan's own pointer to a workspace file would let the agent's file tools rewrite text that is
  injected into every future prompt — a persistent prompt-injection path — so the files live in the metadata folder /
  config root and are edited from the Memory panel. Per-workspace + global files; atomic writes; ≤ 6000 chars/file;
  PUT is JSON-only. The preview shows the notes and warns when they alone exceed the model's budget. This also
  delivers the "protect a fact from being cut" acceptance criterion **for operator notes** (they are never cut); the
  agent-written facts still have no per-fact priority (the 1.4 column).
- **Phase 1.4 priority implemented (2026-10-01):** `memories.priority` (0–2, default 1, additive migration tested
  on a pre-existing database), hot order `priority DESC, updated_at DESC`, `Store.SetPriority`, operator-only
  (PUT/POST `priority`, validated before any write), UI: priority control on a fact (enabled only for always-on
  facts), "High priority" tag, priority in the add form, preview warns when a high-priority fact was cut anyway.
  **Phase 4 acceptance now:** (a) see what the model received — YES; (b) protect a fact from being cut — YES
  (priority, plus never-cut operator notes; protection is not absolute if protected facts alone exceed the budget);
  (c) add a fact in under 15 s — YES; (d) see which facts are never used — **NO** (usage counters, not built).
- **Usage counters implemented (2026-10-01; closes Phase 4 acceptance (d)):** `injected_count` / `searched_count` /
  `last_used_at`, recorded in memory and flushed off-path (`UsageFlusher` 30 s + shutdown flush + synchronous flush in
  `App.Shutdown`; failed flush keeps counts; `-race` test with concurrent record/flush). Only facts actually in the
  block count as sent, once per run. UI: usage line on every fact, "Unused" filter (never sent, never found) with the
  counting-began-late caveat, "Last used" in the detail. **Phase 4 acceptance now: (a)(b)(c)(d) all met.** Still open
  from Phase 4: import/export (4.5) and a per-entry "why injected" line (4.6).
- **Sieve warning fixed (2026-10-01; the Phase 2 observation):** reproduced first — up to 4 `ContextSieveWarning`
  copies in one request and the sieve firing on every request after the first prune on a small window. Fix:
  each prune replaces the previous note/ledger/warning (`withoutSieveMessages`), and the "deliver your final answer
  NOW" warning is sent once per run (user decision: "warn once per run"; alternatives were never-warn and keep-as-is).
  The warning dates from the completion rework (#27) and had no documented rationale. **Not measured live:** whether
  small models now finish more tasks — needs the live 8K run with the scoreboard.
- **Live comparison runbook written (2026-10-01):** `docs/guides/memory-testing.md` Part B (8K window, two automations off/hot, 3 runs each, scoreboard columns and how to read them). Found while writing it: `settings.yml → model_overrides.<model>.context_budget` (e.g. 50000 on some Qwen entries) overrides the derived budget, so an 8K test would never prune unless the override is removed. The operator runs it; I do not touch :4001.
- **Markdown import/export implemented (2026-10-01; plan 4.5):** lossless round trip (test with `###`, `---`,
  `<!--`, backslashes, unicode in a fact), hand-editable, strict about typos, 200-fact cap, JSON-only import, UI
  Export/Import with a result summary (added / already saved / entries that could not be imported, by line). Operator
  notes are separate files and not part of it. **Phase 4 remaining:** only 4.6, a per-entry "why injected" line.
- **Assistant chats were not receiving hot memory — fixed (2026-10-01).** Found by the operator ("Never used" next to
  an Always fact after a chat run). `conversationService.buildAgent` enabled `WithHotMemory(true)` but never called
  `WithMemoryStore()`; since commit `ac74c0b` the store was nil, so `snapshotHotMemory` returned early and no chat
  got memory (or counted usage). My earlier agent-level tests passed because they inject the store directly, and the
  conversation-service test double hard-coded `MemoryStore()` to nil. Reproduced through the real service first
  (`TestConversationService_Execute_InjectsHotMemoryAndCountsIt`), then fixed with one line. **Lesson:** my earlier
  claim that "assistant gets hot memory" was taken from the plan and not verified end to end. Automations were
  unaffected (their options set the store explicitly).
- **UI follow-ups from the same review:** an open "what the model receives" preview now reloads on every change to
  memory (`memoryRevision` in `useMemory`, not only on notes saves); the list shows Saved / Edited / Last used instead
  of only the creation time.
- **Comparison prompt corrected:** the first draft let a model do all file work in two shell loops (observed), so the
  window never filled; Part B now forces one `write_file` + `read_file` per file (8 files).
- **First real run read back (2026-10-01, chat, remote Qwen3.6 35B A3B, budget 50,000):** memory in 5/5 requests; one
  prune fired when nine large tool results arrived in one turn (server log: `chars 60223` over 50,000); the model got
  the note, a ledger listing all 21 finished calls, and the one-time wrap-up warning, then finalised in the next step
  with `repeats 0`. **Memory did not change behaviour:** the model still ran `node --version`, `npm --version`,
  `uname -s`, `uname -r` although the facts were in its head message (the instruction-hierarchy problem). Found and
  fixed: the block printed every auto-titled fact twice (`- Title: content` with the title being the content's first
  60 chars). **Not a valid A/B:** one run, no memory-off arm, remote model with a 50K budget (not an 8K window), and the
  workspace still had the old template (60 lines, "one file at a time"). **My own errors in this analysis, corrected:**
  I first called the scoreboard's `sieve = 1` a false positive and "the window could never fill" — both wrong (I had
  measured the post-prune request, 35.9K, not the 60K history the sieve measured); the heuristic was reverted and the
  test replaced by the real case.
- **Live verification done and simplified (2026-10-01):** chat — stored (`memory_update`, source `agent`) and used (answers
  from the injected block, unsaved control UNKNOWN); automation — stored (source `run:<id>`), `memory_search` works, and with
  `memory_mode: hot` the block is injected (system message byte-identical across turns). Finding: the automation wrapper says
  the report must be "traced to an actual tool result", which can make a model re-verify by `memory_search` even when the
  fact is in its prompt (suggestive from one pair of runs; not measured). The long A/B marathon is now optional; the
  recommended check is two short templates (`memory-store-test`, `memory-recall-test`; guide `memory-testing.md` Part A) that
  forbid tools in the recall run so a correct answer can only come from the injected block.

## Closed out (2026-10-01)

**Delivered and verified live** (chat and automation, on a remote 35B with a 50K budget override): per-run frozen head-system
memory block, window-scaled budget, priority, usage counters, session reaper, operator notes (`MEMORY.md`), markdown
import/export, injection preview, memory panel, per-automation `memory_mode` (default off), progress ledger and one-time
wrap-up warning. How to re-verify in two short runs: `docs/guides/memory-testing.md` Part A.

**Not done, deliberately:**
- **A/B benefit unproven.** No interleaved run of at least 3 per arm on the corrected prompt exists, and nothing ran on a real 8K
  window (`docs/guides/memory-testing.md` Part B is the optional procedure). Treat "memory helps small models" as a hypothesis;
  the shipped default (off) reflects that.
- **Phase 0.3** replay-harness check and **Phase 1.6/1.7** were not done.
- **Phase 5** (`session_search`, skills) remains in `memory-improvements-implementation-plan.md`.
- **Phase 6** experiments (fact cache, background consolidation, per-model settings) were never started; D-M4 stands: decide only after
  A/B results exist.
- **Possible follow-up:** for `memory_mode: hot`, one extra wrapper line saying the `<memory>` block counts as a verified source,
  so models stop re-checking by `memory_search`. Evidence is one pair of runs; measure first. It changes the automation prompt
  contract, so it needs the user's approval.
