// evaluator_optimizer_strategy.go — the evaluator-optimizer loop strategy: the
// react loop plus a bounded generator/evaluator pass. A stop guard injects a
// self-review nudge before the run finalizes so the model verifies/fixes its
// work instead of finishing prematurely.
package assistant

import (
	"context"

	"llm-proxy/internal/core/proxy"
)

// EvaluatorOptimizerStrategy drives the react loop with one stop guard
// configured. The loop body is unchanged; the guard only intercepts the
// natural-completion branch via runSession.maybeNudge.
type EvaluatorOptimizerStrategy struct {
	inner *ReactStrategy
}

func newEvaluatorOptimizerStrategy() *EvaluatorOptimizerStrategy {
	return &EvaluatorOptimizerStrategy{
		inner: newReactStrategy(newEvaluatorGuard()),
	}
}

func (s *EvaluatorOptimizerStrategy) Name() LoopStrategyName { return LoopEvaluatorOptimizer }

func (s *EvaluatorOptimizerStrategy) Run(ctx context.Context, run *runSession) (string, []proxy.Message, error) {
	return s.inner.Run(ctx, run)
}
