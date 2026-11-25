package brief

import (
	"slices"
	"strings"
	"time"

	"llm-proxy-stacks/sdk"
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
	return Snapshot{Topics: slices.Clone(s.doc.Topics), Editions: slices.Clone(s.doc.Editions), Pool: slices.Clone(s.pool), Shown: slices.Clone(s.doc.Shown)}
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
	s.doc.Deleted = pruneDeleted(s.doc.Deleted, now.UTC())
	return is, s.saveLocked()
}

// recordShown notes each story of the sections as shown now (a story already noted is replaced), and forgets what is older than the window.
func recordShown(shown []Shown, is Issue, now time.Time) []Shown {
	latest := map[string]Shown{}
	for _, sh := range shown {
		if now.Sub(sh.At) <= MaxRepeatAfter { // kept for the longest window an edition may ask for
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

// IssueSummary is one written edition in a list: enough to pick it, never its stories.
type IssueSummary struct {
	ID        int64     `json:"id"`
	EditionID int64     `json:"edition_id"`
	MadeAt    time.Time `json:"made_at"`
	Read      bool      `json:"read"`
}

// Summary is the issue as a list shows it.
func (is Issue) Summary() IssueSummary {
	return IssueSummary{ID: is.ID, EditionID: is.EditionID, MadeAt: is.MadeAt, Read: !is.ReadAt.IsZero()}
}

// LatestIssue is the newest edition made for one edition, and whether it was read: what a tab's dot and the dashboard card ask, without the issue itself.
type LatestIssue struct {
	EditionID int64     `json:"edition_id"`
	Name      string    `json:"name"`
	IssueID   int64     `json:"issue_id"`
	MadeAt    time.Time `json:"made_at"`
	Read      bool      `json:"read"`
	// Critical lists the ids of the critical home facts of that issue (ADR-0084): fixed ids, never a sentence.
	Critical []string `json:"critical,omitempty"`
}

// Latest returns the newest issue of each edition that still exists, in edition id order. Older unread issues do not count: a morning paper missed
// is not a pile of papers owed.
func (s *Store) Latest() []LatestIssue {
	s.mu.Lock()
	defer s.mu.Unlock()
	newest := map[int64]Issue{}
	for _, is := range s.doc.Issues {
		if cur, ok := newest[is.EditionID]; !ok || is.ID > cur.ID {
			newest[is.EditionID] = is
		}
	}
	var out []LatestIssue
	for _, e := range s.doc.Editions {
		if is, ok := newest[e.ID]; ok {
			out = append(out, LatestIssue{EditionID: e.ID, Name: e.Name, IssueID: is.ID, MadeAt: is.MadeAt, Read: !is.ReadAt.IsZero(), Critical: criticalIDs(is.Home)})
		}
	}
	return out
}

func criticalIDs(home []stacksdk.HomeLine) []string {
	var out []string
	for _, h := range home {
		if h.Level == stacksdk.HomeLevelCritical {
			out = append(out, h.ID)
		}
	}
	return out
}

// MarkRead notes that the owner read the issue. Reading it again keeps the first time.
func (s *Store) MarkRead(id int64, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := slices.IndexFunc(s.doc.Issues, func(is Issue) bool { return is.ID == id })
	if i < 0 {
		return ErrNotFound
	}
	if !s.doc.Issues[i].ReadAt.IsZero() {
		return nil
	}
	s.doc.Issues[i].ReadAt = now.UTC()
	return s.saveLocked()
}

// Made says whether the edition was already made for the ready time, including a brief made then and deleted by the owner.
func (s *Store) Made(editionID int64, readyFor time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.ContainsFunc(s.doc.Issues, func(is Issue) bool { return is.EditionID == editionID && is.ReadyFor.Equal(readyFor) }) ||
		slices.ContainsFunc(s.doc.Deleted, func(d DeletedSlot) bool { return d.EditionID == editionID && d.ReadyFor.Equal(readyFor) })
}

// DeletedSlot is the slot of a brief the owner deleted: the edition and the ready time it was made for.
type DeletedSlot struct {
	EditionID int64     `json:"edition_id"`
	ReadyFor  time.Time `json:"ready_for"`
}

// DeleteIssues deletes one brief (older false) or every brief of the same edition made before it (older true, the one named stays), and reports how
// many went. The already-shown record is kept, so the next edition does not repeat their stories, and the slot of each is remembered, so the scheduler
// does not write it again.
func (s *Store) DeleteIssues(id int64, older bool) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := slices.IndexFunc(s.doc.Issues, func(is Issue) bool { return is.ID == id })
	if i < 0 {
		return 0, ErrNotFound
	}
	named := s.doc.Issues[i]
	drop := func(is Issue) bool {
		if older {
			return is.EditionID == named.EditionID && is.ID != named.ID && is.MadeAt.Before(named.MadeAt)
		}
		return is.ID == named.ID
	}
	kept := s.doc.Issues[:0:0]
	gone := 0
	for _, is := range s.doc.Issues {
		if !drop(is) {
			kept = append(kept, is)
			continue
		}
		gone++
		if !is.ReadyFor.IsZero() {
			s.doc.Deleted = append(s.doc.Deleted, DeletedSlot{EditionID: is.EditionID, ReadyFor: is.ReadyFor})
		}
	}
	if gone == 0 {
		return 0, nil
	}
	s.doc.Issues = kept
	return gone, s.saveLocked()
}

// pruneDeleted forgets the deleted slots past the issue retention: the scheduler never looks that far back.
func pruneDeleted(in []DeletedSlot, now time.Time) []DeletedSlot {
	return slices.DeleteFunc(in, func(d DeletedSlot) bool { return now.Sub(d.ReadyFor) > IssueRetention })
}
