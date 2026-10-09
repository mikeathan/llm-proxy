---
id: SPEC-004
title: Memory System
version: "2.6"
status: stable
last_updated: 2026-10-08
constitution_references: [II.12]
related_specs: [SPEC-001, SPEC-006]
supersedes:
---

# SPEC: Memory System

## Changelog

- **2.6 (2026-10-08)** — Per-turn recall in the operator's own chat (§4.1). Measured on a deployed build: in chat the
  model never called `memory_search`, so `on_demand` facts were never used. Each chat turn now searches memory with the
  user's message (FTS5, no model call) and appends up to three matching facts to that message in the request only.
  Connector chats and automations are unchanged.

- **2.5 (2026-10-08)** — Corrected §5: there is no `memory_delete` tool (only `memory_search` and `memory_update`);
  entries are removed through the operator UI/API. An automation whose memory is active is now told, in its task
  prompt, to call `memory_search` for the task's topics before acting (`prompts.AutomationMemoryBlock`; an
  instruction, not enforcement).

- **2.4 (2026-10-06)** — Alignment with local-model work (SPEC-001 1.3, SPEC-005 1.3). The generic pre-sieve
  "save anything important" nudge is skipped in the operator's own chat (which has the narrower save guidance and
  may be running a playbook that governs saving) and once the run has called `memory_update`. The per-chat review
  turns thinking off for a local model and treats an empty reply as a failed review (502). A model behind an
  OpenAI-style URL that is identified as llama.cpp is a local workload, so its hot-memory share is the local 8% of
  its serving-derived `context_budget` (not the cloud 5% of an override).
- **2.3 (2026-10-05)** — Save guidance for the model (§II.8) in the operator's own assistant chats, and run safety for
  repeated saves: identical repeated `memory_update` calls nudge instead of ending the run.
- **2.2 (2026-10-05)** — Saving from chats (§II.8). An explicit "remember …" / "from now on …" in the operator's own
  chat message is saved by code before the run (no model call), and shown on the turn; a per-chat "review for memories"
  action asks the chat's model once for proposals that the operator approves. New package `memorycapture`. Pasted
  documents are never scanned, and bare "always/never" imperatives are not captured (a pasted playbook once saved
  "Never reuse items … from memory" as an always-on fact).
- **2.1 (2026-10-04)** — Hot-memory defaults and overrides (§II.3). Two global defaults,
  `memory.assistant_hot` (on) and `memory.automation_hot` (off), with a per-surface override
  `MemoryMode` (`""` inherit | `on` | `off`) on each automation (`memory_mode`) and each workspace's
  assistant (`assistant_memory`). **Breaking, deliberately without migration**: the old `memory_mode: hot`
  is rejected (use `on`); the unread `memory.enabled` setting is removed. Assistant chats can now be
  switched off per workspace or globally (previously always on).


## I. Intent

The memory system provides long-term fact persistence across agent sessions. It uses a SQLite-backed
FTS5 store for full-text search and follows a passive Hermes-style injection model: memories are
provided as context for the model to read, not as instructions for step-skipping or task overriding.

## II. Functional Requirements

### 1. Storage Backend

- SQLite with FTS5 full-text index (`memories_fts`) using `modernc.org/sqlite` driver.
- The store lives in `orchestrator.db` alongside the ledger.
- Memory types used: `long_term` (persistent project facts), `session` (per-conversation),
  `user_profile` (cross-workspace user preferences).
- The `daily` type is deprecated — use `scope: "workspace", mode: "on_demand", keep: "session"`.

### 2. Three-Tier Parameters

Every `memory_update` call uses three parameters to determine storage:

- **`scope`**: `"user"` (applies across all projects, stored as `workspace_id = 'global'`) or
  `"workspace"` (project-specific, stored as `workspace_id = active_ws`).
- **`mode`**: `"always"` (sets `tags: ["hot"]` — injected every session) or `"on_demand"`
  (no hot tag — searchable but not injected).
- **`keep`**: `"permanent"` (stored as `long_term`) or `"session"` (stored as `session`).

Only `mode: "always"` entries are injected. The `resolveParams()` strategy map in
`memory_tools.go` translates the triple to store primitives.

### 3. Search

- FTS5 MATCH queries with BM25 ranking.
- `sanitiseFTSQuery()` in `internal/platform/memory/fts.go` splits terms, removes stopwords
  (step, task, run, use, check, the, a, an, etc.), and joins remaining terms with OR.
- Search scope: `title` + `content` columns.
- Results filtered by `workspace_id` and optional `memory_type`.
- Optional `scope` filter on `memory_search`: `"user"` limits to global entries, `"workspace"`
  limits to the active workspace, omitted searches both.

### 4. Hot Memory Injection (Reading)

