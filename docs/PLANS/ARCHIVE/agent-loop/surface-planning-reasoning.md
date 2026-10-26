---
status: complete
date: 2026-08-18
related_specs: [SPEC-010, SPEC-001, SPEC-003]
---

# Plan: Surface Plan-Generation Reasoning (stream the planner)

## 0. TL;DR

Plan-and-execute is the only loop strategy that hides the LLM's reasoning: its
plan-generation call is a non-streaming `Chat` (tool_exec.go:73), so the planner's
`reasoning_content` is discarded and the UI shows a synthetic "Generating execution
plan…" indicator instead. React and evaluator-optimizer already surface reasoning
because every turn streams through the shared `executeTurn` path. Industry-standard
plan-capable agents (Claude Code plan mode, ChatGPT/Codex, Gemini CLI, Cursor/Windsurf,
OpenHands, Aider) surface thinking during planning.

Fix **one shared primitive**, not one strategy: make the pre-loop `generatePlan`
primitive (SPEC-010 §IV.2) stream and relay reasoning deltas, and extract the reasoning
wire-param application into a shared request-config helper. Every present and future
loop strategy that plans — today `plan_execute`, tomorrow map-reduce / orchestrator-
workers — gets reasoning visibility **with zero per-strategy code**. No frontend
changes (`EventReasoning` is strategy-agnostic).

## 1. Root cause (verified)

`runSession.generatePlan` (session.go:509-518) emits `agent_thinking` + the
"Generating execution plan…" message, then calls
`ExecutionPlanStrategy.Generate` (tool_exec.go:52-77), which:

- builds a bare `proxy.ChatRequest` (system plan prompt + user prompt, `MaxTokens =
  planGenMaxTokens`) with **no temperature and no reasoning params** (the
  `reasoning.NewReasoningResolver(...).Apply(&req, spec)` block that `buildChatRequest`
  applies — stream.go:81-94 — is absent here), and
- calls `s.llm.Chat(ctx, req)` — **non-streaming** — so any `reasoning_content` the
  model emits is discarded; only `Choices[0].Message.Content` (the plan JSON) is used.

Result: during the plan-generation LLM call (which can take 60–100 s on slow reasoning
models, per `docs/audits/known-performance-findings.md` §1), the UI shows a static
"planning" state with no reasoning and no liveness signal.

## 2. Behavior map across strategies (why the fix belongs in the primitive)

| Strategy | LLM touches | Reasoning surfaced today | Needs change? |
|---|---|---|---|
| `react` | streams every turn (`executeTurn`) | ✅ per turn | no |
| `plan_execute` | plan-gen (`Chat`), tools, finalize (`Stream`) | ❌ plan-gen only | **yes** |
| `evaluator_optimizer` | wraps react + one self-review turn | ✅ like react, + one extra pass | no |

All three already compose the shared turn primitives (SPEC-010 §II.4). The single
divergent path is `generatePlan`. Fixing `generatePlan` therefore unifies the behavior
across every present and future strategy — this is the "no custom logic per strategy"
argument: the capability lives in the primitive, strategies just compose it.

Frontend: `EventReasoning` → `liveReasoning` → reasoning inset is generic
(`useTurnInset.ts`, `useLiveConsole.ts:127`); not gated on strategy or lifecycle phase.
No frontend change required.

## 3. Design

### 3.1 Shared request-config application (single source of truth)

`buildChatRequest` (stream.go:78-94) currently inlines two model-config concerns:
temperature (`if a.config.Temperature > 0`) and the reasoning wire params
(`reasoning.NewReasoningResolver(...).Apply`). The codebase already declares this the
"Single source of truth for reasoning wire params" (stream.go:81).

Extract both into one agent method:

```go
// applyRequestConfig applies per-model request defaults shared by every LLM
// call path (turn loop and plan generation): temperature and reasoning wire
// params. Single source of truth for both.
func (a *Agent) applyRequestConfig(req *proxy.ChatRequest)
```

`buildChatRequest` calls it (removing the inlined block); `ExecutionPlanStrategy.Generate`
calls it via an injected hook (below). Any future LLM-call path reuses it. No behavior
change to the turn loop.

