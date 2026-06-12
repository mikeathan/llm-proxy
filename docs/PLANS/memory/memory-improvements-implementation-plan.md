---
status: partial
related_specs: [SPEC-004]
---
# Memory Improvements — Detailed Implementation Plan

Based on the Hermes Agent comparison analysis (`docs/hermes-memory-comparison.md`).
Each phase is an independent, fully-testable unit of work.

---

## Phase 1 — Week 1: Quick Wins (3 independent tasks)

---

### Task 1.1: System Prompt Nudge ✅ DONE

**Goal:** Tell the agent to proactively save important facts using `memory_update`, so it stores data without needing explicit user instructions.

#### Files to modify

| File | Change |
|------|--------|
| `internal/core/assistant/prompts/templates.go` | Add `MemoryProactiveNudge` constant |
| `internal/core/assistant/registry.go` | Append nudge when `r.Memory != nil` |

#### Implementation

Add to `templates.go` in the memory section (after line 284):

```go
const MemoryProactiveNudge = "Proactively use `memory_update` to save important facts, decisions, preferences, and environment details you discover — they persist across sessions."
```

In `registry.go` `GetSystemPrompt()`, after building the prompt, append:

```go
if r.Memory != nil {
    prompt += "\n\n" + prompts.MemoryProactiveNudge
}
```

#### Test Plan

**`registry_test.go`**:
- Test that when `Memory != nil`, system prompt contains nudge text
- Test that when `Memory == nil`, system prompt does NOT contain nudge text

#### Verification

```bash
cd backend && go build ./... && go test ./internal/core/assistant/ -run TestSystemPrompt_IncludesMemoryNudge -v
```

---

### Task 1.2: Usage Meter in Memory Injection ✅ DONE

**Goal:** Show the agent how full its memory store is (e.g., `45% — 450/1000 chars`) so it can self-manage capacity.

#### Files to modify

| File | Change |
|------|--------|
| `internal/platform/memory/store.go` | Add `WorkspaceCharCount()` method |
| `internal/platform/memory/store_test.go` | Test `WorkspaceCharCount` |
| `internal/core/assistant/stream.go` | Append usage meter to memory block |
| `internal/core/assistant/prompts/templates.go` | Add `SoftMemoryCharLimit` constant |
| `internal/core/assistant/agent_memory_test.go` | Test meter appears in prompt |

#### Implementation

**`store.go`**:

```go
func (s *Store) WorkspaceCharCount(ctx context.Context, workspaceID string) (int, error) {
    var total sql.NullInt64
    err := s.db.QueryRowContext(ctx,
        `SELECT COALESCE(SUM(LENGTH(content)), 0) FROM memories WHERE workspace_id = ?`,
        workspaceID,
    ).Scan(&total)
    if err != nil {
        return 0, fmt.Errorf("memory char count: %w", err)
    }
    return int(total.Int64), nil
}
```

**`templates.go`**:

```go
const SoftMemoryCharLimit = 4000  // soft limit for usage meter percentage
```

**`stream.go` — in `injectActiveMemory()`**, after building the relevant_memories block (or when no results), append:

```
[memory store: 45% — 450/4000 chars]
```

This always appears, even when no relevant memories matched, so the agent is always aware of capacity.

#### Test Plan

**`store_test.go`**: Insert entries with known lengths, verify sum matches.

**`agent_memory_test.go`**: Seed memory, execute agent, assert system prompt contains `[memory store:`.

#### Verification

```bash
cd backend && go test ./internal/platform/memory/ -run TestMemoryStore_WorkspaceCharCount -v
go test ./internal/core/assistant/ -run TestAgent_UsageMeterInPrompt -v
```

---

### Task 1.3: Duplicate Prevention ✅ DONE

**Goal:** Prevent storing exact duplicate memory content (same `content` + `workspace_id`).

#### Files to modify

| File | Change |
|------|--------|
| `internal/platform/memory/store.go` | Add `Exists()` method |
| `internal/core/tools/memory_tools.go` | Check exists before insert |
| `internal/platform/memory/store_test.go` | Test duplicate detection |
| `internal/core/tools/memory_tools_test.go` | Test duplicate tool response |

