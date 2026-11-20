package topics

import (
	"sort"
	"time"
)

// Filter narrows the feed. The zero value shows everything.
type Filter struct {
	TopicID    int64
	Source     string
	UnreadOnly bool
}

// TopicDay is one topic's findings on one day. The newest finding is Current and is shown first; every earlier finding of the day stays visible
// beneath it, newest first, so a topic that found something five times today reads as one card with five entries, not as five cards.
type TopicDay struct {
	TopicID int64   `json:"topic_id"`
	Topic   string  `json:"topic"`
	Updates int     `json:"updates"`
	Unread  int     `json:"unread"`
	Current Entry   `json:"current"`
	Earlier []Entry `json:"earlier,omitempty"`
	// LastAt is when the newest finding of the day arrived.
	LastAt time.Time `json:"last_at"`
}

// DayCard is one day of the feed: its topics, the busiest recent first.
type DayCard struct {
	Day     string     `json:"day"` // 2006-01-02 in the owner's time zone
	Label   string     `json:"label"`
	Updates int        `json:"updates"`
	Unread  int        `json:"unread"`
	Topics  []TopicDay `json:"topics"`
}

// Feed groups the journal by day (in loc, the owner's time zone), then by topic, newest day first and, inside a day, the topic that found something
// most recently first. At most maxDays days are returned (zero means all). A day with no matching entry is not shown: a run that found nothing
// leaves no trace here, only the topic's last-checked time.
func (s *Store) Feed(now time.Time, loc *time.Location, maxDays int, f Filter) []DayCard {
	if loc == nil {
		loc = time.UTC
	}
	names := map[int64]string{}
	for _, t := range s.List() {
		names[t.ID] = t.Name
	}
	type key struct {
		day   string
		topic int64
	}
	groups := map[key][]Entry{}
	var days []string
	seen := map[string]bool{}
	for _, e := range s.entriesFor(f) { // newest first
		if _, ok := names[e.TopicID]; !ok {
			continue
		}
		d := e.At.In(loc).Format("2006-01-02")
		if !seen[d] {
			seen[d] = true
			days = append(days, d)
		}
		k := key{d, e.TopicID}
		groups[k] = append(groups[k], e)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(days)))
	if maxDays > 0 && len(days) > maxDays {
		days = days[:maxDays]
	}
	today := now.In(loc).Format("2006-01-02")
	yesterday := now.In(loc).AddDate(0, 0, -1).Format("2006-01-02")
	out := []DayCard{} // never nil: an empty feed is the JSON [], not null, which the page reads with .length
	for _, d := range days {
		card := DayCard{Day: d, Label: dayLabel(d, today, yesterday, loc)}
		for k, es := range groups {
			if k.day != d {
				continue
			}
			td := topicDay(k.topic, names[k.topic], es)
			card.Updates += td.Updates
			card.Unread += td.Unread
			card.Topics = append(card.Topics, td)
		}
		sort.Slice(card.Topics, func(i, j int) bool { return card.Topics[i].LastAt.After(card.Topics[j].LastAt) })
		out = append(out, card)
	}
	return out
}

// topicDay is one topic's entries of one day, newest first: the newest is Current, the rest Earlier.
func topicDay(id int64, name string, es []Entry) TopicDay {
	td := TopicDay{TopicID: id, Topic: name, Updates: len(es), Current: es[0], Earlier: es[1:], LastAt: es[0].At}
	for _, e := range es {
		if !e.Read {
			td.Unread++
		}
	}
	return td
}

func dayLabel(day, today, yesterday string, loc *time.Location) string {
	switch day {
	case today:
		return "Today"
	case yesterday:
		return "Yesterday"
	}
	if t, err := time.ParseInLocation("2006-01-02", day, loc); err == nil {
		return t.Format("Mon 2 Jan")
	}
	return day
}
