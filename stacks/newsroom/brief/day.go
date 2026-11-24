package brief

import (
	"context"

	"llm-proxy-stacks/sdk"
)

// DayBlock is the "Your day" part of an issue: the owner's reminders for one day as the core's Day port gave them (ADR-0083). Unavailable says Reminders was off
// or could not be read, which is not the same as a quiet day; the screen says which.
type DayBlock struct {
	Tomorrow    bool               `json:"tomorrow,omitempty"`
	Unavailable bool               `json:"unavailable,omitempty"`
	Items       []stacksdk.DayItem `json:"items"`
}

// NoonHour is the hour from which an edition shows the next day: an evening brief is about tomorrow, a morning one about today.
const NoonHour = 12

// ShowsTomorrow says whether the edition's "Your day" is about tomorrow: it is ready at or after noon.
func (e Edition) ShowsTomorrow() bool {
	m := readyByRe.FindStringSubmatch(e.ReadyBy)
	return m != nil && atoi(m[1]) >= NoonHour
}

// dayBlock reads the Day port for the edition. A port that fails is a block that says so; it never fails the edition. The answer is kept as it came, bounded
// to stacksdk.MaxDayItems, and goes nowhere but the stored issue.
func dayBlock(ctx context.Context, read func(ctx context.Context, tomorrow bool) ([]stacksdk.DayItem, error), e Edition) *DayBlock {
	tomorrow := e.ShowsTomorrow()
	items, err := read(ctx, tomorrow)
	if err != nil {
		return &DayBlock{Tomorrow: tomorrow, Unavailable: true, Items: []stacksdk.DayItem{}}
	}
	if items == nil {
		items = []stacksdk.DayItem{}
	}
	return &DayBlock{Tomorrow: tomorrow, Items: items[:min(len(items), stacksdk.MaxDayItems)]}
}
