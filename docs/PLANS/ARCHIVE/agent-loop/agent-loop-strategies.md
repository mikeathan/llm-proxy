---
status: complete
related_specs: [SPEC-001, SPEC-010]
---
# Agent Loop Strategies Plan

Add a pluggable **loop-strategy engine** so the agent can run different agenting-loop
techniques (ReAct, plan-and-execute, evaluator-optimizer) and so future techniques
(map-reduce, strict-auditor, human-in-the-loop, orchestrator-workers) land as single-file
additions instead of edits to the loop core. Mirrors Hermes Agent's decomposition
(tools for escalation, config for policy, deterministic per-model selection) but adds the
one thing Hermes lacks: an explicit per-model `loop_strategy` selector.

**Status:** complete. Phases 1–4 shipped (SPEC-010 §14, engine, ReAct extraction,
plan-execute promotion, evaluator-optimizer stop-guard pipeline, config/UI plumbing,
per-run automation override). Deferred items per §12: automation edit-form UI field,
per-provider default table, verification-evidence ledger, map-reduce/auditor archetypes.

**Implementation deviations from the sketches below** (behavior matches SPEC-010, which is
the authoritative contract): `LoopStrategyBuilder` is `func() LoopStrategy` — strategies are
stateless and read the agent from the run session, so `Build` takes no agent (§3.2);
`providerDefaultStrategy` was intentionally omitted — `resolveLoopStrategyName` returns
static `react` for the default per SPEC-010 §II.1, with no unused table function (§3.3).

---

## 1. Intent

The agent loop today is one hardcoded ReAct driver in `runSession.run()`
(`internal/core/assistant/session.go:402`). Plan-and-execute exists as a **binary
short-circuit** in `Agent.Execute()` (`internal/core/assistant/agent.go:581`): if
`PlanStrategy != nil`, generate a plan and run it; otherwise run the ReAct loop. There is
no way to add a third technique without editing `Execute()`/`session.go` again.

This plan:

1. Extracts the ReAct loop into a `ReactStrategy` (zero behavior change).
2. Promotes plan-and-execute into a `PlanExecuteStrategy` (same behavior, no short-circuit).
3. Adds an evaluator-optimizer `EvaluatorOptimizerStrategy` (generator/evaluator
   self-correction before finalization).
4. Builds the **engine** (registry + resolver) that loads and selects these, driven by a
   per-model `loop_strategy` config (default `react`), with a stub for future provider-tier
   defaults.
5. Deletes the `PlanStrategy`/`EnableExecutionPlan` short-circuit plumbing (replaced
   outright by `LoopStrategy`) so the loop selection has exactly one home.

**Non-goals (deferred — see §12):** map-reduce fan-out, strict-auditor, human-in-the-loop,
orchestrator-workers, Mixture-of-Agents, a global "verify-on-stop" policy toggle, and a
per-provider default table. Each becomes a one-file strategy + one enum + one dropdown later.

---

## 2. Technique Catalog

| # | Technique | Hermes reference | llm-proxy today | This plan |
|---|---|---|---|---|
| 1 | **Autonomous ReAct** (single looping node) | `conversation_loop.py` | ✅ hardcoded in `session.go:402` | extract → `ReactStrategy` (default) |
| 2 | **Plan-and-Execute** | `execute_code`, plan tools | ⚠️ partial short-circuit (`agent.go:581`, `tool_exec.go:42/286`) | promote → `PlanExecuteStrategy` |
| 3 | **Evaluator-Optimizer** (generator/evaluator loop) | `verification_stop.py`, `background_review.py` | ❌ none | add → `EvaluatorOptimizerStrategy` + `StopGuard` |
| 4 | **Sequential / Map-Reduce** (fan-out parallel) | `delegate_tool.py` batch, `moa_loop.py` | ⚠️ parallel tool exec only (Const. II.1) | **deferred** |
| 5 | **Orchestrator-Workers** (router tree) | `delegate_tool.py`, `subagent_lifecycle.py` | ❌ none | **deferred** |
| 6 | **Strict Auditor** (guardrailed state machine) | `kanban_stop.py`, `tool_guardrails.py` | ⚠️ GBNF + guardrails, no terminal-tool gate | **deferred** |
| 7 | **Human-in-the-Loop** (interruptible) | `approval.py` | ⚠️ guardrail decision flow (Const. II.10) | **deferred** (already exists as II.10; formalize later) |

### Decision model (who selects the pattern)

Three layers, matching Hermes and Anthropic's "Building Effective Agents":

1. **Primary — operator config** (deterministic): `ModelConfig.LoopStrategy` (per-model,
   `react` default). The LLM never chooses its own loop shape.
2. **Per-run override** (automation only): optional `Automation.loop_strategy` field
   (§8 Phase 4), applied after the model config so it wins.
3. **Policy invariants** (always on, never LLM-chosen): existing sieve, repetition
   detector, guardrail flow, iteration caps. These live in the shared turn primitives, not
   in any single strategy.

There is **no slash command** — llm-proxy fronts external chat clients and owns no chat
surface. The operator selects the default in the Agent Tuning grid; an automation may
override per run.

---

## 3. Core Engine (loading + choosing)

### 3.1 Strategy interface — `internal/core/assistant/loop_strategy.go` (new)

