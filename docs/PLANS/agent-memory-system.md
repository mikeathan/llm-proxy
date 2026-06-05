# Agent Memory System — Implementation Plan

**Status: COMPLETE ✅**
**Last updated: 2026-05-29**

---

## I. Overview

Add a persistent, searchable memory system to the agent, inspired by OpenClaw's approach. The agent can write facts to SQLite-backed memory using a `memory_update` tool and recall them later using a `memory_search` tool. Memory survives server restarts, context window rotations, and sieve compression.

### Semantic Deduplication (2026-05-31)

When the agent calls `memory_update`, the `insertEntry` method now performs a **three-layer dedup** instead of two:

1. **Exact topic match** (`FindByTitle`) — same topic → update in-place
2. **Semantic topic overlap** (`findOverlappingEntry`) — FTS4 search + topic word overlap ≥2 unique words → update in-place
3. **Exact content match** (`Exists`) — same content → skip

Layer 2 catches cases where the model invents different topic strings across runs (e.g. `"smoke-test-status"` vs `"llm-smoke-test-results"`) that share meaningful overlapping words, while avoiding false merges between unrelated entries like `"first entry"` and `"second entry"` that only share generic words like "entry".

### Prompt Guidance Improvements (2026-05-31)

Two prompt constants (`prompts/templates.go`) were updated to teach the agent how to save USEFUL memories and how to LEVERAGE them:

**`MemoryProactiveNudge`** — guides the agent to save specific, actionable facts rather than summaries:

```
- Installed tools: "TypeScript 6.0.3 is installed in this workspace"
- Environment state: "node_modules exists — npm install not needed"
- File states: "dev-test/index.ts compiles and runs correctly"
- Working commands: "use 'go build ./...' to verify Go builds"
- User preferences: "prefers CommonJS over ESM for Node.js projects"
- Decisions: "chose port 5433 for the test database"
```

**`MemoryRecallNudge`** — teaches the agent to USE automatically-injected memories:

```
- If memory says a tool is installed, use the tool — skip re-installing
- If memory records a file state, verify with read_file instead of re-creating the file
- If memory records a decision, follow it without re-asking
- If memory records a completed task, acknowledge the past result instead of redoing it
```

Both nudges are injected conditionally into the system prompt (via `registry.go:GetSystemPrompt()`) only when memory tools are registered.

### How Memory Helps the Agent

| Scenario | Before (no memory) | After (with memory) |
|----------|-------------------|---------------------|
| User says "test DB port is 5433" | Forgotten when sieve drops old messages | Written to memory, recalled in future sessions |
| Agent discovers "run `go build ./...`" | Lost after context rotation | Saved to memory, injected next session |
| User preference: "I prefer TypeScript" | Forget after ~3 turns | Available across all conversations |
| Sieve drops old history | Information gone permanently | Agent flushed key facts to memory before sieve ran |
| "What did we decide about X last week?" | No idea — session is long gone | `memory_search` finds the exact decision |

### Key Design Decisions

1. **SQLite-only** — No markdown files, no vector embeddings, no external services. Single persistence layer.
2. **FTS5 search** — Full-text search built into `modernc.org/sqlite`. Zero cost, instant, good quality. No build tags needed.
3. **Same DB file as ledger** — Memory tables live alongside `icu_ledger`, `active_slots`, etc. in `orchestrator.db`. Shared WAL, single backup.
4. **Separate package** — `internal/platform/memory/` owns the schema, queries, and types. Ledger stays focused on ICU tracking.
5. **Workspace-isolated** — All queries filter by `workspace_id`.
6. **Disable-able** — Config toggle in `UserSettings` to turn memory on/off per deployment.

---

## II. Architecture

```
bootstrap.go
│
├── ledger.Open(path)    → *ledger.Store  (existing)
│   └── store.DB()       → *sql.DB        (NEW — exported getter)
│
└── memory.New(store.DB())  → *memory.Store  (NEW)
    └── creates tables:
        ├── memories         — content storage
        └── memories_fts     — FTS5 virtual table + sync triggers
```

### SQLite Schema

