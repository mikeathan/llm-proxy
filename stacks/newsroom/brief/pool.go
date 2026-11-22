package brief

import (
	"slices"
	"strings"
	"time"
	"unicode"

	"llm-proxy-stacks/newsroom/sources"
)

// The pool's bounds: a story stays three days, and a hard cap keeps a runaway source from growing the file.
const (
	PoolRetention = 3 * 24 * time.Hour
	MaxPool       = 3000
	maxKeyRunes   = 160
	maxReports    = 12
)

// Report is one outlet's report of a story: who, where, and when it was posted.
type Report struct {
	Source    string    `json:"source"`
	Publisher string    `json:"publisher,omitempty"`
	Link      string    `json:"link,omitempty"`
	At        time.Time `json:"at,omitzero"`
}

// Outlet is the name a report is counted under: the publisher when the source names one, else the source.
func (r Report) Outlet() string {
	if r.Publisher != "" {
		return r.Publisher
	}
	return r.Source
}

// PoolItem is one story collected for one or more topics. The same story from several outlets is one item with several Reports; that is what lets an
// edition say "reported by 3 sources". Key is the normalised title, because a news search gives no stable publisher link to compare.
type PoolItem struct {
	ID        int64     `json:"id"`
	Key       string    `json:"key"`
	Title     string    `json:"title"`
	Text      string    `json:"text,omitempty"`
	Link      string    `json:"link,omitempty"`
	Posted    time.Time `json:"posted,omitzero"`
	FirstSeen time.Time `json:"first_seen"`
	Topics    []int64   `json:"topics"`
	Reports   []Report  `json:"reports"`
}

// HeadlineOnly says the story has no text from any source, so nothing but its headline may be shown or summarised.
func (p PoolItem) HeadlineOnly() bool { return p.Text == "" }

// Outlets is how many different outlets reported the story.
func (p PoolItem) Outlets() int {
	seen := map[string]bool{}
	for _, r := range p.Reports {
		seen[r.Outlet()] = true
	}
	return len(seen)
}

// TitleKey reduces a headline to its identity: lower case, letters and digits only, words joined by one space, cut to a bound. Two outlets' wording of
// the same headline, differing in quotes or punctuation, give one key.
func TitleKey(title string) string {
	var b strings.Builder
	space := true
	for _, r := range strings.ToLower(title) {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			b.WriteRune(r)
			space = false
		case !space:
			b.WriteByte(' ')
			space = true
		}
	}
	key := strings.TrimSpace(b.String())
	if r := []rune(key); len(r) > maxKeyRunes {
		key = strings.TrimSpace(string(r[:maxKeyRunes]))
	}
	return key
}

// ignored says whether the headline or text holds one of the topic's ignore words.
func ignored(it sources.Item, words []string) bool {
	if len(words) == 0 {
		return false
	}
	hay := strings.ToLower(it.Title + " " + it.Text)
	return slices.ContainsFunc(words, func(w string) bool { return strings.Contains(hay, w) })
}

// poolIndex finds a story already in the pool by its title key or its link.
type poolIndex struct{ byKey, byLink map[string]int }

func indexPool(pool []PoolItem) poolIndex {
	ix := poolIndex{byKey: map[string]int{}, byLink: map[string]int{}}
	for i, p := range pool {
		ix.add(i, p.Key, p.Link)
	}
	return ix
}

func (ix poolIndex) add(i int, key, link string) {
	ix.byKey[key] = i
	if link != "" {
		ix.byLink[link] = i
	}
}

func (ix poolIndex) find(key, link string) (int, bool) {
	if i, ok := ix.byKey[key]; ok {
		return i, true
	}
	i, ok := ix.byLink[link]
	return i, ok && link != ""
}

// wanted says whether a collected item may enter the pool: it has a headline, the topic does not ignore it and it was not posted before the retention window.
func wanted(it sources.Item, key string, topic Topic, now time.Time) bool {
	return key != "" && !ignored(it, topic.Ignore) && (it.At.IsZero() || now.Sub(it.At) <= PoolRetention)
}

// absorb adds one more report of a story already in the pool: the topic, text a headline-only story lacked, an earlier posting time, and the report itself
// unless that outlet's link is already there.
func (p *PoolItem) absorb(topicID int64, it sources.Item, report Report) {
	if !slices.Contains(p.Topics, topicID) {
		p.Topics = append(p.Topics, topicID)
	}
	if p.Text == "" {
		p.Text = it.Text
	}
	if p.Posted.IsZero() || (!it.At.IsZero() && it.At.Before(p.Posted)) {
		p.Posted = it.At
	}
	same := func(r Report) bool { return r.Outlet() == report.Outlet() && r.Link == report.Link }
	if len(p.Reports) < maxReports && !slices.ContainsFunc(p.Reports, same) {
		p.Reports = append(p.Reports, report)
	}
}

// merge puts a topic's collected items into the pool: a story already there (the same title key, or the same link) gains the topic and the report, a
// new one is added. Items the topic ignores, with no title or posted before the retention window are dropped. It returns the pool, the next id and how
// many stories were new. The pool is then pruned (prune).
func merge(pool []PoolItem, nextID int64, topic Topic, items []sources.Item, now time.Time) ([]PoolItem, int64, int) {
	ix := indexPool(pool)
	added := 0
	for _, it := range items {
		key := TitleKey(it.Title)
		if !wanted(it, key, topic, now) {
			continue
		}
		report := Report{Source: it.Source, Publisher: it.Publisher, Link: it.Link, At: it.At}
		if i, found := ix.find(key, it.Link); found {
			pool[i].absorb(topic.ID, it, report)
			continue
		}
		pool = append(pool, PoolItem{ID: nextID, Key: key, Title: it.Title, Text: it.Text, Link: it.Link, Posted: it.At, FirstSeen: now, Topics: []int64{topic.ID}, Reports: []Report{report}})
		ix.add(len(pool)-1, key, it.Link)
		nextID, added = nextID+1, added+1
	}
	return prune(pool, now), nextID, added
}

// prune drops stories first seen before the retention window, then the oldest while the pool is over its cap. The order of what stays is kept.
func prune(pool []PoolItem, now time.Time) []PoolItem {
	kept := pool[:0:0]
	for _, p := range pool {
		if now.Sub(p.FirstSeen) <= PoolRetention {
			kept = append(kept, p)
		}
	}
	if over := len(kept) - MaxPool; over > 0 {
		sorted := slices.Clone(kept)
		slices.SortStableFunc(sorted, func(a, b PoolItem) int { return a.FirstSeen.Compare(b.FirstSeen) })
		drop := map[int64]bool{}
		for _, p := range sorted[:over] {
			drop[p.ID] = true
		}
		kept = slices.DeleteFunc(kept, func(p PoolItem) bool { return drop[p.ID] })
	}
	return kept
}
