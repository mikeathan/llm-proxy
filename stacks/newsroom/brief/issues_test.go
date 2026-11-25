package brief

import (
	"errors"
	"testing"
	"time"
)

// The owner deletes a brief, or every brief of that edition older than the one on screen. A deleted brief is not written again for its slot (the
// scheduler asks Made), and its stories stay in the already-shown record, so the next edition does not repeat them.
func TestStore_DeleteAnIssueOrTheOlderOnesOfItsEdition(t *testing.T) {
	s, dir, t0 := twoEditions(t)
	slot := func(i int) time.Time { return t0.Add(time.Duration(i) * 24 * time.Hour) }
	for i, in := range []Issue{{EditionID: morning}, {EditionID: evening}, {EditionID: morning}, {EditionID: morning}} {
		in.MadeAt, in.ReadyFor = slot(i), slot(i)
		in.Sections = []IssueSection{{Stories: []Story{{Key: "story-" + string(rune('a'+i))}}}}
		if _, err := s.SaveIssue(in, in.MadeAt); err != nil {
			t.Fatal(err)
		}
	}
	shownBefore := len(s.Snapshot().Shown)
	if n, err := s.DeleteIssues(4, true); err != nil || n != 2 {
		t.Fatalf("older than Morning 4 are Morning 1 and 3: deleted %d, %v", n, err)
	}
	if got := ids(s.Issues(0)); len(got) != 2 || got[0] != 4 || got[1] != 2 {
		t.Fatalf("left: %v, want [4 2] (Evening is another edition)", got)
	}
	if n, err := s.DeleteIssues(4, false); err != nil || n != 1 {
		t.Fatalf("delete one: %d, %v", n, err)
	}
	if _, err := s.DeleteIssues(99, false); !errors.Is(err, ErrNotFound) {
		t.Errorf("an unknown issue is not found: %v", err)
	}
	for _, i := range []int{0, 2, 3} {
		if !s.Made(morning, slot(i)) {
			t.Errorf("the deleted Morning of slot %d still counts as made, or the scheduler writes it again", i)
		}
	}
	if got := len(s.Snapshot().Shown); got != shownBefore {
		t.Errorf("the already-shown record keeps every story: %d, want %d", got, shownBefore)
	}
	again, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Issues(0)) != 1 || !again.Made(morning, slot(3)) {
		t.Errorf("the deletion and the made record survive a restart")
	}
}

func ids(in []Issue) []int64 {
	out := make([]int64, len(in))
	for i, is := range in {
		out[i] = is.ID
	}
	return out
}
