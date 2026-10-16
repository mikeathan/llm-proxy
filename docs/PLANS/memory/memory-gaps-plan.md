---
status: proposed
date: 2026-10-16
last_reviewed: 2026-10-16
related_specs: [SPEC-004, SPEC-001, SPEC-005]
constitution_references: [II.12, II.13, IV.4, V.2, V.3]
related_plans: [assistant-memory-capture.md, memory-consolidation.md, small-context-memory.md, ../../audits/2026-10-15-next-work-analysis.md]
evidence: scripts/memory-eval.py (Part D), scripts/memory-recall.py (R1-R6), the runs recorded below
---

# Memory Gaps — Closing the "Assistant That Knows Me" Loop

**Status:** proposed. Nothing here is built. It turns the gaps found by the 2026-10-15/16 live tests into ordered,
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

## Gaps, evidence and the fix each gets

| # | Gap | Evidence | Fix (phase) |
|---|---|---|---|
| G1 | Small local model under-saves when a message states preferences *and* asks a question | Part D D1: Qwen 0/3 (branch), 1/2 (baseline); cloud 2/2 | Measure first (P0), then a guidance/test-driven change only if it moves the number (P2) |
| G2 | Facts about the person are saved as project facts (a birthday said in a project workspace would be lost everywhere else) | `MemorySaveGuidance` says "scope workspace… only if the user says it applies to everything"; runs saved "prefers tabs" at workspace scope; cloud sometimes user scope. **By design today**, so it is a decision, not a model failure | Person-vs-project rule in the guidance (P2, needs D1) |
| G3 | Paraphrased questions miss | R2 2/4; FTS5 `unicode61` has no stemmer (`platform/memory/store.go:40-41`); recall is keyword-only (`recall.go`) | Porter stemming + search-tool wording (P3) |
| G4 | The model cannot correct its own memory | Only `memory_search` and `memory_update` exist; overlapping facts are concatenated (`tools/memory_tools.go` `insertFact`); `old_text` only finds facts in the route's own workspace | Tool contract: overlap returns the id, `memory_forget`, re-routing update (P1) |
| G5 | Wasted steps when memory has no answer | R4: up to 9 tool calls | Tool-description guidance + measure (P3) |
| G6 | Hot block can be crowded out | Operator notes are "never clipped" (up to 12,000 chars with two files) against a 400–6000 char hot cap; the unit of `ContextBudget` (tokens vs chars) is unverified | Budget the notes inside the cap (P4, needs D4: it amends Constitution II.12) |
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

### Phase 0 — Make the evaluation repeatable (de-risk everything after it)
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
- **Acceptance:** one command per model produces the D and R tables; `./scripts/check-agent-harness.sh` passes; the
  baseline (numbers, build commit, models, setup) is written into this plan; the recorded request shows the guidance.

### Phase 1 — Tool contract (no schema change; G4)
- Overlap: the `memory_update` **tool** returns the similar fact instead of concatenating (D2). The shared save path
  (`SaveFact`/`insertFact`) is also used by explicit capture and the per-chat review, which cannot "retry with
  `old_text`": they keep an explicit, tested behaviour of their own (a captured correction such as "remember that we now
  deploy through X" must replace or visibly report the conflict, never silently save nothing). `memory_update` with
  `old_text` searches the workspace and `global` and re-routes when scope/mode/keep differ. Add `memory_forget` (D3) through the file-change
  checklist for a new tool (`docs/architecture.md`): `models/tools.go`, `manifests/memory.json`, `tools/memory_tools.go`,
  `assistant/registry.go`, guardrail/toolpolicy effect, tests.
- **Acceptance:** table-driven unit tests for each branch, including a capture/review correction that replaces or
  reports; `go test ./... && go run ./tools/check-complexity/`; the Constitution II.12 amendment lands with
  `memory_forget`; the Phase 0 D and R tables show no regression on either model.

### Phase 2 — Save guidance (prompt only; G1, G2)
- Only after Phase 0 numbers: the subject-based scope rule (D1) and, if the data says so, a one-line nudge for the
  statement-plus-question case. One constant (`MemorySaveGuidance`), one commit, revertable.
- **Labelled scope set** (added to the eval script in Phase 0, each stated inside a workspace chat): person facts
  (birthday, home city, timezone, answer style) expect `user`; project facts (a host, a path, a convention) expect
  `workspace`; ambiguous ones (a coding preference with and without a project noun) are recorded, not scored.
- **Acceptance:** over ≥5 runs per model, scope accuracy on the labelled set is higher than the Phase 0 baseline with no
  person fact saved as workspace scope more often than before, D2/D3 stay at zero saves, D1 saves do not drop.

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
All phases (0–6). Decisions D1–D6 are open. Small related items tracked elsewhere: webhook run contexts, reminders
partial-failure retry (see `docs/audits/2026-10-15-next-work-analysis.md`).