```go
package assistant

import (
    "context"
    "llm-proxy/internal/core/proxy"
)

// LoopStrategyName is the domain vocabulary for the supported loop archetypes.
// Only values with a registered strategy are declared here — deferred archetypes
// (map_reduce, auditor, human_in_the_loop, orchestrator_workers) are added with
// their own enum constant + registration when implemented, never reserved now.
type LoopStrategyName string

const (
    LoopReact              LoopStrategyName = "react"               // ReAct tool loop (default)
    LoopPlanExecute        LoopStrategyName = "plan_execute"        // plan-first, sequential execution
    LoopEvaluatorOptimizer LoopStrategyName = "evaluator_optimizer" // generator/evaluator self-correction
)

const defaultLoopStrategy = LoopReact

// ParseLoopStrategy normalizes a config string. Empty or unknown -> react.
// Boundary validation (HTTP handlers) rejects unknown values with a 400 so this
// lenient default is defense-in-depth only, never a silent-failure path.
func ParseLoopStrategy(s string) LoopStrategyName {
    n := LoopStrategyName(s)
    if n.Valid() {
        return n
    }
    return defaultLoopStrategy
}

func (n LoopStrategyName) Valid() bool {
    switch n {
    case LoopReact, LoopPlanExecute, LoopEvaluatorOptimizer:
        return true
    }
    return false
}

// LoopStrategy drives one agent run from a shared runSession. It composes the
// existing turn primitives (executeTurn, handleToolTurn, handleTextTurn,
// handleNoToolCalls, executeSingleToolStep, executePlan, sieve, repetition
// detector) — it never reimplements them.
//
// Contract: Run returns the final reply and must finalize through the shared
// completion path (`completeWith`) so the "completed" lifecycle is emitted exactly
// once on success (§9). Run never re-enters runSession setup — `run()` owns the
// `runS` back-pointer (§3.4).
type LoopStrategy interface {
    Name() LoopStrategyName
    Run(ctx context.Context, s *runSession) (reply string, history []proxy.Message, err error)
}
```

### 3.2 Registry (loading) — same file

```go
// LoopStrategyRegistry maps LoopStrategyName -> constructor. Populated explicitly
// (no init()) by newLoopStrategyRegistry, matching the providerTiers pattern
// (agent.go composeProviderTiers).
type LoopStrategyRegistry struct {
    builders map[LoopStrategyName]LoopStrategyBuilder
}

type LoopStrategyBuilder func(a *Agent) LoopStrategy

func NewLoopStrategyRegistry() *LoopStrategyRegistry {
    return &LoopStrategyRegistry{builders: make(map[LoopStrategyName]LoopStrategyBuilder)}
}

func (r *LoopStrategyRegistry) Register(name LoopStrategyName, b LoopStrategyBuilder) {
    r.builders[name] = b
}

// Build returns the strategy for name, or an error when no builder is registered.
func (r *LoopStrategyRegistry) Build(name LoopStrategyName, a *Agent) (LoopStrategy, error) {
    b, ok := r.builders[name]
    if !ok {
        return nil, fmt.Errorf("loop strategy not registered: %s", name)
    }
    return b(a), nil
}

// loopStrategies is the shared registry, built once. Treat as read-only.
var loopStrategies = newLoopStrategyRegistry()

func newLoopStrategyRegistry() *LoopStrategyRegistry {
    r := NewLoopStrategyRegistry()
    r.Register(LoopReact, func(a *Agent) LoopStrategy { return newReactStrategy(a) })
    r.Register(LoopPlanExecute, func(a *Agent) LoopStrategy { return newPlanExecuteStrategy(a) })
    r.Register(LoopEvaluatorOptimizer, func(a *Agent) LoopStrategy { return newEvaluatorOptimizerStrategy(a) })
    return r
}
```

Centralization: `newLoopStrategyRegistry()` is the **single** place strategies are
registered. A new technique = one strategy file + one `Register` line + one enum constant.

Add `RegisteredLoopStrategyNames() []string` (sorted, derived from the registry) so the
admin config view can surface the valid set to the UI. The dropdown is **backend-driven**
(§6, §7) — the frontend never hardcodes the option list, so adding a strategy needs no
frontend edit.

### 3.3 Resolver (choosing) — `internal/core/assistant/loop_resolver.go` (new)

```go
package assistant

// resolveLoopStrategy selects and builds the strategy for one Agent.
func resolveLoopStrategy(a *Agent) LoopStrategy {
    name := resolveLoopStrategyName(a)
    s, err := loopStrategies.Build(name, a)
    if err != nil {
        a.deps.Logger.Error("loop strategy unavailable, falling back to react",
            "strategy", name, "error", err)
        s, _ = loopStrategies.Build(defaultLoopStrategy, a)
    }
    return s
}

// resolveLoopStrategyName applies the deterministic precedence:
// explicit per-model config > provider/workload default > react.
func resolveLoopStrategyName(a *Agent) LoopStrategyName {
    if a.config.LoopStrategy != "" {
        return a.config.LoopStrategy
    }
    if d := providerDefaultStrategy(a.config.ProviderType, a.config.WorkloadClass); d != "" {
        return d
    }
    return defaultLoopStrategy
}

// providerDefaultStrategy returns the provider-tier default loop strategy.
// Intentionally static (react) until Phase N provides measured per-provider data.
// Do NOT populate a table now — an unmeasured loop-shape default is higher risk
// than temperature/max_tokens (it changes the whole execution shape).
func providerDefaultStrategy(providerType string, wc models.WorkloadClass) LoopStrategyName {
    return defaultLoopStrategy
}
```

### 3.4 Dispatch — `runSession.run()` refactor (`session.go:402`)

The loop body moves out verbatim; `run()` shrinks to resolve-and-dispatch:

```go
func (s *runSession) run() (string, []proxy.Message, error) {
    s.agent.runS = s
    defer func() { s.agent.runS = nil }()

    strategy := resolveLoopStrategy(s.agent)
    return strategy.Run(s.ctx, s)
}
```

