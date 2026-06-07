---
id: SPEC-004
title: Memory System
version: "1.0"
status: stable
last_updated: 2026-06-07
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
- Three memory types: `long_term` (durable facts), `short_term` (session-scoped), `user_profile`
  (user preferences and style).

### 2. Search

- FTS5 MATCH queries with BM25 ranking.
- `sanitiseFTSQuery()` in `internal/platform/memory/store.go` splits terms, removes stopwords
  (step, task, run, use, check, the, a, an, etc.), and joins remaining terms with OR.
- Search scope: `title` + `content` columns.
- Results filtered by `workspace_id` and optional `memory_type`.

### 3. Active Memory Injection (Reading)

- Once per session (first LLM turn only), `injectActiveMemory()` searches the store using the last
  user message as query.
- Relevant entries are injected as a `<memory>` system message at the end of prepared messages.
- Capped at 3000 runes. Truncated entries append `...[truncated]`.
- **Not injected for automation tasks** — the full task prompt is too generic as a search query,
  and the positioned `<memory>` block at end of 18+ turns of history overwrites the finalization
  instruction. See `docs/audits/memory-injection-investigation.md`.

### 4. Memory Tools

- `memory_search(query, limit)` — FTS5 search with BM25 ranking, capped at 20 results.
- `memory_update(topic, content, memory_type)` — Save a new memory entry.
- `memory_delete(id)` — Delete by ID.
- All gated by workspace ID.

### 5. Deduplication

- `findOverlappingEntry()` in `memory_tools.go` computes Jaccard similarity on topic words (≥0.70)
  and content (≥0.90) to prevent duplicate entries.
- If `memory_update` returns "already saved", the fact is already stored — do not retry.

### 6. Pre-Sieve Flush

- When context usage exceeds 70% of `ContextBudget`, `maybeFlushMemoryBeforeTurn()` appends a
  nudge: "The conversation history is about to be compressed. Save any important facts before
  they are lost."
- `memoryFlushSent` flag prevents duplicate nudges; resets when the physical sieve prunes history.

## III. Error Handling

- `Search()` with empty query returns nil, not an error.
- Memory store nil when disabled (all operations are no-ops).
- FTS5 rebuild failure logs warning, does not crash.

## IV. Tool Interactions

| Tool | Reads From | Writes To | Calls |
|------|-----------|-----------|-------|
| `memory_search` | Memory store | — | LLM searches stored facts |
| `memory_update` | — | Memory store | Agent saves durable facts |
| `injectActiveMemory` | Memory store | LLM context | Agent pre-fills context |
| `maybeFlushMemoryBeforeTurn` | Context budget | LLM context | Sieve nudge |
