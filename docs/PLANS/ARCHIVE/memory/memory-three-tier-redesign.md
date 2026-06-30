# Plan: Three-Tier Memory Architecture

**Status:** proposed
**Date:** 2026-06-09
**Related specs:** SPEC-004
**Depends on:** Existing tags system, memory types, `injectActiveMemory()`

---

## Problem

The current memory system has no structured tiers. All entries are stored in the
same SQLite FTS5 pool regardless of importance. The model must search for
everything — even critical facts like the user's birthday or project conventions
— which wastes turns and slows runs.

The `injectActiveMemory()` function searches by FTS5 using the last user message
as query. This is unreliable: for automation tasks the query is too generic to
return useful results, and the injection fires once before any facts are saved
in the current session, so freshly-saved facts are never injected.

## Existing Memory Architecture

### Storage

SQLite database (`orchestrator.db`) with FTS5 virtual table for full-text search.

```
TABLE memories (
    id INTEGER PRIMARY KEY,
    workspace_id TEXT,
    memory_type TEXT,       -- long_term | daily | session | user_profile
    title TEXT,
    content TEXT,
    tags TEXT,              -- JSON array, e.g. '["hot"]'
    source TEXT,
    created_at TEXT,
    updated_at TEXT
)
```

### Current injection behaviour

`injectActiveMemory()` at `stream.go:115`:
- Runs ONCE per session (first turn only, `a.memoryInjected` flag)
- Searches by FTS5 using the last user message as query (or cached automation task)
- Also fetches up to 20 `user_profile` entries
- Injects as `<memory>` system message at the end of prepared messages
- For automation: the task prompt is too generic as a search query → returns irrelevant results
- Injection fires before any facts are saved in the current session → freshly-saved facts are never injected

### Tags system

Added 2026-06-08. Optional `tags` parameter on `memory_update` and `memory_search`.
Tags are normalised (lowercased, trimmed, deduplicated) at the Store layer.
On topic-append, tags are merged (union + dedup).
Tag-only search orders by `updated_at DESC` with default limit 5 (can miss older entries).
Combined `query + tags` uses BM25 relevance ordering (most relevant first, regardless of age).

### Known issues

1. **Instruction Hierarchy** — The 4B model ignores injected memory when explicit task
   instructions ("run uname -a") conflict with general guidance ("check memory first").
   Proven across 8+ attempts in `docs/audits/memory-injection-investigation.md`.
   This plan does NOT solve this — injection makes facts available but doesn't guarantee
   the model will use them over explicit commands. Automation speed requires a separate
   solution (tool interception or hash-cached rewriter).

2. **Tag-only search can miss older entries** — `memory_search(tags=["persona"])` returns
   top 5 by `updated_at DESC`. Oldest entries under a tag can be invisible. The fix:
   use `query + tags` combined search, which uses BM25 relevance instead of recency.

3. **`injectActiveMemory()` FTS5 search is unreliable** — The task prompt is too generic
   as a search query. FTS5 finds loose keyword matches that are rarely useful.

---

## Solution: Three Clean Dimensions

Replace the overloaded `memory_type` field and raw `tags` array with three clear
parameters, each asking the model one unambiguous question:

| Parameter | Values | What the model asks itself |
|-----------|--------|--------------------------|
| `scope` | `"user"` / `"workspace"` | "Does this apply to me or to this project?" |
| `mode` | `"always"` / `"on_demand"` | "Do I need this every session or just sometimes?" |
| `keep` | `"permanent"` / `"session"` | "Should this last forever or just this conversation?" |

### All 8 permutations

| scope | mode | keep | `workspace_id` | `memory_type` | `tags` | Injected? |
|-------|------|------|---------------|--------------|--------|-----------|
| user | always | permanent | `"global"` | `user_profile` | `["hot"]` | ✅ User facts, always available |
| user | always | session | `"global"` | `user_profile` | `["hot"]` | ✅ (unlikely — user facts should be permanent) |
| user | on_demand | permanent | `"global"` | `user_profile` | `[]` | ❌ Secondary user facts |
| user | on_demand | session | `"global"` | `user_profile` | `[]` | ❌ Temp user context |
| workspace | always | permanent | active_ws | `long_term` | `["hot"]` | ✅ Project facts, always available |
| workspace | always | session | active_ws | `long_term` | `["hot"]` | ✅ (unlikely — use on_demand for temp data) |
| workspace | on_demand | permanent | active_ws | `long_term` | `[]` | ❌ Historical outputs, past results |
| workspace | on_demand | session | active_ws | `session` | `[]` | ❌ Per-run temporary data |

