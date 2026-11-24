package stackport

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"llm-proxy-stacks/sdk"
)

// The fixed sentences a stack is told when it cannot be given the day.
const (
	msgDayUnavailable = "Reminders is off or could not be read"
	dayTitle          = 160
	dayScheduled      = "scheduled"
)

// Day is the SDK's Day port over the Reminders stack's own list (ADR-0083): the owner's reminders that fall due on one day, as a time and the owner's own
// words. It reads through the host's in-process read route, never the stack's files, and keeps nothing from a reminder but its time, its words clipped to one
// line, and whether it repeats: no id, channel, connector, zone or error text crosses. Nothing here writes, sends or starts a reminder.
type Day struct {
	// Read answers the Reminders stack's list route: {"open":[...]}. It fails while the stack is off or not running.
	Read func(ctx context.Context) (json.RawMessage, error)
	// Zone is the owner's zone: the day starts and ends at its midnight and the times are told in it. Nil or a nil answer means UTC.
	Zone func() *time.Location
	Now  func() time.Time
}

// dayReminder is the part of a reminder the port reads.
type dayReminder struct {
	Message string `json:"message"`
	FireAt  string `json:"fire_at"`
	Cron    string `json:"cron"`
	Status  string `json:"status"`
}

// Items is the answer: the reminders still scheduled that fall due today (or tomorrow), earliest first, at most stacksdk.MaxDayItems. A repeating
// reminder is told by its next time, so it shows on the day that time falls on. Reminders off, or an answer that cannot be read, is CodeUnavailable and
// never an empty day.
func (d Day) Items(ctx context.Context, tomorrow bool) ([]stacksdk.DayItem, error) {
	open, ok := d.open(ctx)
	if !ok {
		return nil, stacksdk.Errorf(stacksdk.CodeUnavailable, msgDayUnavailable)
	}
	loc, start := d.window(tomorrow)
	end := start.AddDate(0, 0, 1)
	type dated struct {
		at   time.Time
		item stacksdk.DayItem
	}
	var found []dated
	for _, r := range open {
		if at, item, ok := r.on(start, end, loc); ok {
			found = append(found, dated{at, item})
		}
	}
	slices.SortStableFunc(found, func(a, b dated) int { return a.at.Compare(b.at) })
	out := make([]stacksdk.DayItem, 0, min(len(found), stacksdk.MaxDayItems))
	for _, f := range found[:min(len(found), stacksdk.MaxDayItems)] {
		out = append(out, f.item)
	}
	return out, nil
}

// open reads the Reminders stack's open list; false when the stack cannot answer or the answer is not a list.
func (d Day) open(ctx context.Context) ([]dayReminder, bool) {
	if d.Read == nil {
		return nil, false
	}
	raw, err := d.Read(ctx)
	if err != nil {
		return nil, false
	}
	var list struct {
		Open []dayReminder `json:"open"`
	}
	if json.Unmarshal(raw, &list) != nil {
		return nil, false
	}
	return list.Open, true
}

// window is the owner's zone and the start of the day asked about in it.
func (d Day) window(tomorrow bool) (*time.Location, time.Time) {
	loc := time.UTC
	if d.Zone != nil {
		if l := d.Zone(); l != nil {
			loc = l
		}
	}
	now := time.Now()
	if d.Now != nil {
		now = d.Now()
	}
	now = now.In(loc)
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	if tomorrow {
		start = start.AddDate(0, 0, 1)
	}
	return loc, start
}

// on is the part of a reminder the port tells, when it is still scheduled and its next time falls in [start, end).
func (r dayReminder) on(start, end time.Time, loc *time.Location) (time.Time, stacksdk.DayItem, bool) {
	at, err := time.Parse(time.RFC3339, r.FireAt)
	title := oneLine(r.Message, dayTitle)
	if r.Status != dayScheduled || err != nil || title == "" || at.Before(start) || !at.Before(end) {
		return time.Time{}, stacksdk.DayItem{}, false
	}
	return at, stacksdk.DayItem{Clock: at.In(loc).Format("15:04"), Title: title, Repeats: r.Cron != ""}, true
}
