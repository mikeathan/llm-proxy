# Automation System — Dispatcher, Executor & Task Lifecycle — Result Delivery, Run Output and Per-Run Model Overrides

Reference for the [`automation`](../SKILL.md) skill, loaded on demand. You are changing how results are delivered (`notify`), where run output goes, or per-run model config.

## Result Delivery (`notify`)

An automation's `notify:` block (`connector`, `dedup`, `dedup_days`, `send_empty`) makes the
**dispatcher** deliver the final report through a communication connector (SPEC-007 §II.6;
`automation/notify.go`, `digest.go`). The agent never sends it — keep `notify_user` out of task
templates. Gotchas:

- `ExecuteResponse.Report` (not `Output`) is what gets delivered; `Output` carries the run header.
- Dedup keys are canonical link URLs from markdown-table rows, so templates that want dedup must
  produce a table with a real link per row (`llm_ai_release_brief.md` does).
- The seen ledger is recorded **after** a successful send; never move that write before `send`.
- Delivery uses a fresh 30 s context off the dispatcher ctx — don't derive it from `execCtx`.
- `Dispatcher` only delivers when built with `WithNotifier` (`app.BuildDispatcher` wires it from the
  tool provider's `CommunicationTools`); tests set `d.notifier` directly.

### The workspace heartbeat, `skip_if_busy` and `HEARTBEAT_OK`

- The heartbeat is `WorkspaceConfig.Heartbeat` (off by default), compiled to a reserved-name
  (`models.HeartbeatAutomationName`) interval automation in `registerWorkspaceAutomations`; never
  hand-write one. Its runtime rules live in `automation/heartbeat.go`: `admitRun` skips a tick whose
  `heartbeat.md` has no checks (`models.HeartbeatBody`) before touching the lane, `prepareRun` sends
  `prompts.HeartbeatTask` (checks + reply rules), and outcomes go to `meta/<ws>/heartbeat-status.json`.
  The starter file is comments only. `cron_schedule` no longer exists.
- `skip_if_busy` is a `runlane.Job` flag (set in `admitRun` only for non-manual fires) via
  `Dispatcher.skipIfBusy`: the heartbeat derives it from its model's lane (local → on), others use
  their own flag. `Submit`
  removes the just-queued entry and returns `DispositionSkipped`; `shouldRequeueLocked` returns false
  for it, so a preempted heartbeat is dropped. Skips are metrics-only (no history).
- A report containing `HEARTBEAT_OK` is blanked in `Output` by `ApplyPulseLogic` and must also be
  withheld from delivery (`deliverReport` checks it) — `Report` is not blanked.
- `AutomationRun`/state `LastPulse` is **per workspace**, not per automation; do not present it as
  an automation's own "last quiet check".

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
