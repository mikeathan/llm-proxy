---
subsystem: memory
tags: [planning, memory-contract]
result: reference
status: reference
date: 2026-10-15
---

# Next-Work Analysis — an assistant that knows the user (2026-10-15)

**Status:** reference. Analysis only; no code was changed. Authentication work is excluded by instruction
(`admin-api-authentication.md`, and the Telegram sender-ID check in `cross-channel-conversations-and-remote-approvals.md`
Phase 0, are left out of the ranking).

**Goal (user's words):** an assistant that knows me, understands what I do and what I need next, learns from my prompts
and automations about me and my goals, and works with cloud and local models.

## Evidence labels

- **[code]** opened in the repo during this analysis.
- **[plan]** stated by a plan/SPEC/doc only; not re-checked against code.
- **[note]** from an earlier session's notes; not re-checked.
- **[advisor]** Opus advisor's own reading or judgement (see "How this was produced").
- **[assumption]** mine.

## Where things stand

| Area | State |
|---|---|
| Plan tracker | ~40 non-complete plans in `docs/PLANS/README.md`; most of the autonomy program (M2–M5) is `proposed` with nothing built. **[plan]** |
| Reminders (M1) | Built (SPEC-011, PRs #73–#78). Open: per-channel retry of partial failures, `/run` outside the command registry. **[note]** |
| Memory | Hot block + per-turn keyword recall + explicit capture (A) + per-chat review (C) + save guidance (D) are built. **[code]** Capture evaluation (Task 6) was never run. **[plan]** |
| Learning from automations | None reaches long-term memory: automation runs default to `session` keep, reaped at 90 days; heartbeats write no memory by decision; automation hot injection is off by default. **[code/plan]** |
| Consolidation ("sleep") | Plan only (`memory/memory-consolidation.md`, proposed). Not started. **[plan]** |
| `// TODO` / `FIXME` in non-test Go | None found. **[code]** |

## Findings (verified)

1. **`memory_update` silently drops the model's scope and mode when `keep` is omitted.**
   `backend/internal/core/tools/memory_tools.go:198-202`: if any of scope/mode/keep is empty, all three are reset to
   workspace / on_demand / default keep. `scope=user, mode=always` with no `keep` is stored as a workspace on-demand
   fact: a personal preference never becomes global or hot, with no error. **[code, confirmed by me and the advisor]**
   Not checked: whether a test pins this.
2. **Overlapping facts are concatenated, not updated.** Different content that overlaps an existing fact is appended
   (`memory_tools.go:317-323`), so facts grow instead of being replaced in place. SPEC-004 §6 and the skill describe
   this as dedup. **[code: survey + advisor]**
3. **A wrong fact cannot be removed by the model.** Only `memory_search` and `memory_update` exist; deletion is
   operator UI/API only. **[code: advisor; plan: SPEC-004 changelog 2.5]**
4. **No fact kind and no "why / how to apply".** Fields: workspace_id, memory_type, title, content, tags, source,
   priority, usage counters. `memory_type` mixes kind, retention and scope. **[code]**
5. **User-scope session facts are never reaped.** `user_on_demand_session` routes to `user_profile`
   (`memory_tools.go:50-56`); the reaper deletes only `session` rows by `created_at`. No `long_term` reaper.
   **[code: survey + advisor]**
6. **Usage counters are collected and never read.** `usage.go` records and flushes; only `ListUnused` (an API) reads
   them. **[code: advisor]**
7. **Recall misses word variants.** FTS5 uses `unicode61` with no stemmer (`store.go:41`), up to 8 OR terms, 3 facts.
   **[code: advisor]**
8. **Operator notes are unbudgeted.** Each notes file may be 6000 chars and is "never clipped"; global + workspace notes
   (12,000 chars) can exceed the whole hot budget (400–6000 chars) and push out every saved fact.
   **[code: advisor; I confirmed `MaxOperatorNotesChars = 6000` at `platform/memory/notes.go:25`]**
   **Unverified:** whether `ContextBudget` is in tokens or chars (`hot_memory.go` defines `hotMemoryCharsPerToken = 4`,
   `contextShare` multiplies the budget by 8%/5%). If tokens, a 16K local model's hot share is ~1,300 chars.
9. **`MemorySaveGuidance` defaults personal facts to workspace scope** (`prompts/templates.go:465`), so what the
   assistant learns about the user stays in one workspace. **[code: advisor]**
10. **Heartbeat `alert` is recorded even when nothing was sent** (no connector). `heartbeatOutcome` downgrades only on
    a send error (`automation/notify.go:131-140`). **[code]**
11. **Webhook runs use `context.Background()`** (`transport/http/handlers/webhook_handlers.go:230,269,276`), not the app
    lifecycle context (Constitution II.2/II.14 per the cross-channel plan). Lifecycle, not auth. **[code]**
12. **Tracker contradiction:** `docs/PLANS/README.md:56` says Agent OS Sandboxing is "complete (pending Linux-CI)";
    line 112 lists it as "proposed, Phases 1–6". **[code]**
13. **Resolved since the 2026-10-12 audit:** the hard-coded "(provider error)" label (#76), and the webhook timezone
    (`agentTimezone()`/`OwnerTimezone` exist). The console cross-contamination fix is merged (#81). **[code/git log]**
14. **SPEC-004 is stale in places:** §5 lists a `topic` parameter that does not exist; §2 says keep=session stores
    `session` (not true for user scope); "newest first" vs `priority DESC, updated_at DESC`. **[code vs plan: survey]**

## Recommended order

Ranked by value to "knows me", cost on a 16K local model, and risk.

| # | Track | Size | Why now | Needs |
|---|---|---|---|---|
| 1 | **Verification debt**: run memory-capture Task 6 (explicit remember, implied preference, research task = 0 saves, secret, near-duplicate, contradiction) on the 16K Qwen and one cloud model. Part D guidance is live without its gate. | S–M | Cheapest way to learn whether D helps or over-saves on each class of model; gates every later memory step. | The user's live models |
| 2 | **Memory tool contract** (finding 1–3, 9): default each field independently; on overlap return the similar fact's id instead of concatenating; add `memory_forget`; let an `old_text` update change kind/scope/expiry; point personal facts at user scope. | S | Fixes silent data loss today; no schema change. | None |
| 3 | **Typed facts + expiry** (finding 4–7): `kind` (user/feedback/project/reference), one-line `why` and `how_to_apply`, `expires_at` replacing the `session` type, `evidence`, porter tokenizer; drop `memory_type` and the dead `Daily` constant. No migration shim (product is new). | M | Makes "feedback" and "project" facts storable with a reason; fixes reaping. | **User approval: changes SPEC-004** |
| 4 | **16K rendering**: compact hot lines (`- [kind] content — apply: …`; `why` never injected), `(as of date)` on recall, cap operator notes inside the budget, resolve the tokens-vs-chars question. | S | Keeps the memory block inside a small window. | Step 3 for the line format |
| 5 | **Typed capture/review with absolute dates**: review JSON gains kind/why/how; a deterministic normalizer rewrites "next Friday"/"tomorrow" using the per-turn local clock already built for reminders. | S | Captures what you say with a reason and a date that stays true. | Step 3 |
| 6 | **Consolidation Phases 0–1** from `memory-consolidation.md`: proposal table, hygiene (usage counters finally drive demotion; never delete), then LLM extraction from chats and automation runs (heartbeats excluded) as typed, reviewable proposals with evidence. Review mode by default. | L | The only path that lets automations and chats teach the assistant about you without silent writes. | Steps 1, 3; weekly eval on recorded sessions |
| 7 | **Then "what I need next"**: agenda `RunReason` → `schedule_followup`, an "open goals" section built from `project` facts, then attention policy. | M–L | Builds on typed facts and reminders. | Steps 3, 6 |

**Small, independent fixes (any time):** webhook run contexts tethered to the app lifecycle (11); honest heartbeat
result when no channel is configured (10, `heartbeat-seen-ledger.md` phase 3); the PLANS tracker contradiction (12);
SPEC-004 wording (14); reminders partial-failure retry **[note]**.

### What not to build (advisor)

Files on disk per fact (the SQLite row is the file; the hot block is the index; the existing markdown export is the
human-editable view); embeddings or a vector store (cost on a 16K local setup; stemming plus consolidation covers most
paraphrase); any automatic LLM write without review; memory writes from heartbeats; a separate index document;
`session_search` before consolidation exists (consolidation reads sessions directly).

### Mapping to the typed-memory pattern

| Pattern element | Mapping |
|---|---|
| Fact types user/feedback/project/reference | `kind` column |
| Why + How to apply | `why`, `how_to_apply` (one line each, ~160 chars) |
| Index file | The rendered hot block plus fact titles |
| Update in place | Overlap returns the existing id; `old_text` update; no concatenation |
| Delete wrong facts | `memory_forget(id)` + consolidation `delete` proposals (accepted by the user) |
| Absolute dates | Deterministic normalizer at capture/review time |
| Verify before recommending | Fixed instruction beside recalled facts: check named files/symbols first |
| Human-written layer | Operator notes (`MEMORY.md`) stay hand-edited and are never touched by consolidation |

### Biggest risk (advisor)

The extraction/merge pass on the 16K local model: it can overflow the window, produce low-quality or contradictory
proposals, and flood the review queue; wrong accepted facts about the user become permanent. Mitigations: one session
or run per call, compare only against FTS top-k candidates, schema-constrained output, a per-night proposal cap, review
mode with evidence on every proposal, and an evaluation of acceptance rate on recorded sessions on both a local and a
cloud model before the nightly schedule is switched on.

## Open questions for the user

1. Approve the SPEC-004 change (typed facts, `expires_at`, breaking, no shim)?
2. Can the Task 6 evaluation run on your 16K Qwen and a cloud model now?
3. Should consolidation read automation runs other than the heartbeat (e.g. the news brief), or chats only at first?

## How this was produced

Direct reading by the main session: plan tracker, autonomy roadmap, memory plans, memory skill, SPEC-004 head, and the
code checks marked **[code]** without an attribution. A haiku worker produced the file:line survey of the memory
code and docs. An Opus agent (acting as the advisor) was given the question, that survey and file paths and read the
files itself; it confirmed finding 1 and added findings 3, 8 and 9 and the "not to build" list. The Opus agent reported
it did not call the separate advisor tool. Not reviewed by anyone: the frontend, the agent-loop plans
(`agent-improvements.md`, GPU/cloud performance, backend hot-paths), and the connectors; they are not ranked above.
