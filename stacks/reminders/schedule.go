package reminders

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"llm-proxy-stacks/sdk"
)

// maxMessageBytes is the longest reminder: what the notify port will send, less the "(late) " prefix a late one carries.
const maxMessageBytes = stacksdk.MaxNotifyBytes - len(latePrefix)

const (
	// MaxHorizon is how far ahead a reminder may be set.
	MaxHorizon = 366 * 24 * time.Hour
	// minCronGap stops a recurring reminder from becoming a message flood.
	minCronGap = 15 * time.Minute
	// gapSamples is how many consecutive occurrences are checked for minCronGap.
	gapSamples = 6
)

// ResolveZone picks the reminder's timezone: the one the caller asked for, else
// the owner's configured zone, else UTC (the server's own zone has no IANA name
// Go can report, and "Local" means nothing to a user or a browser). An unknown IANA
// name is an error naming the bad value, never a silent fallback.
func ResolveZone(requested, ownerZone string) (*time.Location, error) {
	for _, name := range []string{requested, ownerZone} {
		if name == "" {
			continue
		}
		loc, err := time.LoadLocation(name)
		if err != nil {
			return nil, fmt.Errorf("unknown timezone %q: use an IANA name such as Europe/London", name)
		}
		return loc, nil
	}
	return time.UTC, nil
}

// NextFire is the next time r should fire strictly after `after`. A one-off
// fires at FireAt once; a recurring reminder follows its cron in its own
// timezone and stops after Until.
func NextFire(c stacksdk.Schedule, r Reminder, after time.Time) (time.Time, bool) {
	if r.Cron == "" {
		return r.FireAt, r.FireAt.After(after)
	}
	loc, err := time.LoadLocation(r.Timezone)
	if err != nil {
		loc = time.Local
	}
	next, err := c.Next(r.Cron, after.In(loc))
	if err != nil || next.IsZero() || (r.Until != nil && next.After(*r.Until)) {
		return time.Time{}, false
	}
	return next.UTC(), true
}

// ValidateNew checks a reminder before it is stored.
func ValidateNew(c stacksdk.Schedule, r Reminder, now time.Time) error {
	if strings.TrimSpace(r.Message) == "" {
		return errors.New("reminder message is empty")
	}
	if len(r.Message) > maxMessageBytes {
		return fmt.Errorf("reminder message is longer than a message may be (%d bytes)", maxMessageBytes)
	}
	if !r.FireAt.After(now) {
		return fmt.Errorf("reminder time %s is in the past", r.FireAt.Format(time.RFC3339))
	}
	if r.FireAt.After(now.Add(MaxHorizon)) {
		return errors.New("reminder is more than a year away")
	}
	if r.Cron == "" {
		return nil
	}
	return validateCron(c, r, now)
}

func validateCron(c stacksdk.Schedule, r Reminder, now time.Time) error {
	if _, err := c.Next(r.Cron, now); err != nil {
		return fmt.Errorf("invalid recurrence %q: %s", r.Cron, grammarMessage(err))
	}
	if r.Until != nil && !r.Until.After(now) {
		return errors.New("the 'until' time is in the past")
	}
	at := now
	var prev time.Time
	for i := 0; i < gapSamples; i++ {
		next, ok := NextFire(c, r, at)
		if !ok {
			break
		}
		if !prev.IsZero() && next.Sub(prev) < minCronGap {
			return fmt.Errorf("recurrence fires more often than every %d minutes", int(minCronGap.Minutes()))
		}
		prev, at = next, next
	}
	return nil
}

// grammarMessage is the schedule's own words for a bad expression, without the SDK's code prefix.
func grammarMessage(err error) string {
	var coded *stacksdk.Error
	if errors.As(err, &coded) {
		return coded.Message
	}
	return err.Error()
}
