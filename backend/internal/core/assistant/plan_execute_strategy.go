// plan_execute_strategy.go — the plan-and-execute loop strategy: generate a
// step-by-step tool plan for the last user message, then execute it in order.
// Falls back to the react loop when planning cannot proceed (no user message,
// no tools, generation failure). Replaces the old PlanStrategy short-circuit in
// Execute().
package assistant

import (
	"context"

	"llm-proxy/internal/core/proxy"
)

// PlanExecuteStrategy drives one agent run plan-first, with an explicit fallback
// to the react loop. Stateless: it reads the agent from the run session.
type PlanExecuteStrategy struct {
	fallback *ReactStrategy
}

func newPlanExecuteStrategy() *PlanExecuteStrategy {
	return &PlanExecuteStrategy{fallback: newReactStrategy()}
}

func (s *PlanExecuteStrategy) Name() LoopStrategyName { return LoopPlanExecute }

func (s *PlanExecuteStrategy) Run(ctx context.Context, run *runSession) (string, []proxy.Message, error) {
	a := run.agent
	lastUserMsg := lastUserMessage(run.history)
	if lastUserMsg == "" {
		return s.fallback.Run(ctx, run)
	}
	tools, err := a.deps.Provider.ListTools(ctx)
	if err != nil || len(tools) == 0 {
		return s.fallback.Run(ctx, run)
	}
	plan, err := run.generatePlan(ctx, tools, lastUserMsg)
	if err != nil {
		a.deps.Logger.Warn("plan generation failed, falling back to react loop", "error", err)
		return s.fallback.Run(ctx, run)
	}
	_, history, err := a.executePlan(ctx, run.history, plan)
	if err != nil {
		return "", history, err
	}
	// executePlan returns the "[Plan execution complete]" literal — not a
	// deliverable. Produce the real report via the shared tools-disabled
	// finalization turn, then seal with the universal completion path so the
	// "completed" lifecycle is emitted exactly once (matching every other
	// strategy).
	run.history = history
	report, err := run.finalizeReport(ctx)
	if err != nil {
		return "", run.history, err
	}
	return run.completeWith(report)
}

// lastUserMessage returns the content of the most recent user-role message in
// history, or "" when none exists.
func lastUserMessage(history []proxy.Message) string {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == proxy.UserRole {
			return history[i].Content
		}
	}
	return ""
}
