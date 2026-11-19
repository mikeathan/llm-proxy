package reminders

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// testCron is a small stand-in for the core's schedule port, enough for the expressions these tests use: five numeric fields with *, lists,
// ranges and steps, and @daily. The real grammar is the core's (automation.ParseCron), and internal/app tests the stack over it.
type testCron struct{}

var testSchedule = testCron{}

func (testCron) Next(expr string, after time.Time) (time.Time, error) {
	if expr == "@daily" {
		expr = "0 0 * * *"
	}
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return time.Time{}, fmt.Errorf("expected exactly 5 fields, found %d: %s", len(fields), expr)
	}
	var sets [5]map[int]bool
	bounds := [5][2]int{{0, 59}, {0, 23}, {1, 31}, {1, 12}, {0, 6}}
	for i, f := range fields {
		set, err := fieldSet(f, bounds[i][0], bounds[i][1])
		if err != nil {
			return time.Time{}, err
		}
		sets[i] = set
	}
	t := after.Truncate(time.Minute).Add(time.Minute)
	for end := t.Add(5 * 366 * 24 * time.Hour); t.Before(end); t = t.Add(time.Minute) {
		domAny, dowAny := fields[2] == "*", fields[4] == "*"
		dom, dow := sets[2][t.Day()], sets[4][int(t.Weekday())]
		dayOK := (domAny || dowAny) && dom && dow || !domAny && !dowAny && (dom || dow)
		if sets[0][t.Minute()] && sets[1][t.Hour()] && sets[3][int(t.Month())] && dayOK {
			return t, nil
		}
	}
	return time.Time{}, nil
}

func fieldSet(f string, lo, hi int) (map[int]bool, error) {
	set := map[int]bool{}
	for _, part := range strings.Split(f, ",") {
		step := 1
		if base, s, ok := strings.Cut(part, "/"); ok {
			n, err := strconv.Atoi(s)
			if err != nil || n < 1 {
				return nil, fmt.Errorf("failed to parse int from %s", s)
			}
			part, step = base, n
		}
		from, to := lo, hi
		if part != "*" {
			a, b, isRange := strings.Cut(part, "-")
			x, err := strconv.Atoi(a)
			if err != nil || x < lo || x > hi {
				return nil, fmt.Errorf("failed to parse %q", part)
			}
			from, to = x, x
			if isRange {
				y, err := strconv.Atoi(b)
				if err != nil || y < x || y > hi {
					return nil, fmt.Errorf("failed to parse %q", part)
				}
				to = y
			}
		}
		for v := from; v <= to; v += step {
			set[v] = true
		}
	}
	return set, nil
}
