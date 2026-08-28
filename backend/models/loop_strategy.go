// loop_strategy.go — the LoopStrategy value object: the persisted agent loop
// archetype selector.  It lives in the leaf models package (single source of
// truth) because ModelConfig/ModelOverride/Automation persist it — the assistant
// engine aliases this type for its domain vocabulary (assistant.LoopStrategyName)
// and must not invert the dependency by owning the enum.
package models

// LoopStrategy is a typed enum — never a raw string — selecting the agent loop
// archetype for a run.  The empty string means "provider default / react"; the
// runtime resolver applies the same default.  Unknown non-empty values are
// rejected at HTTP boundaries (400) and fallen back to react at the resolver.
type LoopStrategy string

const (
	LoopStrategyReact              LoopStrategy = "react"
	LoopStrategyPlanExecute        LoopStrategy = "plan_execute"
	LoopStrategyEvaluatorOptimizer LoopStrategy = "evaluator_optimizer"
)

// Valid reports whether the value is a registered loop strategy.
func (s LoopStrategy) Valid() bool {
	switch s {
	case LoopStrategyReact, LoopStrategyPlanExecute, LoopStrategyEvaluatorOptimizer:
		return true
	}
	return false
}
