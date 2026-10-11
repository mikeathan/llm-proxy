package reminders

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"llm-proxy/internal/core/automation"
)

const (
	// MaxHorizon is how far ahead a reminder may be set.
	MaxHorizon = 366 * 24 * time.Hour
	// minCronGap stops a recurring reminder from becoming a message flood.
	minCronGap = 15 * time.Minute
	// gapSamples is how many consecutive occurrences are checked for minCronGap.
	gapSamples = 6
)

// ResolveZone picks the reminder's timezone: the one the caller asked for, else
// the owner's configured zone, else the server's local zone. An unknown IANA
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
	return time.Local, nil
}

// NextFire is the next time r should fire strictly after `after`. A one-off
// fires at FireAt once; a recurring reminder follows its cron in its own
// timezone and stops after Until.
func NextFire(r Reminder, after time.Time) (time.Time, bool) {
	if r.Cron == "" {
		return r.FireAt, r.FireAt.After(after)
	}
	sched, err := automation.ParseCron(r.Cron)
	if err != nil {
		return time.Time{}, false
	}
	loc, err := time.LoadLocation(r.Timezone)
	if err != nil {
		loc = time.Local
	}
	next := sched.Next(after.In(loc))
	if next.IsZero() || (r.Until != nil && next.After(*r.Until)) {
		return time.Time{}, false
	}
	return next.UTC(), true
}

// ValidateNew checks a reminder before it is stored.
func ValidateNew(r Reminder, now time.Time) error {
	if strings.TrimSpace(r.Message) == "" {
		return errors.New("reminder message is empty")
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
	return validateCron(r, now)
}

func validateCron(r Reminder, now time.Time) error {
	if _, err := automation.ParseCron(r.Cron); err != nil {
		return fmt.Errorf("invalid recurrence %q: %w", r.Cron, err)
	}
	if r.Until != nil && !r.Until.After(now) {
		return errors.New("the 'until' time is in the past")
	}
	at := now
	var prev time.Time
	for i := 0; i < gapSamples; i++ {
		next, ok := NextFire(r, at)
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
