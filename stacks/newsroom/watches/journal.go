package watches

import (
	"sort"
	"strings"
	"time"

	"llm-proxy-stacks/newsroom/sources"
)

// The feed's bounds: the journal is the record the Newsroom tab reads, so it is kept small.
const (
	JournalRetention = 30 * 24 * time.Hour
	MaxEntries       = 200 // per watch
	entryTextRunes   = 300
	// LeftOutRetention and MaxLeftOut bound the items the model left out (kept aside so the owner can check its calls): they come every run, so they
	// have their own, smaller bound and never push a real finding out.
	LeftOutRetention = 7 * 24 * time.Hour
	MaxLeftOut       = 50 // per watch
)

// Entry is one finding in a watch's journal: what was found, when, and whether the owner has read it.
type Entry struct {
	ID      int64     `json:"id"`
	WatchID int64     `json:"watch_id"`
	At      time.Time `json:"at"`
	// Posted is when the source says the post was published; zero when it gave no date. At is when the watch found it.
	Posted time.Time `json:"posted,omitzero"`
	Ident  string    `json:"ident"`
	Source string    `json:"source"`
	Title  string    `json:"title"`
	Link   string    `json:"link,omitempty"`
	Text   string    `json:"text,omitempty"`
	// Was and Now are the changed fact, when the model step found one.
	Was string `json:"was,omitempty"`
	Now string `json:"now,omitempty"`
	// Why is the model's short reason the entry matches what the watch looks for.
	Why string `json:"why,omitempty"`
	// Headline and Summary are the model's own short headline and one-or-two-sentence summary. The row shows the headline in place of the original
	// Title, which is kept (with Link and Text) and shown when the entry is opened.
	Headline string `json:"headline,omitempty"`
	Summary  string `json:"summary,omitempty"`
	// First marks an entry shown from the watch's first check: it was already there, so it was not announced.
	First bool `json:"first,omitempty"`
	// AlsoSeen lists later reports of the same event under another source or link, folded here instead of shown as new entries.
	AlsoSeen []AlsoSeen `json:"also_seen,omitempty"`
	// Sent is true once the notice carrying it was delivered through the watch's channel; a feed-only watch is never "sent".
	Sent bool `json:"sent,omitempty"`
	Read bool `json:"read,omitempty"`
	// LeftOut marks an item the model judged not to be what the watch looks for: kept aside (Why is the model's reason it was left out), out of the
	// feed and its unread count, until the owner keeps it (Keep) or it ages out.
	LeftOut bool `json:"left_out,omitempty"`
}

// Changed says whether the entry reports a changed fact rather than something new.
func (e Entry) Changed() bool { return e.Was != "" && e.Now != "" }

// AlsoSeen is one repeat folded under an entry.
type AlsoSeen struct {
	Source string    `json:"source"`
	Link   string    `json:"link,omitempty"`
	At     time.Time `json:"at"`
}

// AddEntries puts a run's findings in the watch's journal, once each: an item already there (the same identity) is updated, not duplicated, so a run
// that is retried after a failed send does not double the feed. marks carries what the run knows about each item. Old entries past the
// retention window go and each watch keeps its newest MaxEntries.
func (s *Store) AddEntries(watchID int64, items []sources.Item, marks Marks, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.index(watchID) < 0 {
		return ErrNotFound
	}
	have := map[string]int{}
	for i, e := range s.doc.Entries {
		if e.WatchID == watchID {
			have[e.Ident] = i
		}
	}
	for i, it := range items {
		c, d := marks.Changes[i], marks.Digests[i]
		if at, ok := have[it.Identity()]; ok {
			s.doc.Entries[at].Was, s.doc.Entries[at].Now = c.Was, c.Now
			if d.Headline != "" || d.Summary != "" {
				s.doc.Entries[at].Headline, s.doc.Entries[at].Summary = d.Headline, d.Summary
			}
			// A retried run may judge relevant what an earlier try set aside: it joins the feed with the new reason.
			if s.doc.Entries[at].LeftOut && !marks.LeftOut {
				s.doc.Entries[at].LeftOut, s.doc.Entries[at].Why = false, marks.Whys[i]
			}
			continue
		}
		s.doc.Entries = append(s.doc.Entries, Entry{
			ID: s.doc.NextEntry, WatchID: watchID, At: now.UTC(), Posted: it.At.UTC(), Ident: it.Identity(), Source: it.Source,
			Title: sources.Line(it.Title, MaxTitleRunes), Link: it.Link, Text: sources.Plain(it.Text, entryTextRunes), Was: c.Was, Now: c.Now,
			Why: marks.Whys[i], Headline: d.Headline, Summary: d.Summary, First: marks.First, LeftOut: marks.LeftOut,
		})
		s.doc.NextEntry++
	}
	s.pruneEntries(watchID, now)
	return s.saveLocked()
}

// Marks is what a run knows about the items it adds, by index into the items: a changed fact, why one matched, and whether the run was the first
// check of the watch, or that the items were left out by the model.
type Marks struct {
	Changes map[int]Change
	Whys    map[int]string
	Digests map[int]Digest
	First   bool
	LeftOut bool
}

// MaxTitleRunes bounds an entry's title.
const MaxTitleRunes = 200

// pruneEntries drops the entries past their retention (the journal's, or the shorter one of an item left out), then caps the watch's findings and its
// left-out items, each group on its own.
func (s *Store) pruneEntries(watchID int64, now time.Time) {
	kept := s.doc.Entries[:0]
	for _, e := range s.doc.Entries {
		keep := JournalRetention
		if e.LeftOut {
			keep = LeftOutRetention
		}
		if e.At.After(now.Add(-keep)) {
			kept = append(kept, e)
		}
	}
	s.doc.Entries = kept
	s.capEntries(watchID, false, MaxEntries)
	s.capEntries(watchID, true, MaxLeftOut)
}