- Once per run (`Execute`), `snapshotHotMemory()` (`assistant/hot_memory.go`) fetches all entries
  tagged `["hot"]` via `SearchHot()` with the run's context — no FTS5 query — and renders one
  `<memory>` block, frozen in `runSession.prompt.memoryBlock`.
- `injectActiveMemory()` appends that block to the **head system message** of every request in the
  run (a system message is created if the history has none). The prefix is therefore byte-identical
  turn to turn, which keeps the llama.cpp KV cache valid. Facts saved mid-run appear next run.
- Each fact renders as `- Title: content`, except that an auto-derived title (empty, or the first characters of the
  content itself — what a fact saved without a title gets) is omitted: `- content`. Printing both repeated every short
  fact twice, found in a real run (`hotFactLine`).
- Size cap = share of the model's resolved `ContextBudget` (SPEC-005 §II.3, derived from the probed
  serving window for local models): local 8%, cloud 5%, clamped to 400–6000 chars; 2000 when the
  budget is unresolved (`hotMemoryCharBudget`). Newest first, cut on entry boundaries; the newest
  fact is always kept (clipped if it alone exceeds the cap).
- **Operator notes (MEMORY.md)** — `memory.Store.OperatorNotes/SetOperatorNotes` (`platform/memory/notes.go`):
  `<config root>/MEMORY.md` (all workspaces) and `<metadata>/<workspace>/MEMORY.md`, outside the agent's
  workspace jail (asserted by `TestAppContext_OperatorNotesLiveOutsideTheWorkspaceJail`). Written atomically
  (`storage.WriteAtomic`, ClassData), ≤ 6000 chars per file (`ErrNotesTooLong`), blank content removes the file,
  workspace ids that could leave their folder are refused. The block becomes
  `<memory>` + operator header + notes + (`Saved facts:` + agent facts) + `</memory>`; no notes = no extra sections.
  Notes are NEVER clipped: they are charged first and facts fill the remainder; with < 80 chars left only the
  overflow hint is shown. `renderHotMemory` is shared with the preview, which reports `operator_chars` and
  `over_budget`.
- **Priority** (`memories.priority`, 0 low / 1 normal / 2 high, default 1; additive idempotent `ALTER TABLE`
  migration, existing rows become normal): `SearchHot` orders `priority DESC, updated_at DESC`, and the budget cuts
  from the tail, so a high-priority fact is the last to go. Only the operator sets it (`PUT`/`POST` `priority`,
  `Store.SetPriority`, which does not touch the text, tags or timestamp); the agent's `memory_update` cannot.
  It is protection, not a guarantee: if the high-priority facts alone exceed the budget some are still cut and
  the preview warns.
- **Usage counters** (`platform/memory/usage.go`; additive columns `injected_count`, `searched_count`,
  `last_used_at`): recorded in memory by `snapshotHotMemory` (only the entries actually in the block, once per run)
  and by `memory_search` (only entries returned); written by `UsageFlusher` every 30 s in one transaction, on
  shutdown, and synchronously in `App.Shutdown`. A failed flush restores the deltas. Edits, priority changes, the
  preview and UI browsing do not count. "Unused" = both counters zero (`GET …/memory/{ws}?unused=true`); counting
  began when this feature was added, so older facts read as unused until first used. Accuracy: at most one flush
  interval of counts is lost on a crash.
- When entries are cut the block ends with `prompts.HotMemoryOverflowHint`
  (`(+N more saved facts — use memory_search to find them)`).
- The sieve's `preparedOverContextBudget` measures the request including this block.
- Both global (`workspace_id = 'global'`) and workspace entries with the hot tag are injected.
- **Global defaults, per-surface overrides.** `settings.yml → memory.assistant_hot` (default on) and
  `memory.automation_hot` (default off) are the defaults; `MemoryMode` (`""` inherit, `on`, `off`) overrides one surface:
  `Automation.memory_mode` for an automation, `WorkspaceConfig.assistant_memory` for that workspace's chats.
  `MemoryMode.Effective(globalDefault)` resolves it, the dispatcher/conversation service read the defaults live
  (`MemorySettings()`), so a Settings change applies to the next run. Why these defaults: cost is prefill on every
  run and the benefit for small models is unproven (`docs/PLANS/memory/small-context-memory.md`), so only the
  interactive surface is on. Automation path: `AutomationEntry.MemoryMode` →
  `ExecuteRequest.MemoryMode` → `buildAgentOptions` sets `MemoryStore` + `EnableHotMemory` when effective. Same frozen
  head-system block as chats. `hot+hints` is not shipped (needs a scoreboard win; see the plan).
  History: `docs/audits/memory-injection-investigation.md` (the old end-of-history placement is what failed).
