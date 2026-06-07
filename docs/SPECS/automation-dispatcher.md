---
id: SPEC-007
title: Automation Dispatcher
version: "1.0"
status: stable
last_updated: 2026-05-28
constitution_references: []
related_specs: [SPEC-001, SPEC-005, SPEC-006]
supersedes: docs/PLANS/automation-dispatcher-blueprint.md
---

# SPEC: Automation Dispatcher

## I. Intent

The automation dispatcher schedules, triggers, and manages autonomous task executions (runs).
It provides cron-based scheduling, manual triggers, workspace isolation, and live SSE streaming
of run events to the frontend.

## II. Functional Requirements

### 1. Scheduling

- `CronTrigger` — standard cron expressions for recurring automations.
- `IntervalTrigger` — fixed-interval scheduling.
- `ManualTrigger` — ad-hoc execution via API.
- All triggers are per-workspace.

### 2. Execution

- `LLMTaskExecutor` creates an `Agent` with the configured model and runs the task file content.
- Two strategies:
  - `IsolatedStrategy` — clean workspace each run (no state carryover).
  - `PersistentStrategy` — state continuity across runs (working directory persists).
- Task files are stored per-workspace with CRUD via the API.

### 3. Run Lifecycle

- A run starts when a trigger fires → creates `AutomationRun` with status `pending`.
- Transitions: `pending` → `running` → `completed` | `failed` | `cancelled`.
- Runs are tracked with: model, task file, start time, duration, LLM call count, tool call count,
  step count, output summary, error.
- Run artifacts are stored at `data/runs/{workspace}/{model}/{timestamp}_{session}/`.

### 4. SSE Streaming

- `/dispatcher/workspaces/{ws}/live` — SSE endpoint for per-workspace live events.
- Events: `step_start`, `tool_call`, `tool_result`, `stuck_detected`, `fallback_*`,
  `guardrails:blocked`, `guardrails:resolved`, `run_completed`, `error`.
- `EventBus` in `broadcast.go` manages per-workspace pub/sub.

### 5. Run Artifacts

Each run produces:
- `run-meta.json` — machine-readable metadata (model, status, steps, duration).
- `run.log` — human-readable chronological log (tool calls, results, reasoning budget).
- `events.jsonl` — structured event stream (lifecycle events with timestamps).
- `recording.jsonl` — LLM request/response recordings (when `--record-dir` is active).

## III. Error Handling

- Model not found: run fails with `ErrUnknownModel`.
- Model context maxed: agent sieves and retries.
- Agent times out: 30-minute global timeout, 10-minute per-turn timeout.
- Dispatch to stopped workspace: 404.

## IV. Configuration

- Per-workspace automation definitions in `config.yaml`:
  ```yaml
  automations:
    smoke-test:
      trigger: cron
      cron: "0 */6 * * *"
      model: gemma-4-4b-it
      task_file: llm-smoke-test.md
      strategy: isolated
  ```
