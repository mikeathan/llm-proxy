package models

import (
	"fmt"
	"strings"
	"time"
)

// ReminderTimeLayout is how a reminder's local time is written back to the user.
const ReminderTimeLayout = "Mon 2 Jan 2006 15:04"

// localInstantLayouts are the offset-less forms ParseInstantIn reads in a zone.
var localInstantLayouts = []string{"2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02 15:04"}

// ParseInstant reads a point in time written as RFC 3339 with an offset (for
// example 2026-10-12T09:00:00+01:00) and returns it in UTC. field names the
// argument in the error, so the agent or the API caller sees what to fix.
func ParseInstant(field, value string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, fmt.Errorf("%s must be RFC 3339 with an offset, for example 2026-10-12T09:00:00+01:00", field)
	}
	return t.UTC(), nil
}

// ParseInstantIn is ParseInstant that also reads a local date and time without an
// offset ("2026-10-18 20:00") as wall-clock time in loc, so the caller never does
// the offset arithmetic. An explicit offset always wins over loc.
func ParseInstantIn(field, value string, loc *time.Location) (time.Time, error) {
	value = strings.TrimSpace(value)
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t.UTC(), nil
	}
	for _, layout := range localInstantLayouts {
		if t, err := time.ParseInLocation(layout, value, loc); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("%s must be a local date and time such as 2026-10-18 20:00 (or RFC 3339 with an offset)", field)
}