#### Implementation

**`store.go`**:

```go
func (s *Store) Exists(ctx context.Context, workspaceID, content string) (bool, error) {
    var count int
    err := s.db.QueryRowContext(ctx,
        `SELECT COUNT(*) FROM memories WHERE workspace_id = ? AND content = ?`,
        workspaceID, content,
    ).Scan(&count)
    if err != nil {
        return false, fmt.Errorf("memory exists check: %w", err)
    }
    return count > 0, nil
}
```

**`memory_tools.go` — in `Update()`**, after validating args but before inserting:

```go
exists, err := m.store.Exists(ctx, wsID, args.Content)
if err != nil {
    return "", fmt.Errorf("memory update failed: %w", err)
}
if exists {
    return fmt.Sprintf("already saved — duplicate content for topic %q", args.Topic), nil
}
```

#### Test Plan

**`store_test.go`**: Insert entry A → exists returns true. Non-existent content → false.

**`memory_tools_test.go`**: Insert same content twice, second call returns "already saved". Verify only 1 row in DB.

#### Verification

```bash
cd backend && go test ./internal/platform/memory/ -run TestMemoryStore_Exists -v
go test ./internal/core/tools/ -run TestMemoryUpdateTool_Duplicate -v
```

---

## 🎉 Phase 1 Complete — All 3 tasks implemented

| Task | Status | Key files |
|------|--------|-----------|
| 1.1 System prompt nudge | ✅ | `templates.go`, `registry.go` |
| 1.2 Usage meter | ✅ | `store.go` `WorkspaceCharCount()`, `stream.go` injection |
| 1.3 Duplicate prevention | ✅ | `store.go` `Exists()`, `memory_tools.go` check |

---

## Phase 2 — Week 2: Session Search (1 task, medium effort)

---

### Task 2.1: Session Search Tool

**Goal:** Let the agent search ALL past conversation history using FTS5, not just what's explicitly stored in memory.

#### Architecture

New `sessionsearch` package with an FTS5 index on conversation messages. Indexing happens automatically in `session.go` after each turn. The agent queries via a `session_search` tool with three calling shapes: discovery, scroll, and browse.

#### Files to create/modify

| File | Action |
|------|--------|
| `internal/platform/sessionsearch/store.go` | **New** — FTS5 session search store |
| `internal/platform/sessionsearch/store_test.go` | **New** — tests |
| `internal/core/tools/sessionsearch_tools.go` | **New** — tool implementation |
| `internal/core/tools/manifests/session_search.json` | **New** — manifest |
| `models/tools.go` | Add `ToolSessionSearch` constant |
| `internal/core/assistant/registry.go` | Register tool, add field |
| `internal/core/assistant/agent.go` | Add `SessionSearchStore` field |
| `internal/core/assistant/session.go` | Index messages after each turn |
| `internal/app/app_context.go` | Init session search store |
| `internal/transport/http/services.go` | Add interface method |
| `internal/core/automation/executor.go` | Wire into agent opts |

#### Implementation detail

**`store.go`** — FTS5 schema:

```sql
CREATE VIRTUAL TABLE IF NOT EXISTS session_messages_fts USING fts5(
    session_id, workspace_id, source, role, title, content,
    tokenize='unicode61'
);

CREATE TABLE IF NOT EXISTS session_index_meta (
    session_id TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL,
    source TEXT NOT NULL DEFAULT 'automation',
    title TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    last_msg_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
```

Key methods:

```go
type Store struct { db *sql.DB }

func New(p db.Provider) (*Store, error)

func (s *Store) IndexMessage(ctx context.Context, wsID, sessionID, source, role, title, content string) error

func (s *Store) Search(ctx context.Context, wsID, query string, limit int) ([]SessionResult, error)

func (s *Store) Scroll(ctx context.Context, sessionID string, aroundMsgID int, window int) ([]Message, error)

func (s *Store) Recent(ctx context.Context, wsID string, limit int) ([]SessionSummary, error)
```

**Three calling shapes on the tool:**

1. **Discovery** (`query` provided): FTS5 search, deduped by session, bookends + match window
2. **Scroll** (`session_id` + `around`): fetch window of messages by row index, no FTS
3. **Browse** (no args): chronological recent sessions