The `runS` back-pointer setup/teardown stays in `run()` (exactly once per Execute), so a
strategy that internally delegates to another strategy (e.g. `PlanExecuteStrategy` falling
back to `ReactStrategy`) does **not** re-enter this setup.

---

## 4. Strategies

### 4.1 `ReactStrategy` — `internal/core/assistant/react_strategy.go` (new)

The current `for { ... }` body of `runSession.run()` (`session.go:407–459`), moved verbatim:

- `s.steps++`, `ctx.Err()` check, `checkForcedCompletion`, advisory step-limit warn,
  `maybeFlushMemoryBeforeTurn`, `notifyStepStart`, `notifyThinking`,
  `executeTurn` → `handleTurnError` / `handleToolTurn` / `handleTextTurn`,
  `sieveStreak = 0` reset.

```go
type ReactStrategy struct {
    agent      *Agent
    stopGuards []StopGuard // nil for plain react
}

func newReactStrategy(a *Agent) *ReactStrategy { return &ReactStrategy{agent: a} }

func (r *ReactStrategy) Name() LoopStrategyName { return LoopReact }

func (r *ReactStrategy) Run(ctx context.Context, s *runSession) (string, []proxy.Message, error) {
    s.stopGuards = r.stopGuards // nil for plain react; enables the handleTextTurn hook
    // ... moved loop body, verbatim ...
}
```

**Stop-guard hook** (the only change to the extracted loop): the hook fires at the
**natural-completion branch** of `handleTextTurn` — the one place a successful run
finalizes — *before* `completeWith` emits the `"completed"` lifecycle. It does **not**
fire on forced completion (`checkForcedCompletion`), fallback answers
(`handleNoToolCalls` terminal paths), or error/stall returns (`handleTurnError`), so the
hard cap and fatal paths cannot be nudged past and `"completed"` is emitted exactly once.
Putting the hook at the `done == true` return points of the loop body is wrong: by then
`completeWith` has already fired, and the hard-cap/error paths would be intercepted too.

The hook lives in the shared turn primitive, not the loop body, keyed off the session:

```go
// runSession gains (both nil/zero by default):
stopGuards        []StopGuard
stopGuardAttempts int

// maybeNudge returns the first guard nudge, bounded by stopGuardAttempts (cap 2).
func (s *runSession) maybeNudge() (*proxy.Message, bool) { /* ... */ }
```

`ReactStrategy.Run` sets `s.stopGuards = r.stopGuards` before entering the loop (plain
react leaves it nil). `handleTextTurn`'s natural-completion branch becomes:

```go
if content, ok := checkTaskCompletion(turnMsg, s.history); ok {
    s.history = append(s.history, turnMsg)
    s.agent.notify(EventMessage, turnMsg)
    if nudge, ok := s.maybeNudge(); ok {
        s.history = append(s.history, proxy.Message{Role: proxy.UserRole, Content: nudge.Content})
        return false, "", nil // continue the loop, not done
    }
    reply, _, completeErr := s.completeWith(content)
    return true, reply, completeErr
}
```

`maybeNudge` iterates the guards bounded by the **dedicated** `s.stopGuardAttempts`
counter (cap `2`) — **not** `s.finalizeAttempts`, which `handleNoToolCalls` already owns
for its tools-disabled finalization turn. With `stopGuards == nil` (plain react),
`maybeNudge` always returns `(nil, false)` — **zero behavior change**. The loop body
itself moves verbatim.

### 4.2 `PlanExecuteStrategy` — `internal/core/assistant/plan_execute_strategy.go` (new)

Wraps the existing `ExecutionPlanStrategy.Generate` (`tool_exec.go:52`) and
`Agent.executePlan` (`tool_exec.go:286`), with an explicit fallback to the react loop.
Replaces the short-circuit currently in `Execute()` (`agent.go:581–599`).

```go
type PlanExecuteStrategy struct {
    agent    *Agent
    fallback *ReactStrategy
}

func newPlanExecuteStrategy(a *Agent) *PlanExecuteStrategy {
    return &PlanExecuteStrategy{agent: a, fallback: newReactStrategy(a)}
}

func (s *PlanExecuteStrategy) Name() LoopStrategyName { return LoopPlanExecute }

func (s *PlanExecuteStrategy) Run(ctx context.Context, run *runSession) (string, []proxy.Message, error) {
    a := s.agent
    lastUserMsg := lastUserMessage(run.history)
    if lastUserMsg == "" {
        return s.fallback.Run(ctx, run)
    }
    tools, err := a.deps.Provider.ListTools(ctx)
    if err != nil || len(tools) == 0 {
        return s.fallback.Run(ctx, run)
    }
    plan, err := NewExecutionPlanStrategy(a.deps.Client, tools, a.deps.Logger).Generate(ctx, lastUserMsg)
    if err != nil {
        a.deps.Logger.Warn("plan generation failed, falling back to react loop", "error", err)
        return s.fallback.Run(ctx, run)
    }
    return a.executePlan(ctx, run.history, plan)
}
```

`lastUserMessage(history)` is the existing scan extracted from `Execute()` (scan
`history` backward for `proxy.UserRole`). Reuse it — do not duplicate the loop.

Note: `executePlan` currently returns `"[Plan execution complete]"` **without** emitting
the `"completed"` lifecycle that `completeWith` provides. Per §9, plan-execute must
finalize through the shared completion path (or explicitly document the divergence in
SPEC-010) so the completion signal is uniform across strategies.

### 4.3 `EvaluatorOptimizerStrategy` + `StopGuard` — `internal/core/assistant/stop_guard.go` and `evaluator_optimizer_strategy.go` (new)

