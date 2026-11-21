package watches

import (
	"testing"
	"time"

	"llm-proxy-stacks/newsroom/sources"
)

// The feed can be narrowed to the last N calendar days in the owner's zone: Days 1 is today only, and Today counts from the zone's midnight.
func TestFeedPeriodKeepsOnlyTheLastCalendarDays(t *testing.T) {
	h := newHarness(t, feedWatch())
	now := h.now
	day := func(back int) time.Time { return now.AddDate(0, 0, -back) }
	for _, back := range []int{0, 1, 5, 12, 29} {
		if err := h.store.AddEntries(h.store.List()[0].ID, []sources.Item{{Source: "s", Title: "t", Link: "https://x.test/" + day(back).Format("20060102")}}, Marks{}, day(back)); err != nil {
			t.Fatal(err)
		}
	}
	count := func(days int) int {
		n := 0
		for _, c := range h.store.Feed(now, time.UTC, 0, Filter{Days: days}) {
			n += c.Updates
		}
		return n
	}
	for days, want := range map[int]int{0: 5, 1: 1, 2: 2, 7: 3, 14: 4, 30: 5} {
		if got := count(days); got != want {
			t.Errorf("Days %d: got %d entries, want %d", days, got, want)
		}
	}
}