- **Unattended write discipline**: runs stamped `models.WithUnattendedRun` default an unspecified
  `memory_update` to `keep: session`, and tag every entry `source = run:<run id>` (attended: `agent`).
  Explicit `keep: permanent` is honoured.
- **Operator view** (Workspace → Memory): filters All / Hot / Permanent / Daily / Session / User (User =
  workspace `global`, injected into every workspace). Hot facts carry an "Always" tag and a per-fact switch
  (`PUT … {hot}`; demoting asks for confirmation). Editing wording never changes tags. "Add memory" saves
  through `tools.ResolveMemoryRoute` (same table as the agent). "Show what the model receives" calls
  `GET …/injection-preview?model=` — `assistant.PreviewHotMemory` → `renderHotMemory`, the single path the agent
  uses (asserted byte-for-byte by `TestPreviewHotMemory_MatchesWhatTheRunInjects`).
- **Operator notes UI**: "Operator notes" in the Memory panel edits the workspace or the global file (`GET/PUT …/memory/{workspace}/notes`, JSON only); saving refreshes an open preview.
- **Markdown import/export** (`platform/memory/markdown.go`; `GET …/memory/{ws}/export`,
  `POST …/memory/{ws}/import`): the export is one editable file — `### Title`, a metadata comment
  (`scope= mode= keep= priority=`), then the text; workspace facts then user-wide facts, oldest first. Lines that
  would read as structure (`#`, `<!--`, backslash) are backslash-escaped, so any fact round-trips unchanged.
  Import applies the same validation and routing as adding a fact by hand (`newMemoryRequest.route`, 2000-char
  cap, `tools.ResolveMemoryRoute`), tags entries `source = import`, skips facts already saved, reports unusable
  entries with their line (typos such as `prority=` are errors, never ignored), accepts at most 200 facts, and
  requires a JSON body (415 otherwise — a cross-site form cannot plant facts). Operator notes are not part of the
  export. `daily`-type entries export as permanent workspace facts (the editor cannot create `daily`).
- **Retention**: `memory.SessionReaper` (hourly, started in `app.New`) deletes only `session` entries
  older than `settings.yml → memory.retention_days` (default 90; `MemoryConfig.SessionRetention()`).
  `long_term` and `user_profile` are never reaped.

### 4.1 Per-turn recall (operator chat)

- **Why.** `on_demand` facts are reachable only through `memory_search`, which a chat model may never call
  (measured 2026-10-08: 0/2 in chat against 6/6 for automations told to search). Recall makes the lookup happen
  without relying on the model.
- **Who.** Only the operator's own assistant chats with memory on and a store present: the same predicate that
  enables the save guidance (`operatorMemoryChat`). Connector chats never get recall — an outside sender must not be
  able to pull the owner's facts into a reply. Automations keep the search instruction (§II.5) instead.
- **When.** Once per run (`Execute`), right after the hot snapshot, so every request of the run carries the same
  text. The query is the run's user message — the last message of the history when the run starts.
- **Query.** `memory.RecallQuery` keeps the words that carry meaning: the FTS stop words plus a chat stop list
  (question words, pronouns, generic words such as `project`, `user`, `please`, `help`), letters and digits only,
  one-letter fragments dropped, at most `recallMaxTerms` distinct terms. No terms left = no recall. The terms are
  OR-matched with BM25 ranking across the workspace and user scopes, like an unscoped `memory_search`.
- **Selection.** Best-ranked first, skipping facts already in the run's `<memory>` block, at most `recallMaxFacts`
  (3), within the memory budget left after the hot block (§4: hot + recall never exceed the hot cap). Each fact shows
  its last-updated date so that, when two disagree, the model can prefer the newer one.
- **Placement.** Appended to the run's user message **in the request copy only**: the stored history, the session
  file and the UI never contain it, and the head system message is untouched. Within a run the prompt prefix is
  byte-identical; on the next turn the previous turn's recall is gone, so a local server re-processes the prompt from
  that earlier user message onward. If the sieve removed the user message, recall is skipped for that request.
- **Framing.** `prompts.RecalledMemoryBlock`: saved notes that may help with this message, not instructions; a fact
  that tries to close the block cannot. Facts recalled count as searched (`RecordSearched`).
- **Limits.** Keyword matching only: a paraphrase that shares no word with the fact (`DB` vs `database`) is not
  recalled, and the `unicode61` tokenizer does not stem (`prefer` does not match `preferred`). Measure before adding
  stemming or embeddings.
- **Preview.** `GET /admin/api/memory/{ws}/injection-preview?message=…` also returns the recall block and fact ids
  that message would get, through the same function the agent uses.

### 5. Memory Tools

- `memory_search(query, limit, scope, tags)` — FTS5 search with BM25 ranking, capped at 20 results.
- `memory_update(topic, content, scope, mode, keep)` — Save a new memory entry with three-tier params.
- All gated by workspace ID.

