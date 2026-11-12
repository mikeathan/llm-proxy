package dnscheck

import (
	"testing"
	"time"
)

var tallyNow = time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)

func TestTallyOf_CountsOnlyTheComparisonsOfThePeriod(t *testing.T) {
	items := []Comparison{
		{At: tallyNow.Add(-time.Hour), Outcome: Match},
		{At: tallyNow.Add(-2 * time.Hour), Outcome: Differ},
		{At: tallyNow.Add(-3 * time.Hour), Outcome: CannotCompare, Reason: ReasonRouterSilent},
		{At: tallyNow.Add(-25 * time.Hour), Outcome: Differ},
		{At: tallyNow.Add(time.Minute), Outcome: Differ},
	}
	got := TallyOf(items, tallyNow, RecentPeriod)
	if got.Done != 3 || got.Flagged != 1 || got.Unclear != 1 {
		t.Errorf("want 3 done, 1 differed, 1 unclear (a comparison outside the period or in the future is not counted): %+v", got)
	}
	if empty := TallyOf(nil, tallyNow, RecentPeriod); empty.Done != 0 {
		t.Errorf("an empty log counts nothing: %+v", empty)
	}
}

func TestCollectorRecent_IsZeroWhenNothingWasCheckedInThePeriodSoNoLineIsWritten(t *testing.T) {
	store := &memDNSStore{items: []Comparison{{At: tallyNow.Add(-48 * time.Hour), Outcome: Match}}}
	c := NewCollector(CollectorDeps{Store: store})
	if got, err := c.Recent(tallyNow); err != nil || got.Done != 0 {
		t.Errorf("old comparisons only: %v, %v", got, err)
	}
	store.items = append(store.items, Comparison{At: tallyNow.Add(-time.Hour), Outcome: Match})
	if got, _ := c.Recent(tallyNow); got.Done != 1 {
		t.Errorf("one recent comparison: %+v", got)
	}
}
