# Plan: Memory Tags System

**Status:** complete
**Date:** 2026-06-08
**Related specs:** SPEC-004
**Depends on:** `docs/PLANS/cross-cutting/interactive-user-input.md` (separate effort, not a blocker)

## Problem

Memory entries have no grouping mechanism beyond `title` (topic string) and
`memory_type` (long_term/daily/session). This forces the agent to either:

- Stuff all related facts into one entry (loses individual fact provenance)
- Create many entries with slightly different topics (scattered, hard to retrieve)
- Rely on fragile FTS5 full-text search to find related entries

At workspace scale (hundreds of entries), the agent cannot reliably find all
entries about a given subject without a dedicated grouping primitive.

## Solution: Optional Tags

Add an optional `tags` field to `memory_update` and `memory_search`. Tags are
short strings that label entries into groups. An entry can have multiple tags.
The agent can search by tag to retrieve all entries in a group.

### Example

```
memory_update(
  topic="Aris Thorne Fact 1",
  content="Dr. Aris Thorne works at the Xenolith Research Institute.",
  tags=["persona:aris-thorne", "workplace:xenolith"]    ← new
)
```

```
memory_search(
  query="Thorne",                                        ← traditional FTS
  tags=["persona:aris-thorne"]                          ← new: filter by tag
)
```

Returns all entries tagged `persona:aris-thorne` that also match the query.

## Implementation

### Layer 1: Store Schema (`internal/platform/memory/`)

**Schema change** — SQLite ALTER TABLE ADD COLUMN for the main table. The FTS5
table needs to be DROP/CREATE because FTS5 doesn't support ALTER TABLE ADD
COLUMN. The existing migration code already handles DROP/CREATE of FTS tables
(see `migrateSQL` constant).

```sql
ALTER TABLE memories ADD COLUMN tags TEXT NOT NULL DEFAULT '[]';
```

The `tags` column stores a compact JSON array: `["persona", "exometeorology"]`.
This avoids a separate junction table while keeping the format parseable.

**FTS5 table** — recreate with the `tags` column added to the FTS index so
tags are also searchable via full-text:

```sql
DROP TABLE IF EXISTS memories_fts;

CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(
    title, content, tags, tokenize='unicode61',
    content=memories, content_rowid='id'
);
```

**Trigger updates** — the AFTER INSERT/DELETE/UPDATE triggers also need to
include the `tags` column in their FTS sync statements.

### Layer 2: Type & Scan (`internal/platform/memory/types.go`)

Add `Tags` field to `MemoryEntry`:

```go
type MemoryEntry struct {
    ID          int64      `json:"id"`
    WorkspaceID string     `json:"workspace_id"`
    MemoryType  MemoryType `json:"memory_type"`
    Title       string     `json:"title"`
    Content     string     `json:"content"`
    Tags        []string   `json:"tags"`
    Source      string     `json:"source"`
    CreatedAt   string     `json:"created_at"`
    UpdatedAt   string     `json:"updated_at"`
}
```

Wire the scan helper. Every `Rows.Scan()` call in `store.go` needs the new
column added to its scan list. There are 6 scan sites:

| Function | Line (approx) | Scan columns |
|----------|--------------|--------------|
| `Search` | 172 | +tags |
| `List` | 200 | +tags |
| `Get` | 214 | +tags |
| `FindByTitle` | 262 | +tags |
| `FindByContentSubstring` | 285 | +tags |
| `Search` (by type) | same row | +tags |

To avoid repeating the 9-column scan 6 times, extract a helper:

```go
func scanMemoryEntry(row interface{ Scan(dest ...any) error }) (MemoryEntry, error) {
	var e MemoryEntry
	var tagsStr string
	if err := row.Scan(&e.ID, &e.WorkspaceID, &e.MemoryType, &e.Title,
		&e.Content, &tagsStr, &e.Source, &e.CreatedAt, &e.UpdatedAt); err != nil {
		return MemoryEntry{}, err
	}
	if tagsStr != "" && tagsStr != "[]" {
		json.Unmarshal([]byte(tagsStr), &e.Tags)
	}
	return e, nil
}
```

All SQL queries that return rows must include `tags` in the SELECT. Update:

