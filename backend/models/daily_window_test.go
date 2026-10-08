package models

import (
	"testing"
	"time"
)

func TestParseDailyWindow(t *testing.T) {
	for _, tc := range []struct {
		in      string
		wantErr bool
	}{
		{"08:00-22:00", false},
		{"22:00-06:00", false}, // wraps past midnight
		{"00:00-23:59", false},
		{" 08:00 - 22:00 ", false},
		{"", true},
		{"08:00", true},
		{"08:00-08:00", true}, // empty window is ambiguous: reject
		{"8:00-22:00", true},
		{"24:00-06:00", true},
		{"08:60-22:00", true},
		{"08:00-22:00-23:00", true},
		{"morning-evening", true},
	} {
		_, err := ParseDailyWindow(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("ParseDailyWindow(%q) error = %v, wantErr %v", tc.in, err, tc.wantErr)
		}
	}
}

// Contains is half-open — start in, end out — and reads the wall clock of the
// time it is given, in that time's own location.
func TestDailyWindow_Contains(t *testing.T) {
	day := func(h, m int, loc *time.Location) time.Time { return time.Date(2026, 10, 8, h, m, 0, 0, loc) }
	plus5 := time.FixedZone("UTC+5", 5*60*60)

	for _, tc := range []struct {
		name   string
		window string
		at     time.Time
		want   bool
	}{
		{"a minute before start", "08:00-22:00", day(7, 59, time.UTC), false},
		{"exactly at start", "08:00-22:00", day(8, 0, time.UTC), true},
		{"mid window", "08:00-22:00", day(15, 30, time.UTC), true},
		{"a minute before end", "08:00-22:00", day(21, 59, time.UTC), true},
		{"exactly at end", "08:00-22:00", day(22, 0, time.UTC), false},
		{"wrapping window late evening", "22:00-06:00", day(23, 0, time.UTC), true},
		{"wrapping window early morning", "22:00-06:00", day(5, 0, time.UTC), true},
		{"wrapping window at end", "22:00-06:00", day(6, 0, time.UTC), false},
		{"wrapping window midday", "22:00-06:00", day(12, 0, time.UTC), false},
		{"uses the given time's own wall clock", "08:00-22:00", day(23, 0, plus5), false}, // 18:00 UTC but 23:00 local
		{"same instant, local wall clock inside", "08:00-22:00", day(9, 0, plus5), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, err := ParseDailyWindow(tc.window)
			if err != nil {
				t.Fatal(err)
			}
			if got := w.Contains(tc.at); got != tc.want {
				t.Errorf("%s.Contains(%s) = %v, want %v", tc.window, tc.at.Format("15:04 MST"), got, tc.want)
			}
		})
	}
}