**Only `mode: "always"` triggers injection.** `scope` controls WHERE it's stored,
`keep` controls HOW LONG it lives. They're orthogonal to injection.

### How injection works

`injectActiveMemory()` fetches ALL entries with `tags: ["hot"]` — regardless of
scope — and injects them. The query uses `json_each` for exact tag matching:

```sql
SELECT content FROM memories m
WHERE (m.workspace_id = 'global'
       AND EXISTS (SELECT 1 FROM json_each(m.tags) WHERE value = 'hot'))
   OR (m.workspace_id = ?
       AND EXISTS (SELECT 1 FROM json_each(m.tags) WHERE value = 'hot'))
ORDER BY m.updated_at DESC
```

**Text cap:** `maxHotInjectionChars = 2000` constants in `stream.go`. If hot text
exceeds the cap, truncate from the oldest entries on entry boundaries — never
split a fact mid-content:

```go
func buildHotInjection(entries []MemoryEntry) string {
    var b strings.Builder
    for _, e := range entries {
        if b.Len() + len(e.Content) + len(e.Title) + 4 > maxHotInjectionChars {
            break
        }
        b.WriteString(fmt.Sprintf("- %s: %s\n", e.Title, e.Content))
    }
    return b.String()
}
```

### How the model decides

The tool descriptions guide the model's decision:

> **`mode`**: `"always"` = facts you reference in nearly every session (birthday, tech stack, communication style). Injected into the prompt at session start — no searching needed. Use sparingly — too many bloat the prompt. `"on_demand"` = facts worth keeping but only needed sometimes. Stored and searchable.
>
> **`scope`**: `"user"` = personal facts about you (applies across all projects). `"workspace"` = project-specific facts (applies to this project only).
>
> **`keep`**: `"permanent"` = lasts forever. `"session"` = discarded when this conversation ends.

The model applies a simple frequency heuristic:
- "Do I use this in every conversation?" → `mode: "always"`
- "Is this a one-off or occasional reference?" → `mode: "on_demand"`

---

## Implementation

### Phase 1: Backend Mapping

The tool handler translates model-facing parameters to store primitives. The
store knows nothing about `scope`, `mode`, or `keep` — it only sees
`workspace_id`, `memory_type`, and `tags`. This follows **Command Query
Separation**: the tool handler translates, the store persists.

#### Value Objects

Typed Go types prevent invalid parameter combinations at compile time:

```go
type Scope string
const (
    ScopeUser      Scope = "user"
    ScopeWorkspace Scope = "workspace"
)

type Mode string
const (
    ModeAlways   Mode = "always"
    ModeOnDemand Mode = "on_demand"
)

type Keep string
const (
    KeepPermanent Keep = "permanent"
    KeepSession   Keep = "session"
)

func (s Scope) Validate() error {
    if s != ScopeUser && s != ScopeWorkspace {
        return fmt.Errorf("invalid scope: %s", s)
    }
    return nil
}

func (m Mode) Validate() error {
    if m != ModeAlways && m != ModeOnDemand {
        return fmt.Errorf("invalid mode: %s", m)
    }
    return nil
}

func (k Keep) Validate() error {
    if k != KeepPermanent && k != KeepSession {
        return fmt.Errorf("invalid keep: %s", k)
    }
    return nil
}
```

#### Strategy Pattern for Route Resolution

Instead of a flat `switch` that grows linearly with each new combination, use a
strategy map. This follows the **Open/Closed Principle** — adding a new
`(scope, mode, keep)` combination is a one-line registration in the map,
not a change to existing branching logic.

