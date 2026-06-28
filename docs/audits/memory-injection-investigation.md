---
status: reference
date: 2026-06-03
related_specs: [SPEC-004]
---
# Memory Injection Investigation Report

**Date:** 2026-06-03
**Model:** Gemma 4 4B Instruct (`gemma-4-4b-it-Q4_K_M.gguf`, Q4_K_M quantization)
**Server:** llama.cpp (AMD Radeon 780M, ~21 tokens/sec, 8192 context, --parallel 1)
**Running on:** Remote host (`vertex`, Ubuntu, Vulkan GPU offload)
**Proxy:** Local Mac dev machine (`go run main.go`)

---

## Table of Contents

1. [Problem Statement](#1-problem-statement)
2. [Architecture Overview](#2-architecture-overview)
3. [How the Automation Task Reaches the Model](#3-how-the-automation-task-reaches-the-model)
4. [Memory System Architecture](#4-memory-system-architecture)
5. [What We've Tried — Detailed Timeline](#5-what-weve-tried--detailed-timeline)
6. [Industry Research](#6-industry-research)
7. [Key Logs and Recordings](#7-key-logs-and-recordings)
8. [What We Know vs. What We Don't Know](#8-what-we-know-vs-what-we-dont-know)
9. [Open Options](#9-open-options)
10. [Appendix: Data Flow Diagrams](#10-appendix-data-flow-diagrams)

---

## 1. Problem Statement

### Core problem

The smoke test automation (`llm-smoke-test.md`) contains 10 steps. Several of these are **discovery commands** — commands that check already-known information like OS version (`uname -a`), TypeScript version (`npx tsc --version`), or installed packages (`npm install --save-dev typescript`).

These facts are stored in a memory store (SQLite FTS5) from previous runs. The memory system knows:
- `tool_versions`: TypeScript version installed is 6.0.3
- `system_os_info`: Operating System detected: Darwin Mac 25.5.0

Despite the model having access to these facts via injected memory, it **re-runs the discovery commands anyway**, wasting 30-40 seconds per run.

### Why this matters

| Aspect | Impact |
|---|---|
| Startup delay with rewriter | 77-89 seconds (removed — now 8 seconds) |
| Extra discovery steps | 3 steps × 10-15s each = ~30-45s |
| Total runtime | ~7 minutes (still too slow for a smoke test) |
| With successful memory skip | Estimated ~5.5-6 minutes |

### The constraint

The model is a 4-billion-parameter Gemma 4 Instruct quantized to Q4_K_M. It runs at ~21 tokens/sec on a laptop-grade AMD GPU (Radeon 780M). We cannot change the model. We must work within its instruction-following capabilities at this size.

---

## 2. Architecture Overview

```
┌─────────────────────┐     ┌─────────────────────┐     ┌──────────────────┐
│   Mac Dev Machine   │     │   Linux Server      │     │   Workspace      │
│                     │     │   (vertex)          │     │   (SQLite FTS5)  │
│  go run main.go     │────▶│  llama.cpp server   │────▶│  memory.Store    │
│  LLM Proxy          │     │  gemma-4 GGUF       │     │  memories_fts    │
│  port :4001         │     │  port :8082         │     │  (FTS5 BM25)     │
└─────────────────────┘     └─────────────────────┘     └──────────────────┘
         │                                                       ▲
         │  executor.go ────▶ stream.go ────▶ agent.go          │
         │  LLMTaskExecutor    processStream    Agent.Execute   │
         │  buildPrompt()      injectActive     tool loop       │
         │  buildAssistant-    Memory()                          │
         │  Prefill()                                            │
         └──────────────────────────────────────────────────────────┘
                       FTS5 search (task content as query)
                          top 5 most relevant entries
```

### Component responsibilities

| Component | File | Responsibility |
|---|---|---|
| `LLMTaskExecutor.Execute()` | `executor.go:93` | Orchestrates the full automation run: gets LLM client, builds prompt, injects prefill, calls agent |
| `buildPrompt()` | `executor.go:341` | Prepends `MemoryCheckGate` to task content, wraps with `AutomationTaskPrompt` template |
| `buildAssistantPrefill()` | `executor.go:347` | Generates assistant-role message with memory citations (added 2026-06-03) |
| `Agent.Execute()` | `agent.go` | Runs the agent loop: send prompt → receive response → parse tools → execute → repeat |
| `injectActiveMemory()` | `stream.go:88` | Appends `<relevant_memories>` block to system prompt on every turn |
| `Auto-callback` | `automation/dispatcher.go` | 5-minute timeout for the full automation |

### The agent loop per turn

```
[System prompt]  ── includes rules, tool guidance, <relevant_memories> block
[User/Task]      ── includes MemoryCheckGate, task instructions
[Assistant]      ── model generates reasoning + tool calls
[Tool Result]    ── tool execution output appended to history
                           │
                           ▼
                    next iteration
```

---

## 3. How the Automation Task Reaches the Model

### The final prompt structure

After `executor.go` `Execute()` finishes, the history passed to `agent.Execute()` looks like:

```
History[0] = System:
    [rules, tool guidance, identity]
    [<relevant_memories>]  ← injected by injectActiveMemory() in stream.go
        - tool_versions: TypeScript version installed is 6.0.3
        - system_os_info: Operating System detected: Darwin Mac 25.5.0
        - dev_environment_setup: TypeScript, Node.js, npm installed

History[1] = User:
    TASK: You are an autonomous agent in workspace 'workspace-1'.
    Execute the instructions found in 'llm-smoke-test.md':
    ---
    [Memory Check Gate] Before executing each step below, check if the
    <relevant_memories> block already contains the answer that step is
    trying to discover. If it does, skip the step and use the stored fact.

    Step 1: list_directory
    Step 2: write_file, read_file
    Step 3: uname -a, date -u, echo
    Step 4: mkdir, write_file, chmod, sh
    Step 5: npm init -y, npm install --save-dev typescript, npx tsc --version
    Step 6: mkdir, write_file, npx tsc, node
    Step 7: edit, recompile, rerun
    Step 8: fetch_url
    Step 9: get_network_info
    Step 10: compile results, submit_final_answer
    ---
    Use your tools to complete every step.

History[2] = Assistant (NEW — added 2026-06-03 via buildAssistantPrefill):
    I'll check my relevant memories before each step.

    tool_versions: TypeScript version installed: 6.0.3
    system_os_info: Operating System detected: Darwin Mac 25.5.0
    dev_environment_setup: TypeScript, Node.js, npm installed

    If a memory already contains an answer a step is trying to discover,
    I'll skip that step and use the stored fact.
```

The model then generates its first token based on this history.

### Key insight about attention

The model's attention mechanism weights tokens by **recency** and **role**. For a 4B model:

| Message | Recency weight | Role weight | Content |
|---|---|---|---|
| System | Oldest (lowest) | System = medium | Rules, memory block |
| User | Middle | User = high | Task instructions |
| Assistant (prefill) | **Most recent** | **Assistant = highest** | "I'll check memories" |

The prefill is the **most recent message** and in the **assistant role**. In theory, this should give it maximum attention weight. In practice (as we'll see), the instruction hierarchy still favors the user message's explicit commands.

---

## 4. Memory System Architecture

### Storage

SQLite database (`orchestrator.db`) with FTS5 virtual table for full-text search.

```
TABLE memories (
    id INTEGER PRIMARY KEY,
    workspace_id TEXT,
    memory_type TEXT,     -- long_term | daily | session | user_profile
    title TEXT,           -- "tool_versions", "system_os_info"
    content TEXT,
    source TEXT,
    created_at TEXT,
    updated_at TEXT
)

VIRTUAL TABLE memories_fts USING fts5(
    title, content,
    content=memories,
    content_rowid='id',
    tokenize='unicode61'
)
```

### Retrieval (FTS5 Search)

The search query is the **entire task content** (e.g., all 10 steps of the smoke test). The `sanitiseFTSQuery` function in `store.go`:

1. Strips non-alphanumeric characters (`_`, `-`, `:`, `/`, `.`, etc.)
2. Splits remaining words into terms
3. Wraps each term in double-quotes (FTS5 syntax)
4. Joins with ` OR ` operator
5. Returns top-5 results ranked by BM25 relevance

Example query for the smoke test task:
```
Search: "Step 1: list_directory ... Step 5: npx tsc --version ... httpbin ... get_network_info"
After sanitization:
  "Step" OR "1" OR "list" OR "directory" OR "5" OR "npm" OR "install" OR
  "dev" OR "typescript" OR "npx" OR "tsc" OR "version" OR "httpbin" OR
  "network" OR "info"
```

BM25 then ranks entries by word overlap. Entries about TypeScript (`tool_versions`, `dev_environment`) score highest because the task mentions TypeScript-related words frequently.

### Recent changes to the sanitizer

On 2026-06-03, `sanitiseFTSQuery` was changed to wrap each term in double-quotes. Previously, an agent query like `typescript_version OR tool_versions` would produce `typescript OR version OR OR OR tool OR versions` — two consecutive FTS5 `OR` operators with no term between them crashed with "SQL logic error: fts5: syntax error near 'OR'." Now each term is quoted: `"typescript" OR "version" OR "OR" OR "tool" OR "versions"`.

### Injecting into the prompt

`injectActiveMemory()` in `stream.go`:
1. Walks backward through conversation history to find the last user message
2. Uses the cached automation task prompt as the search query (cached on first call)
3. Searches `memStore.Search(ctx, workspaceID, cachedQuery, 5)` → top 5 entries
4. Appends `<relevant_memories>` block to the system prompt before every turn

### Deduplication (write-time)

`findOverlappingEntry()` in `memory_tools.go` computes Jaccard similarity on normalized topic words. If J ≥ 0.70 against an existing entry:
- Same content → returns "already saved" (no-op)
- Different content → updates existing entry in-place

This prevents the agent from creating duplicate entries with similar topics (e.g., `tool_versions` and `tools_version`).

---

## 5. What We've Tried — Detailed Timeline

### Attempt 1: System prompt `<relevant_memories>` only

**What:** Memories were injected into the system prompt as a `<relevant_memories>` block. The `AutomationTaskPrompt` said "Review the relevant memories block above before each step."

**Result:** The model completely ignored it. It ran every discovery command as written in the task. No observable difference from having no memory at all.

**Log evidence:**
```
No explicit log — model just ran discovery commands as usual.
```

**Why it failed:** System prompt is distant from the task instructions in the model's attention window. The explicit "run X" in the user message overrides the general "review memories" in the system prompt.

---

### Attempt 2: `MemoryCheckGate` nag in task instructions

**What:** Added `MemoryCheckGate` constant prepended to the task content inside the user message: "[Memory Check Gate] Before executing each step below, check if the relevant memories block already contains the answer..."

**Result:** Marginal improvement. The model sometimes checked memory but didn't skip steps even when it found the answer.

**Log evidence:**
```
13:02:32Z memory_search    ← agent searched for typescript version
13:02:33Z (memory found)
13:02:47Z npx tsc --version  ← still ran the command
```

**Why it failed:** The nag is general guidance ("check before each step"). The step instruction ("run npx tsc --version") is specific and imperative. The model prioritizes specific instructions over general guidance — this is a known behavior of smaller models (Instruction Hierarchy, Wallace et al. 2024).

---

### Attempt 3: LLM rewriter (`rewriteTaskWithMemories`)

**What:** Before the agent loop started, send a separate Chat request to the LLM with the original task + memory entries. The LLM rewrites the task instructions to embed memory-check gates directly into each step (e.g., "Step 5: check memory — memory says TypeScript 6.0.3, skip"). The agent never sees the original commands.

**Result:** ✅ Worked perfectly for skipping discovery steps. The agent skipped `npm install` and `npx tsc --version`.

**Log evidence (successful run with rewriter):**
```
# In successful run:
npx tsc --version   ← <-- DID NOT EXECUTE
npm install --save-dev typescript   ← <-- DID NOT EXECUTE
```

**Why it was removed:** The rewriter call takes 77-89 seconds because:
1. The rewriter uses a unique system prompt (`TaskRewriterSystemPrompt`) that never hits llama.cpp's prompt cache
2. Generating up to 2048 tokens at ~21 tokens/sec = ~97 seconds worst case
3. This is a cold-start cost on every automation run, regardless of whether memories changed
4. The 77-89s delay was visible as a frozen UI — user clicks "Start" and nothing happens for 1.5 minutes

**Timeline from the rewriter run:**
```
11:45:18Z Automation execution started
11:45:25Z (model discovery starts — cold request)
11:46:47Z Task instructions rewritten with memory-based check gates | entries 3
          └── 89 seconds later
```

**Timeline WITHOUT the rewriter (current):**
```
12:59:25Z Automation execution started
12:59:33Z First stream request sent
          └── 8 seconds later
```

---

### Attempt 4: Fallback `RewriteFailedNag` → `MemoryCheckGate`

**What:** When the rewriter fails (timeout, server error), inject a fallback nag. Later renamed to `MemoryCheckGate` when the rewriter was removed entirely.

**Result:** Same as Attempt 2 — model reads it but doesn't act on it.

---

### Attempt 5: Warn-only reasoning budget (no `return nil`)

**What:** The proxy terminates the stream when `reasonUsed > reasoningBudget` (budget exceeded). This kills the stream before the first content chunk arrives after the model transitions from thinking to content generation. Changed to warn-only — the warning fires once per stream (added `budgetWarned` flag).

**Result:** ✅ Eliminated the XML/non-streaming fallback chain. Previously, 200+ identical warning lines were logged per stream. Now it fires exactly once.

**Reversed 2026-06-26**: The warn-only behavior above was reverted back to terminate-the-stream. The upstream-server-doesn't-enforce assumption proved true in practice (runaway joke-loop on local Qwen3.5-9B — model kept generating past 7000+ chars with no EOS). `processStream` now honors `ShouldTerminate` by returning nil instead of just logging. A char cap of `maxTokens * 4` (via `exceedsContentCharCap`) is added as a fallback safety net. No budget values were changed. See `docs/PLANS/cross-cutting/assistant-cancel-endpoint.md` § "Fix: honor ShouldTerminate + add char cap safety net" and `docs/architecture.md` invariant #10.

**Log evidence (before):**
```
12:49:21Z WARN reasoning budget exceeded ... (repeated 200+ times)
```

**Log evidence (after):**
```
13:05:30Z WARN reasoning budget exceeded ... (fired exactly once)
```

**Why it worked:** The llama.cpp server already enforces `reasoning_budget` at the API level. The proxy was double-enforcing and killing streams too early.

---

### Attempt 6: Context detach for rewriter (`context.WithoutCancel`)

**What:** The rewriter's context was detached from the parent context so that cancelling the automation (stop button) wouldn't cancel the rewriter mid-generation.

**Result:** Became moot when the rewriter was removed.

---

### Attempt 7: `ApplyModelOverrides` after sync events

**What:** Registry/secrets change handlers and handlers that called `Sync()` were not re-applying model overrides afterward. Settings overrides like `tool_call_format` or `reasoning_budget` set in `settings.yml` were silently wiped on sync.

**Result:** ✅ Fixed. Overrides now persist across sync operations.

---

### Attempt 8: Assistant prefill (`buildAssistantPrefill`)

**What:** Inject an assistant-role message before the agent loop that cites memory entries and states "If a memory already contains an answer, I'll skip that step." The assistant role is the model's own voice — it should treat the memory-check intent as its own most recent thought.

**Result:** Partial improvement. The model reads the prefill (first turn shows `content_len 635` instead of `0` — it acknowledges the memories). But it still runs discovery commands.

**Log evidence (most recent run):**
```
14:41:09Z assistant memory prefill injected | workspace workspace-1 entries 3
14:41:10Z stream request sent
14:41:13Z stream completed | content_len 635 reasoning_len 0 tool_calls 1
          └── Model generated 635 chars before first tool call — acknowledging memory

# But later turns still re-ran discovery:
14:42:42Z uname -a                          ← ran
14:44:10Z npm install --save-dev typescript  ← ran
14:45:05Z npx tsc --version                  ← ran
```

**Why it partially failed:** The prefill expresses an *intent* ("I'll check memories"), not a *binding rule*. When the next user message says "Step 5: run X," the concrete imperative overrides the prior intent for a 4B model. The prefill adds value for models that can maintain intent across long conversations but doesn't override the instruction hierarchy.

---

### Attempt 9: Pattern-based skip directives (`buildSkipDirectives`)

**What:** Scan the task for hardcoded discovery commands (`npx tsc --version`, `uname -a`, etc.) and prepend "SKIP THIS COMMAND" directives when a matching memory exists.

**Result:** Rejected immediately at code review. Hardcoding specific command strings is fragile, impossible to maintain, and doesn't scale to arbitrary tasks.

**Why it was rejected:** Memory can contain ANY fact — `database_connection_string`, `deployment_region`, `api_endpoints`, `file_paths`, `error_workarounds`. We can't predict every discovery command. The pattern list would grow unbounded and still miss edge cases.

---

## 6. Industry Research

### Hermes Agent

**Repository:** [NousResearch/hermes-agent](https://github.com/NousResearch/hermes-agent)

**Memory approach:**
- Memory lives in `~/.hermes/MEMORY.md` — a plain markdown file
- Injected into the system prompt every turn as a **static text block** in the "volatile" tier
- The `MEMORY_GUIDANCE` constant is the only instruction:
  ```
  "You have persistent memory across sessions. Save durable facts using
   the memory tool: user preferences, environment details, tool quirks,
   and stable conventions. Memory is injected into every turn, so keep
   it compact and focused on facts that will still matter later."
  ```
- **No automatic deduplication** — the agent manages its own memory file
- **No "skip discovery" instructions** — the guidance is about what to save, not how to use saved facts

**How they avoid discovery-step re-runs:** They don't face this problem. Hermes is designed for **interactive chat sessions** with humans. The human formats each request conversationally ("configure my TypeScript environment"). There's no static task document with explicit "run X" commands. The agent's discovery behavior is guided by the human's current request, not a pre-written script.

**Why it doesn't help us:** Our use case (automated smoke test execution from a static task file) is fundamentally different from Hermes's interactive chat design.

### OpenClaw

**Repository:** [openclaw/openclaw](https://github.com/openclaw/openclaw)

**Memory approach:**
- Three file system layers: `MEMORY.md` (durable facts), `memory/YYYY-MM-DD.md` (daily notes), `DREAMS.md` (dream diary)
- `MEMORY.md` loaded at session start as part of bootstrap context
- Auto-recall plugin injects snippets before each turn
- **Golden rule:** "Put durable rules in files, not chat. Your AGENTS.md survives compaction."
- **Guidance from maintainers:** "Make retrieval mandatory. Add a rule to AGENTS.md that says 'search memory before acting.' Without it, the agent guesses instead of checking its notes."

**How they avoid discovery re-runs:** Same as Hermes — interactive chat design. The user's current message drives behavior. They explicitly recommend making memory-check mandatory by putting rules in AGENTS.md (analogous to our system prompt).

**Why it doesn't help us:** Same reason as Hermes — interactive chat vs. static task execution.

### Cursor / Claude Code

**Memory approach:**
- No language model backend for memory. Memory is the **files on disk** — the agent reads `.cursorrules`, `AGENTS.md`, `CLAUDE.md`, existing source code
- Context is managed by file paths, not a separate memory store
- The agent decides what to read and what to skip based on the task

**Why it doesn't help us:** Cursor and Claude Code operate on existing files in the workspace. They don't have a separate "memory store" — the file system IS the memory. Our use case has a separate memory store (SQLite FTS5) and an automation task that's executed end-to-end without user intervention.

### Summary of Industry Research

| Project | Memory location | Injection target | Skip instruction | Use case |
|---|---|---|---|---|
| **Hermes Agent** | `~/.hermes/MEMORY.md` | System prompt (volatile tier) | "Save durable facts" | Interactive chat |
| **OpenClaw** | `MEMORY.md` + daily notes | Bootstrap context | "Search memory before acting" | Interactive chat |
| **Cursor** | File system | File contents | None (files = memory) | Coding assistant |
| **Claude Code** | File system | File contents | None (files = memory) | Coding assistant |
| **Our system** | SQLite FTS5 | System prompt + assistant prefill | MemoryCheckGate + prefill | Automated task execution |

**No framework has solved the core problem:** making a 4B model consistently prefer memory guidance over explicit task instructions in an automated execution context. This is a gap in the tooling for small-model automation.

---

## 7. Key Logs and Recordings

### Recording references

| Run | Purpose | Recording file |
|---|---|---|
| With rewriter (89s) | Reference — only run where memory skips worked | `20260603T114647Z_7287d5216e53f810.jsonl` |
| Without rewriter (8s start) | Baseline — prefill added | `20260603T125925Z_b413467387fa98ae.jsonl` |
| Latest (with prefill) | Current state | `20260603T144109Z_626aa9bc04bae5d4.jsonl` |

All recordings: `/Users/mikeathan/dev/llm-proxy/backend/testdata/recordings/gemma-4-4b-it-Q4_K_M.gguf/smoke-test/`

### Turn-by-turn analysis (latest run, with prefill)

```
14:41:09Z assistant memory prefill injected | workspace workspace-1 entries 3
14:41:10Z stream completed | content_len 635 reasoning_len 0 tool_calls 1 → list_directory
         ↑ Model reads prefill, generates 635 chars, calls list_directory
14:41:44Z memory_update | topic workspace_initial_state | action created | id 16
         ↑ Saves session state (incorrect — prompt says NOT to save session outcomes)
14:42:13Z tool → write_file
14:42:29Z tool → read_file
14:42:42Z tool → uname -a
         ↑ STILL RUNS — prefill didn't override the explicit "Step 3" instruction
14:42:51Z tool → date -u
14:43:03Z tool → mkdir -p smoke-test-dir
14:43:14Z tool → write_file (hello.txt)
14:43:31Z tool → chmod +x
14:43:41Z tool → sh smoke-test-dir/hello.txt
14:43:55Z tool → npm init -y
14:44:10Z tool → npm install --save-dev typescript
         ↑ STILL RUNS — prefill didn't override the explicit "Step 5" instruction
14:44:32Z tool → write_file dev-test/index.ts
14:44:46Z tool → npx tsc dev-test/index.ts
14:45:05Z tool → npx tsc --version
         ↑ STILL RUNS
14:45:36Z tool → node dev-test/index.js
14:46:05Z tool → write_file (edit index.ts)
14:46:16Z tool → fetch_url
14:46:38Z tool → npx tsc dev-test/index.ts (recompile)
14:46:54Z tool → get_network_info
14:47:55Z WARN reasoning budget exceeded, letting server enforcement handle it
         │   reasoning_used 915 | budget 910
         └── single warning (budgetWarned flag works)
14:48:13Z tool → submit_final_answer
```

### llama.cpp performance metrics (from server logs)

```
Prompt processing:  ~500-650 tokens/sec (cold) → ~200-350 tokens/sec (warm w/ cache)
Generation:         ~20-22 tokens/sec (steady across all turns)
Reasoning budget:   910 tokens per turn
First-prompt eval:  6-7 seconds (cold — no cache) → 0.3-0.6 seconds (warm)
```

The model is GPU-bound at 21 tokens/sec on an AMD Radeon 780M. This is the hardware limit. Each turn takes ~10-20 seconds for generation.

---

## 8. What We Know vs. What We Don't Know

### What we know

1. **The rewriter works** — it's the only approach that *guarantees* discovery step skipping by removing the imperative command from the task the model sees.

2. **The prefill is read but not followed** — the model acknowledges memories in its first response but doesn't act on them for later steps.

3. **4B models follow specific "run X" over general "skip if known"** — this is consistent with the Instruction Hierarchy research (Wallace et al., 2024): models up to ~7B-8B struggle to reconcile conflicting instructions at different specificity levels.

4. **80s rewriter delay is not acceptable** — the frozen UI for 1.5 minutes is worse than the 30-40s wasted on extra discovery steps.

5. **FTS5 search is effective** — top-5 BM25 results are consistently relevant to the task content.

6. **Memory dedup works** — duplicate save attempts return "already saved" and do not create duplicate entries.

7. **The log spam is fixed** — budget warning fires once per stream, not 200+ times.

### What we don't know

1. **Would the prefill work better on a larger model (8B+)?** — We only have access to gemma-4-4b. The prefill might be effective on Qwen 3.5 4B or DeepSeek-Coder-V2-Lite 16B.

2. **Does the prefill's position in the history matter?** — Currently it's `[System, User, Assistant]` before the first model generation. What if it was `[System, Assistant, User]` — the prefill BEFORE the task, in the assistant role, right next to the system prompt?

3. **Can we detect when the model is about to run a discovery step and inject a warning?** — The agent loop runs tool-by-tool. Before each tool call, the history is known. Could we check "is the model about to call `execute_terminal_command` with `npx tsc --version`?" and inject a memory reminder at that specific point?

4. **Does a "system prompt restructuring" help?** — What if the task was part of the system prompt instead of the user message? System prompts are weighted differently by some models.

5. **Is this model-specific?** — How does Qwen 3.5 4B behave with the same setup? Different 4B models vary significantly in instruction-following capability.

6. **What's the minimum token budget to avoid reasoning budget cuts on final turns?** — The final report turn consistently exceeds the 910-token reasoning budget. Is 910 too low?

---

## 9. Open Options

Each option is evaluated on: reliability (skips discovery), startup impact, and maintenance cost.

| # | Option | Reliability | Startup impact | Maintenance | Notes |
|---|---|---|---|---|---|
| A | **Hash-cached rewriter** | High (proven) | High first run, instant thereafter | Low | Cache by `sha256(task + memories)`. 90s on first run, instant on repeat. Same task runs repeatedly — cache hit in 90%+ of cases. |
| B | **Pessimistic memory-as-constraint prefill** | Unknown | Low (0 added) | Medium | Restructure prefill to be a binding rule, not an intent: "Step 5: DO NOT RUN tsc. Memory has the answer. If you run it, you waste time." |
| C | **Position swap: prefill before task** | Unknown | Low (0 added) | Low | Change order from `[System, User, Assistant]` to `[System, Assistant, User]` — memories first, then task. Model sees memory intent BEFORE explicit commands. |
| D | **Per-turn memory injection at discovery point** | Unknown | Low (~50 tokens) | Medium | When the model calls a tool that matches a memory entry, inject a nag before the next turn: "Memory says X — you just ran a discovery command. Skip it next time." |
| E | **Accept the overhead** | N/A | None | None | 3 extra steps = ~40s. Total run ~7 min. Net lower than with rewriter (89s startup). |
| F | **Use a different model for discovery-heavy tasks** | Depends on model | None | Medium | Route automation tasks to a model with better instruction-following (7B+, or a model known for hierarchical instruction handling). |

### Option A: Hash-cached rewriter (recommended for investigation)

The rewriter is the ONLY proven solution. The problem is its 89s startup cost. A hash cache eliminates the cost in 90%+ of cases:

```go
executor.go (concept):
  cacheKey := sha256(taskContent + allMemoryEntries)
  if cached, ok := rewriterCache.Get(cacheKey); ok {
      taskContent = cached  // instant
  } else {
      rewritten, err := rewriteWithLLM(ctx, taskContent, entries)
      if err == nil {
          rewriterCache.Set(cacheKey, rewritten)
          taskContent = rewritten
      }
  }
```

**First run:** 80-90s (rewriter + cache population)
**Subsequent runs with same memories:** Instant (cache hit)
**When memories change:** 80-90s (cache miss → re-rewrite → cache update)

The cache could be:
- **In-memory** (lives as long as proxy process, simplest)
- **Persistent** (disk-backed, survives restarts — ~50 lines of code)

For the smoke test scenario (same task, same memories 90% of the time), the 80-90s cost would be paid once per memory change, not once per run.

### Option B: Stronger prefill (pessimistic constraint)

The current prefill says *"I'll check my relevant memories before each step."* This is an intention, not a binding constraint. The model can (and does) ignore intentions when explicit step instructions contradict them.

A stronger version would be:

```
=== MEMORY RULES ===
The following steps in the task are DISCOVERY commands that memory
already has answers for. DO NOT execute these commands. Use the
memory value instead. Mark the step as complete.

  Step 3: uname -a → memory already knows the OS info
  Step 5: npm install --save-dev typescript → TypeScript already installed
  Step 5: npx tsc --version → TypeScript 6.0.3 already known

=== TASK ===
...
```

This is the same as Attempt 9 (pattern-based skip directives) but generalized to work as a FROM-TO template with memory inclusion — no hardcoded patterns, but instead using FTS5-results to identify which task command lines are redundant.

The problem remains: the model still sees the original "run X" command in the task. It still has a conflict.

### Option C: Position swap

Currently: `[System, User {with task}, Assistant {prefill}]`

Proposed: `[System, Assistant {prefill + memories}, User {task}]`

The model processes: system prompt → prefill (its own intent) → task (explicit commands). The prefill is closer to the task than the system prompt. But the task is now the LAST message — which gets the highest attention weight. This could make the problem WORSE, not better.

### Option D: Per-turn injection

When the agent completes a turn and the tool call was a discovery command with a known memory answer, inject a post-hoc nag:

```
[The model just called uname -a, but system_os_info was in memory.
 Remind the model: memory already has this answer.]
Assistant: I notice I ran uname -a even though my memory already
has this information. For remaining steps, I'll check memory first
and skip redundant commands.
```

This is reactive (after-the-fact) rather than proactive. It might help for later steps (the model might remember the reminder for Step 5 after it ran Step 3 unnecessarily). But it could also teach the wrong behavior — the model learns "I get reminded after running extra commands, so the reminder is always there as a safety net."

---

## 10. Appendix: Data Flow Diagrams

### Current flow (without rewriter)

```
executor.go Execute()
  │
  ├── Get LLM client (8s)
  ├── Build Agent (0s)
  ├── buildPrompt():
  │     ├── MemoryCheckGate + task content
  │     └── Wrapped in AutomationTaskPrompt template
  ├── buildAssistantPrefill():
  │     ├── FTS5 search (top 5, task content as query)
  │     └── Generate "I'll check memories" assistant message
  ├── agent.Execute(history):
  │     ├── Turn 1: list_directory ✅
  │     ├── Turn 2: write_file ✅
  │     ├── Turn 3: uname -a ❌ (was in memory, still ran)
  │     ├── Turn 4: mkdir, write, chmod, sh ✅
  │     ├── Turn 5: npm install typescript ❌ (was in memory), tsc --version ❌
  │     ├── Turn 6-10: ... (rest of steps)
  │     └── submit_final_answer ✅
  └── Return result
```

### Previous flow (with rewriter, now removed)

```
executor.go Execute()
  │
  ├── Get LLM client (8s)
  ├── rewriteTaskWithMemories():
  │     ├── FTS5 search (top 5)
  │     ├── Send Chat request with TaskRewriterSystemPrompt + memories
  │     │     └── 80-90s waiting for LLM response (FLAMING DEAD)
  │     └── Rewritten task with embedded skip gates
  ├── Build Agent (0s)
  ├── buildPrompt():
  │     └── MemoryCheckGate + rewritten task content (now has skip gates)
  ├── agent.Execute(history):
  │     ├── Agent never sees "npx tsc --version" — it sees "check memory, skip"
  │     ├── npm install typescript → skipped ✅
  │     ├── tsc --version → skipped ✅
  │     └── submit_final_answer ✅
  └── Return result
```

### Recording flow (how the LLM severs the calls)

```
Browser / UI
   │
   ▼
Automation dispatcher (backend internal/core/automation/)
   │
   ▼
LLMTaskExecutor.Execute()
   │
   ├─► rewriteTaskWithMemories()  (removed)
   │     └─► proxy.Client.Chat()   ───► llama.cpp server
   │                                            │
   │                                    (prompt cache warm)
   │
   └─► agent.Execute()
         └─► proxy.Client.Stream()  ───► llama.cpp server
                                               │
                                        21 tokens/sec
                                               │
                                          Streaming chunks
                                            ◄──────── model generates
                                               │
                                          loop ends → next turn
```

---

*This report will be updated as new approaches are tested.*

## Updates: 2026-06-03 — Implementation Findings

### Layer-by-layer results

| # | Layer | Status | Finding |
|---|---|---|---|
| 0 | FTS5 stop-word filter (store.go) | ✅ Implemented | 15 lines, works. Improves BM25 recall for task-relevant entries. |
| 1 | Per-step proximity injection (executor.go) | ⚠️ Fixed: cap 5 + word overlap | First attempt: 46 annotations → ~4600 chars pressure → sieve fire at turn 1 → step repetition (WORSE than before). Fixed: max 5 annotations + word overlap check. |
| 2 | Prior run seeding (executor.go) | ⚠️ Works from run 2+ | Only seeds when a prior completed run exists in state.json. First run in a fresh session has no prior run. Seeds from errored runs too (cancelled runs still have valid events). |

### Critical bug: 46 annotations caused context pressure

The first implementation of `annotateTaskWithMemories()` scanned every line in the task file and ran FTS5 on each. Every line matched some memory entry on common words:

```
"List directory contents" → matched workspace_initial_state (contains "directory")
"Write a file named output.txt" → matched workspace_initial_state (contains "file" and "txt")
"Create directory smoke-test-dir" → matched workspace_initial_state (contains "directory")
```

This generated **46 annotations** for a 15-step task. Each annotation is ~100 chars. Total noise added: **~4600 chars**. This pushed the prompt over the context budget on the FIRST turn, triggering the physical sieve. The sieve truncated history, the model forgot what it had done, and started repeating steps (list_directory ran 3 times, uname ran 3 times).

**Fix applied:**
1. **Cap max annotations to 5** — prevents context bloat regardless of task size
2. **Word overlap filter** — after FTS5 finds a match, verify the task line shares at least one non-stop-word with the memory entry. A line "fetch data from httpbin" won't match a compliance memory "all checks passed" (no shared non-stop words). A line "install TypeScript and run tsc --version" WILL match "tool_versions: TypeScript version 6.0.3" (shared: "typescript", "version").

### Fix applied to Prior Run Seeding

The original implementation skipped runs with errors (`lastRun.Error != ""`). But cancelled runs still have valid completed tool events. Changed to seed from ANY run with events, regardless of error status. Also fixed the tool result type assertion — `payload["result"]` can be `any`, changed from string type assertion to `fmt.Sprintf("%v", ...)`.

### NEW FINDING: Sieve fires from model verbosity, not annotations

After the annotation cap fix, the sieve STILL fires every turn. The cause is now visible: the model generates **700-2000 chars of reasoning before each tool call**. Each turn adds ~1000-2500 chars to history. After 5-6 turns, history exceeds the ~10924 char budget and the sieve fires.

Latest run analysis (annotations capped at 5, sieve still fires):
```
15:47:17Z task annotated with per-step memory references | annotations 5
15:48:30Z stream completed | content_len 0 reasoning_len 3127 tool_calls 1 → memory_search
15:48:30Z WARN critical context pressure - activating physical sieve | chars 11659
         ↑ Sieve fires IMMEDIATELY after turn 2 — before tools even execute
```

The model's verbose reasoning style (3127 chars of reasoning for a simple memory search) is the root cause. Each turn follows this pattern:

```
Assistant: [recaps all steps done so far — 800-2000 chars]
           [explains what to do next — 200-500 chars]
           [calls tool — 50 chars]
```

This is ~1000-2500 chars per turn of pure structural repetition. The model re-states its progress on EVERY turn, even though the conversation history already shows it. This is model behavior, not code behavior.

### Log extraction showing verbosity (from latest run):

```
Step 7 [16:49:26]
Assistant: The user wants me to continue executing the steps outlined...
I have completed:
- Step 1: List contents of the current directory (Done...)
- Step 2: Write and Verify smoke-test-output.txt (Done...)
- Step 3: System Commands (uname -a, date, echo) (Done...)

Next steps:
- Step 4 — Terminal: File Operations
...

I will start with Step 4 now.
Step 4 details:
1. Create a directory smoke-test-dir...
2. Create a file smoke-test-dir/hello.txt...
```

This is ~1500 chars of recapping steps already visible in history. This repeats EVERY turn.

### Options to address the sieve/verbosity issue

The context budget is currently 10924 chars (determined by `context_budget` on the model, derived from llama.cpp's `--ctx-size 8192` at ~0.75 chars/token ratio). The sieve fires when history exceeds this.

| Option | What | Impact | Risk |
|---|---|---|---|
| **A. Increase context budget** | `context_budget` from 10924 to 16000 | More turns before sieve. ~8 turns instead of ~5-6 before truncation. | May approach llama.cpp's 8192 token limit. Higher memory. |
| **B. Keep verbosity-aware sieve** | Improve the sieve to prefer keeping tool results over reasoning recap | More accurate truncation — keep the data, drop the verbosity | Harder to maintain; sieve logic is complex |
| **C. Accept the overhead (current state)** | ~7min run, steps may re-run after sieve | No code changes, runs complete correctly | Slow, repetitive, user-unfriendly |
| **D. Hash-cached rewriter (from Option A)** | Rewrite task to embed skips directly | Only proven approach to skip discovery. 80-90s first run, instant thereafter. | Startup cost for first run or when memories change |
| **E. Prior Run Seeding only** | Seed from last run's events. First run has no prior run. | First run is slow (re-runs discovery + sieve). Subsequent runs are fast (history seeded, model sees completed steps). | First run is bad. Seeding doesn't help the sieve issue. |

### Recommended next steps

**Short-term (no code change):** Increase `context_budget` to give the model more room before the sieve fires. The model is verbose, but with more context it completes steps before truncation. This is a configuration change in the model registry or settings.yml.

**Medium-term (if need full reliability):** The hash-cached rewriter (Option A) is the only approach that definitively solved the discovery-skip problem. The 80-90s startup cost is a one-time fee per (task, memories) pair. Subsequent runs with the same memories are instant. The rewriter + Prior Run Seeding together give both reliability (rewriter) and context preservation (seeding reduces the number of turns the model needs to make).

**Long-term (if model changes):** Use a larger model (8B+) for automation tasks. The verbosity + instruction hierarchy issues are both strongly correlated with model size. A 7B-8B model would:
- Need fewer reasoning tokens per decision (~300-500 vs ~700-2000)
- Follow hierarchical instructions better ("check memory first" overrides "run X")
- Generate faster (2-3x more tokens/sec on the same hardware)

---

### Attempt 7: notify_user description + prompt-level changes (June 7)

After disabling memory injection for automation, the model began calling `notify_user` instead of
`submit_final_answer` at task completion. Several prompt attempts were tried:

#### 7a. Changing the `notify_user` tool description

**What:** Changed from "Send notifications and reports to the user via external platforms"
to "Send a short notification to the user during execution. Do NOT use this to submit
final results — call submit_final_answer instead."

**Result:** No change. The model's behavior was identical with both descriptions.
The root cause was that `submit_final_answer` was never in the native tool schema sent
to the LLM (a trailing comma in `system.json` caused Go's `json.Unmarshal` to reject
the manifest, silently dropping both system tools).

#### 7b. Removing memory nudge prompts from the task prompt

**What:** Removed the `During execution — when you discover a durable fact...save it immediately
with memory_update` paragraph from `AutomationTaskPrompt`. Removed `MemoryProactiveNudge` from
`GetSystemPrompt()`.

**Result:** Reduced noise but root cause was the missing `submit_final_answer` tool.

#### 7c. Adding "Do NOT use notify_user" rules to DefaultRules

**What:** Added a bullet to rule 6: "Do NOT use 'notify_user' to submit results."

**Result:** No change. Negative instructions ("don't do X") are less effective than
positive instructions ("do Y") for small models. The 4B model ignores this at step 10.

---

### Root Cause (June 7, Updated)

The `submit_final_answer` and `system_error` tools were silently dropped from the tool registry
because a trailing comma in `manifests/system.json` caused Go's strict `json.Unmarshal` to reject
the manifest during `LoadManifestAsTool`. The function returned an error, `addTool` logged a
warning and returned without adding the tools. The LLM never received `submit_final_answer` in
its native tool schema, so despite the prompt telling it to call the tool, the model physically
could not emit it.

Event timeline:

1. Memory branch introduced trailing comma in `system.json` line 34
2. On every server restart, `InitializeAgentStack` → `registerAll()` → `registerSystemTools()`
   → `registerTool` → `addTool` → `LoadManifestAsTool("", "system", "submit_final_answer")` fails
   → warning logged, tool silently dropped
3. The LLM receives 12 tools in its schema — none of them `submit_final_answer`
4. The system prompt text still says "call 'submit_final_answer'" (it's built from `templates.go`,
   not from the manifest)
5. With `tool_choice: required`, the model must pick a tool; it falls back to `notify_user`
   (closest match — "Send notifications and reports")
6. Guardrail blocks `notify_user` → run fails with `context canceled`

## Why memory injection works for interactive but not automation

| Factor | Interactive | Automation |
|--------|-------------|------------|
| Search query | Short user message (1-2 sentences) | Full task prompt (hundreds of words) |
| Query specificity | High — specific question | Low — generic instructions |
| Result relevance | High — matches the user's question | Low — broad FTS5 matches |
| Session length | Short (1-5 turns) | Long (18-35 turns) |
| Attention budget | Ample | Exhausted at step 10 |
| Model decision | "Answer the user's question" | "Pick a tool" (must choose one) |

## What would fix memory injection for automation

To re-enable memory injection for automation tasks, three things need to change:

1. **Step-aware queries**: Instead of using the full task prompt, extract the current
   step's keywords (e.g., "npm install typescript" from Step 5) and use those as the
   search query. This returns relevant memories like "TypeScript version installed: 6.0.3".

2. **Early injection**: Place the `<memory>` block near the START of the prompt (after
   the system message, before the conversation history) rather than at the end. This
   keeps it as context without displacing the finalization instructions.

3. **Relevance filtering**: Score each memory entry for relevance to the current step.
   Only inject entries above a threshold. Filter out entries like `system_os_info` that
   are always irrelevant to step execution.

## Files affected

- `internal/core/assistant/stream.go` — `injectActiveMemory()` and `prepareMessagesForTurn()`
- `internal/core/assistant/session.go` — `maybeFlushMemoryBeforeTurn()` automation check
- `internal/core/assistant/prompts/templates.go` — `AutomationTaskPrompt` simplified, `MemoryProactiveNudge` removed
- `internal/core/assistant/registry.go` — `MemoryProactiveNudge` removed from `GetSystemPrompt()`
- `internal/core/automation/executor.go` — `MemoryStore` removed from AgentOptions
- `internal/core/tools/manifests/system.json` — trailing comma fixed

## Key takeaway

Memory injection is not inherently broken — it works well for interactive sessions
where queries are specific and sessions are short. The failure is specific to
automation tasks where the query is too generic, the session is too long, and the
model's attention is exhausted. Future work should focus on step-aware injection
rather than removing the feature entirely.

The `notify_user` mis-selection bug was ultimately caused by a single trailing comma
in `system.json` that silently dropped `submit_final_answer` from the native tool schema.
All prompt-level attempts to fix it were addressing the symptom, not the cause.
