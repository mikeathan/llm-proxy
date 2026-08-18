---
id: SPEC-010
title: Agent Loop Strategies
version: "1.0"
status: stable
last_updated: 2026-08-16
constitution_references: [II.4, II.5, II.6, II.7, II.10, II.13]
related_specs: [SPEC-001, SPEC-005]
supersedes:
---

# SPEC: Agent Loop Strategies

## I. Intent

The agent loop historically was one hardcoded ReAct driver (`runSession.run()` in
`internal/core/assistant/session.go`), with plan-and-execute as a binary short-circuit
in `Agent.Execute()` (`internal/core/assistant/agent.go`). This spec defines a pluggable
**loop-strategy engine**: a registry + resolver that loads and selects one of several
agenting-loop archetypes per run, driven by a per-model `loop_strategy` config
(default `react`). Each technique (ReAct, plan-and-execute, evaluator-optimizer) is a
single-file strategy that composes the shared turn primitives — it never reimplements
them.

The engine is the **single home** of loop selection: `PlanStrategy`/`EnableExecutionPlan`
short-circuit plumbing is deleted outright and replaced by `LoopStrategy`.

## II. Functional Requirements

### 1. Strategy Selection (deterministic, never LLM-chosen)

- `LoopStrategyName` vocabulary: `react`, `plan_execute`, `evaluator_optimizer`.
- Precedence: explicit per-model `ModelConfig.LoopStrategy` > provider/workload default
  (intentionally static `react` until measured) > `react`.
- Unknown/empty config values parse to `react` (defense-in-depth only); HTTP boundaries
  reject unknown non-empty values with `400`.
- An optional per-run `Automation.LoopStrategy` override (automation only) is applied
  after the model config so it wins for that run.

### 2. Registry (loading)

- One registry (`newLoopStrategyRegistry`) maps name → constructor. Explicitly populated,
  no `init()`. A new technique = one strategy file + one `Register` line + one enum constant.
- `RegisteredLoopStrategyNames()` (sorted, registry-derived) is the single source for the
  admin UI dropdown — registering a strategy requires **no frontend edit**.

### 3. Resolver (choosing)

- `resolveLoopStrategy(a)` builds the strategy for one `Agent`; an unregistered name falls
  back to `react` with a logged error (never silent).

### 4. Shared turn primitives

Every strategy reuses the existing primitives: `executeTurn`, `handleToolTurn`,
`handleTextTurn`, `handleNoToolCalls`, `executeSingleToolStep`, `executePlan`, sieve,
repetition detector. `finalizeReport` is the shared completion-side primitive — the mirror of
`generatePlan` — running the deterministic tools-disabled finalization turn and returning the
report text. Fallback chain on finalization failure (LLM error or empty output):
`bestAvailableAnswer()` (last substantive assistant text) → `synthesizeRunSummary()` (a
degraded-but-real summary of the run's tool activity: per-tool call counts and recorded
`{"error": ...}` failures) → error only when the run did no tool work. The synthesized
summary is what saves plan_execute runs (history is pure tool calls, no assistant text) from
dying on a provider outage at the report turn. Strategies compose; they never reimplement
the loop core.

## III. Data Model

- `models.LoopStrategy` is a typed string enum (`models/loop_strategy.go`, leaf
  package — single source of truth) with `react`, `plan_execute`,
  `evaluator_optimizer` and `Valid()`. `assistant.LoopStrategyName` is an alias
  (`type LoopStrategyName = models.LoopStrategy`) so the domain vocabulary stays in
  the assistant package without inverting the leaf dependency.
- `ModelConfig.LoopStrategy models.LoopStrategy` (json `loop_strategy`) — per-model
  override; empty = provider default / `react`. Replaces deleted `EnableExecutionPlan bool`.
- `AgentOptions.LoopStrategy` and `AgentConfig.LoopStrategy` — `LoopStrategyName`
  (the models alias).
- `ModelOverride.LoopStrategy models.LoopStrategy` (settings.yml carrier) so
  `loop_strategy` survives restart via the `ApplyModelOverrides` merge.
- `Automation.LoopStrategy models.LoopStrategy` (workspace `config.yaml` carrier) —
  per-run override.

## IV. Behavior

### 1. ReactStrategy (default)

The pre-refactor loop body, moved verbatim. Stop-guard hook (Phase 3): fires only at the
`handleTextTurn` natural-completion branch, before `completeWith` — never on forced
completion, fallback answers, or error/stall returns.

### 2. PlanExecuteStrategy

Wraps the shared `runSession.generatePlan` primitive + `Agent.executePlan`, with explicit
fallback to the react loop when: no user message, no tools, or plan generation fails. After
`executePlan`, the run produces its final report via the shared `finalizeReport` primitive
(a single tools-disabled finalization turn) and routes it through the shared completion path
(`completeWith`) so the `"completed"` lifecycle is uniform — never the `"[Plan execution
complete]"` literal.

