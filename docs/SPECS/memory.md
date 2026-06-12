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

- Once per session (first LLM turn only), `injectActiveMemory()` fetches all entries tagged
  `["hot"]` via `SearchHot()` — no FTS5 query.
- Entries are injected as a `<memory>` system message right before the last user message.
- Text capped at 2000 chars (`maxHotInjectionChars`), truncated on entry boundaries.
- Both global (`workspace_id = 'global'`) and workspace entries with the hot tag are injected.
- **Not injected for automation tasks** — see `docs/audits/memory-injection-investigation.md`.

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