```go
// stop_guard.go
package assistant

// StopGuard is a bounded policy hook that may refuse to let the loop finalize,
// returning a synthetic follow-up (user-role nudge) to continue the run.
type StopGuard interface {
    // Nudge returns a nudge message to inject, or nil to allow finalization.
    Nudge(s *runSession) (*proxy.Message, error)
}
```

```go
// evaluator_optimizer_strategy.go
package assistant

// EvaluatorOptimizerStrategy is the react loop plus a bounded generator/evaluator
// pass: before the run finalizes, an evaluator guard injects a self-critique
// nudge so the model verifies/fixes its work instead of finishing prematurely.
type EvaluatorOptimizerStrategy struct {
    inner *ReactStrategy
}

func newEvaluatorOptimizerStrategy(a *Agent) *EvaluatorOptimizerStrategy {
    return &EvaluatorOptimizerStrategy{
        inner: &ReactStrategy{agent: a, stopGuards: []StopGuard{newEvaluatorGuard(a)}},
    }
}

func (s *EvaluatorOptimizerStrategy) Name() LoopStrategyName { return LoopEvaluatorOptimizer }

func (s *EvaluatorOptimizerStrategy) Run(ctx context.Context, run *runSession) (string, []proxy.Message, error) {
    return s.inner.Run(ctx, run)
}
```

`EvaluatorGuard` returns a bounded nudge prompt (new template in
`prompts/templates.go`, e.g. `EvaluatorReviewPrompt`): "Review the work so far — run any
build/test/verification, fix issues found, then summarize." The guard is **prompt-based
self-critique only** — it does not implement Hermes's verification-evidence ledger
(`verification_evidence.py`). A ledger-backed `VerifyOnStopGuard` is deferred (§12).

Uses a **dedicated** `s.stopGuardAttempts` counter (cap `2`), **not**
`s.finalizeAttempts` — the latter is owned by `handleNoToolCalls`'s tools-disabled
finalization turn and sharing it would corrupt the empty-turn recovery ladder. The guard
must never nag perpetually (same one-shot-nag principle as `AutomationNagPrompt` in
SPEC-001 §6).

---

## 5. Config & Data Flow

### 5.1 `models/config.go`

Replace the plan flag with the enum (single source of truth):

```go
// Agent tuning — per-model overrides for agent loop behaviour.
// Zero values mean "use the global default."
...
// LoopStrategy selects the agent loop archetype. "" = provider default / react.
LoopStrategy string `json:"loop_strategy,omitempty"`
```

**Delete** `EnableExecutionPlan bool` (`models/config.go:394`) outright and replace it
with `LoopStrategy` — no compatibility shim. Still in dev; no migration needed. It is a
backend-only field (never surfaced in the add/update form or admin view), so the only
possible reference is a hand-edited settings.yml key, which is simply dropped.

### 5.2 `AgentOptions.ApplyModelConfig` (`agent.go:430`)

- Change return type from `bool` to `void` (the bool existed only to signal
  "set `PlanStrategy` after this call" — that coupling is deleted).
- Add:
  ```go
  if cfg.LoopStrategy != "" {
      o.LoopStrategy = ParseLoopStrategy(cfg.LoopStrategy)
  }
  ```
  No `EnableExecutionPlan` alias — the field is deleted outright (§5.1).

### 5.3 `AgentOptions` (`agent.go:260`) and `AgentConfig` (`agent.go:170`)

- Add to `AgentOptions`: `LoopStrategy LoopStrategyName` (empty = unset).
- Add to `AgentConfig`: `LoopStrategy LoopStrategyName`.
- Remove from `AgentOptions`: `PlanStrategy *ExecutionPlanStrategy` (`agent.go:280`).
- Remove from `AgentRuntimeDeps`: `PlanStrategy *ExecutionPlanStrategy` (`agent.go:212`).

### 5.4 `NewAgent` (`agent.go:486`)

- Drop `PlanStrategy: opts.PlanStrategy` from the `AgentRuntimeDeps` literal.
- Add `LoopStrategy: opts.LoopStrategy` to the `AgentConfig` literal.

### 5.5 `Agent.Execute` (`agent.go:567`)

Remove the entire plan short-circuit block (`agent.go:579–599`) and the now-unused
`lastUserMsg` scan. `Execute` becomes uniform:

```go
func (a *Agent) Execute(ctx context.Context, history []proxy.Message) (string, []proxy.Message, error) {
    execCtx, cancel := context.WithTimeout(ctx, a.config.GlobalTimeout)
    defer cancel()
    a.startWatchdog(execCtx, cancel)
    execCtx = WithUsageTracker(execCtx)
    a.rebuildToolCache(execCtx)

    s := newRunSession(a, execCtx, history)
    return s.run()
}
```

The `lastUserMessage` helper (extracted in §4.2) lives in `plan_execute_strategy.go`
(or `session.go`) — one home, reused by `PlanExecuteStrategy`.

### 5.6 `AgentBuilder` (`agent_builder.go:93–105`)

`WithModelConfig` drops the `if b.opts.ApplyModelConfig(cfg) { ... NewExecutionPlanStrategy ... }`
block:

```go
func (b *AgentBuilder) WithModelConfig(ctx context.Context, modelName string, tools ToolProvider, client proxy.Client) *AgentBuilder {
    cfg, ok := b.svc.ModelConfig(modelName)
    if !ok {
        return b
    }
    b.opts.ApplyModelConfig(cfg)
    return b
}
```

Note: the `ctx`/`tools`/`client` params become unused for plan wiring — keep the method
signature (other callers use `ModelConfig`) unless a caller is orphaned; verify before
removing params (do not leave unused params without checking all call sites).

