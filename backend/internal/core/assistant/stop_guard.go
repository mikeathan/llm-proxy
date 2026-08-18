// stop_guard.go — StopGuard: a bounded policy hook that may refuse to let the
// loop finalize, returning a synthetic user-role nudge (its content) to continue
// the run. Guards fire only on successful natural completion — never on forced
// completion, fallback answers, or error/stall returns.
package assistant

// StopGuard is a bounded policy hook that may refuse to let the loop finalize,
// returning a synthetic follow-up (user-role nudge) to continue the run.
type StopGuard interface {
	// Nudge returns the content of a nudge message to inject as a user-role
	// message, or "" to allow finalization.
	Nudge(s *runSession) (string, error)
}