```sql
CREATE TABLE IF NOT EXISTS memories (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    workspace_id TEXT NOT NULL,
    memory_type  TEXT NOT NULL DEFAULT 'long_term',  -- long_term|daily|session
    title        TEXT NOT NULL DEFAULT '',
    content      TEXT NOT NULL,
    source       TEXT NOT NULL DEFAULT 'agent',       -- agent|user|system
    created_at   DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at   DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_memories_ws ON memories(workspace_id, memory_type);

CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(
    title, content, tokenize='unicode61', content=memories, content_rowid=id
);

CREATE TRIGGER IF NOT EXISTS memories_ai AFTER INSERT ON memories BEGIN
    INSERT INTO memories_fts(rowid, title, content)
    VALUES (new.id, new.title, new.content);
END;

CREATE TRIGGER IF NOT EXISTS memories_ad AFTER DELETE ON memories BEGIN
    INSERT INTO memories_fts(memories_fts, rowid, title, content)
    VALUES('delete', old.id, old.title, old.content);
END;

CREATE TRIGGER IF NOT EXISTS memories_au AFTER UPDATE ON memories BEGIN
    INSERT INTO memories_fts(memories_fts, rowid, title, content)
    VALUES('delete', old.id, old.title, old.content);
    INSERT INTO memories_fts(rowid, title, content)
    VALUES (new.id, new.title, new.content);
END;
```

### Integration Flow

```
Agent loop (session.go)
│
├── memory_search tool     — agent-initiated recall (on-demand, zero budget impact)
├── memory_update tool     — agent-initiated write
├── prepareMessagesForTurn — active memory injection (opt-in)
│   └── searches memory with last user query
│   └── injects <relevant_memories> block into system prompt
└── pre-sieve flush         — before sieve drops history
    └── appends nudge: "save important facts to memory"
    └── runs one LLM turn, then sieve proceeds
```

---

## III. Package Structure

### New Files

```
backend/internal/platform/memory/
├── store.go            — MemoryStore: Insert, Search, List, Delete, Update, Clean
├── types.go            — MemoryEntry, MemoryType (LongTerm, Daily, Session)
└── store_test.go       — Tests with temp SQLite DB

backend/internal/core/tools/manifests/memory.json
backend/internal/core/tools/memory_tools.go
    - memory_search(query string, limit int)
    - memory_update(topic string, content string, memory_type string)

backend/internal/transport/http/memory_handlers.go
    - ListMemories     GET    /admin/api/memory/{workspace}
    - SearchMemories   POST   /admin/api/memory/{workspace}/search
    - GetMemory        GET    /admin/api/memory/{workspace}/{id}
    - UpdateMemory     PUT    /admin/api/memory/{workspace}/{id}
    - DeleteMemory     DELETE /admin/api/memory/{workspace}/{id}

frontend/src/types/memory.ts
frontend/src/services/memoryService.ts
frontend/src/composables/useMemory.ts
frontend/src/components/AgentIde/memory/MemoryPanel.vue
frontend/src/components/AgentIde/memory/MemoryDetail.vue
```

### Modified Files

| File | Change |
|------|--------|
| `backend/internal/platform/ledger/store.go` | Add `DB() *sql.DB` getter |
| `backend/models/tools.go` | Add `ToolMemorySearch`, `ToolMemoryUpdate`, `CategoryMemory` |
| `backend/models/infrastructure.go` | Add `MemoryConfig` to `UserSettings` |
| `backend/internal/core/assistant/registry.go` | Add `Memory *tools.MemoryToolProvider` field, `registerMemoryTools()`, wire in `InitializeAgentStack` |
| `backend/internal/core/assistant/agent.go` | Add `memory *memory.Store` field + `MemoryStore` in `AgentOptions` |
| `backend/internal/core/assistant/agent_events.go` | Add `EventMemoryRecall`, `EventMemoryFlush` |
| `backend/internal/core/assistant/stream.go` | Add active memory injection in `prepareMessagesForTurn` |
| `backend/internal/core/assistant/session.go` | Add pre-sieve memory flush before `applyPhysicalSieve` |
| `backend/internal/core/automation/executor.go` | Add `MemoryStore() *memory.Store` to `LLMServiceProvider`, pass to `AgentOptions` |
| `backend/internal/transport/http/services.go` | Add `MemoryStore() *memory.Store` to `AssistantService` |
| `backend/internal/testing/mocks/assistant_service.go` | Add `MemoryStore() *memory.Store` returning `nil` |
| `backend/internal/app/bootstrap.go` | Wire `MemoryStore`, register memory API routes |
| `frontend/src/components/AgentIde/AgentIde.vue` | Add `"memory"` to `leftTab`, add sidebar button, add panel render |
| `frontend/src/types/index.ts` | Add `export * from './memory'` |
| `frontend/src/services/index.ts` | Add `MemoryService` export |

