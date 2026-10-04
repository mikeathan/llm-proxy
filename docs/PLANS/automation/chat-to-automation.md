---
status: proposed
date: 2026-10-04
last_reviewed: 2026-10-04
related_specs: [SPEC-007, SPEC-001, SPEC-003, SPEC-006]
constitution_references: [II.3, II.13, III.2, III.3, V.2, V.3]
related_plans: [../memory/memory-improvements-implementation-plan.md, ../autonomy/event-driven-triggers.md, ../autonomy/standing-goals.md, ../autonomy/autonomy-roadmap.md, ../assistant-ui/assistant-automation-workbench-layout.md]
---

# "Turn This Chat Into an Automation"

**Status:** proposed. Phase 0 is a pre-existing bug fix on the endpoint this feature reuses and can
ship on its own immediately.

## Why this plan exists

Users discover workflows by chatting: "fetch these three pages, compare prices, write a summary to
`prices.md`". When it works they want it every morning — and today they must rewrite the whole thing by
hand as a task file, pick tools, pick a network scope and a trigger. That rewrite is where most
"I'll automate this later" ideas die.

| User benefit | Service benefit |
|---|---|
| One click from a chat that worked to a scheduled job that does the same thing. | More automations created from **proven** runs, not first drafts → fewer failing scheduled runs. |
| The generated automation is **least-privilege by default**: only the tools and network scope the chat actually used. | Unattended runs get a tight `allowed_tools` list without the user knowing that field exists — directly serves `../unattended-run-safety-hardening.md`. |
| A draft the user reviews and edits — nothing runs before they save. | Reuses the existing create-automation and workspace-file APIs; no new storage. |

**How this differs from the planned Skill System** (`../memory/memory-improvements-implementation-plan.md`
Phase 3): skills are *agent-chosen, invisible* procedural memory used inside future runs. This is an
*explicit user action* that produces a visible, editable, schedulable automation. They can share the
trace-extraction step (Phase 1 below) — implement it once.

## Ground truth (verified in code, 2026-10-04)

| Fact | Source |
|---|---|
| Sessions persist at `{root}/meta/{workspaceID}/sessions/{sessionID}.json` | `docs/data-layout.md` (meta/ section) |
| Session messages keep `role`, `content`, `tool_calls`, `tool_call_id`, `error`, and the user turn's `run` record (`model`, tokens) | `backend/models/llm_messages.go:30-68` |
| Session read API: `GET /admin/api/conversation/sessions/{ws}/{session}` | `backend/internal/app/routes.go:265` |
| `Automation` already has `AllowedTools`, `NetworkGrant`, `Model`, `LoopStrategy`, `MemoryMode` | `backend/models/workspace.go:103-121` |
| Create path: `CreateAutomation` → `validateAutomation` → `MutateConfig` → `dispatcher.Register` | `backend/internal/transport/http/handlers/dispatcher_handlers.go:587-626` |
| **Bug:** on a duplicate name the closure inside `MutateConfig` writes a 409 and `return`s from the closure only; the handler then still calls `dispatcher.Register` with the new definition and writes a second response. The registry runs the new definition while `config.yaml` keeps the old one | `dispatcher_handlers.go:603-626` |
| Workspace file write API (for the generated task file): `PUT …/workspaces/{ws}/files/{file...}` | `docs/api-reference.md` (Workspace Files) |
| All prompt strings must live in `core/assistant/prompts/templates.go` | Constitution II.13 |

## Design

### Step 1 — Extract a trace (deterministic, no LLM)
`ExtractWorkflowTrace(session) → Trace` walks the history and returns: the user goals (user messages),
the successful tool calls in order (name + args, results truncated), tools that **failed** and were
retried, files written, hosts fetched, the model used, and the final answer. Pure function; easy to test.

### Step 2 — Derive the safe defaults (deterministic)
- `allowed_tools` = the set of tools with at least one *successful* call in the trace.
- `network_grant` = `none` if no network tool succeeded; `internet_only` if only public hosts;
  otherwise inherit (the user decides — never widen silently).