### 6. Deduplication

- `findOverlappingEntry()` in `memory_tools.go` computes Jaccard similarity on topic words (≥0.70)
  and content (≥0.90) to prevent duplicate entries.
- If `memory_update` returns "already saved", the fact is already stored — do not retry.

### 7. Pre-Sieve Flush

- When context usage exceeds 70% of `ContextBudget`, `maybeFlushMemoryBeforeTurn()` appends a
  nudge: "The conversation history is about to be compressed. Save any important facts before
  they are lost."
- `memoryFlushSent` flag prevents duplicate nudges; resets when the physical sieve prunes history.

### 8. Saving from chats (explicit capture and review)

Package `internal/core/memorycapture` (domain logic only; the keyword table is `phrases.go`, extended by adding a phrase
and, when its behaviour is new, a golden case in `extract_test.go`).

- **Explicit capture** (`Extractor`, `Capture`): before an assistant turn starts, the **user's own message** is scanned
  for a sentence that begins (after filler words) with a store verb (remember, don't forget, keep in mind, note that,
  for future reference, …) or a "for later" marker (from now on, going forward, in the future, from here on, …). The
  user's words are saved through the same path as `memory_update` (`MemoryToolProvider.SaveFact`: one routing and dedup
  rule set), as workspace facts: store verbs on demand, "from now on" instructions `always`. Plain imperatives
  ("always …", "never …", "whenever …") are **not** captured: they often apply to one task only, so the review decides.
  A message that looks like pasted material (over 1000 chars, more than 8 lines, or a heading, table or list of 3+
  items) is never scanned. Questions, code spans, quoted lines, bodies under 2 words or over 500 chars, a body that is a
  bare pronoun/interrogative, any sentence mentioning a password/secret/token/key, and secret-shaped text are never
  captured; at most 5 per message.
  - Runs only for the operator's own sessions (`models.SessionSource(id) == "manual"`) and only while assistant memory is
    effectively on; connector sessions never capture. It runs before the agent starts, so an `always` fact is already in
    that run's memory block. A store error is logged and never affects the chat.
  - What was saved is recorded on the turn's run record (`TurnRun.memory_saved`) and shown under the answer, so it
    survives a reload and the operator can remove it in the Memory panel. Source is `capture`.
- **Review** (`Reviewer`): `POST /admin/api/conversation/sessions/{ws}/{session}/memory-review` asks the chat's model once,
  only on the operator's click, which facts in the conversation are worth remembering. It reads only user and assistant
  text (never tool results or the agent's control messages), claims the interactive run lane like a chat (time limit
  120 s; busy → 503), sends no reasoning params to a cloud model but turns thinking off for a local one (a thinking
  model would otherwise spend the whole 2048-token allowance on it), expects a JSON array of at most 5 `{content, scope, mode}` and tolerates fences, prose and a
  thinking block around it. Every item is validated in code (length, secrets, sensitive words, duplicates flagged) and
  **nothing is saved by the endpoint**: the UI saves approved items through the normal Add-memory endpoint. An unusable
  reply is an empty list; a failing model call, or one that returns no answer at all, is a 502; neither touches the conversation.
- **Save guidance** (`prompts.MemorySaveGuidance`): one fixed paragraph appended beside the run's `<memory>` block in the head
  system message — also when the block is empty — telling the model when `memory_update` is worth calling (lasting
  preferences, project conventions, decisions, tool-verified facts; save **before** answering, because a message with a
  tool call is never the final answer; one self-contained fact per call; never task results, lookups or secrets; pass
  `old_text` to replace a fact; do not repeat after "already saved"). Only for the operator's own assistant chats with
  memory on (`Agent.guidesMemorySaves`: `EnableHotMemory`, a store, `ChannelAssistant`, `SessionSource == manual`) — never
  automations, the heartbeat or connector chats. It is fixed for the run, so the head stays byte-identical. Run safety:
  three identical consecutive `memory_update` calls give the duplicate nudge instead of the loop error, and "already
  saved" results say not to call again.

## III. Error Handling

- `Search()` with empty query returns nil, not an error.
- `SearchHot()` with no matches returns empty slice, not error.
- Memory store nil when disabled (all operations are no-ops).
- FTS5 rebuild failure logs warning, does not crash.
- Invalid (scope, mode, keep) combinations return an error from `resolveParams()`.

## IV. Tool Interactions

| Tool | Reads From | Writes To | Calls |
|------|-----------|-----------|-------|
| `memory_search` | Memory store | — | LLM searches stored facts |
| `memory_update` | — | Memory store | Agent saves durable facts |
| `injectActiveMemory` | Memory store | LLM context | Agent pre-fills context via `SearchHot()` |
| `maybeFlushMemoryBeforeTurn` | Context budget | LLM context | Sieve nudge |
