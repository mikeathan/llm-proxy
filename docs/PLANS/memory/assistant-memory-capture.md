---
status: active
date: 2026-10-05
related_specs: [SPEC-004, SPEC-001, SPEC-003]
related_plans: [memory/small-context-memory.md, memory/memory-improvements-implementation-plan.md]
evidence: docs/audits/memory-injection-investigation.md
---

# Assistant Memory Capture — Make Chats Produce Useful Memory With Any Model

> **Status (2026-10-05): active.** Decided with the user: **A** (explicit capture, deliberately small), **C**
> (per-chat review) and **D** (gated prompt guidance). The earlier "suggestions queue" (B) is dropped: C covers it.
> Built in order A → C → D (D was built on 2026-10-05 at the user's request, **before** its evaluation, Task 6, which must
> still be run). Design reviewed with the advisor.
>
> **Progress (2026-10-05):** Tasks 1–3 done and tested (package `memorycapture`: extractor + golden table + fuzz, `Capture`,
> `Reviewer`; one save path `MemoryToolProvider.SaveFact`; service wiring with `TurnRun.memory_saved`; review endpoint with
> lane claim). The turn's "Saved to memory" line is built and was checked on screen.
>
> **Task 4 (review drawer)** was written while the shell tool was unavailable and then verified on 2026-10-05: full frontend
> and backend suites, build, and complexity pass. **Capture bug found in live use (2026-10-05):** the first real use saved a
> sentence from a pasted playbook; fixed by skipping document-shaped messages and dropping `always`/`never` from auto-capture
> (regression test reads the real playbook file). Part D is built (Task 7). Remaining: Task 6 — run `docs/guides/memory-testing.md` Part D on the small-context local model (16K Qwen) and a cloud model and record the result here.

## Problem

With assistant memory on, a chat saves nothing unless the model decides to call `memory_update`, and nothing steers
that decision. The old proactive nudge was removed on 2026-06-07 with the automation-prompt simplification (no recorded
measurement of harm to chats); only the pre-sieve nudge at 70% of the context budget remains (`session.go:36,1378`),
which short chats never reach. Constraints from the earlier work: small models obey the most specific instruction over
general advice; no extra LLM call on the run path; the memory block must stay byte-stable within a run; automations and
the heartbeat are unaffected.

## Verified facts (code, 2026-10-05)

1. A tool-call message is never the final answer (`checkTaskCompletion`, `session.go:644`) and the UI shows the **last**
   assistant message (`turnGrouper.ts:82-86`): a model that answers *and* saves in one message, then says "Saved.", hides
   its answer. Guidance must say "save first"; capture in code (A) avoids it.
2. Secrets: `validateGlobal` runs for every tool call (`guardrails.go:205-213`) but `tools.SecretPatterns` has only
   three key shapes. Saves that bypass the tool path (A, C) must run their own guard, including sensitive words.
3. Dedup does not merge different facts (probe test), but contradictions coexist unless `old_text` is passed.
4. Three identical consecutive `memory_update` calls end a run ("infinite loop detected", `repetition_detector.go:13,81`).
5. The head system message is built once per session; the `<memory>` block is appended per request from a per-run frozen
   snapshot (`hot_memory.go:188`) and skipped when empty — guidance beside it must not depend on it being non-empty.
6. **Channel gate:** webhook/connector sessions have ids `wb_<platform>_…`; `models.SessionSource(id) == "manual"` is the
   operator's own UI chat. Capture runs **only** for manual sessions, so an outside sender cannot plant memory.
7. **Showing a save:** system-role messages are skipped by `groupTurns` and live events vanish on reload. Each turn already
   has a persisted run record (`models.TurnRun`, set by `startTurnRun`, rendered by `ChatBubble`); saved items are
   recorded there, so they persist and render with no new event type.
8. **Lane admission:** chats are admitted with `lane.ClaimInteractive(ctx, LaneKeyFor(""), ws, chatModel())`
   (`assistant_handlers.go:355`). The review call uses the same claim with a timeout; a busy lane returns "model busy".
9. Capture runs **before** the agent starts, so a fact saved as `always` is already in that same run's memory snapshot.

## Design

