---
status: reference
last_reviewed: 2026-07-11
---

# Automation System — Dispatcher, Executor & Task Lifecycle

**Source docs:** SPEC-007, `docs/PLANS/automation/automation-dispatcher-blueprint.md`

---

## Architecture

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│  Automations UI  │────▶│  Dispatcher      │────▶│  LLMTaskExecutor │
│  (frontend)      │     │  (cron/interval  │     │  (per-run)       │
│                  │     │   /manual)       │     │                  │
└─────────────────┘     └──────────────────┘     └─────────────────┘
                              │                           │
                              ▼                           ▼
                       ┌──────────────┐          ┌─────────────────┐
                       │  Workspace   │          │  Agent.Execute()│
                       │  isolation   │          │  (tool loop)    │
                       └──────────────┘          └─────────────────┘
```

## Execution Lifecycle

1. **Trigger** — Scheduled (cron/interval) or manual via UI
2. **Start** — Dispatcher creates a run, emits SSE `run_start`
3. **Execute** — `LLMTaskExecutor.Execute()` runs the agent loop
4. **Progress** — SSE events stream turn-by-turn to the UI
5. **Complete/Fail** — Result saved to run history, SSE `run_complete` or `run_fail`

## Key Components

### Dispatcher (`internal/core/automation/dispatcher.go`)
- Manages schedules (cron, interval, manual)
- Emits SSE events for live UI updates
- Handles run lifecycle (start, cancel, complete)
- 5-minute timeout for full automation runs

### LLMTaskExecutor (`internal/core/automation/executor.go`)
- Gets LLM client
- Builds AgentOptions from model config (max_steps, temperature, reasoning budget, etc.)
- Creates agent, runs `Execute()`
- Returns result to dispatcher

### Agent Loop (`internal/core/assistant/`)
- `run()` in `session.go` — main loop, handles turn-by-turn execution
- `executeTurn()` — sieve → computeNextResponse → parse tools → execute → check repetition
- Returns final answer via `submit_final_answer` tool

## Run Output Structure

```
backend/data/runs/workspace-<id>/<task-name>/<model>/<timestamp>_<uuid>/
├── run-meta.json      # Duration, LLM calls, tool calls, result
├── run.log            # Chronological agent events
├── recording.jsonl    # Full request/response pairs (for replay)
├── events.jsonl       # SSE events (lifecycle, stuck, fallback)
└── final-report.md    # Agent's final output
```

## Model Config Overrides Applied Per-Run

| Field | Source | Example |
|-------|--------|---------|
| `max_tokens` | Model config + metadata | 2730 |
| `reasoning_budget` | Model config or max_tokens/3 | 910 |
| `temperature` | Model config (default 0.1) | 0.1 |
| `max_steps` | Model config (default 25) | 35 |
| `context_budget` | Model config (default 8000) | 10924 |
| `timeout_minutes` | Model config (default 30) | 0 (use default) |
| `tool_call_format` | Model config | "native" or "xml" |
| `prefill` | Model config | true/false |

## Task Templates

Templates live in `backend/data/templates/`. They're plain markdown files copied to the workspace when a task starts. The template is the first user message the agent sees.

**Template writing conventions:**
- Write for the agent, not for a human tester. Use "Save these facts to memory" not "Present the agent with these facts."
- For search-heavy tests, use "Search at most once" to prevent search spirals.
- The template must produce a clear PASS/FAIL result at the end.
- After changing a template, also update the workspace copy if one exists.

## Important Gotchas

- The workspace is NOT cleaned between runs. Leftover files from previous runs pollute the agent's context and cause confusion.
- `submit_final_answer` must be in the tools manifest. A trailing comma in `system.json` previously dropped it silently.
- `notify_user` is NOT a valid tool for automation. The guardrail blocks it. Use `submit_final_answer` for final results.
- The spiral detector kills runs that repeat the same tool call with varying args 12+ times. This prevents infinite search loops.
- Memory can accumulate across runs in orchestrator.db. To fully reset, delete the db file.
- For memory-tags tests, the persona count for `tags:["persona"]` should be 4 (fact 6 is appended into the same topic).
- **State object concept (not implemented)** — A `[DONE]/[ACTIVE]/[PENDING]` progress block pinned at index [1] in the prompt was proposed but never built. The idea: the model calls a `complete_step` tool, Go updates the block, and the block survives the sieve as ground truth. If reviving this, use an explicit `complete_step` tool (not fragile signature matching) and pin the block at index [1] as a system message to survive truncation.
