---
name: memory-system
description: "Memory architecture: injection, three-tier storage, tags, dedup, and gotchas. Use when working on memory storage or injection."
last_reviewed: 2026-10-06
---

# Memory System — Architecture, Decisions & Patterns

**Source docs:** SPEC-004, `docs/PLANS/ARCHIVE/memory/memory-three-tier-redesign.md`, `docs/PLANS/ARCHIVE/memory/memory-tags-system.md`, `docs/audits/memory-injection-investigation.md`

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
    updated_at TEXT,
    priority INTEGER DEFAULT 1   -- 0 low / 1 normal / 2 high; hot facts order by it, then recency
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

## Injection (`hot_memory.go`)

- `snapshotHotMemory(ctx)` runs ONCE per `Execute` (in `runSession.run()`), fetches ALL entries with
  `tags: ["hot"]` via `SearchHot()` and freezes the rendered block in `runSession.prompt.memoryBlock`
- `injectActiveMemory()` appends the frozen block to the HEAD system message on every request —
  byte-identical prefix = llama.cpp KV-cache reuse. Never insert it at a moving position.
- Cap = `hotMemoryCharBudget(ContextBudget, workload)`: local 8% / cloud 5% of the resolved budget,
  clamped 400–6000 chars (2000 when budget unresolved). Cut on entry boundaries, newest first; the
  overflow hint (`prompts.HotMemoryOverflowHint`) says how many facts were left out
- Non-hot entries are searchable but never injected
- Usage counters: `RecordInjected` (snapshot, only facts in the block) / `RecordSearched` (memory_search returns) are in-memory; `UsageFlusher` + `App.Shutdown` write them — NEVER add a DB write on the run path
- Priority (0–2, operator-only) orders hot facts before recency, so the budget cuts high-priority facts last; `SetPriority` leaves text/tags/timestamp alone
- Markdown import/export (`markdown.go`): `### Title` + `<!-- scope= mode= keep= priority= -->`, backslash-escaped structural lines so facts round-trip; import reuses `newMemoryRequest.route` + `saveNew` (source `import`), skips duplicates, reports bad entries by line
- Operator notes (`MEMORY.md`, `platform/memory/notes.go`): global (config root) + per-workspace (metadata folder),
  outside the agent jail; injected FIRST in the same block, never clipped (≤ 6000 chars/file on write); facts fill
  the remaining budget. One renderer (`renderHotMemory`) serves both the agent and the preview
- Hot memory defaults are global (`settings.yml → memory.assistant_hot` on, `memory.automation_hot` off); `MemoryMode` (`""` inherit | `on` | `off`) overrides per automation (`memory_mode`) and per workspace assistant (`assistant_memory`), resolved by `MemoryMode.Effective`. Unattended runs (`models.IsUnattendedRun`)
  default `memory_update` to `keep: session` with `source = run:<id>`; `memory.SessionReaper` (app.New) deletes
  only old `session` entries (`memory.retention_days`, default 90)
- Saving from chats lives in `internal/core/memorycapture` (domain only: no `assistant`/`tools`/HTTP imports; ports `Library`, `Completer`). Explicit capture = the table in `phrases.go` + `Extractor` (user's message only, before the run, manual sessions only, saved via `MemoryToolProvider.SaveFact` — the ONE save/dedup path, shared with `memory_update`; shown through `TurnRun.memory_saved`). Add a keyword = one phrase + a golden row in `extract_test.go`; never widen it with regex or model calls. The per-chat review (`Reviewer`, `POST …/memory-review`) reads user/assistant text only, saves nothing, and its output is validated in code. Plan: `docs/PLANS/memory/assistant-memory-capture.md` (save guidance D: `prompts.MemorySaveGuidance`, beside the memory block for manual assistant chats only — gated by `Agent.guidesMemorySaves`; its evaluation is `docs/guides/memory-testing.md` Part D)
- The pre-sieve nudge (`PreSieveMemoryNudge`, `maybeFlushMemoryBeforeTurn`) is for unguided runs only: it is skipped when `guidesMemorySaves()` (operator chat) or the run already called `memory_update`, so it cannot contradict the narrower guidance or a playbook's save rule. The share used for the hot block follows the workload class (local 8% / cloud 5%); a llama.cpp server behind an OpenAI-style URL is local (SPEC-005 1.3), and a local model ignores `model_overrides.context_budget`.
- Streaming vs fallback: test with a real `MockClient.StreamFunc` — the default mock fails `Stream`
  and takes the non-streaming fallback, which masks streaming-path defects

## Known Issues

1. **Instruction Hierarchy** — 4B model ignores injected memory when explicit task instructions ("run X") conflict with general guidance ("check memory first"). Proven across 8+ attempts. NOT solved by any current plan.
2. **Tag-only search can miss older entries** — `updated_at DESC` with default limit 5. Fix: use `query + tags` combined search (BM25 relevance).

## Deduplication

`findOverlappingEntry()` in `memory_tools.go` computes **Jaccard similarity** on normalized topic words. If J ≥ 0.70:
- Same content → "already saved" (no-op)
- Different content → appends to existing entry (prevents duplicates under same topic)

## Important Gotchas

- The `<memory>` block lives in the head system message and never changes within a run (KV-cache stable). Nothing is injected before the current user turn.
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
