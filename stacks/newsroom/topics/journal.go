package topics

import (
	"sort"
	"strings"
	"time"

	"llm-proxy-stacks/newsroom/sources"
)

// The feed's bounds: the journal is the record the Newsroom tab reads, so it is kept small.
const (
	JournalRetention = 30 * 24 * time.Hour
	MaxEntries       = 200 // per topic
	entryTextRunes   = 300
)

// Entry is one finding in a topic's journal: what was found, when, and whether the owner has read it.
type Entry struct {
	ID      int64     `json:"id"`
	TopicID int64     `json:"topic_id"`
	At      time.Time `json:"at"`
	Ident   string    `json:"ident"`
	Source  string    `json:"source"`
	Title   string    `json:"title"`
	Link    string    `json:"link,omitempty"`
	Text    string    `json:"text,omitempty"`
	// Was and Now are the changed fact, when the model step found one.
	Was string `json:"was,omitempty"`
	Now string `json:"now,omitempty"`
	// AlsoSeen lists later reports of the same event under another source or link, folded here instead of shown as new entries.
	AlsoSeen []AlsoSeen `json:"also_seen,omitempty"`
	// Sent is true once the notice carrying it was delivered through the topic's channel; a feed-only topic is never "sent".
	Sent bool `json:"sent,omitempty"`
	Read bool `json:"read,omitempty"`
}

// Changed says whether the entry reports a changed fact rather than something new.
func (e Entry) Changed() bool { return e.Was != "" && e.Now != "" }

// AlsoSeen is one repeat folded under an entry.
type AlsoSeen struct {
	Source string    `json:"source"`
	Link   string    `json:"link,omitempty"`
	At     time.Time `json:"at"`
}

// AddEntries puts a run's findings in the topic's journal, once each: an item already there (the same identity) is updated, not duplicated, so a run
// that is retried after a failed send does not double the feed. changes maps an item's index in items to its changed fact. Old entries past the
// retention window go and each topic keeps its newest MaxEntries.
func (s *Store) AddEntries(topicID int64, items []sources.Item, changes map[int]Change, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.index(topicID) < 0 {
		return ErrNotFound
	}
	have := map[string]int{}
	for i, e := range s.doc.Entries {
		if e.TopicID == topicID {
			have[e.Ident] = i
		}
	}
	for i, it := range items {
		c := changes[i]
		if at, ok := have[it.Identity()]; ok {
			s.doc.Entries[at].Was, s.doc.Entries[at].Now = c.Was, c.Now
			continue
		}
		s.doc.Entries = append(s.doc.Entries, Entry{
			ID: s.doc.NextEntry, TopicID: topicID, At: now.UTC(), Ident: it.Identity(), Source: it.Source,
			Title: sources.Line(it.Title, MaxTitleRunes), Link: it.Link, Text: sources.Plain(it.Text, entryTextRunes), Was: c.Was, Now: c.Now,
		})
		s.doc.NextEntry++
	}
	s.pruneEntries(topicID, now)
	return s.saveLocked()
}

// MaxTitleRunes bounds an entry's title.
const MaxTitleRunes = 200

func (s *Store) pruneEntries(topicID int64, now time.Time) {
	cutoff := now.Add(-JournalRetention)
	kept := s.doc.Entries[:0]
	for _, e := range s.doc.Entries {
		if e.At.After(cutoff) {
			kept = append(kept, e)
		}
	}
	s.doc.Entries = kept
	var mine []int
	for i, e := range s.doc.Entries {
		if e.TopicID == topicID {
			mine = append(mine, i)
		}
	}
	if len(mine) <= MaxEntries {
		return
	}
	sort.Slice(mine, func(a, b int) bool { return s.doc.Entries[mine[a]].At.Before(s.doc.Entries[mine[b]].At) })
	drop := map[int]bool{}
	for _, i := range mine[:len(mine)-MaxEntries] {
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

// MarkSent flags the entries of the given identities as delivered through the topic's channel.
func (s *Store) MarkSent(topicID int64, idents []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	want := map[string]bool{}
	for _, id := range idents {
		want[id] = true
	}
	for i, e := range s.doc.Entries {
		if e.TopicID == topicID && want[e.Ident] {
			s.doc.Entries[i].Sent = true
		}
	}
	return s.saveLocked()
}

// FoldRepeat records a repeat under the newest entry of the topic whose title is the one the repeat cites. It returns false when no entry has that
// title (the cited title was older than the journal); the repeat is still recorded as seen by the caller.
func (s *Store) FoldRepeat(topicID int64, citedTitle string, also AlsoSeen) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	target := -1
	for i, e := range s.doc.Entries {
		if e.TopicID == topicID && sources.Line(e.Title, 100) == citedTitle && (target < 0 || e.At.After(s.doc.Entries[target].At)) {
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

// entriesFor returns a copy of the journal entries that match f, newest first.
func (s *Store) entriesFor(f Filter) []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Entry
	for _, e := range s.doc.Entries {
		if f.TopicID != 0 && e.TopicID != f.TopicID {
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
