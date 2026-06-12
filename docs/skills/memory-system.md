# Memory System — Architecture, Decisions & Patterns

**Source docs:** SPEC-004, `docs/PLANS/memory/memory-three-tier-redesign.md`, `docs/PLANS/memory/memory-tags-system.md`, `docs/audits/memory-injection-investigation.md`

---

## Storage Layer

SQLite database (`orchestrator.db`) with FTS5 virtual table for full-text search.

```
TABLE memories (
    id INTEGER PRIMARY KEY,
    workspace_id TEXT,       -- active WS or 'global' for user-profile
    memory_type TEXT,        -- long_term | daily | session | user_profile
    title TEXT,
    content TEXT,
    tags TEXT,               -- JSON array, e.g. '["hot"]'
    source TEXT,
    created_at TEXT,
    updated_at TEXT
)
```

FTS5 indexes `title, content, tags` with BM25 relevance ranking.

## Three-Tier Architecture

The flat `memory_type`/`target`/`tags` model has been replaced by three clean parameters:

| Parameter | Values | What the model asks itself |
|-----------|--------|--------------------------|
| `scope` | `"user"` / `"workspace"` | "Does this apply to me or to this project?" |
| `mode` | `"always"` / `"on_demand"` | "Do I need this every session or just sometimes?" |
| `keep` | `"permanent"` / `"session"` | "Should this last forever or just this conversation?" |

### All 6 valid combinations

| scope | mode | keep | `workspace_id` | `memory_type` | `tags` | Injected? |
|-------|------|------|---------------|--------------|--------|-----------|
| user | always | permanent | `"global"` | `user_profile` | `["hot"]` | ✅ |
| user | on_demand | permanent | `"global"` | `user_profile` | `[]` | ❌ |
| user | on_demand | session | `"global"` | `user_profile` | `[]` | ❌ |
| workspace | always | permanent | active WS | `long_term` | `["hot"]` | ✅ |
| workspace | on_demand | permanent | active WS | `long_term` | `[]` | ❌ |
| workspace | on_demand | session | active WS | `session` | `[]` | ❌ |

**Only `mode: "always"` triggers injection.** The injection query uses `json_each` for exact tag matching:

```sql
SELECT content FROM memories m
WHERE (m.workspace_id = 'global' AND EXISTS (SELECT 1 FROM json_each(m.tags) WHERE value = 'hot'))
   OR (m.workspace_id = ? AND EXISTS (SELECT 1 FROM json_each(m.tags) WHERE value = 'hot'))
ORDER BY m.updated_at DESC
```

**Strategy pattern** for route resolution (in `memory_tools.go`):

```go
var routeStrategies = map[string]RouteStrategy{
    "user_always_permanent":      func(wsID string) MemoryRoute { return MemoryRoute{"global", "user_profile", []string{"hot"}} },
    "user_on_demand_permanent":   func(wsID string) MemoryRoute { return MemoryRoute{"global", "user_profile", nil} },
    "user_on_demand_session":     func(wsID string) MemoryRoute { return MemoryRoute{"global", "user_profile", nil} },
    "workspace_always_permanent":  func(wsID string) MemoryRoute { return MemoryRoute{wsID, "long_term", []string{"hot"}} },
    "workspace_on_demand_permanent": func(wsID string) MemoryRoute { return MemoryRoute{wsID, "long_term", nil} },
    "workspace_on_demand_session":    func(wsID string) MemoryRoute { return MemoryRoute{wsID, "session", nil} },
}
```

**Value Objects** for compile-time safety (in `types.go`):

```go
type Scope string
const ( ScopeUser Scope = "user"; ScopeWorkspace Scope = "workspace" )
func (s Scope) Validate() error { ... }
```

## Injection (`injectActiveMemory()`, stream.go:115)

- Runs ONCE per session (first turn only, `a.memoryInjected` flag)
- Fetches ALL entries with `tags: ["hot"]` via `SearchHot()` — no FTS5 query needed
- Injects as `<memory>` system message right before the last user message
- Text capped at `maxHotInjectionChars` (2000), truncated on entry boundaries
- Non-hot entries are searchable but never injected
- `user_profile` entries now covered by hot tag injection (no separate fetch)

## Known Issues

1. **Instruction Hierarchy** — 4B model ignores injected memory when explicit task instructions ("run X") conflict with general guidance ("check memory first"). Proven across 8+ attempts. NOT solved by any current plan.
2. **Tag-only search can miss older entries** — `updated_at DESC` with default limit 5. Fix: use `query + tags` combined search (BM25 relevance).

## Deduplication

`findOverlappingEntry()` in `memory_tools.go` computes **Jaccard similarity** on normalized topic words. If J ≥ 0.70:
- Same content → "already saved" (no-op)
- Different content → appends to existing entry (prevents duplicates under same topic)

## Important Gotchas

- `injectActiveMemory()` emits `<relevant_memories>` at the end of prepared messages, right before the current user turn. This keeps KV cache stable — only the `<memory>` message changes each turn (if re-injection were enabled).
- Memory is per-workspace. `workspace_id = 'global'` is reserved for cross-workspace user profile entries.
- The `Search` method's `sanitiseFTSQuery` wraps each term in double-quotes and joins with `OR`. Without this, FTS5 crashes on consecutive `OR` operators.
- Stop words (`step`, `task`, `run`, `use`, `check`) are filtered from the FTS5 query to prevent generic matches.

## Search Routes

`memory_search` has two code paths depending on whether a query is provided:

| Input | Path | Query | Order | Use case |
|-------|------|-------|-------|----------|
| `query:""` + no `tags` | `listAllMemories()` → `store.List()` | `SELECT ... ORDER BY updated_at DESC LIMIT ?` | Most recent first | "Return everything." Never errors — returns up to `limit` recent entries. |
| `query:"birthday"` or with `tags` | `store.Search()` → FTS5 | FTS5 BM25 ranking with JOIN on tags | Relevance | "Find specific fact." Targeted keyword search. |

### Why two paths

Models naturally call `memory_search(query:"")` expecting it to mean "give me everything."
Returning an error forced the model to waste 3-4 turns retrying before switching to
keywords. The `List` path satisfies this expectation while being bounded by the same
`limit` cap (default 5, max 20) as FTS5 searches.

The FTS5 path is always available for precise searches — `List` is only a fallback
for when both `query` and `tags` are empty.