---

## IV. Implementation Steps

### Phase 1 — Storage Layer

#### Step 1.1 — Add `DB()` getter to ledger store

**File**: `backend/internal/platform/ledger/store.go`

Add a public getter for the `*sql.DB` so MemoryStore can share the same connection:

```go
func (s *Store) DB() *sql.DB { return s.db }
```

**Validation**: `go build ./...` passes.

**Status**: [ ]

---

#### Step 1.2 — Create MemoryStore

**File**: `backend/internal/platform/memory/store.go`

- `New(db *sql.DB) *Store` — runs migration (memories + memories_fts + triggers), returns store
- `Insert(ctx, workspaceID, memoryType, title, content, source) (int64, error)`
- `Search(ctx, workspaceID, query, limit) ([]MemoryEntry, error)` — FTS5 query with `MATCH ?`, BM25 ranking, scoped by workspace_id
- `List(ctx, workspaceID, memoryType, limit, offset) ([]MemoryEntry, error)`
- `Get(ctx, workspaceID, id) (*MemoryEntry, error)`
- `Update(ctx, workspaceID, id, title, content) error`
- `Delete(ctx, workspaceID, id) error`
- `DeleteOlderThan(ctx, memoryType, before) (int64, error)` — for cleanup

Rules:
- All methods accept `context.Context` as first arg (Constitution IV.1)
- All errors wrapped with `fmt.Errorf` and `%w` (Constitution IV.2)
- FTS5 query uses `SELECT m.* FROM memories m JOIN memories_fts f ON m.id = f.rowid WHERE m.workspace_id = ? AND memories_fts MATCH ? ORDER BY rank LIMIT ?`
- No raw `context.Background()` (Constitution II.2)
- **FTS5 query sanitisation**: The user-supplied `query` string must be sanitised before passing to MATCH. Special FTS5 characters (`"`, `(`, `)`, `:`, `*`, `^`, `-`, `+`, `~`) cause syntax errors. Wrap each word in double quotes or strip/escape non-alphanumeric characters. Also guard: if `query == ""`, return empty results immediately.

**File**: `backend/internal/platform/memory/types.go`

```go
package memory

type MemoryType string
const (
    LongTerm MemoryType = "long_term"
    Daily    MemoryType = "daily"
    Session  MemoryType = "session"
)

// MemoryEntry stores datetime as string to match go-sqlite3 scan behaviour.
// The existing ledger code (store.go:233-242) follows the same pattern:
// scan DATETIME columns as string, parse with time.Parse when needed.
type MemoryEntry struct {
    ID          int64      `json:"id"`
    WorkspaceID string     `json:"workspace_id"`
    MemoryType  MemoryType `json:"memory_type"`
    Title       string     `json:"title"`
    Content     string     `json:"content"`
    Source      string     `json:"source"`
    CreatedAt   string     `json:"created_at"`
    UpdatedAt   string     `json:"updated_at"`
}
```

**Validation**:

```bash
cd backend
go build ./...
```

**Status**: [ ]

---

### Phase 2 — Tool Definitions & Registration

#### Step 2.1 — Add tool name constants

**File**: `backend/models/tools.go`

```go
const (
    ToolMemorySearch = "memory_search"
    ToolMemoryUpdate = "memory_update"
)

const (
    CategoryMemory = "memory"
)
```

**Validation**: `go build ./...`

**Status**: [ ]

---

#### Step 2.2 — Create tool manifest

