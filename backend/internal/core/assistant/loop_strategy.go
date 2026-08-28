// loop_strategy.go — the loop-strategy engine: the domain vocabulary for
// supported loop archetypes, the LoopStrategy interface each strategy implements,
// and the registry that loads them. Strategies compose the shared turn primitives
// (executeTurn, handleToolTurn, handleTextTurn, handleNoToolCalls,
// executeSingleToolStep, executePlan, sieve, repetition detector) — they never
// reimplement them.
package assistant

import (
	"context"
	"fmt"
	"sort"

	"llm-proxy/internal/core/proxy"
	"llm-proxy/models"
)

// LoopStrategyName is the assistant's domain vocabulary for the supported loop
// archetypes, aliased to the persisted models.LoopStrategy enum (single source
// of truth — the leaf models package owns it because ModelConfig persists it).
// Only values with a registered strategy are declared here — deferred archetypes
// (map_reduce, auditor, human_in_the_loop, orchestrator_workers) are added with
// their own enum constant + registration when implemented, never reserved now.
type LoopStrategyName = models.LoopStrategy

const (
	LoopReact              = models.LoopStrategyReact
	LoopPlanExecute        = models.LoopStrategyPlanExecute
	LoopEvaluatorOptimizer = models.LoopStrategyEvaluatorOptimizer
)

const defaultLoopStrategy = LoopReact

// ParseLoopStrategy normalizes a config value. Empty or unknown -> react.
// Boundary validation (HTTP handlers) rejects unknown values with a 400 so this
// lenient default is defense-in-depth only, never a silent-failure path.
func ParseLoopStrategy(s LoopStrategyName) LoopStrategyName {
	if s.Valid() {
		return s
	}
	return defaultLoopStrategy
}

// LoopStrategy drives one agent run from a shared runSession. It composes the
// existing turn primitives (executeTurn, handleToolTurn, handleTextTurn,
// handleNoToolCalls, executeSingleToolStep, executePlan, sieve, repetition
// detector) — it never reimplements them.
//
// Contract: Run returns the final reply and must finalize through the shared
// completion path (`completeWith`) so the "completed" lifecycle is emitted exactly
// once on success. Run never re-enters runSession setup — `run()` owns the
// `runS` back-pointer.
type LoopStrategy interface {
	Name() LoopStrategyName
	Run(ctx context.Context, s *runSession) (reply string, history []proxy.Message, err error)
}

// LoopStrategyRegistry maps LoopStrategyName -> constructor. Populated explicitly
// (no init()) by newLoopStrategyRegistry, matching the providerTiers pattern
// (agent.go composeProviderTiers).
type LoopStrategyRegistry struct {
	builders map[LoopStrategyName]LoopStrategyBuilder
}

type LoopStrategyBuilder func() LoopStrategy

func NewLoopStrategyRegistry() *LoopStrategyRegistry {
	return &LoopStrategyRegistry{builders: make(map[LoopStrategyName]LoopStrategyBuilder)}
}

func (r *LoopStrategyRegistry) Register(name LoopStrategyName, b LoopStrategyBuilder) {
	r.builders[name] = b
}

// Build returns the strategy for name, or an error when no builder is registered.
// Strategies are stateless (they read the agent from the run session), so Build
// takes no agent.
func (r *LoopStrategyRegistry) Build(name LoopStrategyName) (LoopStrategy, error) {
	b, ok := r.builders[name]
	if !ok {
		return nil, fmt.Errorf("loop strategy not registered: %s", name)
	}
	return b(), nil
}

// RegisteredLoopStrategyNames returns the sorted names of registered strategies —
// the single source for the admin UI dropdown's option list. Registering a new
// strategy automatically surfaces here; the frontend never hardcodes the list.
func RegisteredLoopStrategyNames() []string {
	names := make([]string, 0, len(loopStrategies.builders))
	for name := range loopStrategies.builders {
		names = append(names, string(name))
	}
	sort.Strings(names)
	return names
}

// loopStrategies is the shared registry, built once. Treat as read-only.
var loopStrategies = newLoopStrategyRegistry()

func newLoopStrategyRegistry() *LoopStrategyRegistry {
	r := NewLoopStrategyRegistry()
	r.Register(LoopReact, func() LoopStrategy { return newReactStrategy() })
	r.Register(LoopPlanExecute, func() LoopStrategy { return newPlanExecuteStrategy() })
	r.Register(LoopEvaluatorOptimizer, func() LoopStrategy { return newEvaluatorOptimizerStrategy() })
	return r
}
