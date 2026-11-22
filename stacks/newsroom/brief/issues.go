package brief

import (
	"slices"
	"strings"
	"time"
)

// What is kept of the editions already made, so nothing grows without limit.
const (
	IssueRetention = 30 * 24 * time.Hour
	MaxIssues      = 100
)

// Snapshot is what writing an edition reads, copied so the store's lock is not held while the model is asked.
type Snapshot struct {
	Topics   []Topic
	Editions []Edition
	Pool     []PoolItem
	Shown    []Shown
}

// Snapshot copies the topics, editions, pool and the record of what was shown.
func (s *Store) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Snapshot{Topics: slices.Clone(s.doc.Topics), Editions: slices.Clone(s.doc.Editions), Pool: slices.Clone(s.doc.Pool), Shown: slices.Clone(s.doc.Shown)}
}

// SaveIssue stores an edition as it was made, gives it its id, records every story in it as shown, and drops what is past its retention.
func (s *Store) SaveIssue(is Issue, now time.Time) (Issue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	is.ID = s.doc.NextIssue
	s.doc.NextIssue++
	s.doc.Issues = append(s.doc.Issues, is)
	s.doc.Shown = recordShown(s.doc.Shown, is, now.UTC())
	s.doc.Issues = pruneIssues(s.doc.Issues, now.UTC())
	return is, s.saveLocked()
}

// recordShown notes each story of the sections as shown now (a story already noted is replaced), and forgets what is older than the window.
func recordShown(shown []Shown, is Issue, now time.Time) []Shown {
	latest := map[string]Shown{}
	for _, sh := range shown {
		if now.Sub(sh.At) <= ShownWindow {
			latest[sh.Key] = sh
		}
	}
	for _, sec := range is.Sections {
		for _, st := range sec.Stories {
			latest[st.Key] = Shown{Key: st.Key, At: now, Outlets: len(st.Outlets)}
		}
	}
	out := make([]Shown, 0, len(latest))
	for _, sh := range latest {
		out = append(out, sh)
	}
	slices.SortFunc(out, func(a, b Shown) int { return strings.Compare(a.Key, b.Key) })
	return out
}

// pruneIssues drops editions made before the retention window, then the oldest while there are more than the cap.
func pruneIssues(in []Issue, now time.Time) []Issue {
	out := in[:0:0]
	for _, is := range in {
		if now.Sub(is.MadeAt) <= IssueRetention {
			out = append(out, is)
		}
	}
	if over := len(out) - MaxIssues; over > 0 {
		out = out[over:]
	}
	return out
}

// Issues returns the editions made, newest first; editionID limits it to one edition, and zero means all.
func (s *Store) Issues(editionID int64) []Issue {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Issue
	for i := len(s.doc.Issues) - 1; i >= 0; i-- {
		if is := s.doc.Issues[i]; editionID == 0 || is.EditionID == editionID {
			out = append(out, is)
		}
	}
	return out
}

// Issue returns one edition as it was made.
func (s *Store) Issue(id int64) (Issue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, is := range s.doc.Issues {
		if is.ID == id {
			return is, nil
		}
	}
	return Issue{}, ErrNotFound
}

// Made says whether the edition was already made for the ready time.
func (s *Store) Made(editionID int64, readyFor time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.ContainsFunc(s.doc.Issues, func(is Issue) bool { return is.EditionID == editionID && is.ReadyFor.Equal(readyFor) })
}