**Indexing hook in `session.go`:**

After a turn completes, index user + assistant messages into FTS5. Add a `SessionSearchStore` field to `Agent`, nil-safe like `memoryStore`.

#### Test Plan

**`store_test.go`**: Index messages, search by keyword, scroll, browse, no-match edge case.

**`sessionsearch_tools_test.go`**: Mock store, verify response format for each calling shape.

**Integration**: Execute agent conversation, verify messages indexed, verify tool can find them.

#### Verification

```bash
cd backend && go build ./... && go test ./internal/platform/sessionsearch/... -v
go test ./internal/core/tools/ -run TestSessionSearch -v
```

---

## Phase 2 — Week 3: Memory Tool Improvements ✅ DONE

---

### Task 2.2: Replace by Substring ✅ DONE

**Goal:** Let `memory_update` update entries by substring matching (`old_text`), in addition to existing ID-based approach.

#### Files to modify

| File | Change |
|------|--------|
| `internal/platform/memory/store.go` | Add `FindByContentSubstring()` |
| `internal/platform/memory/store_test.go` | Add test |
| `internal/core/tools/memory_tools.go` | Handle `old_text` param |
| `internal/core/tools/manifests/memory.json` | Add `old_text` to schema |
| `internal/core/tools/memory_tools_test.go` | Add test |

#### Implementation

**`store.go`**:

```go
func (s *Store) FindByContentSubstring(ctx context.Context, workspaceID, substr string) (*MemoryEntry, error) {
    rows, err := s.db.QueryContext(ctx,
        `SELECT id, workspace_id, memory_type, title, content, source, created_at, updated_at
         FROM memories WHERE workspace_id = ? AND content LIKE ?`,
        workspaceID, "%"+substr+"%",
    )
    if err != nil {
        return nil, fmt.Errorf("memory find substring: %w", err)
    }
    defer rows.Close()
    var entries []MemoryEntry
    for rows.Next() {
        var e MemoryEntry
        if err := rows.Scan(...); err != nil { return nil, err }
        entries = append(entries, e)
    }
    if len(entries) == 0 {
        return nil, fmt.Errorf("no memory entry matching %q", substr)
    }
    if len(entries) > 1 {
        return nil, fmt.Errorf("substring %q matches %d entries — be more specific", substr, len(entries))
    }
    return &entries[0], nil
}
```

**`memory_tools.go`** — add `OldText` to params struct. If provided, use substring match + update instead of insert. Fully backward compatible.

#### Test Plan

**`store_test.go`**: Exact match, ambiguous match (error), no match (error).

**`memory_tools_test.go`**: Update by old_text, verify content changed. Without old_text, creates new entry (existing behavior).

#### Verification

```bash
cd backend && go test ./internal/platform/memory/ -run TestMemoryStore_FindByContentSubstring -v
go test ./internal/core/tools/ -run TestMemoryUpdateTool_UpdateByOldText -v
```

---

### Task 2.3: User Profile Target ✅ DONE

**Goal:** Add a separate `user_profile` memory type for user identity, preferences, communication style. `injectActiveMemory` injects it as a separate `<user_profile>` block.

#### Files to modify

| File | Change |
|------|--------|
| `internal/platform/memory/types.go` | Add `UserProfile` constant |
| `internal/platform/memory/store.go` | Add `memoryType` filter to `Search()` |
| `internal/core/tools/memory_tools.go` | Handle `target` param ("memory" vs "user") |
| `internal/core/tools/manifests/memory.json` | Add `target` enum |
| `internal/core/assistant/stream.go` | Inject `<user_profile>` block |
| `internal/core/assistant/prompts/templates.go` | Add header/footer constants |
| `internal/core/assistant/agent_memory_test.go` | Test user profile injection |

#### Implementation

**`types.go`**:

```go
const UserProfile MemoryType = "user_profile"
```

**`store.go`** — extend `Search()` to accept optional `memoryType` filter. If provided, add `AND m.memory_type = ?` to WHERE clause.

**`memory_tools.go`** — add `Target` param to Update args struct. When `target == "user"`, force `mt = memory.UserProfile`.