**File**: `backend/internal/core/tools/manifests/memory.json`

```json
{
  "tool_name": "memory_search",
  "version": "1.0",
  "description": "Search your long-term memory for facts, decisions, preferences, and notes from past conversations using keyword search.",
  "parameters": {
    "type": "object",
    "properties": {
      "query": { "type": "string", "description": "Keywords to search for in memory" },
      "limit": { "type": "integer", "description": "Max results to return (1-20)", "default": 5 }
    },
    "required": ["query"]
  },
  "runtime": {
    "memory_search": {
      "description": "Search your long-term memory for facts, decisions, preferences, and notes from past conversations using keyword search."
    },
    "memory_update": {
      "description": "Save an important fact, decision, preference, or note to your long-term memory so you can recall it in future conversations.",
      "parameters": {
        "type": "object",
        "properties": {
          "topic": { "type": "string", "description": "Short topic/keyword for this memory" },
          "content": { "type": "string", "description": "The fact, decision, or note to remember" },
          "memory_type": { "type": "string", "enum": ["long_term", "daily", "session"], "default": "long_term", "description": "How long to remember this" }
        },
        "required": ["topic", "content"]
      }
    }
  },
  "guardrails": {}
}
```

**Validation**: Manifest JSON is valid (loadable by `tools.LoadManifestAsTool`).

**Status**: [ ]

---

#### Step 2.3 — Create memory tool handlers

**File**: `backend/internal/core/tools/memory_tools.go`

- `MemoryToolProvider` struct holding `*memory.Store`
- `MemorySearch(ctx, MemorySearchArgs) (any, error)` — calls store.Search, formats as markdown
- `MemoryUpdate(ctx, MemoryUpdateArgs) (any, error)` — calls store.Insert, returns confirmation

Rules:
- Args structs match manifest parameter schema
- Error paths return useful messages the model can understand
- Workspace ID extracted from context via `models.GetWorkspaceID(ctx)`

**Validation**: `go build ./...`

**Status**: [ ]

---

#### Step 2.4 — Register tools in the agent

**File**: `backend/internal/core/assistant/registry.go`

- Add `Memory *tools.MemoryToolProvider` field to `LocalToolRegistry`
- Add `registerMemoryTools()` method using `registerTool[T]` pattern
- Call from `registerAll()`
- Add `initMemoryTools(store)` helper
- Pass `*memory.Store` through `InitializeAgentStack` signature
- Wire into `NewLocalToolRegistry`
- **Update tests**: `registry_test.go` calls `InitializeAgentStack` — pass `nil` as the memory store argument to fix build breakage

**Validation**: `go build ./...`

**Status**: [ ]

---

### Phase 3 — Configuration

#### Step 3.1 — Add MemoryConfig to UserSettings

**File**: `backend/models/infrastructure.go`

```go
type UserSettings struct {
    Local          LocalSettings               `yaml:"local" json:"local"`
    Guardrails     *AgentGuardrailsConfig      `yaml:"guardrails,omitempty" json:"guardrails,omitempty"`
    ModelOverrides map[string]ModelOverride    `yaml:"model_overrides,omitempty" json:"model_overrides,omitempty"`
    Memory         *MemoryConfig               `yaml:"memory,omitempty" json:"memory,omitempty"`
}

type MemoryConfig struct {
    Enabled        bool    `yaml:"enabled" json:"enabled"`
    SearchTopK     int     `yaml:"search_top_k,omitempty" json:"search_top_k,omitempty"`
    FlushThreshold float64 `yaml:"flush_threshold,omitempty" json:"flush_threshold,omitempty"`
    RetentionDays  int     `yaml:"retention_days,omitempty" json:"retention_days,omitempty"`
}

func DefaultMemoryConfig() MemoryConfig {
    return MemoryConfig{
        Enabled:        true,
        SearchTopK:     5,
        FlushThreshold: 0.7,
        RetentionDays:  90,
    }
}
```

**Validation**: `go build ./...`

**Status**: [ ]

---

### Phase 4 — Agent Loop Integration

#### Step 4.1 — Add MemoryStore to Agent