```go
type MemoryRoute struct {
    WorkspaceID string
    MemoryType  string
    Tags        []string
}

type RouteStrategy func(wsID string) MemoryRoute

var routeStrategies = map[string]RouteStrategy{
    "user_always_permanent":     func(wsID string) MemoryRoute { return MemoryRoute{"global", "user_profile", []string{"hot"}} },
    "user_on_demand_permanent":  func(wsID string) MemoryRoute { return MemoryRoute{"global", "user_profile", nil} },
    "user_on_demand_session":    func(wsID string) MemoryRoute { return MemoryRoute{"global", "user_profile", nil} },
    "workspace_always_permanent": func(wsID string) MemoryRoute { return MemoryRoute{wsID, "long_term", []string{"hot"}} },
    "workspace_on_demand_permanent": func(wsID string) MemoryRoute { return MemoryRoute{wsID, "long_term", nil} },
    "workspace_on_demand_session":   func(wsID string) MemoryRoute { return MemoryRoute{wsID, "session", nil} },
}

func resolveParams(scope, mode, keep string, wsID string) (MemoryRoute, error) {
    key := fmt.Sprintf("%s_%s_%s", scope, mode, keep)
    s, ok := routeStrategies[key]
    if !ok {
        return MemoryRoute{}, fmt.Errorf("invalid combination: %s", key)
    }
    return s(wsID), nil
}
```

The mapper lives in `memory_tools.go` alongside the tool handlers — the store
layer never sees `scope`, `mode`, or `keep`. The handler validates, resolves,
then calls `store.Insert()` with primitive types.

#### Handler Integration

```go
type memoryUpdateParams struct {
    Content string `json:"content"`
    Scope   string `json:"scope"`
    Mode    string `json:"mode"`
    Keep    string `json:"keep"`
    Topic   string `json:"topic"`
    OldText string `json:"old_text"`
}

func (m *MemoryToolProvider) Update(ctx context.Context, args memoryUpdateParams) (any, error) {
    wsID := models.GetWorkspaceID(ctx)
    route, err := resolveParams(args.Scope, args.Mode, args.Keep, wsID)
    if err != nil {
        return "", fmt.Errorf("invalid memory parameters: %w", err)
    }
    ctx = models.WithWorkspaceID(ctx, route.WorkspaceID)
    return m.insertEntry(ctx, route.WorkspaceID, args.Topic, args.Content, route.Tags, route.MemoryType)
}
```

### Phase 2: Tool Manifest Changes

**`memory_update`** — replace `memory_type`, `tags`, `target` with the new params:

```json
{
  "memory_update": {
    "description": "Save a fact to memory. Use scope, mode, and keep to control where it goes and how long it lasts.",
    "parameters": {
      "type": "object",
      "properties": {
        "content": {
          "type": "string",
          "description": "The fact or rule to remember."
        },
        "scope": {
          "type": "string",
          "enum": ["user", "workspace"],
          "description": "Scope: 'user' for personal facts (applies everywhere), 'workspace' for project facts (this project only)."
        },
        "mode": {
          "type": "string",
          "enum": ["always", "on_demand"],
          "description": "Mode: 'always' = reference in nearly every session, injected at start (use sparingly). 'on_demand' = searchable when needed."
        },
        "keep": {
          "type": "string",
          "enum": ["permanent", "session"],
          "description": "Keep: 'permanent' = lasts forever. 'session' = forgotten when conversation ends."
        }
      },
      "required": ["content", "scope", "mode", "keep"]
    }
  }
}
```

**`memory_search`** — add optional `scope` filter:

```json
{
  "scope": {
    "type": "string",
    "enum": ["user", "workspace"],
    "description": "Optional: 'user' to search user facts, 'workspace' to search project facts. Omit to search both."
  }
}
```

### Phase 3: `injectActiveMemory()` Change

Replace the FTS5-based search with the `json_each` tag query. Remove the
separate `user_profile` fetch (it's now covered by the hot tag query for global
entries).