### 5.7 `automation/executor.go` (`buildAgentOptions`, line 276)

Drop the `if opts.ApplyModelConfig(cfg) { ... NewExecutionPlanStrategy ... }` block
(`executor.go:313–319`) and call `opts.ApplyModelConfig(cfg)` directly.

### 5.8 Settings.yml persistence + runtime merge (round-trip)

`loop_strategy` is an agent-tuning override and must survive restart like the other
tuning fields (Constitution III.5). The §6 checklist covers the HTTP/UI surface but not
the settings.yml carrier and the runtime merge — both are required, or a `loop_strategy`
chosen in the Agent Tuning grid is never persisted and silently reverts to `react` on
restart:

- `models/infrastructure.go` — `ModelOverride`: add
  `LoopStrategy string \`yaml:"loop_strategy,omitempty" json:"loop_strategy,omitempty"\``.
- `internal/transport/http/handlers/model_handlers.go` — `hasModelOverrides` (`:406`): add
  `|| cfg.LoopStrategy != ""`; `writeModelOverrides` (`:455`): add
  `LoopStrategy: cfg.LoopStrategy`.
- `internal/core/llm/manager.go` — `ApplyModelOverrides` (`:567`): add
  `if override.LoopStrategy != "" { cfg.LoopStrategy = override.LoopStrategy }`.

---

## 6. Config Plumbing to the UI (model-field checklist)

Per architecture.md "File Change Checklist" (adding a model-level field) and pitfall #5:

1. `models/config.go` — `LoopStrategy` field (§5.1).
2. `internal/transport/http/handlers/model_handlers.go` — `modelFormRequest`
   (`model_handlers.go:130`): add `LoopStrategy string \`json:"loop_strategy"\``.
   Map `cfg.LoopStrategy` → request in the read path and `req.LoopStrategy` → config in
   the write path (mirror the existing `MaxPlanSteps` handling at lines 470/509).
3. `internal/transport/http/handlers/admin_handlers.go` — `adminModelView`
   (`admin_handlers.go:92`): add `LoopStrategy string \`json:"loop_strategy"\``; add the
   same to `adminTuningDefaults` (`admin_handlers.go:146`) and `baseAdminTuningDefaults`
   (`admin_handlers.go:388`) with value `""` (empty = react default). Also add to
   `adminConfigView` (`admin_handlers.go:177`, built at `:307`):
   `LoopStrategyOptions []string \`json:"loop_strategy_options"\``, populated from
   `assistant.RegisteredLoopStrategyNames()` — this is the single source of the dropdown's
   option list (§7).
4. `internal/transport/http/handlers/admin_view.go` — extend `modelViewTuning`
   (`admin_view.go:16`) to return `loopStrategy` (default `""` = react) and copy it into
   the `adminModelView` in `getModelsView` (`admin_view.go:70`, mapping at `:98`).
5. `internal/testing/mocks/manager.go` — only if a `RuntimeManager` method changes
   (it does **not** here — `ModelConfig` is a struct, not an interface). Skip.
6. Frontend (§7).
7. `models/infrastructure.go` — `ModelOverride.LoopStrategy` (settings.yml carrier) — §5.8.
8. `internal/core/llm/manager.go` — `ApplyModelOverrides` merge of `loop_strategy` — §5.8.
9. `internal/core/assistant/session.go` — `isAgentControlMessage` allowlist (`:345`): add
   `prompts.EvaluatorReviewPrompt`. Evaluator nudges are control messages; without this
   entry `previousConversationMessage` / `bestAvailableAnswer` / `countConsecutiveChat`
   mistake them for user text and corrupt completion detection.

Boundary validation: in `model_handlers.go` (and the automation handler, §8 Phase 4),
reject an unknown non-empty `loop_strategy` with `400` + a clear message listing valid
values. Add a shared helper (e.g. `assistant.LoopStrategyName.Valid()`) so validation is
not reimplemented in two handlers.

---

## 7. Frontend

Follow `frontend-vue-engineer.md` (no `any`, named `const`s for option values, type
imports, tests in `src/__TESTS__/` mirroring the source tree).

1. `src/types/model.ts` — add `loop_strategy?: string` to `TuningFields` (`:29`),
   `Model` (`:47`), and `NewModelForm` (`:119`).
2. `src/types/admin.ts` — add `loop_strategy: string` to `AgentDefaults` (`:123`), and
   `loop_strategy_options?: string[]` to `GlobalConfig` (`:144`, maps to
   `adminConfigView.loop_strategy_options`). The model *view* type is `Model` in
   `model.ts` (item 1), not in `admin.ts`.
3. `src/utils/model/modelUtils.ts` — add `loop_strategy: string` to the `ModelForm`
   interface (`:4`); add `loop_strategy: defaults.loop_strategy ?? ""` to
   `getDefaultModelSettings` (`:67`) and its return-type annotation. `createEmptyModelForm`
   spreads `...tuning`, so it inherits the field automatically. Add the **copy map** — the
   only frontend-owned part (labels + help text keyed by value; the option *list* itself
   comes from the backend, §6 item 3):
   ```ts
   export const LOOP_STRATEGY_COPY = {
     react: {
       label: "ReAct (auto)",
       description: "Classic Thought → Action → Observation loop. The model decides each next step and stops when it produces a final report. Simplest, most flexible; best default for most tasks.",
     },
     plan_execute: {
       label: "Plan & Execute",
       description: "The model writes a step-by-step tool plan first, then executes each step in order. More predictable ordering for multi-step tasks; falls back to ReAct if planning fails.",
     },
     evaluator_optimizer: {
       label: "Evaluator-Optimizer",
       description: "ReAct loop plus a self-review pass: before finalizing, the model is prompted to verify/fix its work (up to 2 rounds). Slower but higher quality when verification matters.",
     },
   } as const
   ```
   and two pure helpers: `loopStrategyOptions(available?: string[])` builds the option
   list from the **backend-surfaced** names (falls back to the three known values when the
   list is empty; unknown values get the raw value as label with no description) and
   `loopStrategyDescription(value)` returns the selected option's description (empty →
   `react`'s). Both unit-testable; the component stays dumb.
