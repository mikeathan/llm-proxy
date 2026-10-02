---
id: SPEC-004
title: Memory System
version: "2.0"
status: stable
last_updated: 2026-06-09
constitution_references: [II.12]
related_specs: [SPEC-001, SPEC-006]
supersedes:
---

# SPEC: Memory System

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
- **Automations opt in** with `memory_mode: hot` (default off/unset): `AutomationEntry.MemoryMode` →
  `ExecuteRequest.MemoryMode` → `buildAgentOptions` sets `MemoryStore` + `EnableHotMemory`. Same frozen
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

### 5. Memory Tools

- `memory_search(query, limit, scope, tags)` — FTS5 search with BM25 ranking, capped at 20 results.
- `memory_update(topic, content, scope, mode, keep)` — Save a new memory entry with three-tier params.
- `memory_delete(id)` — Delete by ID.
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