- `selectColumnsSQL`: add `, tags`
- `searchSQL`: already uses `m.*`? No — it lists columns explicitly. Add `tags`.
- `searchByTypeSQL`: same
- `insertSQL`: add `?` placeholder for tags
- `updateSQL`: add `tags = ?` to SET clause
- `listByTypeSQL`, `listSQL`, `getSQL`, `findByTitleSQL`, `findSubstringSQL`: all
  use `selectColumnsSQL` as prefix → covered by the single update.

**Search by tag** — three query variants depending on whether query and/or tags
are provided. The project uses `modernc.org/sqlite` (pure Go) which includes the
JSON1 extension (`json_each`, `json_array`) by default — no compile flags needed.

**Variant A: FTS + tags** (query AND tags provided) — uses `EXISTS` subquery
per tag, chained with `AND` for "all of these" semantics:
```sql
SELECT m.id, m.workspace_id, m.memory_type, m.title, m.content, m.tags, m.source, m.created_at, m.updated_at
FROM memories m
JOIN memories_fts f ON m.id = f.rowid
WHERE m.workspace_id = ?
  AND memories_fts MATCH ?
  AND EXISTS (SELECT 1 FROM json_each(m.tags) WHERE value = ?)
ORDER BY rank
LIMIT ?
```

For 2+ tags, repeat the `EXISTS` subquery: `AND EXISTS(...) AND EXISTS(...)`.
This correctly enforces "must have all of these tags" semantics. At runtime,
build the query by appending one `AND EXISTS (...)` per tag using
`strings.Builder`, then pass tag values as individual `?` args to
`QueryContext` — Go's `database/sql` doesn't support array binding.

**Variant B: Tags only** (tags provided, query empty) — no FTS join, uses
`updated_at` ordering:
```sql
SELECT m.id, m.workspace_id, m.memory_type, m.title, m.content, m.tags, m.source, m.created_at, m.updated_at
FROM memories m
WHERE m.workspace_id = ?
  AND EXISTS (SELECT 1 FROM json_each(m.tags) WHERE value = ?)
ORDER BY m.updated_at DESC
LIMIT ?
```

Without this variant, a tag-only call with empty query would crash on FTS5
`MATCH ''` — the agent asking "list all entries tagged X" is legitimate.

**Variant C: FTS only** (no tags) — unchanged from today's query:
```sql
JOIN memories_fts ... WHERE workspace_id = ? AND MATCH ?
```

### Layer 3: Store Methods (`internal/platform/memory/store.go`)

**`Search` method** — add optional `tags` parameter via `SearchOption` struct.
The existing `memoryType ...MemoryType` variadic is replaced by
`opts ...SearchOption`. This is a backward-compatible change: no existing
call sites pass `MemoryType` (verified — only 2 call sites in the codebase,
neither uses memoryType), so they compile unchanged.

```go
type SearchOption struct {
    Tags    []string
    MemType MemoryType
}

func (s *Store) Search(ctx context.Context, workspaceID, query string, limit int, opts ...SearchOption) ([]MemoryEntry, error)
```

The method selects query variant by branching on `query` and `Tags`:
- `query != "" && tags != nil` → Variant A: FTS + tags
- `query == "" && tags != nil` → Variant B: tags only
- `query != "" && tags == nil` → Variant C: FTS only (backward compatible)
- `query == "" && tags == nil` → `[]MemoryEntry{}, nil` (non-nil empty slice)

**`Insert` method** — add `tags []string` parameter. Tags are lowercased,
trimmed, deduplicated, and serialized to JSON inside this method (not at the
tool handler layer), ensuring defense-in-depth for any future callers:

```go
func (s *Store) Insert(ctx context.Context, workspaceID string, memoryType MemoryType, title, content string, tags []string, source string) (int64, error)
```

**`Update` method** — add `tags []string` parameter. Same normalization as
Insert. When called from the topic-append path, tags are **merged** with
existing entry tags: union of old and new, deduplicated. This prevents tag
explosion from repeated appends:

```go
func (s *Store) Update(ctx context.Context, workspaceID string, id int64, title, content string, tags []string) error
```

### Layer 4: Memory Tool (`internal/core/tools/memory_tools.go`)

**Manifest update** — add `tags` parameter to `memory_update` and `memory_search`:

`memory_update`:
```json
"tags": {
    "type": "array",
    "items": {"type": "string"},
    "description": "Optional labels for grouping this entry (e.g. [\"persona\", \"exometeorology\"]). Search by tag later to find all related entries."
}
```