4. `frontend/src/composables/settings/useProviderModels.ts` + `ProviderModelsCard.vue` —
   `useProviderModels` exposes a `loopStrategyOptions` computed (from
   `config.loop_strategy_options` via the util); `ProviderModelsCard` passes it as a prop
   to `ModelTuningFields` at all four call sites (`:118/:202/:240/:277`).
5. `src/components/settings/ModelTuningFields.vue` — new optional prop
   `loopStrategyOptions: { value: string; label: string; description: string }[]`; render
   the `<select>` bound to `model.loop_strategy` with `<option v-for>`, placed next to
   `tool_call_format` (line 154); empty value shows "Provider default (ReAct)". Two help
   surfaces (native `<option>` cannot show hover tooltips, so per-strategy guidance is a
   dynamic line, not per-option popovers):
   - **Label tooltip** — `<InfoTooltip>` next to the "Loop Strategy" label explaining the
     concept: "Controls how the agent loops: ReAct (react), plan-first execution, or a
     self-review (evaluator-optimizer) loop. Empty = provider default (ReAct)."
   - **Selected-option helper line** — a `<p class="form-helper">` under the select
     showing `loopStrategyDescription(model.loop_strategy)` via a small `computed`. Reuses
     the existing `form-helper` styling already in this file; no new component.
6. `npm test` + `npm run build`.

---

## 8. Phased Implementation (TDD — red → green → refactor)

Baseline first (must be green before any change):
`cd backend && go build ./... && go test ./...` and
`cd frontend && npm test && npm run build`.

### Phase 1 — Engine + React extraction + Plan-Execute promotion (backend)

1. Add `loop_strategy.go` (interface, name, registry), `react_strategy.go`,
   `plan_execute_strategy.go`, `loop_resolver.go`.
2. Move the loop body from `runSession.run()` into `ReactStrategy.Run`; shrink `run()`
   to resolve-and-dispatch (§3.4). No logic change.
3. Remove `PlanStrategy` from `AgentOptions`/`AgentRuntimeDeps`; add `LoopStrategy`
   config fields; update `NewAgent`/`ApplyModelConfig`/`Execute` (§5).
4. Delete `EnableExecutionPlan` from `ModelConfig` (§5.1) — no alias.
5. Update `agent_builder.go` + `executor.go` (§5.6, §5.7).
6. Update `agent_test.go` (the ~line 2837 `PlanStrategy` construction) and any other
   `PlanStrategy` references → `opts.LoopStrategy = assistant.LoopPlanExecute`.

**Tests:** existing `agent_test.go` (166 KB) stays green unchanged (react is the default
and reproduces the current loop). Add:
- `loop_strategy_test.go` — `ParseLoopStrategy` (empty/unknown → react, valid passthrough),
  `Valid()` truth table, registry `Build` (registered vs unregistered), resolver precedence
  (explicit > provider default > react).
- `plan_execute_strategy_test.go` — generation success → plan executed; generation failure
  → react fallback; no tools → fallback; no user message → fallback. (These mirror the
  existing `strategy_plan_test.go` cases, now asserted through the strategy, not `Execute`.)
- `react_strategy_test.go` — regression: `ReactStrategy` reproduces the loop (an
  end-to-end mock run asserts the same turn sequence as the pre-refactor behavior).

### Phase 2 — Config plumbing + frontend dropdown (§6, §7)

Backend struct wiring (`ModelOverride`/`ApplyModelOverrides` round-trip + boundary
validation + surface `loop_strategy_options` from the registry), then the Vue dropdown
driven by the backend list + copy map + tests.

### Phase 3 — Stop-guard pipeline + Evaluator-Optimizer (§4.3)

`stop_guard.go`, `evaluator_guard.go`, `evaluator_optimizer_strategy.go`, new prompt
template in `prompts/templates.go`, registry entry.

**Tests:** `evaluator_optimizer_strategy_test.go` — completion candidate triggers one
evaluator nudge then continues; second nudge caps at `stopGuardAttempts` and finalizes;
plain react (no guards) finalizes immediately; guard never fires on empty/error paths and
`"completed"` is emitted exactly once.

### Phase 4 — Per-run override (automation, backend)

`models/workspace.go` `Automation` struct (`workspace.go:44`): add
`LoopStrategy string \`yaml:"loop_strategy,omitempty" json:"loop_strategy,omitempty"\``.
Thread it through the full automation chain:

1. `internal/core/automation/registry.go` — `AutomationEntry` (`:12`): add
   `LoopStrategy string`; set it in `Register` (`:50`).
2. `internal/core/automation/dispatcher.go` — `executeAutomation` (`:544`): copy
   `entry.LoopStrategy` into `ExecuteRequest.LoopStrategy`.
3. `internal/core/automation/executor.go` — `ExecuteRequest` (`:42`): add
   `LoopStrategy string`; `buildAgentOptions` (`:276`): apply **after** `ApplyModelConfig`
   so the per-run value wins:

```go
opts.ApplyModelConfig(cfg)
if req.LoopStrategy != "" {
    opts.LoopStrategy = assistant.ParseLoopStrategy(req.LoopStrategy)
}
```

