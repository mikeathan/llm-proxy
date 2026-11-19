package stackport

import (
	"time"

	"llm-proxy-stacks/sdk"
)

// Schedule is the SDK's Schedule port over the service's one cron grammar (ADR-0068). Parse is the core's own parser (the one automations use), handed
// in by the composition root so this package imports no scheduler: a stack therefore accepts exactly what an automation accepts, and no more.
type Schedule struct {
	Parse func(expr string) (interface{ Next(time.Time) time.Time }, error)
}

// Next is the next time after the given one at which the expression fires, in the given time's location, or the zero time when it never does.
// An expression that does not parse is invalid, with the grammar's own message.
func (s Schedule) Next(expr string, after time.Time) (time.Time, error) {
	sched, err := s.Parse(expr)
	if err != nil {
		return time.Time{}, stacksdk.Errorf(stacksdk.CodeInvalid, err.Error())
	}
	return sched.Next(after), nil
}