`memory_search`:
```json
"tags": {
    "type": "array",
    "items": {"type": "string"},
    "description": "If set, only return entries that have all of these tags"
}
```

**Handlers** — update `Update` and `Search` methods to accept and forward tags.

In the topic-append path (`insertEntry`): when content is appended to existing
entry, merge tags: union of existing tags + new tags, deduplicate.

In the search tool: if `tags` is provided, query with tag filter. If not,
use existing FTS-only search.

### Layer 5: Prompts (`internal/core/assistant/prompts/templates.go`)

Optionally add a hint to the system prompt about tags. Not required for
functionality but helps the model discover the feature:

> "Use the `tags` parameter on `memory_update` to label related facts
> (e.g. `tags: ["persona"]` for all facts about a person)."

This can be added later after the feature is validated.

## Edge Cases

**Backward compatibility with existing entries:**
- Existing rows have `tags = '[]'` (DEFAULT on ALTER TABLE)
- Code that scans tags handles `"[]"` or `""` as "no tags"
- Existing `memory_search` calls without the tags parameter work identically
- No existing `Search()` callers pass `MemoryType` — verified zero-impact change

**Tag deduplication on topic-append:**
- When `insertEntry` appends content to an existing entry, tags are merged:
  union of `existing.Tags` and `newTags`, sorted, deduplicated
- Prevents tag list from growing unbounded on repeated updates

**Tag normalization (Store layer, defense-in-depth):**
- `Insert()` / `Update()` lowercases and trims each tag before saving
- Search tags are lowercased before comparison in the method
- Prevents "Persona" vs "persona" mismatch regardless of which caller provides tags

**FTS5 index update:**
- The `tags` column is included in FTS5 so full-text search also matches
  tag values (e.g., searching "persona" returns tagged entries even if the
  word doesn't appear in title/content)
- The AFTER INSERT/UPDATE/DELETE triggers sync the tags column to FTS5

## Backward Compatibility Strategy

| Existing behavior | New behavior | Compatible? |
|---|---|---|
| `memory_update(topic, content)` | Same + optional `tags` | ✅ Yes — tags omitted = no tags saved |
| `memory_search(query)` | Same + optional `tags` filter | ✅ Yes — tags omitted = no filter, query empty = tag-only search |
| `Search(ctx, wsID, query, limit)` | `Search(ctx, wsID, query, limit, opts ...SearchOption)` | ✅ Yes — variadic tail, no existing callers pass MemoryType |
| `Insert(ctx, wsID, type, title, content, source)` | `Insert(..., tags, source)` | ⚠️ Tags param added before source — call sites need `nil` inserted |
| `Update(ctx, wsID, id, title, content)` | `Update(..., tags)` | ⚠️ Tags param added — call sites need `nil` appended |

The `Insert`/`Update` signature changes require updating call sites in
`memory_tools.go`. The tool callers (model) are unaffected — they interface
through the tool manifest, not the Go methods directly.

## File Change Checklist

| File | Change |
|------|--------|
| `internal/platform/memory/types.go` | Add `Tags []string` to `MemoryEntry`, `scanMemoryEntry` helper, and `SearchOption` struct |
| `internal/platform/memory/store.go` | Schema + triggers (ALTER TABLE, DROP/CREATE FTS5); update all 5 query consts to include `tags` column; add 3 query variants for search; replace variadic with `SearchOption`; add `tags` param to `Insert`/`Update` with normalization; replace 6 raw Scans with `scanMemoryEntry` helper |
| `internal/platform/memory/store_test.go` | Update every test that calls `Scan()` — the column count changes from 8→9, these are compile-time failures |
| `internal/core/tools/manifests/memory.json` | Add `tags` (optional array of strings) to `memory_update` and `memory_search` parameter schemas |
| `internal/core/tools/memory_tools.go` | Wire tags through `Update` handler (line 91, 140, 155), `Insert` handler (lines 172, 189), and `Search` handler (lines 44, 210); merge tags in `insertEntry` topic-append path |
| `internal/core/tools/memory_tools_test.go` | Add tests: save with tags and verify via search, save with tags and verify via tag-only search, multi-tag AND search, tag normalization, topic-append tag merge |

## Future Work (not in scope)

- Frontend tag display in memory browser
- Tag auto-suggestion from existing entries
- Tag-based memory pruning (delete all entries with tag X)
- Hierarchical tags (`persona:aris-thorne` → returns on `persona:*` prefix search)
