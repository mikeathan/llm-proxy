// loop_resolver.go — selects and builds the loop strategy for one Agent.
// Selection is deterministic: explicit per-model config > provider/workload
// default > react. The LLM never chooses its own loop shape.
package assistant

// resolveLoopStrategy selects and builds the strategy for one Agent.
func resolveLoopStrategy(a *Agent) LoopStrategy {
	name := resolveLoopStrategyName(a)
	s, err := loopStrategies.Build(name)
	if err != nil {
		a.deps.Logger.Error("loop strategy unavailable, falling back to react",
			"strategy", name, "error", err)
		s, err = loopStrategies.Build(defaultLoopStrategy)
		if err != nil {
			// Invariant: react is always registered (the registry is populated
			// statically at package init). A failure here is a programming
			// error — fail loudly instead of returning a nil strategy that
			// would nil-panic inside Run.
			panic("loop strategy registry invariant violated: react not registered: " + err.Error())
		}
	}
	return s
}

// resolveLoopStrategyName applies the deterministic precedence:
// explicit per-model config > provider/workload default > react.
func resolveLoopStrategyName(a *Agent) LoopStrategyName {
	if a.config.LoopStrategy != "" {
		return a.config.LoopStrategy
	}
	// Provider/workload loop defaults are intentionally static (react) until a
	// measured per-provider table exists — an unmeasured loop-shape default is
	// higher risk than temperature/max_tokens because it changes the whole
	// execution shape (SPEC-010 §IV).
	return defaultLoopStrategy
}
