# Agent Loop — Execution, Sieve, Stuck Detection & Fallback — executePlan (plan_execute primitive)

Reference for the [`agent-loop`](../SKILL.md) skill, loaded on demand. You are changing the plan-execute strategy.

## executePlan Execution (plan_execute primitive)

`executePlan` is the shared plan primitive used by `PlanExecuteStrategy` (SPEC-010); the old
`execute_plan` tool is gone. When a plan is generated it bypasses the standard turn loop:

- **Pre-check:** if `len(plan.Steps) > MaxPlanSteps` (default 50), plan fails before any step executes.
- **Plan-level timeout:** whole plan wrapped in `context.WithTimeout(MaxPlanDuration)` (default 15 min).
- **Per-step timeout:** each step uses `executeSingleToolStep` with the same `ToolTimeout`/`FilesystemToolTimeout` as regular tool calls.
- **Inline check:** after each step, `if i >= MaxPlanSteps → abort`.
- **Guardrail checks:** each step/tool goes through `resolveGuardrail` with `GuardrailTimeout`.
- **Step execution failures continue (record-and-continue):** a step whose tool *execution* fails (shell exit code, compile error, missing file, tool timeout) is logged as `plan step failed, continuing` and the next step runs — the failure is already in history as a tool result, so the final report notes it. A single failed step never aborts the run, matching the react loop's `processToolCalls` behavior (model-agnostic: a weak model's mistakes become reported issues, not run-killers). Only structural errors abort: args marshal failure, `validateToolArgs` failure, `MaxPlanSteps` exceeded, plan deadline exceeded.
- **Guardrail-denied steps continue** (`plan step guardrail denied, continuing`); in unattended automation the denial is immediate (Constitution II.10).
