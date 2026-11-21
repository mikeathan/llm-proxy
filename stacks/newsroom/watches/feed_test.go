package watches

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"llm-proxy-stacks/newsroom/sources"
)

// The owner's scenario: a watch is read every hour, and five of the day's runs find something. The feed shows one card for the day, with the newest
// finding on top and the four earlier ones still in view; the runs that found nothing leave no trace.
func TestFiveFindingsInADayAreOneCard(t *testing.T) {
	watch := feedWatch()
	watch.Channel = "" // feed only
	h := newHarness(t, watch)
	var titles []string
	h.fetch.bodies[feedURL] = itemsRSS("seed")
	h.tick(t, 0) // 08:00 baseline
	h.forgetFirstFindings()
	findAt := map[int]bool{2: true, 3: true, 5: true, 6: true, 9: true}
	for hour := 1; hour <= 10; hour++ {
		if findAt[hour] {
			titles = append(titles, fmt.Sprintf("finding at %d", hour))
		}
		pairs := [][2]string{{"seed", "https://e.example/1"}}
		for i, ti := range titles {
			pairs = append(pairs, [2]string{ti, fmt.Sprintf("https://f.example/%d", i)})
		}
		h.fetch.bodies[feedURL] = rss(pairs...)
		h.tick(t, time.Hour)
	}
	cards := h.store.Feed(h.now, time.UTC, 0, Filter{})
	if len(cards) != 1 || len(cards[0].Watches) != 1 {
		t.Fatalf("want one day card with one watch, got %+v", cards)
	}
	td := cards[0].Watches[0]
	if cards[0].Label != "Today" || td.Updates != 5 || cards[0].Updates != 5 || td.Unread != 5 {
		t.Errorf("five updates today, all unread: %+v", td)
	}
	if td.Current.Title != "finding at 9" || len(td.Earlier) != 4 || td.Earlier[0].Title != "finding at 6" || td.Earlier[3].Title != "finding at 2" {
		t.Errorf("newest on top, earlier ones newest first: current %q earlier %v", td.Current.Title, td.Earlier)
	}
	if got, _ := h.store.Get(h.id); got.LastStatus != StatusNothingNew {
		t.Errorf("the last run found nothing and says so: %q", got.LastStatus)
	}
	for _, e := range append([]Entry{td.Current}, td.Earlier...) {
		if e.Sent {
			t.Errorf("a feed-only watch never sends: %+v", e)
		}
	}
}

func TestFeedGroupsByDayThenWatchAndFilters(t *testing.T) {
	s, _ := NewStore(newMemDir())
	a, _ := s.Create(feedWatch(), t0)
	bWatch := feedWatch()
	bWatch.Name = "Security"
	b, _ := s.Create(bWatch, t0)
	now := time.Date(2026, 11, 20, 18, 0, 0, 0, time.UTC)
	add := func(id int64, src, title string, at time.Time) {
		if err := s.AddEntries(id, []sources.Item{{Source: src, Title: title, Link: "https://e.example/" + title}}, Marks{}, at); err != nil {
			t.Fatal(err)
		}
	}
	add(a.ID, "HN", "a-yesterday", now.Add(-24*time.Hour))
	add(a.ID, "HN", "a-morning", now.Add(-9*time.Hour))
	add(b.ID, "Feed", "b-noon", now.Add(-6*time.Hour))
	add(a.ID, "HN", "a-afternoon", now.Add(-2*time.Hour))
	cards := s.Feed(now, time.UTC, 0, Filter{})
	if len(cards) != 2 || cards[0].Label != "Today" || cards[1].Label != "Yesterday" {
		t.Fatalf("days newest first with plain labels: %+v", cards)
	}
	if got := cards[0].Watches; len(got) != 2 || got[0].Watch != "LLMs" || got[1].Watch != "Security" {
		t.Errorf("inside a day the watch with the newest finding comes first: %+v", got)
	}
	if n := len(s.Feed(now, time.UTC, 1, Filter{})); n != 1 {
		t.Errorf("maxDays must cut the list: %d", n)
	}
	if cs := s.Feed(now, time.UTC, 0, Filter{WatchID: b.ID}); len(cs) != 1 || len(cs[0].Watches) != 1 || cs[0].Watches[0].Watch != "Security" {
		t.Errorf("watch filter: %+v", cs)
	}
	if cs := s.Feed(now, time.UTC, 0, Filter{Source: "feed"}); len(cs) != 1 || cs[0].Updates != 1 {
		t.Errorf("source filter is case-insensitive: %+v", cs)
	}
	// Read state.
	first := cards[0].Watches[0]
	if err := s.MarkRead([]int64{first.Current.ID}, true); err != nil {
		t.Fatal(err)
	}
	if cs := s.Feed(now, time.UTC, 0, Filter{UnreadOnly: true}); cs[0].Updates != 2 {
		t.Errorf("unread filter drops the read entry: %+v", cs[0])
	}
	if cs := s.Feed(now, time.UTC, 0, Filter{}); cs[0].Unread != 2 || cs[0].Updates != 3 {
		t.Errorf("counts: %+v", cs[0])
	}
}