// capEntries keeps the newest max entries of one watch's findings (leftOut false) or of its left-out items (leftOut true).
func (s *Store) capEntries(watchID int64, leftOut bool, max int) {
	var mine []int
	for i, e := range s.doc.Entries {
		if e.WatchID == watchID && e.LeftOut == leftOut {
			mine = append(mine, i)
		}
	}
	if len(mine) <= max {
		return
	}
	sort.Slice(mine, func(a, b int) bool { return s.doc.Entries[mine[a]].At.Before(s.doc.Entries[mine[b]].At) })
	drop := map[int]bool{}
	for _, i := range mine[:len(mine)-max] {
		drop[i] = true
	}
	out := s.doc.Entries[:0]
	for i, e := range s.doc.Entries {
		if !drop[i] {
			out = append(out, e)
		}
	}
	s.doc.Entries = out
}

// MarkSent flags the entries of the given identities as delivered through the watch's channel.
func (s *Store) MarkSent(watchID int64, idents []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	want := map[string]bool{}
	for _, id := range idents {
		want[id] = true
	}
	for i, e := range s.doc.Entries {
		if e.WatchID == watchID && want[e.Ident] {
			s.doc.Entries[i].Sent = true
		}
	}
	return s.saveLocked()
}

// FoldRepeat records a repeat under the newest entry of the watch whose title is the one the repeat cites. It returns false when no entry has that
// title (the cited title was older than the journal); the repeat is still recorded as seen by the caller.
func (s *Store) FoldRepeat(watchID int64, citedTitle string, also AlsoSeen) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	target := -1
	for i, e := range s.doc.Entries {
		if e.WatchID == watchID && sources.Line(e.Title, 100) == citedTitle && (target < 0 || e.At.After(s.doc.Entries[target].At)) {
			target = i
		}
	}
	if target < 0 {
		return false, nil
	}
	e := &s.doc.Entries[target]
	for _, a := range e.AlsoSeen {
		if a.Link != "" && a.Link == also.Link {
			return true, nil
		}
	}
	if len(e.AlsoSeen) < 10 {
		e.AlsoSeen = append(e.AlsoSeen, also)
	}
	return true, s.saveLocked()
}

// MarkRead sets the read flag of the given entries.
func (s *Store) MarkRead(ids []int64, read bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	want := map[int64]bool{}
	for _, id := range ids {
		want[id] = true
	}
	for i, e := range s.doc.Entries {
		if want[e.ID] {
			s.doc.Entries[i].Read = read
		}
	}
	return s.saveLocked()
}

// Keep puts items the model left out into the feed: the owner says the call was wrong. They are marked read, because the owner just looked at them.
func (s *Store) Keep(ids []int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	want := map[int64]bool{}
	for _, id := range ids {
		want[id] = true
	}
	for i, e := range s.doc.Entries {
		if want[e.ID] && e.LeftOut {
			s.doc.Entries[i].LeftOut, s.doc.Entries[i].Read = false, true
		}
	}
	return s.saveLocked()
}

// LeftOutCount is how many items the model left out are kept, for every watch that exists.
func (s *Store) LeftOutCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, e := range s.doc.Entries {
		if e.LeftOut && s.index(e.WatchID) >= 0 {
			n++
		}
	}
	return n
}

// UnreadCount is how many feed entries of the watches that exist are unread; items left out are not in the feed and do not count.
func (s *Store) UnreadCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, e := range s.doc.Entries {
		if !e.Read && !e.LeftOut && s.index(e.WatchID) >= 0 {
			n++
		}
	}
	return n
}

// DeleteEntries removes the given entries from the journal for good and reports how many went. The seen ledger is separate, so an item cleared here is
// not found again from the same source.
func (s *Store) DeleteEntries(ids []int64) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	want := map[int64]bool{}
	for _, id := range ids {
		want[id] = true
	}
	return s.dropLocked(func(e Entry) bool { return want[e.ID] })
}

// ClearEntries removes every feed entry (readOnly false) or every read one (readOnly true) for good, and reports how many went. The items the model left
// out are not in the feed, so clearing it leaves them; they age out on their own.
func (s *Store) ClearEntries(readOnly bool) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dropLocked(func(e Entry) bool { return !e.LeftOut && (!readOnly || e.Read) })
}

// dropLocked removes the entries drop says yes to and saves when any went. The caller holds s.mu.
func (s *Store) dropLocked(drop func(Entry) bool) (int, error) {
	kept := s.doc.Entries[:0]
	gone := 0
	for _, e := range s.doc.Entries {
		if drop(e) {
			gone++
			continue
		}
		kept = append(kept, e)
	}
	s.doc.Entries = kept
	if gone == 0 {
		return 0, nil
	}
	return gone, s.saveLocked()
}

// Entries returns a copy of the journal entries that match f, newest first.
func (s *Store) Entries(f Filter) []Entry { return s.entriesFor(f) }

// entriesFor returns a copy of the journal entries that match f, newest first.
func (s *Store) entriesFor(f Filter) []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Entry
	for _, e := range s.doc.Entries {
		if e.LeftOut != f.LeftOut || (f.WatchID != 0 && e.WatchID != f.WatchID) {
			continue
		}
		if f.Source != "" && !strings.EqualFold(e.Source, f.Source) {
			continue
		}
		if f.UnreadOnly && e.Read {
			continue
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].At.Equal(out[j].At) {
			return out[i].At.After(out[j].At)
		}
		return out[i].ID > out[j].ID
	})
	return out
}