Validate in the automation/template handler (reject unknown with 400). Frontend automation
edit-form field is **optional/deferred** (note in §12).

### Checks after each phase

`cd backend && go build ./... && go test ./... && go run ./tools/check-complexity/`
(complexity ≤ 12) and `cd frontend && npm test && npm run build` when UI touched.
Run `go test -race ./internal/core/assistant/` in Phase 1 (loop dispatch touched).

---

## 9. Invariants (must hold in every strategy)

Each strategy must preserve the SPEC-001 contracts, none of which are loosened:

- **II.4/II.5** XML + native dual path; XML parser stays as fallback (pitfall #7).
- **II.6** sieve — strategies reuse `s` sieve primitives; never reimplement.
- **II.7** natural completion — plain-text answer with a prior tool result is the
  canonical completion; `tool_choice` is not forced (pitfall #8).
- **II.10** guardrail decision flow — `executeSingleToolStep`/`executePlan` are shared and
  unchanged; fan-out is deferred so no new parallel-approval path is introduced now.
- **`ctx` first, no ctx in structs** (go rule); every blocking call observes `ctx.Done()`.
- **Goroutine lifecycle** — `run()` keeps the single `runS` setup/teardown; no new
  goroutines in Phase 1–4.
- **Prompt centralization (II.13)** — the evaluator nudge is a named constant in
  `prompts/templates.go`, never an inline string.
- **No `init()`** — registry built via a plain package `var` (mirrors `providerTiers`).
- **Centralization** — one registry (`newLoopStrategyRegistry`), one resolver, one
  selection precedence. No `if PlanStrategy != nil` branches remain anywhere.
- **Completion lifecycle** — every strategy emits the `"completed"` lifecycle exactly
  once on success, via the shared `completeWith` path. A strategy that finalizes without
  `completeWith` (currently `executePlan`, which returns `"[Plan execution complete]"`)
  must be routed through the same completion path so the SSE/EventBus contract is uniform.
- **Stop-guard scope** — guards fire only on *successful natural completion*
  (`checkTaskCompletion` branch, `err == nil`), never on forced completion
  (`checkForcedCompletion`), fallback answers (`handleNoToolCalls` terminal paths), or
  error/stall returns (`handleTurnError`). Guards use a dedicated `stopGuardAttempts`
  counter, never `finalizeAttempts`.
- **Errors** — wrap with `%w`; no `_ =` without a comment; no silent fallback (log on
  unknown-strategy fallback).

---

## 10. Tests (summary)

| File | Covers |
|---|---|
| `loop_strategy_test.go` (new) | name parse/validate, registry build, resolver precedence |
| `react_strategy_test.go` (new) | refactor regression — identical turn sequence |
| `plan_execute_strategy_test.go` (new) | plan success / fallback paths through the strategy |
| `evaluator_optimizer_strategy_test.go` (new) | guard nudge + `stopGuardAttempts` cap + plain-react no-nudge + `"completed"` emitted exactly once |
| `stop_guard_test.go` (new) | guard nil/first-nudge/bounded behavior + scope: no nudge on hard-cap / fallback-answer / error-stall paths; `finalizeAttempts` untouched |
| `agent_test.go` (existing) | unchanged — proves react default is non-breaking |
| `strategy_plan_test.go` (existing) | unchanged — `ExecutionPlanStrategy.Generate` still tested |
| `model_handlers` / `admin` tests | `loop_strategy` round-trip + 400 on unknown value + `loop_strategy_options` surfaced from registry |
| `modelUtils.test.ts` | `loopStrategyOptions(available)` (empty→fallback, unknown→raw label, known→copy) + `loopStrategyDescription` |

---

## 11. Files Changed (complete list)

```
models/config.go                                    (LoopStrategy in; EnableExecutionPlan deleted)
models/infrastructure.go                            (ModelOverride.LoopStrategy — §5.8)
models/workspace.go                                 (Automation.LoopStrategy — Phase 4)
internal/core/assistant/loop_strategy.go            (new — name, interface, registry, RegisteredLoopStrategyNames)
internal/core/assistant/loop_resolver.go            (new — resolveLoopStrategy/Name, providerDefaultStrategy)
internal/core/assistant/react_strategy.go           (new — extracted loop body; sets s.stopGuards)
internal/core/assistant/plan_execute_strategy.go    (new — plan driver + react fallback + completion path)
internal/core/assistant/stop_guard.go               (new — StopGuard interface, Phase 3)
internal/core/assistant/evaluator_guard.go          (new — EvaluatorGuard, Phase 3)
internal/core/assistant/evaluator_optimizer_strategy.go (new — Phase 3)
internal/core/assistant/agent.go                    (AgentOptions/AgentConfig fields; ApplyModelConfig; NewAgent; Execute)
internal/core/assistant/agent_builder.go            (drop PlanStrategy wiring)
internal/core/assistant/session.go                  (run() → dispatch; lastUserMessage helper; stopGuards/stopGuardAttempts + maybeNudge; isAgentControlMessage allowlist)
internal/core/assistant/tool_exec.go                (unchanged — ExecutionPlanStrategy + executePlan reused)
internal/core/assistant/prompts/templates.go        (EvaluatorReviewPrompt, Phase 3)
internal/core/automation/executor.go                (drop PlanStrategy wiring; per-run override Phase 4)
internal/core/automation/registry.go                (AutomationEntry.LoopStrategy — Phase 4)
internal/core/automation/dispatcher.go              (copy entry.LoopStrategy → ExecuteRequest — Phase 4)
internal/core/llm/manager.go                        (ApplyModelOverrides merge of loop_strategy — §5.8)
internal/transport/http/handlers/model_handlers.go  (form field + validation + hasModelOverrides/writeModelOverrides)
internal/transport/http/handlers/admin_handlers.go  (adminModelView + adminTuningDefaults + loop_strategy_options)
internal/transport/http/handlers/admin_view.go      (view mapping + modelViewTuning)
internal/transport/http/handlers/admin_template_handlers.go (automation form validation — Phase 4)
internal/core/assistant/agent_test.go               (migrate PlanStrategy test reference)

frontend/src/types/model.ts
frontend/src/types/admin.ts                           (+ loop_strategy_options on GlobalConfig)
frontend/src/utils/model/modelUtils.ts                 (+ LOOP_STRATEGY_COPY + loopStrategyOptions/loopStrategyDescription)
frontend/src/composables/settings/useProviderModels.ts (+ loopStrategyOptions computed)
frontend/src/components/settings/ProviderModelsCard.vue (+ pass loopStrategyOptions prop, 4 sites)
frontend/src/components/settings/ModelTuningFields.vue
frontend/src/__TESTS__/utils/model/modelUtils.test.ts     (loopStrategyOptions + loopStrategyDescription + defaults)
frontend/src/__TESTS__/composables/models/modelBanner.test.ts (update fixture object with loop_strategy)
```

**Not changed:** `tool_exec.go` (plan generator/executor are reused as-is),
`strategy_plan_test.go` (still valid), `internal/testing/mocks/manager.go` (no interface
change).

---

## 12. What We Skip (dead-code / scope prevention)

| Deferred | Why | Unblocks when |
|---|---|---|
| `map_reduce`, `auditor`, `human_in_the_loop`, `orchestrator_workers` enum values | no implementation → would be dead code / reserved-but-unwired | one strategy file + one `Register` + one dropdown each |
| `iterationBudget` type | no consumer until fan-out/subagents → dead code now | first orchestrator/map-reduce phase |
| Per-provider `providerDefaultStrategy` table | no measured data → speculative default | after Phase 1–3 ships + an A/B or recorded evidence |
| Hermes-style verification-evidence ledger (`VerifyOnStopGuard`) | requires a verification ledger llm-proxy lacks | separate plan (cross-cutting) |
| Automation edit-form UI for `loop_strategy` | backend override is enough initially | follow-up UI phase |
| MoA / Mixture-of-Agents | separate multi-model effort (redaction + per-model cost) | Tier 3 |
| Slash-command opt-in | client-side concern, not proxy-owned | never (by design) |
| `enable_execution_plan` settings.yml key | deleted field — any hand-edited key is dropped (dev, no migration) | now (in Phase 1) |

---

## 13. Risks & Sequencing

- **Regression surface is `agent_test.go` (166 KB) + record-replay fixtures.** Phase 1 is a
  pure extraction; react is the default so the whole suite is the regression harness.
  Run `go test ./...` and `go test -race ./internal/core/assistant/` before/after.
- **`fix-final-report-realignment.md` (active, SPEC-001).** The evaluator-optimizer
  finalize hook is additive and must not regress the canonical empty-finalization fix.
  Sequence: land Phases 1–2 first; Phase 3's hook is inserted at the `handleTextTurn`
  natural-completion branch (before `completeWith`), using a dedicated `stopGuardAttempts`
  counter — verify no double-completion path and that `finalizeAttempts` is untouched. If
  the realignment plan lands first, rebase this hook on top of its finalization logic.
- **`applyDefaults` / `ApplyModelConfig` callers** — the bool→void signature change is
  compile-enforced; the two callers (`agent_builder.go`, `executor.go`) are updated in the
  same commit. Grep for `ApplyModelConfig` to confirm no third caller.
- **Config migration** — none. Still in dev: `enable_execution_plan` is deleted outright;
  any hand-edited key is ignored. No compat path to maintain or test.

---

## 14. SPEC-010 (prerequisite deliverable)

Before implementation, add `docs/SPECS/agent-loop-strategies.md` as **SPEC-010** (status
`draft`), with sections Intent / Functional Requirements / Data Model / Behavior /
Lifecycle & Completion / Error Handling,
`constitution_references: [II.4, II.5, II.6, II.7, II.10, II.13]`,
`related_specs: [SPEC-001, SPEC-005]`, and update `docs/INDEX.md` (SPEC table + plan table)
and `docs/PLANS/README.md` (add this plan row). SPEC-010 is the behavioral contract this
plan implements; SPEC-001 §I gets a one-line cross-reference to it (non-breaking).

---

## 15. Definition of Done

- React strategy reproduces the pre-refactor loop (full existing suite green, `-race` clean).
- Plan-execute and evaluator-optimizer selectable per model; default `react`; unknown
  values rejected at the boundary (400) and fallen back at the resolver (logged).
- `EnableExecutionPlan` is deleted (no alias, no `// Deprecated` shim); no other plan
  wiring remains (`grep -r PlanStrategy` and `grep -r EnableExecutionPlan` return nothing
  in production code).
- `loop_strategy` survives a restart (UI save → `ModelOverride` → `ApplyModelOverrides`
  round-trip); evaluator nudges are registered as control messages in
  `isAgentControlMessage`; `"completed"` is emitted exactly once per run; stop-guards never
  fire on forced-completion or error/stall paths.
- The dropdown is backend-driven (`loop_strategy_options` from the registry): registering a
  new strategy requires **no frontend edit** (frontend copy map has a raw-value fallback).
- SPEC-010 drafted + INDEX/PLANS README updated.
- Complexity ≤ 12; `go build ./...` / `go test ./...` / `go run ./tools/check-complexity/`
  and `npm test` / `npm run build` all pass.