**File**: `backend/internal/core/assistant/agent.go`

- Add `memory *memory.Store` field to `Agent` struct
- Add `MemoryStore *memory.Store` to `AgentOptions`
- Wire in `NewAgent()` — only set if `opts.MemoryStore != nil` (nil-safe, like `orch`)

**Status**: [ ]

---

#### Step 4.1b — Wire MemoryStore through automation executor

**File**: `backend/internal/core/automation/executor.go`

Add `MemoryStore() *memory.Store` to the `LLMServiceProvider` interface. The executor creates agents in `executeTask()` — pass `e.svc.MemoryStore()` to `AgentOptions.MemoryStore`.

**File**: `backend/internal/transport/http/services.go`

Add `MemoryStore() *memory.Store` to the `AssistantService` interface.

**File**: `backend/internal/app/bootstrap.go`

Add a `MemoryStore()` accessor on `AppServices` that returns the `*memory.Store`.

**File**: `backend/internal/testing/mocks/assistant_service.go`

Add `MemoryStore() *memory.Store` returning `nil` to prevent build breakage.

**Validation**: `go build ./...` passes (mock implements the interface).

**Status**: [ ]

---

#### Step 4.2 — Active memory injection

**File** (prompts): `backend/internal/core/assistant/prompts/templates.go`

Add prompt constants (Constitution II.12 — ALL prompt strings in templates.go):

```go
const RelevantMemoriesHeader = "<relevant_memories>\n"
const RelevantMemoriesFooter = "\n</relevant_memories>"
```

**File**: `backend/internal/core/assistant/stream.go`

In `prepareMessagesForTurn()` (line 60), after tool instructions are injected:

1. Extract the last user message from history as search query
2. If `a.memory != nil` and a user message exists, call `a.memory.Search(ctx, wsID, query, topK)`
3. If results found, inject `RelevantMemoriesHeader + formatted entries + RelevantMemoriesFooter` into the system message
4. Keep the injected block under ~500 chars

Rules:
- No-op if `a.memory == nil` (disabled or no store)
- No-op if search returns no results (zero budget impact)
- FTS5 search is local, <1ms — no latency concern

**Status**: [ ]

---

#### Step 4.3 — Pre-sieve memory flush

**File** (prompt): `backend/internal/core/assistant/prompts/templates.go`

Add nudge constant (Constitution II.12):

```go
const PreSieveMemoryNudge = "The conversation history is about to be compressed. Save any important facts, decisions, or preferences to memory using `memory_update` before they are lost."
```

**File**: `backend/internal/core/assistant/session.go`

In `executeTurn()` (line 233), before `applyPhysicalSieve()` (line 242):

1. Calculate current context usage percentage
2. If > `FlushThreshold` (default 0.7 = 70% of budget) **and** `s.memoryFlushSent == false`, append nudge:
   - `PreSieveMemoryNudge` as a user-role message (like `SieveSystemNote`)
3. Run one LLM turn (the agent calls `memory_update` to save context)
4. Then proceed with the sieve
5. Set `s.memoryFlushSent = true`
6. Reset `s.memoryFlushSent = false` only when the physical sieve actually runs and prunes history

Add to `runSession` struct:
```go
type runSession struct {
    // ... existing fields ...
    memoryFlushSent bool  // NEW: prevents repeated nudges across turns
}
```

Rules:
- No-op if `a.memory == nil`
- No-op if `FlushThreshold` is 0 (disabled)
- The `memoryFlushSent` guard prevents injecting the nudge on every consecutive turn above threshold

**Status**: [ ]

---

#### Step 4.4 — Memory lifecycle events

**File**: `backend/internal/core/assistant/agent_events.go`

Add event types:

```go
const (
    EventMemoryRecall AgentEventType = "memory_recall"
    EventMemoryFlush  AgentEventType = "memory_flush"
)
```

Add notification methods on Agent:

```go
func (a *Agent) notifyMemoryRecall(query string, count int) {
    a.notify(EventMemoryRecall, map[string]any{"query": query, "count": count})
}

func (a *Agent) notifyMemoryFlush(count int) {
    a.notify(EventMemoryFlush, map[string]any{"saved_count": count})
}
```

