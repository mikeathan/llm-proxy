---
name: automation
description: "Automation system: dispatcher, executor, run lifecycle, and templates. Use when working on scheduled tasks or automations."
metadata:
  last_reviewed: "2026-10-26"
---

# Automation System — Dispatcher, Executor & Task Lifecycle

**Source docs:** SPEC-007 (the dispatcher + run-scheduler/model-residency contract), `docs/architecture/map.md` (package layout), `docs/architecture/pitfalls.md` #34–#36

---

## Reference files

Open only the one your task needs:

| File | Open it when |
|------|--------------|
| [`references/components.md`](references/components.md) | You need the role of a specific dispatcher/executor component. |
| [`references/delivery-output-and-overrides.md`](references/delivery-output-and-overrides.md) | You are changing how results are delivered (`notify`), where run output goes, or per-run model config. |
| [`references/run-lane.md`](references/run-lane.md) | You are changing admission, concurrency or model residency for runs. |

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
2. **Start** — Dispatcher creates a run and publishes an `EventMessage` (system "▶ Booting automation…")
3. **Execute** — `LLMTaskExecutor.Execute()` runs the agent loop
4. **Progress** — `AgentEvent`s stream turn-by-turn to the UI on the shared bus
5. **Complete/Fail** — `handleAgentSuccess` publishes a final `EventMessage` (`MsgExecutionComplete`); failures publish `EventError`. The run + result are persisted via `recordRun` (run dir/ledger), and workspace run state is exposed via `/active-runs`.

> There is no `run_start` / `run_complete` / `run_fail` event type — run status is inferred from the
> streamed `AgentEvent`s plus the run ledger / active-runs APIs.

## Task Templates

Templates live in `backend/data/templates/`. They're plain markdown files copied to the workspace when a task starts. The template is the first user message the agent sees.

**Header fields:** a playbook starts with `## Task:`, `**ID:**` and `**Category:**`; it may also declare the automation settings it
needs with `**Agent:**` (a profile id), `**Network:**` (`none|lan|internet_only|internet`) and `**Journal:**` (`on|off`), `**Schedule:**` (cron) and, for a network audit, `**Depth:**` (`quick|thorough`), `**Checks:**` (`on|off`), `**Ports:**` and `**Targets:**` before its first
`###` section. `TemplateStore.Get` returns them as `defaults`; the automation form applies what is declared (the agent only if it exists)
when it is opened from the playbook, and each stays editable. Do not add a second creation path for a kind of automation: declare it here.

**Template writing conventions:**
- Write for the agent, not for a human tester. Use "Save these facts to memory" not "Present the agent with these facts."
- For search-heavy tests, use "Search at most once" to prevent search spirals.
- The template must produce a clear PASS/FAIL result at the end.
- After changing a template, also update the workspace copy if one exists.

## Important Gotchas

- The `EventError` payload must be `map[string]string{"error": ..., "hint": ...}` — the same
  shape the assistant failure path (`conversation_service.handleErrorResult`) publishes. The shared
  frontend consumer (`messageBuilder` `case 'error'`) reads `payload.error` and falls back to
  **"Unknown error"** otherwise; publishing a `proxy.Message` (as `failRun` once did) silently
  discards the classified cause.
- The workspace is NOT cleaned between runs. Leftover files from previous runs pollute the agent's context and cause confusion.
- `notify_user` is NOT a valid tool for automation. The guardrail blocks it. Write the final report directly as natural-language output.
- A trailing comma in a tools manifest (`system.json`) can silently drop a tool entry — validate manifests after edits.
- The spiral detector kills runs that repeat the same tool call with varying args 12+ times. This prevents infinite search loops.
- Memory can accumulate across runs in orchestrator.db. To fully reset, delete the db file.
- For memory-tags tests, the persona count for `tags:["persona"]` should be 4 (fact 6 is appended into the same topic).
- **State object concept (not implemented)** — A `[DONE]/[ACTIVE]/[PENDING]` progress block pinned at index [1] in the prompt was proposed but never built. The idea: the model calls a `complete_step` tool, Go updates the block, and the block survives the sieve as ground truth. If reviving this, use an explicit `complete_step` tool (not fragile signature matching) and pin the block at index [1] as a system message to survive truncation.