### New package `backend/internal/core/memorycapture` (domain only; no HTTP, SQL, `assistant` or `tools` imports)

Kept small and boring on purpose (target ≤ 8 source files + tests): plain functions in sequence, a list of guard
functions, and consumer-owned ports. No framework, no YAML loader, no registries.

| File | Responsibility |
|---|---|
| `types.go` | `Candidate{Content, Scope, Mode, Origin}`, `Outcome` (created / updated / duplicate), the ports `Library` (`Save`, `Has`) and `Completer` (`Complete(ctx, system, user)`), `SecretCheck func(string) bool` |
| `phrases.go` | **The keyword table** (Go slice literals, literal phrases only, lowercase): the single place to extend |
| `extract.go` | `Extractor.Extract(message) []Candidate`: strip code/quotes → split sentences → drop fillers → match a family at the sentence start → build the candidate |
| `guards.go` | `admit` guards shared by A and C (non-empty, length cap, secret, sensitive terms) and the A-only guards (too short, unresolved pronoun/interrogative start) |
| `capture.go` | `Capture(ctx, ex, lib, workspaceID, message) Result` — extract + save, returns what was saved; never returns an error to the caller's run |
| `review.go` | `Reviewer.Review(ctx, turns)` — bounded transcript (user/assistant text only) → `Completer` → tolerant JSON parse → guards → duplicate flag |

**The keyword table (A).** Matching is anchored at the start of a sentence after filler words, case/apostrophe
insensitive (`don't` = `dont` = `do not`, generated at construction, no regex in the data):
- *Explicit store verbs → on-demand, trigger stripped:* remember (that / to), don't forget (that / to), never forget,
  keep in mind, bear in mind, make a note (of / that), note that, take note, for future reference, for the record,
  save this, store this, write this down, log this, memorize/memorise, add (this) to memory, put this in memory.
- *"For later" markers → always:* from now on, going forward, in the future / in future, from here on (out),
  henceforth — trigger stripped. **Not** `always`/`never`/`whenever`/`every time`/`by default`: they are ordinary imperatives
  that often apply to one task only (a pasted playbook saved "Never reuse items … from memory" as an always-on fact on
  2026-10-05), so the review decides. A message shaped like a pasted document is never scanned.
- *Never saved:* questions; text inside code spans/blocks or `>` quotes; bodies under 3 words or over 500 chars; bodies
  starting with a bare pronoun (it, this, that, those, them) or an interrogative (what, when, how, why, who, which);
  `never mind`; any sentence mentioning password / secret / token / api key / private key / credential; secret-shaped text.
- At most 5 captures per message. Other languages are out of scope for A (C covers them); adding a language is adding
  phrase rows plus golden cases.

**Maintainability by tests, not by structure:** a golden table test (`phrase → expected`) where adding a keyword is one
phrase plus one case; a table-integrity test (lowercase, no duplicates, no phrase shadowing another, every phrase has a
golden case); `FuzzExtract` proving it never panics.

### Part A — wired into `conversationService.Execute`
After `startTurnRun`, for `SessionSource == "manual"` chats whose assistant memory is effectively on: `memorycapture.Capture`
saves through one path — `tools.MemoryToolProvider` gains a typed `SaveFact`/`HasFact` (`insertEntry` returns a typed
`Outcome`; the tool formats its existing strings from it, so there is **one** dedup path). Saved items are written to
`TurnRun.MemorySaved` and rendered on the turn ("Saved to memory: …", removable in the Memory panel). Failures are
logged only.

### Part C — "Review this chat for memories"
`POST /admin/api/assistant/{workspace}/memory-review` `{conversation_id}` (in the assistant handlers, which own the lane):
claims the lane like a chat, reads the saved session (user + assistant text only, last 12 messages / 6000 chars — never
tool results), calls the chat's model once with `prompts.MemoryReviewPrompt` (asks only for things the user stated or
decided, ≤ 5 `{content, scope, mode}` JSON items), validates in code, flags duplicates, returns the list. **It saves
nothing**: the UI shows a checklist and saves approved items through the existing Add-memory endpoint. Parse failure or
timeout → an empty list / "model busy", never an error in the chat.

