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
- Run timeout = `max(automationTimeout (default 10m), pinned model's timeout_minutes)`, so a slow
  model configured with a longer timeout is not cut off mid-run (`runContext` in dispatcher.go)
- Cron jobs register their **real schedule** (`triggerToCron`), so pre-executor failures no longer
  retry every minute and a new automation does not fire immediately
- Panic containment: every execution goroutine runs through `safe.Go` and cron jobs through
  `cron.Recover`; `executeAutomation` recovers panics and clears the run's running state, so a
  single bad run can never crash the service or leave a workspace stuck marked running
- `RunReaper` (`runreaper.go`, started in `app.New`) prunes completed run directories older than
  `DefaultRunRetention` (30 days) so a long-lived service does not fill the disk

### StopAutomation Behavior
- `StopAutomation()` cancels the execution context immediately (best-effort).
- A 30-second diagnostic goroutine monitors whether the run actually terminated.
- If still running after 30 seconds and a shell PGID is available, force-kills the process group via `syscall.Kill(-pgid, SIGKILL)` and removes the run from `activeRuns`.
- If no shell PGID (network-only run), logs a warning (graceful degradation).
- Subsequent `StopAutomation` calls cancel the previous diagnostic goroutine via a cancellable context (prevents accumulation).
- Shell PGID is polled lazily after agent creation — the persistent shell session is created on first `terminal_execute` call.

### LLMTaskExecutor (`internal/core/automation/executor.go`)
- Gets LLM client
- Builds AgentOptions from model config (max_steps, temperature, reasoning budget, etc.)
- Creates agent, runs `Execute()`
- Returns result to dispatcher

### Cold Local-Model Startup
When an automation targets a local model that is not running (e.g. idle-reaped before a
scheduled run), the first `GetClientForModel` triggers `llama-server` startup and returns
`models.ErrModelStarting` while it warms up. `getLLMClient` now **polls** on that sentinel
(`waitForModelReady`) instead of failing the run immediately, so an unattended run can auto-start
the local LLM and wait for it. The poll runs every `modelStartPollInterval` (3s) up to
`modelStartWaitTimeout` (5 min), mirroring the idle reaper's own 5-minute startup window, and
aborts immediately on any non-starting error or on context cancellation.

### Local Model Failure Detection
A local model that crashes on launch (bad launch args, missing model) is detected via a shared
process-exit watch (`internal/platform/procwatch`, reused by both the model lifecycle and the
persistent shell). `clearCrashedModelLocked` (in `internal/core/llm/lifecycle.go`) is the single
place that turns an exited model into a failure: it records the error (`LastModelError`, surfaced
in the admin status / process-logs endpoints) and clears the dead model. Both `GetInstance` (so a
request surfaces a real error instead of looping on `ErrModelStarting`) and the idle reaper (so a
crashed model is cleared promptly instead of after the 5-minute startup timeout) call it. The
automation path additionally fails with a clear `model <name> did not become ready within 5m`
message via `waitForModelReady` rather than falsely proceeding on a dead model.

### Agent Loop (`internal/core/assistant/`)
- `run()` in `session.go` — main loop, handles turn-by-turn execution
- `executeTurn()` — sieve → computeNextResponse → parse tools → execute → check repetition
- Returns final answer via natural completion (content-only message)

## Run Output Structure

```
backend/data/runs/workspace-<id>/<task-name>/<model>/<timestamp>_<uuid>/
├── run-meta.json      # Duration, LLM calls, tool calls, result
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
| `context_budget` | Model config (default 8000, `(ctx−max_tokens)×4` chars/token for local) | 21848 |
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
- `notify_user` is NOT a valid tool for automation. The guardrail blocks it. Write the final report directly as natural-language output.
- A trailing comma in a tools manifest (`system.json`) can silently drop a tool entry — validate manifests after edits.
- The spiral detector kills runs that repeat the same tool call with varying args 12+ times. This prevents infinite search loops.
- Memory can accumulate across runs in orchestrator.db. To fully reset, delete the db file.
- For memory-tags tests, the persona count for `tags:["persona"]` should be 4 (fact 6 is appended into the same topic).
- **State object concept (not implemented)** — A `[DONE]/[ACTIVE]/[PENDING]` progress block pinned at index [1] in the prompt was proposed but never built. The idea: the model calls a `complete_step` tool, Go updates the block, and the block survives the sieve as ground truth. If reviving this, use an explicit `complete_step` tool (not fragile signature matching) and pin the block at index [1] as a system message to survive truncation.
