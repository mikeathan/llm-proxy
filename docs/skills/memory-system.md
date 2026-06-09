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

## Three-Tier Architecture (upcoming)

The current system is being redesigned from flat storage to a three-tier model.
**Do NOT implement this until the plan is approved.**

| Tier | scope | mode | keep | `workspace_id` | `memory_type` | `tags` | Injected? |
|------|-------|------|------|---------------|--------------|--------|-----------|
| Hot+Global | user | always | permanent | `"global"` | `user_profile` | `["hot"]` | ✅ Every session |
| Hot+Local | workspace | always | permanent | active WS | `long_term` | `["hot"]` | ✅ This workspace |
| Cold | workspace | on_demand | permanent | active WS | `long_term` | `[]` | ❌ Search only |
| Session | workspace | on_demand | session | active WS | `session` | `[]` | ❌ Search only |

**Only `mode: "always"` triggers injection.** The injection query uses `json_each` for exact tag matching:

```sql
SELECT content FROM memories m
WHERE (m.workspace_id = 'global' AND EXISTS (SELECT 1 FROM json_each(m.tags) WHERE value = 'hot'))
   OR (m.workspace_id = ? AND EXISTS (SELECT 1 FROM json_each(m.tags) WHERE value = 'hot'))
ORDER BY m.updated_at DESC
```

**Strategy pattern** for route resolution (Open/Closed — adding a combo is one map entry, not a new `case`):

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

**Value Objects** for compile-time safety:

```go
type Scope string
const ( ScopeUser Scope = "user"; ScopeWorkspace Scope = "workspace" )
func (s Scope) Validate() error { ... }
```

## Current Injection (`injectActiveMemory()`, stream.go:115)

- Runs ONCE per session (first turn only, `a.memoryInjected` flag)
- Searches by FTS5 using last user message as query (or cached automation task)
- Also fetches up to 20 `user_profile` entries
- Injects as `<memory>` system message right before the last user message
- For automation: the task prompt is too generic as search query — returns irrelevant results
- Injection fires BEFORE any facts are saved → freshly-saved facts never injected

## Known Issues

1. **Instruction Hierarchy** — 4B model ignores injected memory when explicit task instructions ("run X") conflict with general guidance ("check memory first"). Proven across 8+ attempts. NOT solved by any current plan.
2. **Tag-only search can miss older entries** — `updated_at DESC` with default limit 5. Fix: use `query + tags` combined search (BM25 relevance).
3. **FTS5 injection path is unreliable** — Task prompt too generic as search query.

## Deduplication

`findOverlappingEntry()` in `memory_tools.go` computes **Jaccard similarity** on normalized topic words. If J ≥ 0.70:
- Same content → "already saved" (no-op)
- Different content → appends to existing entry (prevents duplicates under same topic)

## Important Gotchas

- `injectActiveMemory()` emits `<relevant_memories>` at the end of prepared messages, right before the current user turn. This keeps KV cache stable — only the `<memory>` message changes each turn (if re-injection were enabled).
- Memory is per-workspace. `workspace_id = 'global'` is reserved for cross-workspace user profile entries.
- The `Search` method's `sanitiseFTSQuery` wraps each term in double-quotes and joins with `OR`. Without this, FTS5 crashes on consecutive `OR` operators.
- Stop words (`step`, `task`, `run`, `use`, `check`) are filtered from the FTS5 query to prevent generic matches.
