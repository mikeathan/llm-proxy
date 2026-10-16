---
status: active
date: 2026-10-16
last_reviewed: 2026-10-16
related_specs: [SPEC-004, SPEC-001, SPEC-005]
constitution_references: [II.12, II.13, IV.4, V.2, V.3]
related_plans: [assistant-memory-capture.md, memory-consolidation.md, small-context-memory.md, ../../audits/2026-10-15-next-work-analysis.md]
evidence: scripts/memory-eval.py (Part D), scripts/memory-recall.py (R1-R6), the runs recorded below
---

# Memory Gaps — Closing the "Assistant That Knows Me" Loop

**Status:** active — Phase 0 done 2026-10-16; Phases 1–6 proposed. It turns the gaps found by the 2026-10-15/16 live tests into ordered,
verifiable phases. Each phase that changes model-facing behaviour is gated on a before/after run of the same fixed
scripts on both models (Phase 0 makes that cheap), so no prompt or tool change ships on a hunch.

**Goal (the user's words):** an assistant that knows me, understands what I do and what I need next, learns from my
prompts and automations about me and my goals, and works with cloud and local models.

## What the tests showed (2026-10-15/16)

Setup: scratch backends; `Qwen3.5 35B A3B` (16K window, reached through vertex's `/v1`) and `deepseek-v4.1-flash`;
2–3 runs per cell, so these are signals, not statistics. **Two Qwen setups:** the Part D runs had Qwen classified
**cloud** (budget 10,923 chars, hot-memory share 5%) because of the classification bug fixed in PR #83; the recall
runs had it classified **local** (budget 43,692 chars, share 8%). G1's numbers were therefore measured at a different
budget from the one a tuning run would use, which is why Phase 0 re-baselines Part D at the local class.

**Saving (Part D, recorded in `assistant-memory-capture.md` Task 6): pass.** No unwanted saves, no failed runs, branch ==
baseline. Open signals: Qwen saved on D1 (a statement plus a question) 0 of 3 on the branch and 1 of 2 on the
baseline; on D4/D5 it mostly restates its earlier answer; cloud is clean.

**Recall (new, 2 runs per model; fresh conversation per question, facts seeded through the API):**

| Check | Cloud | Qwen | Reading |
|---|---|---|---|
| R1 keyword question ("Which host do we deploy through?") | 2/2 | 2/2 | works; Qwen answered with no tool call (per-turn recall did it) |
| R2 paraphrase ("Where do we ship releases?") | 1/2 | 1/2 | **gap:** recall has no synonyms; the model must search itself and half the time wanders |
| R3 user-wide fact in another workspace | 2/2 | 2/2 | works |
| R4 workspace fact must not leak to another workspace | 2/2 | 2/2 | holds, but the model spent 3–9 tool steps (files, terminal) before saying "I don't know" |
| R5 hot preference ("one short sentence") shapes the answer | 2/2 (1 sentence each) | 1/2 (one run gave 2 long sentences) | mostly works; the check is now "one sentence" |
| R6 unrelated question: no memory noise | 2/2 | 2/2 | works; no memory tool calls |

Caveat: the facts were seeded through the operator API, so this tests recall, not the loop as the user lives it (the model
saves in one chat and uses the fact in another); Phase 0 adds that case. Deterministic preview (no model): the keyword recall finds "deploy" and (by the shared word "Go") the tabs fact, finds
nothing for "ship releases", and recalls nothing for unrelated questions.

## Phase 0 baseline (2026-10-16, build 9667031 + header fix; scratch backend; Qwen classified **local**, budget 43,692)

Models: `deepseek-v4.1-flash` (5 runs) and `Qwen3.5 35B A3B` on the 16K reference (4 runs). A cell stops when it is
decisive (0/4 or 5/5 needs no more runs); unclear recall cells got 3 extra runs. Driver: `scripts/memory-eval.py`
(sets D, V, S, E) and `scripts/memory-recall.py` (R1–R6). Terminal tool excluded so no run waits on an approval.
The recorded request confirms `MemorySaveGuidance` is in `messages[0]` (the head system message) for these chats.

**Saving** (pass = what the case expects):

| Case | what it checks | Cloud | Qwen |
|---|---|---|---|
| D1 | statement + question: facts saved before answering | 5/5 | **0/4** |
| D2 / D3 | task / password: nothing saved, password never stored | 5/5 · 5/5 | 4/4 · 4/4 |
| D4 | a repeat does not re-call `memory_update` | 5/5 | **0/4** (it saves again) |
| D5 | a correction updates the stored fact | 4/5 | 4/4 |
| V1 | statement + question (person fact) | 5/5 | **0/4** |
| V2 | bare statement (project fact) | 5/5 | 3/4 |
| V3 | explicit "remember that…" | 5/5 | 4/4 |
| S1–S3 | person fact + question → user scope | 5/5 each, user | **0/4 each** (nothing saved) |
| S4–S6 | project fact + question → workspace scope | 4/5 · 5/5 · 5/5 | **0/4 each** (nothing saved) |
| S7 / S8 (not scored) | "I prefer tabs in Go files" without / with a project noun | user ×5 / workspace ×5 | user ×4 / workspace ×3, none ×1 |
| E1 | explicit remember in one chat, used in a fresh chat | 5/5 | 4/4 |
| E2 | person fact + question in one chat, asked in another workspace | 5/5 | **0/4** (never saved) |

**Recall** (all recall runs combined: 5 per model for R2 and R5, 2 for the rest; R5 now means exactly one sentence):

| Check | Cloud | Qwen | Notes |
|---|---|---|---|
| R1 keyword question | 2/2 | 2/2 | Qwen answers with no tool call (per-turn recall) |
| R2 paraphrase | 4/5 | **1/5** | cloud searches memory every time but wanders through files first (5.8 steps); Qwen used `memory_search` in only 2 of 5 runs and in none of the last 3 |
| R3 user fact in another workspace | 2/2 | 2/2 | |
| R4 project fact stays in its workspace | 2/2 | 2/2 | 6–7 wasted tool steps before "I don't know" |
| R5 hot preference ("one short sentence") | 5/5 | **2/5** | |
| R6 unrelated question | 2/2 | 2/2 | no memory tool calls |

**Reading it:** cloud already does everything the plan asks for, including person-vs-project scope (so D1 as a *prompt
rewrite* is not needed for cloud). The local model has two separate problems. (1) It saves a bare statement or an
explicit "remember that…" but **never saves when the message also asks a question** (0 of 24 scope-set runs, plus D1,
V1, E2); scope therefore cannot even be measured for it yet. (2) It rarely uses `memory_search`, so a paraphrase finds
nothing (R2 1/5), and it follows a hot preference less reliably (R5 2/5).

**Speed notes (for the next phases):** a cloud run of all sets takes about 2.5 minutes, a Qwen run about 8; the poll
interval is now 1 s (it was 4 s, which doubled a cloud chat), `EVAL_ONLY=S1,S4` / `R2,R5` runs just the unclear cells, and
cells are stopped when decisive instead of repeated to a fixed count. Cases cannot run concurrently against one backend
(a user-scope fact lands in the shared `global` store, so a new fact cannot be attributed to a chat).

## Gaps, evidence and the fix each gets

| # | Gap | Evidence | Fix (phase) |
|---|---|---|---|
| G1 (**mostly fixed by the save reminder, Phase 2**) | The local model **never saved when a message states a fact and also asks a question** (it saves bare statements and explicit "remember that") | Phase 0: Qwen 0/4 on D1, V1, S1–S6, E2 (0 of 24 scope-set runs); cloud 5/5 | Test interventions against the same cases (P2): guidance wording, a one-line reminder next to the user message |
| G2 | Facts about the person might be saved as project facts | **Cloud already scopes correctly** (birthday, city, timezone → user 15/15; hosts, ports, paths → workspace 14/15). The guidance text still says workspace unless "everything"; the model follows the subject anyway. Not measurable for Qwen until G1 is fixed | Keep the labelled set as a regression guard; rewrite the guidance wording (D1) only if Qwen mis-scopes once it saves (P2) |
| G3 | Paraphrased questions miss, and the local model rarely searches itself | R2: cloud 4/5, Qwen 1/5 (Qwen used `memory_search` in 2 of 5 runs, 0 of the last 3); FTS5 `unicode61` has no stemmer (`platform/memory/store.go:40-41`); recall is keyword-only (`recall.go`) | Porter stemming, search-tool wording, and (if needed) a fallback so a miss still shows the user's few facts (P3) |
| G4 (**fixed, Phase 1**) | The model could not correct its own memory | Only `memory_search` and `memory_update` exist; overlapping facts are concatenated (`tools/memory_tools.go` `insertFact`); `old_text` only finds facts in the route's own workspace | Tool contract: overlap returns the id, `memory_forget`, re-routing update (P1) |
| G5 | Wasted steps when memory has no answer | R4: 6–7 tool steps on both models | Tool-description guidance + measure (P3) |
| G6 | Hot block can be crowded out | Operator notes are "never clipped" (up to 12,000 chars with two files) against a 400–6000 char hot cap; the unit of `ContextBudget` (tokens vs chars) is unverified | Budget the notes inside the cap (P4, needs D4: it amends Constitution II.12) |
| G9 | Nothing in code stopped the agent from saving a secret (only the prompt said so) | Qwen saved "User's password is hunter2." after the reminder was widened | **Fixed (Phase 2):** one screen in the shared save path |
| G7 | No kind, reason or expiry on facts; user-scope session facts are never reaped | analysis #3–#5 | Typed facts (P5, SPEC-004 change, needs D5) |
| G8 | Automations teach it nothing | automation runs default to `session` keep (reaped at 90 days), heartbeats write none, no consolidation exists | Consolidation, review mode (P6, existing plan) |

## Decisions for the user

| # | Decision | Recommendation |
|---|---|---|
| D1 | Scope rule in the save guidance | **Classify by what the fact is about, not by the pronoun** (refined with the user 2026-10-16): facts about the person (birthday, home, timezone, name, how they like answers written, general working habits) → scope user, even when said inside a project workspace; facts about this project or environment (hosts, paths, conventions, decisions) → workspace. A preference that names nothing project-specific ("I prefer tabs in Go files") → user; one that does → workspace. When genuinely unclear, workspace: a person fact saved as a project fact is only forgotten elsewhere, a project fact saved as a user fact leaks into every workspace. Ship only if scope accuracy improves on a labelled set (below) on both models with no regression on D2/D3 |
| D2 | When `memory_update` finds an overlapping fact | Return the existing fact (id + text) and ask for `old_text` instead of concatenating; measured on D4/D5 |
| D3 | Let the model delete? | Yes, `memory_forget`, only on an explicit user request, exactly one match required, the deleted text echoed back and shown in the chat's "saved to memory" line. **A third memory tool amends Constitution II.12**, which names `memory_search` and `memory_update` as the registered memory tools |
| D4 | Amend Constitution II.12 so operator notes are bounded by the hot cap | Yes, with the preview and the overflow hint making any cut visible |
| D5 | Typed facts (kind, why, expires_at) — breaking, no shim | Yes, after P1–P3 are measured |
| D6 | Which automation runs feed consolidation | Chats first; add the news brief only after the review queue proves useful; never heartbeats |

**Approved by the user 2026-10-16:** D2, D3, D4, D5, D6 as recommended; D1 as refined above (the user's example: in a
network-scan workspace, "my birthday is …" or "I live in …" is a fact about the person, so it is global, not workspace).

## Phases

### Phase 0 — Make the evaluation repeatable (de-risk everything after it) — DONE 2026-10-16
- Both drivers exist (`scripts/memory-eval.py` for D1–D5, `scripts/memory-recall.py` for R1–R6). Add D1 variants
  (statement + question, statement only, "remember that…") and one **end-to-end case: the model saves in chat A, a
  fresh chat B uses the fact**. Re-baseline Part D with Qwen classified **local**, **≥5 runs per cell** on both models.
- **Check the guidance is really in the request** before anyone tunes `MemorySaveGuidance`: run the scratch backend with
  `-enable-runs -record` and read the head system message of a D1 request (the stored session history never shows it,
  so G1/G2 currently rest on inference).
- **Avoid the guardrail stall:** D1's gofmt question can trigger `execute_terminal_command`, which waits 5 minutes for an
  approval nobody gives; run unattended cells with `EVAL_EXCLUDE_TOOLS=execute_terminal_command` (both scripts support it).
- **Cloud needs the user's key** in a fresh scratch home (it is deleted with the job directory): a later session must ask
  for it instead of stalling. Cloud was reached with the user's own `command-code` key, never copied from vertex.
- **Acceptance (met):** `scripts/memory-eval.py` (D, V, S, E sets, `summarize`) and `scripts/memory-recall.py` produce
  the tables; the harness check passes; the baseline is recorded above; the recorded request shows the guidance in
  `messages[0]`.

### Phase 1 — Tool contract (G4) — DONE 2026-10-16
- **1a Overlap:** a fact whose topic overlaps a stored one is no longer concatenated onto it. The `memory_update` tool writes
  nothing and returns the similar fact (id + text); the model replaces it with `old_text` or keeps both with the new
  `separate: true`. Explicit capture and the per-chat review cannot retry, so they **replace the similar fact in place**
  (never dropped, never glued on). `MemoryUpdateArgs` is now a named type.
- **1b `old_text` across scopes:** it finds the fact in the workspace or the user scope and moves it when the new scope
  differs ("moved and updated memory entry N to M"), so "that applies to all my projects" no longer leaves a second copy.
- **1c `memory_forget(text)` (D3):** deletes the one saved fact containing the text; zero or several matches delete nothing;
  operator's own chat only (refused in automations and connector chats); echoes the deleted text; a mutating tool, not
  housekeeping. The step shows as "Forgot a memory" in the chat. **Constitution II.12 amended** (tool list, security
  review, and the no-secrets rule). The save guidance gained one sentence: when to call it, forget only what the user
  named, never delete project facts when asked to forget things about the user.
- **Measured:** forget set F1–F4 (real request, a user-scope request, a statement containing "forget", and "forget
  everything about me"): both models pass all four with the guidance. Without the sentence cloud **deleted all three
  seeded facts** on the mass request (project facts included) and Qwen one, which is why the sentence bounds it. Saving
  regression on the final build: cloud D1–D4 and S1–S6, E1, E2 pass (D5 correctly replaced the fact; the old score
  missed `the "new" host`, fixed); Qwen D1, D2, D3, D5, S1, S2, S5, S6, E2 pass and D4 saved the *missing* half of D1
  (correct; the score now only expects "no repeat" when D1 saved both).
- **Acceptance (met):** table-driven unit tests for each branch including the capture correction (`TestSaveFact_OverlapReplacesTheSimilarFact`),
  the cross-scope move, and every `memory_forget` branch; `go test ./... && go run ./tools/check-complexity/`; the Phase 0 tables show no
  regression on either model.

### Phase 2 — Make the local model save (request only; G1) — DONE 2026-10-16, with a generalisation audit
- **Tried, in order:** (a) guidance wording naming the statement-plus-question case — no effect, removed. (b) a one-line
  save reminder appended to the user's message in the request copy (the per-turn recall mechanism) — **works for the 16K
  Qwen** (0 of 24 → person facts and bare statements save). A first widening listed examples ("a host, port, path or
  convention") after those cases failed; that was **fitted to the test data**, and it made the model save a password, which
  added the code-level secret screen below.
- **Generalisation audit (2026-10-16, requested by the user: fixes must be generic, never fitted to test prompts):** a
  held-out set H (new domains and phrasings never tuned against: 8 save/scope cases and 3 do-not-save cases) was added to
  `scripts/memory-eval.py`. Result for the tuned wording on the local model: **6 of 11**, project facts stated with a
  question **0 of 3**, and it saved the content of a "summarize this" task. The reminder was rewritten **in category terms
  with no examples** (lasting things about the user; the project or environment; never the content of a task or text to
  process; never a credential). Held-out: local model **7 of 11** (the over-save gone; personal facts 3/4; a bare project
  statement saves), cloud **11 of 11**; earlier set: cloud 24 of 24 scored cases, local 4 of 6 (S1, S2, S3, S5). The
  forget guidance sentence was generalised the same way (a broad request covers only the kind of fact it is about). The
  held-out set is now spent for tuning: any further wording work needs a fresh one.
- **What wording cannot fix (honest ceiling):** a project fact stated together with a question is not saved by the local
  model (held-out 0 of 3) whatever the wording; the safety net for that is the per-chat memory review and consolidation
  (Phase 6), not more prompt text.
- **Code screen (G9):** `memory_update` and `SaveFact` refuse secret-shaped strings and statements of a credential's value
  (`tools.ErrSecretContent`); capture's broader word list was rejected for the agent path because it would refuse ordinary
  facts about tokens and keys. The red test showed three secrets were stored before the fix.
- **Acceptance (partly met):** do-not-save guards hold on both models; cloud saves and scopes everything on both sets; the
  local model saves personal facts and bare statements but not project facts stated with a question. **Watch item:** on
  a recall-hit question the model sometimes re-calls `memory_update` with the fact it just recalled (a duplicate the
  store dedups, one wasted call).

### Phase 3 — Recall strength (G3, G5)
- `memories_fts` tokenizer to `porter unicode61` (`store.go:40-41`; the schema already drops and rebuilds the index from the rows at startup, so there is no data migration and no shim);
  `memory_search` description tells the model to try synonyms once and not to browse files for facts about the user.
- **Acceptance:** `RecallQuery`/store tests for stemmed matches (prefer/preferred, deploy/deployment); R2 pass rate
  ≥ 80% over ≥5 runs per model; R4 tool steps down from the baseline; no R6 noise.
- **Not doing:** embeddings or a vector store (cost on a 16K local setup); revisit only if this phase misses.

### Phase 4 — Hot-block budget (G6; needs D4)
- Verify whether `ContextBudget` is tokens or chars (`assistant/hot_memory.go`, `hotMemoryCharsPerToken`); budget
  operator notes inside the hot cap; amend Constitution II.12 and SPEC-004.
- **Acceptance:** a test with two 6000-char notes files on a 16K model keeps the block within its cap and leaves room
  for facts or shows the overflow hint; `injection-preview` reports the cut.

### Phase 5 — Typed facts and expiry (G7; needs D5; SPEC-004 change)
- `kind` (user, feedback, project, reference), one-line `why` / `how_to_apply`, `expires_at` replacing the `session`
  type, a deterministic date normalizer in capture and review. Compact hot-line rendering for small windows.
- **Acceptance:** SPEC-004 bumped; store/tool/capture tests; the Phase 0 tables unchanged or better; user-scope
  session facts are reaped.

### Phase 6 — Learn from chats and automations (G8)
- Execute `memory-consolidation.md` Phases 0–2 on top of Phase 5: proposal table, hygiene, extraction from chats
  (D6), review mode by default, never silent writes, heartbeats excluded.
- **Acceptance:** per that plan, plus an acceptance-rate evaluation on recorded sessions on both models before any
  schedule is switched on.

## Scope note
"What I need next" (agenda, follow-ups, attention policy) is **not** in this plan; it is owned by the autonomy plans
(`../autonomy/`) and builds on Phase 5's typed facts. It is not forgotten, it is sequenced after this.

## Risks and non-goals
- **Prompt changes on thin data.** Mitigation: Phase 0 first, ≥5 runs, revert on regression.
- **`memory_forget` deleting the wrong fact.** One-match rule, echoed text, explicit-request-only guidance.
- **Local model quality.** Everything is measured on the 16K Qwen as the reference.
- **Non-goals:** automatic LLM writes without review; memory writes from heartbeats; embeddings now.

## Remaining Work
Phases 3–6. Decisions D2–D6 are approved and D1 is refined (see above); Phase 2 is now led by the local model's
save failure. Small related items tracked elsewhere: webhook run contexts, reminders
partial-failure retry (see `docs/audits/2026-10-15-next-work-analysis.md`).