### Part D — guidance (built; evaluation pending)
One constant in `templates.go`, appended beside the memory block for assistant chats with memory on (also when the block is
empty); not for automations/heartbeat. *Save lasting preferences, decisions and project conventions the user states, and
facts a tool verified; never task results, search results, things you can look up or secrets; call `memory_update`
before writing your answer; if a new fact replaces an old one pass `old_text`.* With it: `memory_update` "already saved"
says nothing more is needed and identical repeated saves produce the duplicate nag, not the loop error (fact 4).
**Built only after Task 6 passes** (no flag: sequencing).

## Tasks

### Task 1: `memorycapture` package (A engine) — pure, test first
- [ ] `phrases_test.go`: table integrity (lowercase, unique, no shadowing, each phrase covered by a golden case).
- [ ] `extract_test.go`: golden table (each family, contractions, fillers, mid-sentence ignored, questions, code spans,
  quotes, `never mind`, sensitive words, short/long bodies, pronoun/interrogative starts, several per message, cap of 5);
  `FuzzExtract`.
- [ ] `guards_test.go`, `capture_test.go` (fake `Library`: saved / duplicate / store error never propagates).

### Task 2: Single save path + wiring (A)
`tools/memory_tools.go` (`SaveFact`, `HasFact`, typed outcome; existing tests stay green), `tools/security.go`
(`ContainsSecret`), `models.TurnRun.MemorySaved`, `conversation_service.go` (after `startTurnRun`, manual sessions,
memory effectively on), frontend run line renders the saved items.
- [ ] Service tests: saved once and recorded on the run; webhook session saves nothing; memory off saves nothing;
  store error does not fail or delay `Execute`; a saved `always` fact is in the same run's memory block.

### Task 3: Review backend (C)
`prompts.MemoryReviewPrompt`, `memorycapture.Reviewer`, `handlers/assistant_memory_review_handlers.go` + route,
`Completer` adapter over the resolved model client.
- [ ] Reviewer tests (fake `Completer`): valid / fenced / prose-wrapped / malformed JSON, cap of 5, secret and over-length
  dropped, duplicates flagged, user+assistant text only, transcript bounds.
- [ ] Handler tests: lane claim and release, timeout → "model busy", unknown conversation → 404, nothing is saved.

### Task 4: Review UI (C)
Action in the assistant chat, checklist dialog, `useMemoryReview`; tokens only, plain copy, loading / empty / error /
saved states; screenshots desktop + phone.

### Task 5: Docs
SPEC-004 changelog + behaviour, api-reference, `docs/guides/memory-testing.md` (capture + review checks),
`.agents/skills/memory-system`, architecture directory map entry for the new package.

### Task 6: Evaluation (gates D) — needs the user's live models
Fixed scripts on the small-context local model (16K Qwen) and one cloud model, per `docs/guides/memory-testing.md`: explicit remember, implied
preference, pure research task (zero saves expected), a message containing a secret, a near-duplicate, a contradiction.
Record per model: saves that should happen, unwanted saves, final answer intact, run completed. Gate: unwanted saves ≈ 0
and no failed runs on both. Record results here even if negative.

### Task 7: Part D — built 2026-10-05 at the user's request, evaluation (Task 6) still to be run
Guidance constant `prompts.MemorySaveGuidance`, injection beside the block (`memorySystemText`, gated by
`guidesMemorySaves`), repetition-detector and tool-result hardening, with tests. It is **on** for the operator's own assistant
chats with memory on; if Task 6 shows over-saving, remove the one constant's use in `memorySystemText` (the rest stands).

## Review Focus

- "remember" inside a question, quote or code span must not save; "never mind" must not become a standing instruction.
- A capture or review failure must never change, delay or fail a chat run.
- Nothing from tool results, fetched pages, or non-manual (connector) sessions may reach memory without a user action.
- No secret or credential-like sentence may be saved by A or C.
- The user can always see what was saved automatically and remove it.

## Decisions (resolved 2026-10-05)

A tiny, C and D yes; B (suggestions queue) no; `from now on…` saves as `always`; the review uses the chat's own model;
the keyword list is a Go table with golden tests; new code lives in its own small package.
