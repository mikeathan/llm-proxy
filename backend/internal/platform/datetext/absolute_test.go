package datetext

import (
	"testing"
	"time"
)

// Wednesday 14 October 2026, in a zone ahead of UTC so a date taken in UTC would be wrong near midnight.
var wed = time.Date(2026, time.October, 14, 23, 30, 0, 0, time.FixedZone("ahead", 3*60*60))

func TestAbsolutize(t *testing.T) {
	for _, tc := range []struct {
		name, in, want string
	}{
		{"today", "The freeze starts today.", "The freeze starts 2026-10-14 (Wednesday)."},
		{"tomorrow", "Demo tomorrow at ten.", "Demo 2026-10-15 (Thursday) at ten."},
		{"yesterday", "It broke yesterday.", "It broke 2026-10-13 (Tuesday)."},
		{"next weekday is the first one after today", "Review next Friday.", "Review 2026-10-16 (Friday)."},
		{"next of today's weekday is a week away", "Sync next Wednesday.", "Sync 2026-10-21 (Wednesday)."},
		{"this weekday on or after today", "Ship this Wednesday.", "Ship 2026-10-14 (Wednesday)."},
		{"this weekday later in the week", "Ship this Saturday.", "Ship 2026-10-17 (Saturday)."},
		{"last weekday is the most recent one before today", "Met last Monday.", "Met 2026-10-12 (Monday)."},
		{"in days", "Due in 3 days.", "Due 2026-10-17 (Saturday)."},
		{"in weeks", "Due in 2 weeks.", "Due 2026-10-28 (Wednesday)."},
		{"case and several phrases", "TOMORROW or Next friday", "2026-10-15 (Thursday) or 2026-10-16 (Friday)"},
		{"a bare weekday is ambiguous and left alone", "Standup on Friday.", "Standup on Friday."},
		{"next week is not a date and is left alone", "Plan it for next week.", "Plan it for next week."},
		{"words that only contain a phrase are left alone", "A todayish tomorrowland note.", "A todayish tomorrowland note."},
		{"an absolute date is untouched", "Due 2026-11-01.", "Due 2026-11-01."},
		{"no relative date", "We use tabs in Go files.", "We use tabs in Go files."},
		{"empty", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Absolutize(tc.in, wed); got != tc.want {
				t.Errorf("Absolutize(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// The same text saved on another day reads differently: the point of rewriting at save time.
func TestAbsolutize_UsesTheClockItIsGiven(t *testing.T) {
	next := wed.AddDate(0, 0, 1)
	if a, b := Absolutize("Demo tomorrow.", wed), Absolutize("Demo tomorrow.", next); a == b {
		t.Errorf("the date must follow the clock: %q vs %q", a, b)
	}
}