**Status**: [ ]

---

### Phase 5 — REST API

#### Step 5.1 — Create memory handlers

**File**: `backend/internal/transport/http/memory_handlers.go`

- `ListMemories(w, r)` — GET → `GET /admin/api/memory/{workspace}` — paginated, optional `?type=long_term` filter
- `SearchMemories(w, r)` — POST → `POST /admin/api/memory/{workspace}/search` — body: `{query, limit}`
- `GetMemory(w, r)` — GET → `GET /admin/api/memory/{workspace}/{id}`
- `UpdateMemory(w, r)` — PUT → `PUT /admin/api/memory/{workspace}/{id}` — body: `{title, content}`
- `DeleteMemory(w, r)` — DELETE → `DELETE /admin/api/memory/{workspace}/{id}`

Follow existing handler pattern from `assistant_handlers.go` / `process_handlers.go`:
- Struct with injected `*memory.Store` dependency
- JSON helpers using `writeJSON` / `writeJSONError`
- Route value extraction via `r.PathValue()`

**Validation**: `go build ./...`

**Status**: [ ]

---

#### Step 5.2 — Register routes

**File**: `backend/internal/app/bootstrap.go`

In `buildRouter()`, add:

```go
router.Get("/admin/api/memory/{workspace}", memoryHandlers.ListMemories, jsonMethodNotAllowed)
router.Post("/admin/api/memory/{workspace}/search", memoryHandlers.SearchMemories, jsonMethodNotAllowed)
router.Get("/admin/api/memory/{workspace}/{id}", memoryHandlers.GetMemory, jsonMethodNotAllowed)
router.Put("/admin/api/memory/{workspace}/{id}", memoryHandlers.UpdateMemory, jsonMethodNotAllowed)
router.Delete("/admin/api/memory/{workspace}/{id}", memoryHandlers.DeleteMemory, jsonMethodNotAllowed)
```

Wire `*memory.Store` into `MemoryHandlers` in `buildHTTP()`.

**Validation**: `go build ./...`

**Status**: [ ]

---

### Phase 6 — Frontend

#### Step 6.1 — Frontend types

**File**: `frontend/src/types/memory.ts`

```typescript
export type MemoryType = 'long_term' | 'daily' | 'session'

export interface MemoryEntry {
  id: number
  workspace_id: string
  memory_type: MemoryType
  title: string
  content: string
  source: string
  created_at: string
  updated_at: string
}

export interface MemorySearchResult {
  query: string
  entries: MemoryEntry[]
}
```

Add to barrel: `frontend/src/types/index.ts` → `export * from './memory'`

**Status**: [ ]

---

#### Step 6.2 — Frontend service

**File**: `frontend/src/services/memoryService.ts`

- `listMemories(workspaceID, type?, limit?, offset?)` → `GET /admin/api/memory/{workspace}`
- `searchMemories(workspaceID, query, limit?)` → `POST /admin/api/memory/{workspace}/search`
- `deleteMemory(workspaceID, id)` → `DELETE /admin/api/memory/{workspace}/{id}`
- `updateMemory(workspaceID, id, title, content)` → `PUT /admin/api/memory/{workspace}/{id}`

Stateless class pattern — same as `assistantService.ts`.

**Status**: [ ]

---

#### Step 6.3 — Frontend composable

**File**: `frontend/src/composables/useMemory.ts`

Singleton pattern (module-level `ref()` state):
- `memories: Ref<MemoryEntry[]>`
- `searchQuery: Ref<string>`
- `searchResults: Ref<MemoryEntry[]>`
- `selectedMemory: Ref<MemoryEntry | null>`
- `loading: Ref<boolean>`
- `error: Ref<string | null>`

Methods: `fetchMemories(wsID)`, `search(wsID, query)`, `deleteMemory(wsID, id)`, `updateMemory(wsID, id, data)`

**Status**: [ ]

---

#### Step 6.4 — Memory sidebar button + panel

**File**: `frontend/src/components/AgentIde/AgentIde.vue`

