// react_strategy.go — the ReAct loop strategy: the classic Thought → Action →
// Observation loop. This is the pre-refactor `runSession.run()` loop body moved
// verbatim; `run()` now resolves and dispatches to this strategy.
package assistant

import (
	"context"
	"fmt"

	"llm-proxy/internal/core/proxy"
)

// ReactStrategy drives one agent run with the ReAct loop. Plain react has no
// stop guards; the evaluator-optimizer strategy (Phase 3) wraps a ReactStrategy
// with a bounded stop guard configured. The strategy is stateless beyond the
// guard list — the loop body operates on the session's agent (s.agent).
type ReactStrategy struct {
	stopGuards []StopGuard // nil for plain react
}

func newReactStrategy(stopGuards ...StopGuard) *ReactStrategy {
	return &ReactStrategy{stopGuards: stopGuards}
}

func (r *ReactStrategy) Name() LoopStrategyName { return LoopReact }

func (r *ReactStrategy) Run(ctx context.Context, s *runSession) (string, []proxy.Message, error) {
	// Wire the strategy's stop guards into the session so the handleTextTurn
	// completion hook consults them. Nil for plain react — zero behavior change.
	s.stopGuards = r.stopGuards

	for {
		s.steps++
		if err := s.ctx.Err(); err != nil {
			return "", s.history, fmt.Errorf("agent execution halted: %w", err)
		}

		if done, reply, err := s.checkForcedCompletion(); done {
			return reply, s.history, err
		}

		if s.steps >= s.agent.config.MaxSteps && !s.warnedAdvisory {
			s.warnedAdvisory = true
			s.agent.deps.Logger.Warn("agent exceeded advisory step limit, continuing", "steps", s.steps)
		}

		s.maybeFlushMemoryBeforeTurn()

		s.agent.notifyStepStart(s.steps)
		s.agent.notifyThinking()

		turnMsg, parseErr, toolsList, err := s.agent.executeTurn(s.ctx, &s.history)
		if err != nil {
			done, reply, turnErr := s.handleTurnError(err)
			if done {
				return reply, s.history, turnErr
			}
			if turnErr != nil {
				return "", s.history, turnErr
			}
			continue
		}

		s.sieveStreak = 0

		if len(turnMsg.ToolCalls) > 0 {
			done, reply, turnErr := s.handleToolTurn(turnMsg, toolsList)
			if done {
				return reply, s.history, turnErr
			}
			if turnErr != nil {
				return "", s.history, turnErr
			}
			continue
		}

		done, reply, turnErr := s.handleTextTurn(turnMsg, parseErr, toolsList)
		if done {
			return reply, s.history, turnErr
		}
		if turnErr != nil {
			return "", s.history, turnErr
		}
	}
}