### 3.2 `generatePlan` streams + surfaces reasoning (shared primitive)

`ExecutionPlanStrategy` gains two injected hooks (idiomatic Go, keeps the strategy
decoupled from `Agent` internals):

```go
NewExecutionPlanStrategy(llm, tools, logger,
    withApplyRequest(func(req *proxy.ChatRequest) { /* bound a.applyRequestConfig */ }),
    withOnReasoning(func(reasoning string) { /* bound a.notify(EventReasoning, reasoning) */ }),
)
```

`Generate` becomes stream-first with a Chat fallback:

1. Build the prompt + base request exactly as today; apply the `applyRequest` hook
   (temperature + reasoning params now sent, matching normal turns).
2. `ch, streamErr := s.llm.Stream(ctx, req)`.
3. If `streamErr != nil`:
   - user-canceled (`isUserCanceled`) → propagate `ctx.Err()` (mirrors
     `computeNextResponse`, stream.go:357-364);
   - otherwise **fall back to the existing non-streaming `Chat` path** (preserves
     SSE-less providers; mirrors `computeNextResponse`'s streaming→non-streaming
     fallback, stream.go:374-376).
4. Otherwise accumulate content deltas into a `strings.Builder` (the plan JSON) and
   relay reasoning deltas via `onReasoning`. Delta extraction uses the exact field
   semantics of `processStream` (stream.go:617-635): `Delta.Content`/`Message.Content`,
   `Delta.ReasoningContent`/`Message.ReasoningContent`, `Delta.Reasoning`/
   `Message.Reasoning`, `Delta.ReasoningDetails`/`Message.ReasoningDetails`.
