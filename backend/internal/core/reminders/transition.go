package reminders

import (
	"fmt"
	"strings"
	"time"
)

// Notice events: what the in-app record and the log are told about a reminder.
const (
	EventDue    = "due"
	EventLate   = "late"
	EventMissed = "missed"
	EventFailed = "failed"
	// EventPartial: delivered to at least one channel, failed on another.
	EventPartial = "partial"
)

// maxErrorLength bounds the delivery error kept on a reminder.
const maxErrorLength = 300

// Timing is how a due reminder stands against the clock.
type Timing int

const (
	OnTime Timing = iota // within normal timer jitter
	Late                 // overdue, still worth sending (the service was down)
	Missed               // overdue past the window: shown in-app, never sent
)

// Policy holds the numbers of the reminder lifecycle. It is data, so the whole
// lifecycle (Classify, Advance) is a pure function of its inputs and is tested
// without a database, a clock or a connector.
type Policy struct {
	LateWindow  time.Duration   // overdue by more than this is Missed
	LateGrace   time.Duration   // overdue by no more than this is OnTime
	MaxAttempts int             // delivery tries before giving up
	RetryDelays []time.Duration // wait before retry 2, 3, … (the last repeats)
}

// DefaultPolicy is the shipped lifecycle (SPEC-011 §2): late up to 12 h, 3
// attempts, retried after 1 then 5 minutes.
func DefaultPolicy() Policy {
	return Policy{
		LateWindow:  12 * time.Hour,
		LateGrace:   2 * time.Minute,
		MaxAttempts: 3,
		RetryDelays: []time.Duration{time.Minute, 5 * time.Minute},
	}
}

// Classify says whether a reminder due at r.FireAt is on time, late or missed at now.
func (p Policy) Classify(r Reminder, now time.Time) Timing {
	overdue := now.Sub(r.FireAt)
	switch {
	case overdue > p.LateWindow:
		return Missed
	case overdue > p.LateGrace:
		return Late
	default:
		return OnTime
	}
}

type outcomeKind int

const (
	outcomeDelivered outcomeKind = iota
	outcomeOverdue
	outcomeFailed
)

// Outcome is what happened when the service tried to act on a due reminder.
type Outcome struct {
	kind outcomeKind
	late bool
	err  string
	// warn is what failed on a channel while another took the reminder.
	warn string
}

// Overdue: the reminder was found past the late window and is not sent.
var Overdue = Outcome{kind: outcomeOverdue}

// Delivered: it was sent (or, in-app only, recorded) with the given timing.
func Delivered(t Timing) Outcome { return Outcome{kind: outcomeDelivered, late: t == Late} }

// DeliveredPartly: it reached at least one channel but not all; warn says what failed.
// It is not retried, since a retry would send the text again to the channels that have it.
func DeliveredPartly(t Timing, warn string) Outcome {
	return Outcome{kind: outcomeDelivered, late: t == Late, warn: warn}
}

// Failed: the send returned an error.
func Failed(err string) Outcome { return Outcome{kind: outcomeFailed, err: err} }

// Step is the result of an outcome: the reminder's next state, and the event to
// record ("" when nothing is to be reported, such as a retry that will be quiet).
type Step struct {
	Next  Reminder
	Event string
}

// Advance is the one place that decides what a delivery outcome does to a
// reminder: a one-off closes, a recurring one moves to its next occurrence, a
// failed send is retried and finally reported. It never changes its input.
func (p Policy) Advance(r Reminder, o Outcome, now time.Time) Step {
	switch o.kind {
	case outcomeDelivered:
		event := EventDue
		if o.late {
			event = EventLate
		}
		next := moveOnOr(reset(r), now, StatusDone)
		if o.warn != "" {
			event, next.LastError = EventPartial, truncate(o.warn, maxErrorLength)
		}
		return Step{Next: next, Event: event}
	case outcomeOverdue:
		return Step{Next: moveOnOr(reset(r), now, StatusMissed), Event: EventMissed}
	default:
		return p.failed(r, o.err, now)
	}
}

func (p Policy) failed(r Reminder, cause string, now time.Time) Step {
	r.Attempts++
	r.LastError = truncate(cause, maxErrorLength)
	if r.Attempts < p.MaxAttempts {
		r.FireAt = now.Add(p.retryDelay(r.Attempts))
		return Step{Next: r}
	}
	// Out of attempts: a recurring reminder reports it and goes on to its next
	// occurrence, so a short connector outage does not end it for good.
	r.Attempts = 0
	next := moveOnOr(r, now, StatusFailed)
	if next.Status == StatusFailed {
		next.Attempts = p.MaxAttempts
	}
	return Step{Next: next, Event: EventFailed}
}

func (p Policy) retryDelay(attempts int) time.Duration {
	i := min(attempts-1, len(p.RetryDelays)-1)
	return p.RetryDelays[max(i, 0)]
}

// reset clears the failure record of a reminder that is moving on.
func reset(r Reminder) Reminder {
	r.Attempts, r.LastError = 0, ""
	return r
}

// moveOnOr moves a recurring reminder to its next occurrence after now, or
// closes it with the given status when it is a one-off or has no occurrence left.
func moveOnOr(r Reminder, now time.Time, closed Status) Reminder {
	if next, ok := NextFire(r, now); r.Cron != "" && ok {
		r.FireAt = next
		return r
	}
	r.Status = closed
	return r
}

func truncate(msg string, n int) string {
	msg = strings.TrimSpace(msg)
	if len(msg) <= n {
		return msg
	}
	return msg[:n]
}

// Pause silences a repeating reminder: it stays in the list but does not fire.
// A one-off is deleted or rescheduled instead, so pausing one is refused.
func Pause(r Reminder) (Reminder, error) {
	if r.Cron == "" {
		return r, fmt.Errorf("%w: only a repeating reminder can be paused", ErrInvalid)
	}
	if r.Status != StatusScheduled {
		return r, fmt.Errorf("%w: the reminder is not scheduled", ErrInvalid)
	}
	r.Status = StatusPaused
	return r, nil
}

// Resume un-silences a paused reminder at its next occurrence after now; what
// was silenced in between is not replayed.
func Resume(r Reminder, now time.Time) (Reminder, error) {
	if r.Status != StatusPaused {
		return r, fmt.Errorf("%w: the reminder is not paused", ErrInvalid)
	}
	next, ok := NextFire(r, now)
	if !ok {
		return r, fmt.Errorf("%w: it has no occurrence left before its end date: delete it or edit the end date", ErrInvalid)
	}
	r.Status, r.FireAt, r.Attempts, r.LastError = StatusScheduled, next, 0, ""
	return r, nil
}

// SkipNext moves a repeating reminder past its next occurrence (the one coming
// up is not sent; the one after it is).
func SkipNext(r Reminder) (Reminder, error) {
	if r.Cron == "" || r.Status != StatusScheduled {
		return r, fmt.Errorf("%w: only a scheduled repeating reminder has a next occurrence to skip", ErrInvalid)
	}
	next, ok := NextFire(r, r.FireAt)
	if !ok {
		return r, fmt.Errorf("%w: that is its last occurrence: delete it instead", ErrInvalid)
	}
	r.FireAt, r.Attempts, r.LastError = next, 0, ""
	return r, nil
}
