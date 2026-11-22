package brief

import (
	"time"

	"llm-proxy-stacks/sdk"
)

// Timing is how the Brief module tells time: the core's cron grammar to find when an edition is ready, the stack's timezone to read that in, and the
// clock. The collector and the publisher both embed it, so they agree on when an edition is due.
type Timing struct {
	// Schedule is the core's cron grammar; Location is the stack's timezone, in which an edition's ready time is read.
	Schedule stacksdk.Schedule
	Location func() *time.Location
	// Now is the clock; nil means time.Now.
	Now func() time.Time
}

func (t Timing) now() time.Time {
	if t.Now != nil {
		return t.Now().UTC()
	}
	return time.Now().UTC()
}

func (t Timing) loc() *time.Location {
	if t.Location != nil {
		if l := t.Location(); l != nil {
			return l
		}
	}
	return time.UTC
}

// NextReady is the first moment after the given time at which the edition is ready, or the zero time when it never is again.
func (t Timing) NextReady(e Edition, after time.Time) (time.Time, error) {
	return t.Schedule.Next(e.Cron(), after.In(t.loc()))
}