5. Honor `ctx.Done()` throughout (the call is already bounded by `AgentTurnTimeout`
   via `generatePlan`'s `planCtx`). Mid-stream errors are returned, **not** retried —
   matches `processStream` semantics.
6. Parse the accumulated content with the existing plan parsing (strip ``` fences,
   `json.Unmarshal`), extracted into a shared `parsePlanContent(content string)`
   used by both the stream and Chat paths — no duplicated parsing.

`generatePlan` (session.go:509-518) wires the hooks and **keeps** the `agent_thinking`
signal + "Generating execution plan…" message as the neutral fallback for models that
emit no structured reasoning. When reasoning streams, it shows alongside/replaces the
static indicator in the reasoning inset.

### 3.3 Liveness during long planning (shared heartbeat component)

The plan-generation call can span 60–100 s. Reuse the shared `core.NewHeartbeat`
component (as `processStream` does, stream.go:582-611) inside the plan-gen stream loop:
when no content/reasoning has advanced since the last tick, emit
`PhaseStillThinking` with elapsed so the UI never shows a dead bubble during plan-gen
TTFT. Same emit semantics as `processStream` (stream.go:605-611).

### 3.4 Non-goals / out of scope

- **No strategy-file changes.** `react` / `evaluator_optimizer` are untouched;
  `plan_execute_strategy.go` unchanged. The entire change is in the shared primitives.
- **No frontend changes.** `EventReasoning` is already strategy-agnostic.
- **Do NOT surface the raw plan JSON as content** (no `EventToolStream` for the plan) —
  the UI shows reasoning, then the executed steps, then the report.
- **Do NOT remove the "planning" message** — it is the neutral fallback.
- **No speed change**: still 2 LLM calls for plan-execute; streaming adds no round trips.
- **Do NOT change `finalizeReport`** — it already streams; if the model emits reasoning
  there it is already surfaced.
- **Do NOT reuse `processStream` wholesale** for plan-gen: it surfaces content as
  `EventToolStream` and applies the no-tool content cap (runaway-loop guard), both wrong
  for a long JSON plan. The dedicated small loop is justified; it reuses the shared
  pieces (reasoning-param config, delta-field semantics, heartbeat).

## 4. File-by-file changes

- `backend/internal/core/assistant/stream.go` — extract temperature+reasoning block
  (78-94) into `Agent.applyRequestConfig`; `buildChatRequest` calls it. No behavior change.
- `backend/internal/core/assistant/tool_exec.go` — `ExecutionPlanStrategy` gains
  `applyRequest`/`onReasoning` hooks; `Generate` streams with Chat fallback; extract
  `parsePlanContent`.
- `backend/internal/core/assistant/session.go` — `generatePlan` wires the hooks
  (`a.applyRequestConfig`, `a.notify(EventReasoning, …)`) and the heartbeat; keeps
  planning message + `agent_thinking`.
- `backend/internal/core/assistant/plan_execute_strategy_test.go` — new reasoning-
  surfacing test; update `TestRunSession_GeneratePlan_BoundedByTimeout` (needs
  `StreamFunc`).
- `backend/internal/core/assistant/stream_test.go` — `applyRequestConfig` coverage.
- `docs/SPECS/agent-loop-strategies.md` — §IV.2: `generatePlan` primitive streams and
  surfaces reasoning (at implementation, per documentation-stewardship).
- `docs/INDEX.md` / `docs/PLANS/README.md` — plan entry (at implementation).

## 5. Test plan

1. `TestPlanExecuteStrategy_SurfacesPlanningReasoning` (new) — `MockClient.StreamFunc`
   emits a reasoning delta then a content delta (valid plan JSON); assert the plan
   executes (engine called) **and** an `EventReasoning` event is observed.
2. `TestPlanExecuteStrategy_StreamFallback` (new) — `StreamFunc` returns a
   non-cancel error; assert `Generate` falls back to `Chat` and the plan parses
   (covers SSE-less providers / existing `ChatFunc`-driven tests staying green).
3. `TestRunSession_GeneratePlan_BoundedByTimeout` — update: block in `StreamFunc` on
   `ctx.Done()` (currently blocks in `ChatFunc`); assert plan-gen still bounded by the
   context. `ExecutionPlanStrategy.Generate` error on expiry preserved.
4. Existing plan-execute tests (Success, EmitsPlanningEvent, GenerationFailureFallsBack,
   WrongParamNameFailsFast, NoTools/NoUserMessage fallbacks) — must stay green:
   their `StreamFunc` returns "streaming not supported" → Chat fallback → same plan.
5. `applyRequestConfig` — turn-loop request unchanged (reasoning resolver + temperature
   still applied) and plan-gen request now carries them.

## 6. Verification

```bash
cd backend && go build ./...
cd backend && go test ./...          # incl. -race on assistant package
cd backend && go run ./tools/check-complexity/   # ≤12
```

Live smoke: run the `dev-test` automation under both `react` and `plan_execute`; confirm
reasoning is visible in the planning phase and the plan still executes + reports.

## 7. Risks / notes

- **Plan-gen now sends reasoning params it never sent before** (via `applyRequestConfig`).
  `reasoning_budget` splits `planGenMaxTokens` (4096) between reasoning and plan text.
  The resolver's shared budget semantics are the same as normal turns; if a model burns
  the budget on reasoning, the plan parse can fail → existing fallback to the react loop
  (`TestPlanExecuteStrategy_GenerationFailureFallsBack`) covers it. Verify no
  regression in plan-completeness for `deepseek-v4-flash-0731`.
- **Stream→Chat fallback on stream-start error is the only double-call path** and it
  mirrors `computeNextResponse`; mid-stream errors are not retried (no thundering
  double-calls).
- **`buildChatRequest` refactor is small and covered** by existing turn-loop tests
  (`stream_test.go`); the extracted helper is a pure move.
- Plan-gen reasoning is real model output, not fabricated; the synthetic planning
  message remains as the no-reasoning fallback.

## 8. Relationship to prior work

- SPEC-010 §IV.2 defines `generatePlan` as the **single** pre-loop primitive — this plan
  upgrades that primitive in place (no new primitive, no strategy branching).
- Follows the plan-step schema-validation + truthful-write-results fix
  (`strategy-agnostic-completion-and-tool-schema.md` §9): same philosophy — close the
  divergence at the shared waist, not per strategy.
- Reuses the shared heartbeat component from the assistant-liveness work
  (`cross-cutting/assistant-liveness-heartbeat-package-split.md`).