`generatePlan` is the **single** pre-loop LLM-call primitive: it emits a visible
"planning" message + a neutral `agent_thinking` lifecycle signal before the call (so the
UI is never blank during the pre-loop LLM call) and bounds the call by
`AgentTurnTimeout` (same per-call timeout the main loop applies in `executeTurn`). The
call itself **streams** (with a non-streaming `Chat` fallback for SSE-less providers,
mirroring `computeNextResponse`): the planner's reasoning deltas are relayed as
`EventReasoning` snapshots (same field semantics as `processStream`), a shared heartbeat
emits `still_thinking` liveness during silent stalls, and the shared request-config
helper (`applyRequestConfig`) applies the same temperature and reasoning wire params as
normal turns. The plan JSON itself is never surfaced as content — the UI shows reasoning,
then the executed steps, then the report. Every loop strategy — present or future — that
needs a pre-loop plan must compose this primitive, never reimplement
timeout/streaming/feedback logic. `runSession.run()` emits a one-time `agent_thinking`
signal at strategy dispatch so all strategies produce an immediate UI indicator.

`executePlan` validates each step's args against the tool manifest schema exactly like the
react loop (`validateToolArgs`): required parameters must be present and non-empty, so a
plan step that guesses a parameter name fails fast with a clear `invalid tool call` error
instead of executing a tool with an empty value. `BuildExecutionPlanPrompt` renders each
tool's parameter names/types/required so the planner emits schema-correct steps.

**Step-execution failures are recorded, not run-killers** (model-agnostic, matches the
react loop): when a plan step's tool *execution* fails (shell command exiting non-zero, a
compile error, a missing file, a tool timeout), `executeSingleToolStep` has already
appended the failure to history as a tool result; `executePlan` logs
`plan step failed, continuing` and proceeds to the next step, so the final report can note
the failure and no earlier successful step is discarded. This mirrors
`processToolCalls` (react loop) and `TestReactStrategy_ToolErrorContinues`; a single
failed step never aborts the run, regardless of which model produced the plan. Only
**structural** plan errors abort: args marshal failure, `validateToolArgs` failure
(guessed/missing parameter), `MaxPlanSteps` exceeded, and plan-context deadline exceeded —
those mean the plan itself is malformed, not that a step's outcome failed. Guardrail-denied
steps also continue (`plan step guardrail denied, continuing`); in unattended automation
the denial is immediate (Constitution II.10, `resolveGuardrail` automation channel).

### 3. EvaluatorOptimizerStrategy (Phase 3)

React loop plus a bounded generator/evaluator pass: an `EvaluatorGuard` injects a
self-critique nudge before the run finalizes, so the model verifies/fixes its work instead
of finishing prematurely. Prompt-based self-critique only; no verification-evidence ledger.

## V. Lifecycle & Completion

- `run()` owns the `runS` back-pointer setup/teardown (exactly once per `Execute`); a
  strategy that delegates to another (plan → react) does **not** re-enter this setup.
- Every strategy emits the `"completed"` lifecycle **exactly once** on success, via the
  shared `completeWith` path. `executePlan`'s `"[Plan execution complete]"` return is a
  completion marker only — the strategy discards it and produces the real report via
  `finalizeReport` before sealing with `completeWith` (the literal never reaches the user).
- Stop-guards use a dedicated `stopGuardAttempts` counter (cap 2), never
  `finalizeAttempts` (owned by `handleNoToolCalls`'s tools-disabled finalization turn).
- Evaluator nudge messages are registered in `isAgentControlMessage` so completion
  detection never mistakes them for user text.

## VI. Error Handling

- Unknown strategy at the boundary: `400` + clear message listing valid values.
- Unknown strategy at the resolver: log + fall back to `react`.
- Plan generation failure: log `Warn` + fall back to `react` loop.
- All errors wrap with `%w`; no silent fallback, no `_ =` without a comment.

## VII. Constitutional References

- II.4: XML-only tool boundaries (strategies reuse `executeTurn`/parser; unchanged).
- II.5: dual-path tool interface (native vs text/XML; unchanged).
- II.6: sieve — strategies reuse the `s` sieve primitives; never reimplement.
- II.7: natural completion — the completion gate and fallback chain are shared.
- II.10: guardrail decision flow — `executeSingleToolStep`/`executePlan` shared and unchanged.
- II.13: prompt centralization — evaluator nudge is a named constant in `templates.go`.

## VIII. Out of Scope

map-reduce fan-out, strict-auditor, human-in-the-loop, orchestrator-workers,
Mixture-of-Agents, a global verify-on-stop toggle, a per-provider default table, and a
verification-evidence ledger. Each is a follow-up plan; deferred archetypes are **never**
reserved as enum values or registry entries until implemented.
