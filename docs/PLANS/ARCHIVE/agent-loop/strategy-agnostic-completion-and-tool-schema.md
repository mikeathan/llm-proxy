---
status: complete
date: 2026-08-18
related_specs: [SPEC-010, SPEC-006, SPEC-001]
---

# Plan: Strategy-Agnostic Completion + Tool-Schema/Policy Consistency

## 0. TL;DR

Plan-execute automations fail with `plan step N: guardrail denied notify_user`, and the
strategy returns the literal string `"[Plan execution complete]"` as its final answer.
Root cause is **four coupled defects**: plan-execute (a) has no final-report synthesis,
(b) plans with a prompt that carries no "deliverable contract", (c) is offered tools the
guardrail has statically disabled (schema ≠ policy), and (d) aborts the whole plan on a
single guardrail denial. The fix is four agnostic layers: a shared `finalizeReport`
primitive, a hardened plan-generation prompt, a guardrail-derived tool schema resolved at
one narrow waist, and non-aborting plan-step denial handling. Nothing is channel- or
strategy-specific; adding a future strategy requires zero completion/tool-policy work.

---

## 1. Root cause (verified)

Run `20260818T150003Z_e8e381932d93fc5c` (workspace-1, dev-test): the plan generator emitted
5 steps, the last being `notify_user`, which the guardrail denied because
`Communication.Enabled == false` (manifest default, `communication.json` → `"enabled": false`).
`executePlan` then aborted the whole run. Four defects, in causal order:

1. **Plan-execute has no final report.** `executePlan` returns the literal
   `"[Plan execution complete]"` (`tool_exec.go:358`); `PlanExecuteStrategy.Run` routes it
   through `completeWith` (`plan_execute_strategy.go:50`). React — and
   `evaluator_optimizer`, which wraps react — get the deterministic tools-disabled
   finalization turn (`handleNoToolCalls` → `textOnlyNextTurn` + `AutomationFinalizePrompt`,
   `session.go:788`); plan-execute bypasses the react loop and never reaches it. With no
   "report" outlet in the plan, the planner reaches for the nearest "output" tool.