**`stream.go`** — in `injectActiveMemory`, search separately for non-user-profile memories and user-profile memories. Build two blocks:

```
<relevant_memories>
**build command**: run go build ./...
</relevant_memories>

<user_profile>
**name**: Alice
**prefers**: concise responses
</user_profile>
```

#### Test Plan

**`store_test.go`**: Search with type filter, verify only matching type returned.

**`agent_memory_test.go`**: Seed both types, assert both blocks in system prompt.

#### Verification

```bash
cd backend && go test ./internal/platform/memory/ -run TestMemoryStore_SearchByType -v
go test ./internal/core/assistant/ -run TestAgent_UserProfileInjection -v
```

---

### Task 2.4: User Profile UI (Frontend) ✅ DONE

**Goal:** Add a "User" filter tab and badge styling for `user_profile` type entries in the MemoryPanel, so users can view and manage profile entries separately from workspace facts.

#### Files to modify

| File | Change |
|------|--------|
| `frontend/src/components/AgentIde/memory/MemoryPanel.vue` | Add "User" filter button, add `user_profile` badge styling |

#### Implementation

**Filter buttons** (lines 82-98): Add a "User" button next to the existing "Daily" button:

```html
<button
  :class="{ 'filter-btn--active': filterType === 'user_profile' }"
  class="filter-btn"
  @click="filterType = 'user_profile'"
>User</button>
```

**Badge styling** (lines 123-161): Add a case for `user_profile` in the type badge:

```html
<span class="memory-type-badge" :class="'memory-type-badge--' + entry.memory_type">
  {{ entry.memory_type === 'long_term' ? 'LT' : entry.memory_type === 'daily' ? 'D' : entry.memory_type === 'session' ? 'S' : 'U' }}
</span>
```

Add CSS class:

```css
.memory-type-badge--user_profile { @apply bg-purple-100 dark:bg-purple-900 text-purple-700 dark:text-purple-300; }
```

**Badge label logic:** The existing ternary already handles `long_term` and `daily`. Add `session` and fall through to `U` for `user_profile`. Alternatively, the badge just shows the memory type since it's already a readable short string.

#### Test Plan

No automated frontend tests exist for this component. Manual verification:
1. Create a user profile memory via the API or agent
2. Navigate to the Memory panel
3. Verify "User" filter tab exists
4. Click "User" — verify only user_profile entries show
5. Verify badge shows `user_profile` with purple styling

#### Verification

```bash
cd frontend && npm run build
```

Then start the backend + frontend, verify in browser.

---

## Phase 3 — Week 4+: Skill System (1 task, large effort)

---

### Task 3.1: Procedural Memory (Skills)

**Goal:** After complex multi-step automations, the agent saves the workflow as a reusable `SKILL.md` file. On similar tasks, it loads the skill and follows the known steps.

#### Architecture

Skills are stored as files: `data/workspaces/<ws>/skills/<skill-name>/SKILL.md`. Two tools:

- `skill_manage` — create (full SKILL.md), patch (old_string → new_string), delete
- `skill_load` — load full content into conversation

Progressive disclosure: the system prompt lists available skills in `<available_skills>` block with name + description. Full content loaded on demand.

#### Files to create/modify

| File | Action |
|------|--------|
| `internal/platform/skills/store.go` | **New** — file-based CRUD |
| `internal/platform/skills/store_test.go` | **New** — tests |
| `internal/core/tools/skill_tools.go` | **New** — Manage + Load providers |
| `internal/core/tools/manifests/skill_manage.json` | **New** — manifest |
| `internal/core/tools/manifests/skill_load.json` | **New** — manifest |
| `models/tools.go` | Add constants |
| `internal/core/assistant/registry.go` | Register tools, add fields |
| `internal/core/assistant/agent.go` | Add `SkillStore` field |
| `internal/core/assistant/session.go` | Post-task skill creation nudge |
| `internal/core/assistant/stream.go` | Inject `<available_skills>` block |

#### Implementation detail

**`skills/store.go`**: CRUD on `SKILL.md` files. `Create` builds a SKILL.md with YAML frontmatter (`name`, `description`, `category`). `Patch` does string replacement. `Delete` removes directory. `List` walks directory and parses frontmatter. `Load` reads full file.

