package models

import (
	"fmt"
	"strings"
	"time"
)

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