2. **The planner prompt has no deliverable contract.** `ExecutionPlanSystemPrompt` is one
   sentence; `BuildExecutionPlanPrompt` (`prompts/templates.go:459-473`) lists tools + task
   with no instruction that the report is text, not a tool call. The workspace `AGENTS.md`
   already encodes this contract ("write your final report as a normal assistant message …
   only call write_file when the task EXPLICITLY asks") — but the planner never sees it.

3. **Tool schema ≠ guardrail policy.** `notify_user` is statically disabled by default yet
   still advertised in `ListTools`. Filtering is scattered across three callers
   (`conversation_service.go:78` `NewFilteredToolProvider`, `executor.go:169`
   `NewAllowedToolsProvider`, `webhook_handlers.go:163` hardcoded `ExcludeTools`) — the
   Centralization anti-pattern. There is no single point that derives the exposed schema
   from guardrail policy.

4. **`executePlan` aborts on guardrail denial** (`tool_exec.go:346-349`), asymmetric with
   react's `processToolCalls`, which records the denial and continues the loop.

---

## 2. Goals / non-goals

**Goals**
- Plan-execute produces a real final report (never `"[Plan execution complete]"`), via the
  same tools-disabled finalization mechanism react already uses.
- The planner is told the deliverable contract (report ≠ tool; communication tools only on
  explicit external-notification request).
- The tool schema mirrors guardrail static `Enabled` gates, resolved once — no strategy or
  channel can see a statically-disabled tool.
- A guardrail-denied plan step does not abort the remaining plan.
- Agnostic: identical behavior across `react` / `plan_execute` / `evaluator_optimizer` and
  assistant / automation / webhook channels.

**Non-goals**
- No channel-based tool assumptions (an automation with `Communication.Enabled` may still
  plan `notify_user` when the task explicitly requests it — e.g. "ping Slack on failure").
- No change to per-call guardrail behavior (`RequireReview`, allowlists, blocked domains) —
  those remain execution-time gates.
- No new dependencies, config, or CI changes.

---

## 3. Design

### 3.1 Shared `finalizeReport` primitive (Part 1)

The completion step is extracted as a post-loop primitive — the mirror of the existing
`generatePlan` pre-loop primitive (SPEC-010 §IV.2). `completeWith` stays the universal seal
(emit `completed` lifecycle + return); `finalizeReport` becomes the universal
"produce the content" step that strategies invoke when they have no natural text turn.

```go
// finalizeReport runs the deterministic tools-disabled finalization turn and
// returns report text (or bestAvailableAnswer fallback). Shared by every strategy
// that has no natural text completion.
func (s *runSession) finalizeReport(ctx context.Context) (string, error)
```

Mechanics (all shared): append `prompts.AutomationFinalizePrompt` (strategy may supply its
own wording), set `textOnlyNextTurn = true`, run ONE tools-disabled turn via `executeTurn`
(nil tools + `ToolChoiceNone`, bounded by `AgentTurnTimeout`), emit the report as an
`EventMessage`, fall back to `bestAvailableAnswer()` on empty/tool-marker output.

- `plan_execute_strategy.go`: after `executePlan`, call `run.finalizeReport(ctx)` and
  `completeWith(report)`.
- `session.go` `handleNoToolCalls` step (2): delegate to `finalizeReport` (full DRY — avoids
  two drifting copies of the finalization turn).
- `evaluator_optimizer_strategy.go`: no change (inherits react).

### 3.2 Plan-generation prompt hardening (Part 2)

Rewrite `ExecutionPlanSystemPrompt` + `BuildExecutionPlanPrompt` to mirror the `AGENTS.md`
Completion contract:
- Plan steps are tool work only; the final report is produced separately as text — never a
  tool step.
- `notify_user`/communication tools only when the task *explicitly* requests an external
  notification — never to deliver task results.

### 3.3 Tool schema derived from guardrail policy (Part 3)

- `guardrails/guardrails.go`: add `DisabledToolNames(workspaceID string) []string` — merge
  workspace overrides (same as `ValidateToolCall`), return tools whose category has a hard
  "disabled by policy" gate: `notify_user` (`!Communication.Enabled`), `internet_search`
  (`!Search.Enabled`), `fetch_url`/`scan_local_network`/`get_network_info`
  (`!Network.Enabled`). Terminal/filesystem are allowlist-based (no `Enabled` hard gate) and
  are intentionally not covered.
- `agent.go` `NewAgent`: add `AllowedTools []string` / `ExcludedTools []string` to
  `AgentOptions`; resolve once via `resolveToolProvider(base, gr, workspaceID, allowed,
  excluded)` and wire into `deps.Provider`.
- Fold the three scattered caller wraps into options (remove hand-wrapping). `NewAgent` is
  the single narrow waist; every strategy consumes `deps.Provider.ListTools`.

### 3.4 `executePlan` guardrail resilience (Part 4)

On `stopBatch` (guardrail denied), record the denial as a tool-result error and continue to
the next step (matching `processToolCalls`), rather than aborting the whole plan.

---

## 4. File-by-file changes

- `backend/internal/core/assistant/session.go` — add `finalizeReport`; `handleNoToolCalls`
  step (2) delegates to it.
- `backend/internal/core/assistant/plan_execute_strategy.go` — call `finalizeReport` +
  `completeWith(report)`.
- `backend/internal/core/assistant/prompts/templates.go` — rewrite plan prompts.
- `backend/internal/core/assistant/guardrails/guardrails.go` — `DisabledToolNames`.
- `backend/internal/core/assistant/agent.go` — `AllowedTools`/`ExcludedTools` options;
  `resolveToolProvider` wiring.
- `backend/internal/core/assistant/tool_availability.go` (new) — `resolveToolProvider`.
- `backend/internal/core/assistant/conversation_service.go` — drop manual filter wrap; pass
  `ExcludedTools`.
- `backend/internal/core/automation/executor.go` — drop manual allow-wrap; pass
  `AllowedTools`.
- `backend/internal/transport/http/handlers/webhook_handlers.go` — drop hardcoded
  `ExcludeTools{notify_user}`; rely on guardrail-derived exclusion.
- `backend/internal/core/assistant/tool_exec.go` — non-aborting guardrail-denied step.

---

## 5. Test plan (`backend/internal/core/assistant/`)

1. `finalizeReport` unit tests: tools-disabled turn emits nil tools + `ToolChoiceNone`;
   empty output → `bestAvailableAnswer` fallback; report text emitted as `EventMessage`.
2. Plan-execute: reply after steps is the synthesized report, not `[Plan execution complete]`.
   Flip the two existing assertions that expect the old literal
   (`agent_test.go:2849`, `plan_execute_strategy_test.go:47`).
3. `DisabledToolNames`: default-off communication; enabled preserves; workspace override merge.
4. `resolveToolProvider`: allow ∩ exclude ∩ guardrail-disabled.
5. Plan-execute regression: communication-disabled → plan tool set excludes `notify_user`;
   enabled → includes it.
6. `executePlan` guardrail-denied step: remaining steps still run.

---

## 6. Verification

```bash
cd backend && go build ./...
cd backend && go test ./...
cd backend && go run ./tools/check-complexity/   # ≤12
```

Live smoke: run the `dev-test` automation (plan_execute) and confirm the report is real text
(not `[Plan execution complete]`, not a `notify_user` plan step).

---

## 7. Risks / notes

- `finalizeReport` reuse of `executeTurn` + `textOnlyNextTurn` is safe: `executeTurn` is
  called only from the loop + the new primitive, and the flag is already consumed/reset there.
- The `handleNoToolCalls` delegation is low-risk (identical logic: append prompt +
  `textOnlyNextTurn` + one turn); the react recovery ladder's ordering (nudge → finalize →
  terminal) is preserved.