**`skill_tools.go`**: Two providers wrapping the store. `Manage` handles create/patch/delete actions. `Load` returns full content.

**Manifests**: Standard JSON format matching `memory.json` pattern (tool_name, version, description, runtime with parameters, guardrails).

**`stream.go` — `injectAvailableSkills()`**: Lists skills in `<available_skills>` block, appended to system prompt.

**`session.go` — post-complex-task trigger**: After `submit_final_answer`, if `steps >= 5` and skill store exists, inject a user message suggesting the agent save the workflow as a skill.

#### Test Plan

**`skills/store_test.go`**: Create, load, patch, delete, list, not-found.

**`skill_tools_test.go`**: Mock store, test each action.

**Integration**: Run agent with 5+ steps, verify nudge appears, verify skill creation tool works.

#### Verification

```bash
cd backend && go build ./... && go test ./internal/platform/skills/... -v
go test ./internal/core/tools/ -run TestSkill -v
go test ./internal/core/assistant/ -run TestAgent_CreatesSkillAfterComplexTask -v
```

---

## File Change Checklist (All Phases)

| Phase | Task | File | Action |
|-------|------|------|--------|
| ~~1.1~~ | ~~Nudge~~ | ~~`internal/core/assistant/prompts/templates.go`~~ | ~~Add `MemoryProactiveNudge`~~ ✅ |
| ~~1.1~~ | ~~Nudge~~ | ~~`internal/core/assistant/registry.go`~~ | ~~Inject when `r.Memory != nil`~~ ✅ |
| ~~1.1~~ | ~~Nudge~~ | ~~`internal/core/assistant/registry_test.go`~~ | ~~Add test~~ ✅ |
| ~~1.2~~ | ~~Meter~~ | ~~`internal/platform/memory/store.go`~~ | ~~Add `WorkspaceCharCount`~~ ✅ |
| ~~1.2~~ | ~~Meter~~ | ~~`internal/platform/memory/store_test.go`~~ | ~~Add test~~ ✅ |
| ~~1.2~~ | ~~Meter~~ | ~~`internal/core/assistant/stream.go`~~ | ~~Append usage meter~~ ✅ |
| ~~1.2~~ | ~~Meter~~ | ~~`internal/core/assistant/agent_memory_test.go`~~ | ~~Add test~~ ✅ |
| ~~1.2~~ | ~~Meter~~ | ~~`internal/core/assistant/prompts/templates.go`~~ | ~~Add `SoftMemoryCharLimit`~~ ✅ |
| ~~1.3~~ | ~~Dedup~~ | ~~`internal/platform/memory/store.go`~~ | ~~Add `Exists` method~~ ✅ |
| ~~1.3~~ | ~~Dedup~~ | ~~`internal/core/tools/memory_tools.go`~~ | ~~Check before insert~~ ✅ |
| ~~1.3~~ | ~~Dedup~~ | ~~`internal/platform/memory/store_test.go`~~ | ~~Add test~~ ✅ |
| ~~1.3~~ | ~~Dedup~~ | ~~`internal/core/tools/memory_tools_test.go`~~ | ~~Add test~~ ✅ |
| 2.1 | Session search | `internal/platform/sessionsearch/store.go` | **New** |
| 2.1 | Session search | `internal/platform/sessionsearch/store_test.go` | **New** |
| 2.1 | Session search | `internal/core/tools/sessionsearch_tools.go` | **New** |
| 2.1 | Session search | `internal/core/tools/manifests/session_search.json` | **New** |
| 2.1 | Session search | `models/tools.go` | Add `ToolSessionSearch` |
| 2.1 | Session search | `internal/core/assistant/registry.go` | Register, add field |
| 2.1 | Session search | `internal/core/assistant/agent.go` | Add field |
| 2.1 | Session search | `internal/core/assistant/session.go` | Index after each turn |
| 2.1 | Session search | `internal/app/app_context.go` | Init store |
| 2.1 | Session search | `internal/transport/http/services.go` | Add interface method |
| 2.1 | Session search | `internal/core/automation/executor.go` | Wire into agent opts |
| ~~2.2~~ | ~~Substring~~ | ~~`internal/platform/memory/store.go`~~ | ~~Add `FindByContentSubstring`~~ ✅ |
| ~~2.2~~ | ~~Substring~~ | ~~`internal/platform/memory/store_test.go`~~ | ~~Add test~~ ✅ |
| ~~2.2~~ | ~~Substring~~ | ~~`internal/core/tools/memory_tools.go`~~ | ~~Handle `old_text`~~ ✅ |
| ~~2.2~~ | ~~Substring~~ | ~~`internal/core/tools/manifests/memory.json`~~ | ~~Add `old_text`~~ ✅ |
| ~~2.2~~ | ~~Substring~~ | ~~`internal/core/tools/memory_tools_test.go`~~ | ~~Add test~~ ✅ |
| ~~2.3~~ | ~~User profile~~ | ~~`internal/platform/memory/types.go`~~ | ~~Add `UserProfile`~~ ✅ |
| ~~2.3~~ | ~~User profile~~ | ~~`internal/platform/memory/store.go`~~ | ~~Type filter in Search~~ ✅ |
| ~~2.3~~ | ~~User profile~~ | ~~`internal/core/tools/memory_tools.go`~~ | ~~Handle `target`~~ ✅ |
| ~~2.3~~ | ~~User profile~~ | ~~`internal/core/tools/manifests/memory.json`~~ | ~~Add `target`~~ ✅ |
| ~~2.3~~ | ~~User profile~~ | ~~`internal/core/assistant/stream.go`~~ | ~~Inject `<user_profile>`~~ ✅ |
| ~~2.3~~ | ~~User profile~~ | ~~`internal/core/assistant/prompts/templates.go`~~ | ~~Add constants~~ ✅ |
| ~~2.3~~ | ~~User profile~~ | ~~`internal/core/assistant/agent_memory_test.go`~~ | ~~Add test~~ ✅ |
| ~~2.4~~ | ~~User profile UI~~ | ~~`frontend/src/components/AgentIde/memory/MemoryPanel.vue`~~ | ~~Add "User" filter tab + badge styling~~ ✅ |
| 3.1 | Skills | `internal/platform/skills/store.go` | **New** |
| 3.1 | Skills | `internal/platform/skills/store_test.go` | **New** |
| 3.1 | Skills | `internal/core/tools/skill_tools.go` | **New** |
| 3.1 | Skills | `internal/core/tools/manifests/skill_manage.json` | **New** |
| 3.1 | Skills | `internal/core/tools/manifests/skill_load.json` | **New** |
| 3.1 | Skills | `models/tools.go` | Add constants |
| 3.1 | Skills | `internal/core/assistant/registry.go` | Register, add fields |
| 3.1 | Skills | `internal/core/assistant/agent.go` | Add field |
| 3.1 | Skills | `internal/core/assistant/session.go` | Post-task nudge |
| 3.1 | Skills | `internal/core/assistant/stream.go` | Inject `<available_skills>` |

---

## Repository Rules Compliance

Each task must satisfy:

- [ ] `go build ./...` passes
- [ ] `go vet ./...` passes
- [ ] All tests pass (`go test ./...`)
- [ ] **No comments unless WHY is non-obvious**
- [ ] **Functions under 80 lines**, cyclomatic complexity under 10
- [ ] **Happy path to the left** (early returns, guard clauses)
- [ ] **Prompts in `templates.go`** — no inline strings in logic files
- [ ] **`MemoryStore` nil-safety** — check `== nil` before dereferencing
- [ ] **Table-driven tests** for all store methods
- [ ] **Both success + error paths** tested
- [ ] **New tool checklist**: constant → manifest → implementation → registry → prompt
- [ ] **New field in `Agent`**: add to struct + `AgentOptions` + mock update if interface changes

---

## Integration Verification

After all phases, run:

```bash
cd backend && go build ./... && go vet ./... && go test ./... -v
```

Then smoke test:

```bash
cd backend && go run main.go --data ./data --record-dir=testdata/recordings
```

Verify recording JSONL contains:
- System prompt with memory nudge
- System prompt with usage meter
- No duplicate memories in DB
- `session_search` tool calls in recording