- `model` = the model the session used; `loop_strategy` = unchanged default.
- Name = slug of the session title, de-duplicated against existing automations.

### Step 3 — Draft the task file (one LLM call)
A new template in `prompts/templates.go` turns the trace into a task file: goal, numbered steps, inputs,
expected outputs, and a "done when" line. Instructions in the template: generalise one-off values
(today's date, a specific file name) into parameters, keep the user's own wording for the goal, never
include secrets (the trace is passed through `tools.RedactSecrets` first —
`backend/internal/core/tools/security.go:22`).

### Step 4 — Suggest a trigger (deterministic heuristics, user confirms)
"every morning/daily" in user messages → `cron 0 7 * * *`; mentions of a folder → `file` trigger
(if `../autonomy/event-driven-triggers.md` has landed); otherwise `manual`.

**Automation or goal?** If the chat was about reaching an *outcome* rather than repeating a *task*
("find me…", "keep an eye on … until …"), the draft offers **"make this a standing goal instead"**,
which pre-fills the new-goal form from the same trace and least-privilege defaults
(`../autonomy/standing-goals.md` Phase 5). The trace extraction in Phase 1 below is shared by both.

### API
`POST /admin/api/conversation/sessions/{ws}/{session}/automation-draft` → returns
`{automation: {...}, task_file_path, task_content, trace_summary, warnings[]}`. **Writes nothing.**
The UI shows the draft in the existing automation form; saving uses the existing
`PUT …/files/{file}` + `POST …/automations` calls. Warnings include "this chat used a tool that
failed", "this chat asked for approval of a blocked call — the automation will pause at the same point".

## Decisions for the user

| # | Decision | Recommendation |
|---|---|---|
| D1 | Which model drafts the task file? (a) the session's model, (b) the configured primary | (a) — it already "knows" the task shape; fall back to (b) if it is not available |
| D2 | Offer the button on any session, or only sessions that ended successfully? | Successful only; failed sessions show it disabled with a reason |
| D3 | Should the draft include a "dry run now" button before saving? | Yes, as a manual trigger right after save — no new run mode |

## Phases

### Phase 0 — Fix the duplicate-name bug in `CreateAutomation` (independent)
- Return a sentinel error from the `MutateConfig` closure (or check before mutating) so a duplicate
  name writes exactly one 409 and never calls `Register`.
- **Acceptance:** new handler test — create `a`, create `a` again with a different task file → single
  409 response, registry entry still has the original task file. `go test ./internal/transport/http/handlers/ -count=1`.

### Phase 1 — Trace extraction + defaults
- `ExtractWorkflowTrace` and default derivation as pure functions with table tests (successful tools only,
  failed-then-retried, network none vs public, secret redaction).
- **Acceptance:** `go test ./internal/core/automation/... -count=1` (package location decided at
  implementation; must not import transport).

### Phase 2 — Draft endpoint + prompt template
- Template in `prompts/templates.go`; endpoint; record-replay test using a recorded LLM response
  (`.agents/skills/testing-guide/SKILL.md` → record-replay) so CI needs no live model.
- **Acceptance:** endpoint test returns a draft and writes no file and no config; `go run ./tools/check-complexity/`.

### Phase 3 — UI
- "Make this an automation" action in the assistant session header; opens the automation form
  pre-filled, shows warnings and the trace summary; Save = existing APIs.
- **Acceptance:** component tests; `npm test && npm run build && npm run test:visual`.

### Phase 4 — Docs
- SPEC-007 (new endpoint, least-privilege defaults rule), SPEC-003 (UI action), `docs/api-reference.md`.
- **Acceptance:** `./scripts/check-agent-harness.sh` green.

## Risks and non-goals
- **Over-fitting** — the draft repeats one-off values. Mitigated by the template's generalisation rule and
  by always showing the draft for review.
- **Privilege drift** — never widen `allowed_tools`/`network_grant` beyond the trace; the user can widen
  in the form.
- **Non-goal:** automatic creation without review; editing existing automations from chat.

## Remaining Work
All phases (0–4). Phase 0 has no dependencies and should land first.
