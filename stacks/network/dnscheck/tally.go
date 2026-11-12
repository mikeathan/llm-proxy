package dnscheck

import (
	"fmt"
	"time"

	"llm-proxy-stacks/sdk"
)

// RecentPeriod is how far back the tally the module reports looks.
const RecentPeriod = 24 * time.Hour

// TallyOf counts the comparisons made in the period ending at now: how many, how many found the router's answer differing from the outside
// resolver's, and how many could not be compared. It is counts only, so nothing a resolver answered can leave through it.
func TallyOf(items []Comparison, now time.Time, period time.Duration) stacksdk.Tally {
	var t stacksdk.Tally
	for _, c := range items {
		if c.At.Before(now.Add(-period)) || c.At.After(now) {
			continue
		}
		t.Done++
		switch c.Outcome {
		case Differ:
			t.Flagged++
		case CannotCompare:
			t.Unclear++
		}
	}
	return t
}

// Recent is the tally of the last RecentPeriod, read from the log. It is the zero value when the log holds no comparison in the period,
// so a check that is off or has not run reports no line rather than "0 checks".
func (c *Collector) Recent(now time.Time) (stacksdk.Tally, error) {
	items, err := c.deps.Store.Read()
	if err != nil {
		return stacksdk.Tally{}, fmt.Errorf("failed to read the DNS check log: %w", err)
	}
	return TallyOf(items, now, RecentPeriod), nil
}
