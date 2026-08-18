// evaluator_guard.go — the EvaluatorGuard: the evaluator-optimizer stop guard.
// Before the run finalizes it returns a bounded self-review nudge so the model
// verifies/fixes its work instead of finishing prematurely. Prompt-based
// self-critique only — no verification-evidence ledger (deferred, §12 of the
// plan). The nudge is capped by runSession.stopGuardAttempts (never perpetual).
package assistant

import "llm-proxy/internal/core/assistant/prompts"

// EvaluatorGuard implements StopGuard by returning the evaluator-review nudge.
// It is stateless: the nudge is a fixed synthetic control prompt.
type EvaluatorGuard struct{}

func newEvaluatorGuard() *EvaluatorGuard {
	return &EvaluatorGuard{}
}

func (g *EvaluatorGuard) Nudge(s *runSession) (string, error) {
	return prompts.EvaluatorReviewPrompt, nil
}