1. Add `"memory"` to the `leftTab` ref union type
2. Add a sidebar tab button after Recordings (around line 525):
   ```vue
   <button @click="leftTab = 'memory'" class="sidebar-tab"
     :class="leftTab === 'memory' ? 'sidebar-tab--active' : 'sidebar-tab--inactive'">
     Memory
   </button>
   ```
3. Add conditional render in sidebar content (around line 598):
   ```vue
   <MemoryPanel v-if="leftTab === 'memory'" :workspace-id="selectedWorkspace" />
   ```

**File**: `frontend/src/components/AgentIde/memory/MemoryPanel.vue`

Sidebar panel with:
- Search box input
- Filter by type (long_term / daily / session)
- Scrollable list of memory entries (title + snippet + type badge + timestamp)
- Delete button on hover
- Click entry → emit event to open MemoryDetail in main pane
- Empty state: "No memories yet."

**File**: `frontend/src/components/AgentIde/memory/MemoryDetail.vue`

Main pane with:
- Full content view (markdown rendered)
- Title + type badge + source + timestamps
- Inline edit toggle
- Copy button

**Status**: [ ]

---

### Phase 7 — Testing

#### Step 7.1 — MemoryStore unit tests

**File**: `backend/internal/platform/memory/store_test.go`

- `TestMemoryStore_Insert` — insert a memory, verify it returns a valid ID
- `TestMemoryStore_Search` — insert multiple, search by keyword, verify FTS5 ranking
- `TestMemoryStore_SearchNoMatch` — search for something that doesn't exist, verify empty results
- `TestMemoryStore_WorkspaceIsolation` — insert in ws-1, search in ws-2, verify no cross-contamination
- `TestMemoryStore_Delete` — insert then delete, verify gone from both tables
- `TestMemoryStore_Update` — insert then update title/content, verify FTS index updated
- `TestMemoryStore_DeleteOlderThan` — insert memories with different timestamps, clean old ones
- `TestMemoryStore_List` — insert multiple types, list by type, verify pagination
- `TestMemoryStore_Get` — insert then get by ID, verify all fields

Pattern: `newTestStore(t)` creates temp SQLite DB via `ledger.Open` temp file, wraps in `memory.New`. Same as `ledger/store_test.go`.

**Status**: [ ]

---

#### Step 7.2 — Memory tool unit tests

**File**: `backend/internal/core/tools/memory_tools_test.go`

- `TestMemorySearchTool` — mock store returns results, verify handler formats correctly
- `TestMemorySearchTool_EmptyQuery` — verify validation returns error
- `TestMemoryUpdateTool` — mock store records insert, verify handler calls Insert
- `TestMemoryUpdateTool_MissingRequired` — verify missing args returns error

Use `MockMemoryStore` implementing a small interface or using function fields.

**Status**: [ ]

---

#### Step 7.3 — Agent integration tests

**File**: `backend/internal/core/assistant/agent_test.go`

- `TestAgent_RecallsMemoryInNewSession` — seed memory, create agent with MemoryStore, execute with matching query, verify system prompt contains `<relevant_memories>`
- `TestAgent_WritesMemoryBeforeSieve` — build long history near budget, mock LLM to return `memory_update` tool call, verify store has the memory after execution

Use `MockClient`, `MockProvider`, real `*memory.Store` backed by temp SQLite DB.

**Status**: [ ]

---

## V. Post-Implementation

### CONSTITUTION.md Updates

After completion, update:

1. **Section II.6** — Add sub-section about pre-sieve memory flush. Describe the nudge mechanism and how memory reduces context pressure.
2. **New subsection in II** — Add memory as a first-class agent capability:
   - Memory is stored in SQLite via `memory.Store`, same DB file as ledger
   - `memory_search` and `memory_update` are registered tools available to all agents
   - Active memory injection is opt-in via `UserSettings.Memory.Enabled`
   - All memory queries are workspace-scoped
   - Pre-sieve flush is the canonical mechanism for preventing context loss
3. **Section III.4** — Update the `entity_metadata` paragraph to mention the dedicated `memories` + `memories_fts` tables.

### SPECS Updates