func TestFeedDaysFollowTheOwnersTimeZone(t *testing.T) {
	s, _ := NewStore(newMemDir())
	a, _ := s.Create(feedWatch(), t0)
	at := time.Date(2026, 11, 20, 23, 30, 0, 0, time.UTC)
	_ = s.AddEntries(a.ID, []sources.Item{{Source: "x", Title: "late", Link: "https://e.example/late"}}, Marks{}, at)
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("no tz database")
	}
	cards := s.Feed(at.Add(time.Hour), berlin, 0, Filter{})
	if len(cards) != 1 || cards[0].Day != "2026-11-21" {
		t.Errorf("23:30 UTC is already the 21st in Berlin: %+v", cards)
	}
}

func TestFeedEntryIsNotDuplicatedWhenASendIsRetried(t *testing.T) {
	h := newHarness(t, feedWatch())
	h.fetch.bodies[feedURL] = itemsRSS("One")
	h.tick(t, 0)
	h.forgetFirstFindings()
	h.fetch.bodies[feedURL] = itemsRSS("One", "Two")
	h.fail = errors.New("connector down")
	h.tick(t, time.Hour)
	h.tick(t, time.Hour) // retried, still failing
	cards := h.store.Feed(h.now, time.UTC, 0, Filter{})
	if len(cards) != 1 || cards[0].Updates != 1 || cards[0].Watches[0].Current.Sent {
		t.Fatalf("the change is in the feed once, unsent, while the send keeps failing: %+v", cards)
	}
	h.fail = nil
	h.tick(t, time.Hour)
	cards = h.store.Feed(h.now, time.UTC, 0, Filter{})
	if cards[0].Updates != 1 || !cards[0].Watches[0].Current.Sent {
		t.Fatalf("once delivered the same entry is marked sent, not duplicated: %+v", cards)
	}
}

func TestNoConnectorStillShowsTheChangeInTheFeed(t *testing.T) {
	h := newHarness(t, feedWatch())
	h.fetch.bodies[feedURL] = itemsRSS("One")
	h.tick(t, 0)
	h.forgetFirstFindings()
	h.fetch.bodies[feedURL] = itemsRSS("One", "Two")
	h.run.Notify = func(context.Context, Watch, Notice) error { return ErrNoChannel }
	if o := h.tick(t, time.Hour); o[0].Status != StatusNoChannel {
		t.Fatalf("%+v", o)
	}
	if cards := h.store.Feed(h.now, time.UTC, 0, Filter{}); len(cards) != 1 || cards[0].Watches[0].Current.Title != "Two" {
		t.Errorf("the owner can still read the change in the feed: %+v", cards)
	}
}

func TestJudgedRepeatIsFoldedUnderItsOriginal(t *testing.T) {
	m := &scriptedModel{answers: []string{`{}`, `{"repeats":[{"item":1,"of":1}]}`}}
	h := modelHarness(t, m)
	h.fetch.bodies[feedURL] = itemsRSS("seed")
	h.tick(t, 0)
	h.forgetFirstFindings()
	h.fetch.bodies[feedURL] = rss([2]string{"seed", "https://e.example/1"}, [2]string{"GPT-5.5 retirement set", "https://orcarouter.example/a"})
	m.answers = []string{`{}`}
	h.tick(t, time.Hour) // announced and recorded
	h.fetch.bodies[feedURL] = rss([2]string{"seed", "https://e.example/1"}, [2]string{"GPT-5.5 retirement set", "https://orcarouter.example/a"}, [2]string{"GPT-5.5 retirement set", "https://trustloud.example/b"})
	m.answers = []string{`{"repeats":[{"item":1,"of":1}]}`}
	h.tick(t, time.Hour)
	cards := h.store.Feed(h.now, time.UTC, 0, Filter{})
	td := cards[0].Watches[0]
	if td.Updates != 1 || td.Current.Title != "GPT-5.5 retirement set" || len(td.Current.AlsoSeen) != 1 || td.Current.AlsoSeen[0].Link != "https://trustloud.example/b" {
		t.Fatalf("the repeat is folded under the original, not a second entry: %+v", td)
	}
}

