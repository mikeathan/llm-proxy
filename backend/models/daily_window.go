// daily_window.go — a recurring wall-clock range within a day, shared by every
// feature that runs or stays quiet "between HH:MM and HH:MM" (heartbeat active
// hours today; quiet hours in the attention policy).
package models

import (
	"fmt"
	"strings"
	"time"
)

const (
	dailyWindowSeparator = "-"
	dailyWindowLayout    = "15:04"
	minutesPerHour       = 60
	minutesPerDay        = 24 * minutesPerHour
)

// DailyWindow is the half-open range [start, end) of a day, in minutes since
// midnight. An end before the start wraps past midnight (22:00-06:00).
type DailyWindow struct {
	start, end int
}

// ParseDailyWindow reads "HH:MM-HH:MM". An empty window (start == end) is
// rejected as ambiguous: it would mean either never or always.
func ParseDailyWindow(s string) (DailyWindow, error) {
	from, to, ok := strings.Cut(s, dailyWindowSeparator)
	if !ok {
		return DailyWindow{}, fmt.Errorf("use HH:MM-HH:MM, such as 08:00-22:00 (got %q)", s)
	}
	start, err := parseMinuteOfDay(from)
	if err != nil {
		return DailyWindow{}, err
	}
	end, err := parseMinuteOfDay(to)
	if err != nil {
		return DailyWindow{}, err
	}
	if start == end {
		return DailyWindow{}, fmt.Errorf("window %q is empty: start and end must differ", s)
	}
	return DailyWindow{start: start, end: end}, nil
}

func parseMinuteOfDay(s string) (int, error) {
	s = strings.TrimSpace(s)
	t, err := time.Parse(dailyWindowLayout, s)
	// time.Parse tolerates a single-digit hour; the stored form is strictly HH:MM.
	if err != nil || len(s) != len(dailyWindowLayout) {
		return 0, fmt.Errorf("invalid time %q: use HH:MM (24-hour)", s)
	}
	return t.Hour()*minutesPerHour + t.Minute(), nil
}

// Contains reports whether t's wall-clock time falls inside the window. It reads
// the clock in t's own location, so callers choose the zone by what they pass.
func (w DailyWindow) Contains(t time.Time) bool {
	now := t.Hour()*minutesPerHour + t.Minute()
	if w.start < w.end {
		return now >= w.start && now < w.end
	}
	return now >= w.start || now < w.end
}