```go
func (a *Agent) injectActiveMemory(prepared []proxy.Message, history []proxy.Message) []proxy.Message {
    if a.memoryStore == nil || a.memoryInjected {
        return prepared
    }
    a.memoryInjected = true

    ctx := context.Background()
    entries, err := a.memoryStore.SearchHot(ctx, a.workspaceID)
    if err != nil || len(entries) == 0 {
        return prepared
    }

    content := buildHotInjection(entries)
    if content == "" {
        return prepared
    }

    msg := proxy.Message{
        Role:    proxy.SystemRole,
        Content: fmt.Sprintf("<memory>\n%s</memory>", content),
    }
    // Place right before the last user message for KV cache stability.
    insertIdx := len(prepared) - 1
    if insertIdx < 0 {
        insertIdx = 0
    }
    result := make([]proxy.Message, 0, len(prepared)+1)
    result = append(result, prepared[:insertIdx]...)
    result = append(result, msg)
    result = append(result, prepared[insertIdx:]...)
    return result
}
```

Add a new `SearchHot` method to the Store:

```go
func (s *Store) SearchHot(ctx context.Context, wsID string) ([]MemoryEntry, error) {
    query := `SELECT m.id, m.workspace_id, m.memory_type, m.title, m.content,
                     m.tags, m.source, m.created_at, m.updated_at
              FROM memories m
              WHERE (m.workspace_id = 'global'
                     AND EXISTS (SELECT 1 FROM json_each(m.tags) WHERE value = 'hot'))
                 OR (m.workspace_id = ?
                     AND EXISTS (SELECT 1 FROM json_each(m.tags) WHERE value = 'hot'))
              ORDER BY m.updated_at DESC`
    rows, err := s.db.QueryContext(ctx, query, wsID)
    if err != nil {
        return nil, fmt.Errorf("memory search hot: %w", err)
    }
    defer rows.Close()
    return scanRows(rows)
}
```

### Phase 4: Tool Handler Changes

**`Update` handler** — parse new params, call `resolveParams`, then existing
`insertEntry` with resolved route:

```go
func (m *MemoryToolProvider) Update(ctx context.Context, args memoryUpdateParams) (any, error) {
    wsID := models.GetWorkspaceID(ctx)
    route, err := resolveParams(args.Scope, args.Mode, args.Keep, wsID)
    if err != nil {
        return "", fmt.Errorf("invalid memory parameters: %w", err)
    }
    ctx = models.WithWorkspaceID(ctx, route.WorkspaceID)
    return m.insertEntry(ctx, route.WorkspaceID, args.Topic, args.Content, route.Tags, route.MemoryType)
}
```

**`Search` handler** — add optional `scope` filter:<｜end▁of▁thinking｜>

<｜｜DSML｜｜tool_calls>
<｜｜DSML｜｜invoke name="read">
<｜｜DSML｜｜parameter name="offset" string="false">130
```go
func (m *MemoryToolProvider) Search(ctx context.Context, args struct {
    Query interface{} `json:"query"`
    Limit int         `json:"limit"`
    Scope string      `json:"scope"`
    Tags  []string    `json:"tags"`
}) (any, error) {
    wsID := models.GetWorkspaceID(ctx)
    var searchWSID string
    switch args.Scope {
    case "user":
        searchWSID = "global"
    case "workspace":
        searchWSID = wsID
    default:
        searchWSID = "" // search both (no filter)
    }

    opt := memory.SearchOption{Tags: args.Tags}
    if searchWSID != "" {
        opt.WorkspaceID = searchWSID
    }

    query, _ := args.Query.(string)
    entries, err := m.store.Search(ctx, wsID, query, args.Limit, opt)
    // ... rest of existing handler (format results, return)
}
```

Add `WorkspaceID` field to `SearchOption` in `types.go`:

```go
type SearchOption struct {
    Tags        []string
    MemType     MemoryType
    WorkspaceID string // optional: override workspace filter for global searches
}
```

The `Search` method in `store.go` uses `WorkspaceID` when set, otherwise
defaults to the `workspaceID` parameter.

### Phase 5: Migrate `user_profile` Exclusions

The current `List()`, `Search()`, and `DeleteAllByWorkspace()` methods filter
by `workspace_id = ?`. Global entries (`workspace_id = 'global'`) are already
excluded from these operations because they don't match the active workspace
ID. No code change needed — they're invisible by default.

The only paths that need to see global entries are `SearchHot()` (the injection
query) and explicit `scope: "user"` searches. Both explicitly query
`workspace_id = 'global'`.

### Phase 6: Test Prompt

Create `backend/data/templates/memory-three-tier-test.md`:

```
## Task: Three-Tier Memory Test

**ID:** `memory-three-tier-test`
**Category:** Testing

Validate the three-tier memory architecture: scope, mode, and keep parameters.

### Phase 1: Save Facts

Save the following facts with the specified parameters:

1. `scope: "user", mode: "always", keep: "permanent"`
   Content: "My birthday is January 1st. I prefer concise answers."

2. `scope: "workspace", mode: "always", keep: "permanent"`
   Content: "This project uses TypeScript 6.0.3. Run npx tsc to compile."

3. `scope: "workspace", mode: "on_demand", keep: "permanent"`
   Content: "Smoke test completed successfully on 2026-06-09."

4. `scope: "workspace", mode: "on_demand", keep: "session"`
   Content: "Currently testing memory tier system. Debug mode enabled."

### Phase 2: Verify Injection

Do NOT call memory_search. Check if facts 1 and 2 are already present
in the <memory> block at the top of the conversation. If they are,
report them. If not, report a failure.

### Phase 3: Verify Search Scoping

1. Search `scope: "user"` — must return fact 1 only.
2. Search `scope: "workspace"` — must return facts 2, 3, 4.
3. Search without scope — must return all 4 facts.

### Phase 4: Write Report

1. Injection check: facts 1 and 2 present in <memory>? ✅ / ❌
2. User scope search: correct count? ✅ / ❌
3. Workspace scope search: correct count? ✅ / ❌
4. Unscoped search: correct count? ✅ / ❌
5. Result: PASS if all checks pass. Otherwise ❌ FAIL.
```

---

## Edge Cases

| Case | Handling |
|------|----------|
| **Empty scope/mode/keep** | Default to `scope: "workspace"`, `mode: "on_demand"`, `keep: "permanent"` for backward compatibility |
| **Existing entries without hot tag** | Remain searchable via FTS5. Not injected. No migration needed. |
| **Too many hot entries** | Text cap (2000 chars) truncates oldest-first. Model must delete old hot entries to make room for new ones. |
| **SearchHot returns no results** | `injectActiveMemory()` returns `prepared` unchanged — no injection, no error. |
| **Multiple workspaces, same user** | Global entries injected into every workspace session. Model sees same user profile everywhere. |
| **`json_each` performance** | Fast for tables under ~10,000 rows. Add `is_hot` indexable column if profiling shows a bottleneck. |

---

## File Change Checklist

| File | Change |
|------|--------|
| `internal/platform/memory/types.go` | Add `WorkspaceID` field to `SearchOption`; add `Scope`, `Mode`, `Keep` value types |
| `internal/platform/memory/store.go` | Add `SearchHot()` method; use `WorkspaceID` in `Search()` when set |
| `internal/platform/memory/store_test.go` | Add `TestSearchHot`, `TestSearchHot_ScopeExclusion` |
| `internal/core/tools/manifests/memory.json` | Replace old params with `scope`, `mode`, `keep` in `memory_update`; add optional `scope` to `memory_search` |
| `internal/core/tools/memory_tools.go` | Add `MemoryRoute`, `routeStrategies` map, `resolveParams()` function; update `Update` and `Search` handlers with validated value types |
| `internal/core/tools/memory_tools_test.go` | Update test structs; add save+search scope tests |
| `internal/core/assistant/stream.go` | Replace FTS5 injection with `SearchHot()` + `buildHotInjection()`; add `maxHotInjectionChars` constant |
| `internal/core/assistant/agent_memory_test.go` | Update active memory injection tests |
| `backend/data/templates/memory-three-tier-test.md` | New test prompt |
| `docs/INDEX.md` | Add plan entry, status → complete |

---

## What This Does NOT Fix

**Instruction Hierarchy** — The 4B model ignores injected memory when explicit
task instructions ("run uname -a") conflict with general guidance ("check memory
first"). The audit proved this across 8+ attempts. Hot injection makes facts
available but doesn't guarantee the model will use them over explicit commands.
Automation speed improvements require a separate solution (tool interception or
hash-cached rewriter).

This design makes memory **correct** and **reliable** — the model always has
birthday, preferences, and conventions available without searching. It does not
make automation runs faster. Those are two separate goals addressed by separate
mechanisms.