func TestChangedFactIsKeptOnTheEntry(t *testing.T) {
	m := &scriptedModel{answers: []string{`{}`, `{"changes":[{"item":1,"was":"beta","now":"stable"}]}`}}
	h := modelHarness(t, m)
	h.fetch.bodies[feedURL] = itemsRSS("seed")
	h.tick(t, 0)
	h.forgetFirstFindings()
	h.fetch.bodies[feedURL] = rss([2]string{"seed", "https://e.example/1"}, [2]string{"Release 2 is stable", "https://e.example/r2"})
	h.tick(t, time.Hour)
	e := h.store.Feed(h.now, time.UTC, 0, Filter{})[0].Watches[0].Current
	if !e.Changed() || e.Was != "beta" || e.Now != "stable" {
		t.Errorf("the was and now pair belongs on the feed entry: %+v", e)
	}
}

func TestJournalIsBoundedAndFollowsTheWatch(t *testing.T) {
	dir := newMemDir()
	s, _ := NewStore(dir)
	a, _ := s.Create(feedWatch(), t0)
	now := t0.Add(60 * 24 * time.Hour)
	_ = s.AddEntries(a.ID, []sources.Item{{Source: "x", Title: "old", Link: "https://e.example/old"}}, Marks{}, now.Add(-JournalRetention-time.Hour))
	for i := range MaxEntries + 20 {
		_ = s.AddEntries(a.ID, []sources.Item{{Source: "x", Title: fmt.Sprintf("t%d", i), Link: fmt.Sprintf("https://e.example/%d", i)}}, Marks{}, now.Add(time.Duration(i)*time.Second))
	}
	if n := len(s.entriesFor(Filter{})); n != MaxEntries {
		t.Errorf("a watch keeps its newest %d entries, got %d", MaxEntries, n)
	}
	again, _ := NewStore(dir)
	if n := len(again.entriesFor(Filter{})); n != MaxEntries {
		t.Errorf("the journal survives a restart: %d", n)
	}
	_ = again.Delete(a.ID)
	if n := len(again.entriesFor(Filter{})); n != 0 {
		t.Errorf("deleting a watch deletes its entries: %d", n)
	}
}

func TestChannelIsValidated(t *testing.T) {
	for _, bad := range []string{"Tele gram", "x", "../x", "a-b"} {
		x := feedWatch()
		x.Channel = bad
		if _, err := Normalize(x); !errors.Is(err, ErrInvalid) {
			t.Errorf("channel %q must be refused: %v", bad, err)
		}
	}
	x := feedWatch()
	x.Channel = " Email "
	if got, err := Normalize(x); err != nil || got.Channel != "email" {
		t.Errorf("channel is normalised: %+v %v", got, err)
	}
}

// An empty feed is the JSON [], never null: the page reads the answer with .length and a null crashed it (a watch that had only recorded its baseline).
func TestAnEmptyFeedIsAnEmptyListNotNull(t *testing.T) {
	var s Store
	got, err := json.Marshal(s.Feed(time.Now(), time.UTC, 7, Filter{}))
	if err != nil || string(got) != "[]" {
		t.Fatalf("empty feed = %s (%v), want []", got, err)
	}
}

func TestClearedEntriesAreDeletedAndOnlyTheOnesAskedFor(t *testing.T) {
	s, _ := NewStore(newMemDir())
	a, _ := s.Create(feedWatch(), t0)
	for i, title := range []string{"one", "two", "three"} {
		_ = s.AddEntries(a.ID, []sources.Item{{Source: "x", Title: title, Link: "https://e.example/" + title}}, Marks{}, t0.Add(time.Duration(i)*time.Minute))
	}
	_ = s.MarkRead([]int64{1}, true)
	if n, _ := s.ClearEntries(true); n != 1 {
		t.Errorf("clearing what is read removes only the read entry, removed %d", n)
	}
	if n, _ := s.DeleteEntries([]int64{2, 99}); n != 1 {
		t.Errorf("an unknown id is ignored, removed %d", n)
	}
	if cs := s.Feed(t0.Add(time.Hour), time.UTC, 0, Filter{}); len(cs) != 1 || cs[0].Updates != 1 || cs[0].Watches[0].Current.Title != "three" {
		t.Errorf("one entry is left: %+v", cs)
	}
	if n, _ := s.ClearEntries(false); n != 1 || len(s.Feed(t0.Add(time.Hour), time.UTC, 0, Filter{})) != 0 {
		t.Errorf("clear all empties the feed, removed %d", n)
	}
}