- `docs/SPECS/agent-loop.md`: Add section on memory tools, active injection, and pre-sieve flush flow.

### AGENTS.md Updates

- Add memory to "Architecture" overview section
- Add memory to "When adding a tool" checklist
- Add `memory.Store` nil-safety to "Common Pitfalls"

---

## VI. Clean Code & Architecture Rules

1. **Comments**: Only when WHY is non-obvious. No docstrings on self-documenting methods. Single-line only.
2. **Error handling**: `fmt.Errorf` with `%w`. Use sentinel errors from `models/llm.go` for known conditions.
3. **DRY**: Repeat 3 times before extracting. Memory search query is written once.
4. **No premature abstractions**: No `MemoryProvider` interface until a second backend exists. Use concrete `*memory.Store`.
5. **Nil-safe**: `Agent.memory` is nil when memory is disabled. All code paths check `if a.memory == nil { return }`.
6. **No `context.Background()`**: Every operation derives from the request context.
7. **Functions ≤60 lines**: Keep functions focused. Extract FTS5 query building, trigger migration, etc. into helpers.
8. **Cyclomatic complexity <10**: Early returns, guard clauses, no deep nesting.
9. **One primary type per file**: `MemoryEntry` in `types.go`, `Store` in `store.go`.
10. **No feature flags**: `MemoryConfig.Enabled` is config, not a feature flag. No dead code paths when disabled — the agent simply never creates the store.
11. **Idiomatic Go**: Zero-value init, `:=` for locals, table-driven tests, `range` over index loops.

---

## VII. Progress Tracker

| Step | File(s) | Description | Status |
|------|---------|-------------|--------|
| 1.1 | `ledger/store.go` | Add `DB()` getter | [x] |
| 1.2 | `memory/store.go`, `memory/types.go` | Create MemoryStore with CRUD + FTS4 | [x] |
| 1.3 | `tools/memory_tools.go`, `tools/memory_tools_test.go` | Semantic dedup: FTS4 + topic word overlap ≥2 before insert | [x] |
| 1.4 | `prompts/templates.go`, `assistant/registry.go` | Improve memory prompts: concrete save guidance + recall usage nudge | [x] |
| 2.1 | `models/tools.go` | Add tool name constants | [x] |
| 2.2 | `tools/manifests/memory.json` | Create tool manifest | [x] |
| 2.3 | `tools/memory_tools.go` | Create memory tool handlers | [x] |
| 2.4 | `assistant/registry.go` | Register tools in agent | [x] |
| 3.1 | `models/infrastructure.go` | Add MemoryConfig to UserSettings | [x] |
| 4.1 | `assistant/agent.go` | Add MemoryStore to Agent | [x] |
| 4.1b | `executor.go`, `services.go`, `assistant_service.go`, `bootstrap.go` | Wire MemoryStore through automation executor + service interface + mock | [x] |
| 4.2 | `assistant/stream.go` | Active memory injection | [x] |
| 4.3 | `assistant/session.go` | Pre-sieve memory flush | [x] |
| 4.4 | `assistant/agent_events.go` | Memory lifecycle events | [x] |
| 5.1 | `transport/http/memory_handlers.go` | REST API handlers | [x] |
| 5.2 | `app/bootstrap.go` | Register routes + wire store | [x] |
| 6.1 | `frontend/src/types/memory.ts` | Frontend types | [x] |
| 6.2 | `frontend/src/services/memoryService.ts` | Frontend service | [x] |
| 6.3 | `frontend/src/composables/useMemory.ts` | Frontend composable | [x] |
| 6.4 | `frontend/src/components/AgentIde/*.vue` | Memory sidebar panel + detail | [x] |
| 7.1 | `memory/store_test.go` | MemoryStore unit tests | [x] |
| 7.2 | `tools/memory_tools_test.go` | Memory tool unit tests | [x] |
| 7.3 | `assistant/agent_memory_test.go` | Agent integration tests | [x] |

---

**Plan Completion Criteria**: All 21 steps marked [x], `go build ./...` passes ✅, `go test ./...` passes ✅, `frontend` build passes ✅, CONSTITUTION.md updated ✅, AGENTS.md updated ✅.