- `DisabledToolNames` must mirror `ValidateToolCall`'s hard "disabled by policy" gates
  exactly — do NOT include `RequireReview` or allowlist/blocked-domain categories.
- Part 3 is the widest diff (three call sites); land behind tests, last.
- Already-correct and out of scope: `notify_user` manifest description ("Do NOT use to
  deliver task results"), workspace `AGENTS.md` Completion contract, evaluator-optimizer
  report path.

## 8. Implementation record (2026-08-18)

All four parts landed behind tests; `go build ./...`, `go test ./...` (incl. `-race` on the
affected packages), and `check-complexity` pass.

- **Part 1** — `runSession.finalizeReport` (variadic prompt override, default
  `AutomationFinalizePrompt`) added in `session.go`. `plan_execute_strategy.go` calls
  `finalizeReport` then `completeWith(report)`. `handleNoToolCalls` step (2) delegates to it
  (full DRY) and is now terminal; `handleTextTurn`'s `shouldExit` branch seals via
  `completeWith` so the "completed" lifecycle fires exactly once (SPEC-010 §V) — the
  frontend message builder also finalizes from the report `EventMessage`.
- **Part 2** — `ExecutionPlanSystemPrompt` + `BuildExecutionPlanPrompt` now carry the
  deliverable contract: steps are tool work only; reports are text, never a step; comms tools
  only on explicit external-notification requests.
- **Part 3** — `GuardrailEngine.DisabledToolNames` (Communication/Search/Network `Enabled`
  gates, workspace-override merge, override-cache skip). `resolveToolProvider` in the new
  `tool_availability.go` is the single narrow waist (allow ∩ exclude ∩ guardrail-disabled),
  wired in `NewAgent` via new `AgentOptions.AllowedTools`/`ExcludedTools`. The three scattered
  wraps were folded: `conversation_service.go` → `WithExcludedTools`, `executor.go` →
  `AgentOptions.AllowedTools`, `webhook_handlers.go` dropped the hardcoded `notify_user`
  exclusion. `NewFilteredToolProvider`/`NewAllowedToolsProvider` were removed (dead after the
  fold; `filteredToolProvider.ListTools` now applies allow ∩ exclude).
- **Part 4** — `executePlan` records a guardrail-denied step as a tool-result error (already
  done by `resolveGuardrail`) and continues with the remaining steps instead of aborting.

Outstanding: the plan's live smoke run (dev-test automation, plan_execute) is a manual
operator step — automated verification passed; the report path was not smoke-verified against
a live model.

## 9. Follow-up (2026-08-18): plan-step schema validation + truthful write results

The live smoke run (`20260818T181506Z_8053aa6f873444a0`) exposed a sibling of Part 3:
the planner guessed `file_path` instead of the manifest's `path` for `write_file`, and
`executePlan`'s name-only `ValidateToolCall` let it through. The empty `path` resolved to the
workspace root → `plan step 2 failed: open …workspace-1: is a directory`. Three agnostic fixes:

1. **Plan-step schema validation** — `executePlan` now uses `validateToolArgs` (the react
   loop's required-param check) instead of name-only `ValidateToolCall`; a step missing a
   required param fails fast with `plan step N: invalid tool call: missing required
   parameter 'path'` and the tool never executes.
2. **Plan prompt carries the schema** — `BuildExecutionPlanPrompt` renders each tool's
   parameter names/types/required (`formatToolParameters`) so the planner emits
   schema-correct steps instead of guessing.
3. **Truthful write results** — `write_file`/`append_file` handlers return `("", err)` on
   failure instead of `("File written successfully", err)`, so failed writes are no longer
   recorded as successes in events/history (the react loop's output-preserving
   `executeSingleToolStep` string-preference is untouched — terminal still returns partial
   output with an error).

Verified: `go build ./...`, `go test ./...` (incl. the new
`TestPlanExecuteStrategy_WrongParamNameFailsFast`,
`TestLocalToolRegistry_WriteErrorIsTruthful`,
`TestBuildExecutionPlanPrompt_ParameterSchemas`), and `check-complexity` pass.
