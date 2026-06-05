# Memory: Relevance Search + Jaccard Dedup

**Status:** ✅ Complete

## Problem

Two issues degrade the memory system in multi-run, multi-task workspaces:

1. **Search returns chronological order, not relevance** — FTS4 uses `ORDER BY f.docid DESC`, so the most recently created entry always appears first regardless of query relevance. FTS5 with BM25 ranking fixes this. ✅

2. **Write-time dedup uses fragile word-overlap** — `findOverlappingEntry` checks for ≥2 shared content words, causing false positives on unrelated entries that happen to share common words. Replaced by Jaccard similarity on normalized topic tokens. ✅

3. **FTS5 build tag required** — The old `mattn/go-sqlite3` driver needed `-tags sqlite_fts5` for FTS5 support, which was easy to forget. Switched to `modernc.org/sqlite` which includes FTS5 by default with zero build tags. ✅

4. **Memory injection not visible on subsequent turns** — After turn 1, the last user message (a tool result) no longer matched the task prompt, so memory search returned nothing. Cached the original task prompt and reused it as the search query on every turn. ✅

## Changes

### 1. SQLite driver: `mattn/go-sqlite3` → `modernc.org/sqlite` ✅

No build tags needed. FTS5 works out of the box. Two-line change in `db.go`.

### 2. FTS4 → FTS5 migration (`store.go`) ✅

BM25 ranking via `ORDER BY rank` in FTS5. The `modernc.org/sqlite` driver includes FTS5 support by default.

| Component | Before (FTS4) | After (FTS5) |
|---|---|---|
| Virtual table | `USING fts4(...)` | `USING fts5(..., content_rowid='id')` |
| Join column | `m.id = f.docid` | `m.id = f.rowid` |
| Sort order | `ORDER BY f.docid DESC` | `ORDER BY rank` |
| Tokenizer | `tokenize=unicode61` | `tokenize='unicode61'` |

Added `DROP TABLE IF EXISTS memories_fts` before the CREATE to handle migration from FTS4 databases. Added `rebuildFTSIndexSQL` constant and post-migration rebuild to repopulate the FTS index for existing rows. ✅

### 3. Jaccard topic dedup (`memory_tools.go`) ✅

Replaced `findOverlappingEntry` content-word overlap with Jaccard similarity ≥ 0.70 on normalized topic words only:

```go
func topicJaccard(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	setA := make(map[string]bool, len(a))
	for _, w := range a {
		setA[w] = true
	}
	intersect := 0
	for _, w := range b {
		if setA[w] {
			intersect++
			setA[w] = false
		}
	}
	union := len(a) + len(b) - intersect
	return float64(intersect) / float64(union)
}
```

Search: FTS5 search the new topic + content against existing entries, compute Jaccard on topic words.
- J ≥ 0.70 AND content identical → "already saved" (no update)
- J ≥ 0.70 AND content differs → update in-place
- No content word overlap check (removed `sharedWordCount`)

### 4. `injectActiveMemory` skip nags (`stream.go`) ✅

When walking backward through `history` to find `lastUserMsg`, skip messages whose content starts with `"SYSTEM"`, `"REJECTED"`, or matches known nudge patterns (`"conversation history is about to be compressed"`, `"Stop analyzing"`, `"Stop writing"`).

### 5. Cached task prompt for memory search (`agent.go`, `stream.go`) ✅

Added `automationTaskPrompt` field to Agent struct. On first call to `injectActiveMemory`, scan history for the original automation task prompt (containing `AutomationMarker`) and cache it. On all subsequent turns, use the cached task prompt as the FTS5 search query instead of the degrading `lastUserMsg`. Ensures task-relevant memories are visible for the entire run.

### 6. Memory failure logging (`app_context.go`) ✅

Changed memory initialization failure from `logging.Warn` to `logging.Error`. If memory fails, the error is clearly visible but the server still starts — memory is nil-safe throughout the agent code.

### 7. Docs ✅

- `CONSTITUTION.md`: Updated FTS4 references → FTS5 in §II.12
- `AGENTS.md`: Updated quick start (removed `-tags sqlite_fts5`), updated pitfall #18 for Jaccard approach
- `memory/types.go`: Package comment already said FTS5 (no change needed)

## Files Changed

| File | Change | Status |
|---|---|---|
| `internal/platform/db/db.go` | Switch `mattn/go-sqlite3` → `modernc.org/sqlite` | ✅ |
| `internal/platform/memory/store.go` | FTS4→FTS5 in `migrateSQL`, `searchSQL`, `searchByTypeSQL`, added DROP+rebuild | ✅ |
| `internal/core/tools/memory_tools.go` | Add `topicJaccard`, refactor `findOverlappingEntry`, remove `sharedWordCount` | ✅ |
| `internal/core/assistant/agent.go` | Add `automationTaskPrompt` field | ✅ |
| `internal/core/assistant/stream.go` | Fix `injectActiveMemory` skip nags + cached task prompt + debug log | ✅ |
| `internal/core/assistant/prompts/templates.go` | Updated AutomationTaskPrompt guidance | ✅ |
| `internal/app/app_context.go` | Change memory init failure WARN→ERROR | ✅ |
| `AGENTS.md` | Remove `-tags sqlite_fts5`, update pitfall #18 | ✅ |
| `CONSTITUTION.md` | Update FTS4→FTS5 in §II.12 | ✅ |
| `go.mod` / `go.sum` | Modernc.org/sqlite added, mattn/go-sqlite3 removed | ✅ |

## Tests Added

| Test | File | What it verifies |
|---|---|---|
| `TestMemoryUpdateTool_JaccardDedup_ContentIdentical` | `memory_tools_test.go` | Jaccard match + identical content → "already saved" |
| `TestMemoryUpdateTool_SemanticDedup` | `memory_tools_test.go` | Jaccard match ≥ 0.70 + different content → update in-place |
| `TestInjectActiveMemory_CachesTaskPrompt` | `agent_memory_test.go` | Task prompt cached on first call, reused on subsequent calls |
| `TestMemoryUpdateTool_ContentDedup_DifferentTopic` | removed (replaced by Jaccard approach) | Content-word overlap dedup no longer exists |
