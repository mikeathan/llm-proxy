package brief

import (
	"errors"
	"testing"
	"time"
)

// The topic takes id 1, so the two editions are 2 (Morning) and 3 (Evening).
const morning, evening = 2, 3

// twoEditions makes a store with the topic and the two editions.
func twoEditions(t *testing.T) (*Store, *memDir, time.Time) {
	t.Helper()
	s, dir := newStore(t)
	t0 := time.Date(2026, 11, 20, 7, 0, 0, 0, time.UTC)
	topic, err := s.CreateTopic(worldTopic, t0)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Morning", "Evening"} {
		if _, err := s.CreateEdition(Edition{Name: name, ReadyBy: "07:00", Enabled: true, Sections: []Section{{Kind: SectionNews, TopicID: topic.ID}}}, t0); err != nil {
			t.Fatal(err)
		}
	}
	return s, dir, t0
}

func TestStore_LatestIsTheNewestEditionOfEachAndItsReadState(t *testing.T) {
	s, _, t0 := twoEditions(t)
	for i, in := range []Issue{{EditionID: morning, Name: "Morning"}, {EditionID: evening, Name: "Evening"}, {EditionID: morning, Name: "Morning"}} {
		in.MadeAt = t0.Add(time.Duration(i) * time.Hour)
		if _, err := s.SaveIssue(in, in.MadeAt); err != nil {
			t.Fatal(err)
		}
	}
	got := s.Latest()
	if len(got) != 2 || got[0].EditionID != morning || got[0].IssueID != 3 || got[1].EditionID != evening || got[1].IssueID != 2 {
		t.Fatalf("Latest = %+v, want Morning at issue 3 and Evening at issue 2", got)
	}
	if got[0].Read || got[1].Read {
		t.Fatalf("a new edition must be unread: %+v", got)
	}
}

func TestStore_MarkReadIsKeptAcrossARestartAndANewerIssueIsUnreadAgain(t *testing.T) {
	s, dir, t0 := twoEditions(t)
	if _, err := s.SaveIssue(Issue{EditionID: morning, Name: "Morning", MadeAt: t0}, t0); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkRead(1, t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkRead(1, t0.Add(time.Hour)); err != nil {
		t.Fatalf("reading twice must be fine: %v", err)
	}
	back, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if l := back.Latest(); len(l) != 1 || !l[0].Read {
		t.Fatalf("read state lost over a restart: %+v", l)
	}
	if is, _ := back.Issue(1); !is.ReadAt.Equal(t0.Add(time.Minute)) {
		t.Fatalf("the first reading time must stay: %v", is.ReadAt)
	}
	if _, err := back.SaveIssue(Issue{EditionID: morning, Name: "Morning", MadeAt: t0.Add(24 * time.Hour)}, t0.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if l := back.Latest(); len(l) != 1 || l[0].IssueID != 2 || l[0].Read {
		t.Fatalf("the next edition must be unread: %+v", l)
	}
}

func TestStore_MarkReadOfAnUnknownIssueIsNotFound(t *testing.T) {
	s, _, _ := twoEditions(t)
	if err := s.MarkRead(9, time.Now()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}

func TestStore_ADeletedEditionLeavesNothingToRead(t *testing.T) {
	s, _, t0 := twoEditions(t)
	if _, err := s.SaveIssue(Issue{EditionID: evening, Name: "Evening", MadeAt: t0}, t0); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteEdition(evening); err != nil {
		t.Fatal(err)
	}
	if l := s.Latest(); len(l) != 0 {
		t.Fatalf("an edition that is gone has no latest issue: %+v", l)
	}
}
