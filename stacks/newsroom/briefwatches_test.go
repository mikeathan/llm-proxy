package newsroom

import (
	"testing"
	"time"

	"llm-proxy-stacks/newsroom/sources"
	"llm-proxy-stacks/newsroom/watches"
)

func newWatch(t *testing.T, store *watches.Store, name string) watches.Watch {
	t.Helper()
	w, err := store.Create(watches.Watch{Name: name, Enabled: true, Sources: []sources.Spec{{Kind: sources.KindFeed, Label: "Feed", Target: "https://example.com/feed.xml"}}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return w
}

// What the block shows: a watch's newest finding of the window with its headline, a title when the model wrote none, no first-check finding, and a watch
// with an unread source even when it found nothing; a quiet watch is left out.
func TestWatchLines_NewestFindingAndProblems(t *testing.T) {
	store, err := watches.NewStore(&memDir{files: map[string][]byte{}})
	if err != nil {
		t.Fatal(err)
	}
	noisy, broken, quiet := newWatch(t, store, "AI news"), newWatch(t, store, "Prices"), newWatch(t, store, "Quiet")
	now := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	add := func(id int64, title string, at time.Time, marks watches.Marks) {
		if err := store.AddEntries(id, []sources.Item{{Source: "Feed", Title: title, Link: "https://example.com/" + title}}, marks, at); err != nil {
			t.Fatal(err)
		}
	}
	add(noisy.ID, "old", now.Add(-30*time.Hour), watches.Marks{})
	add(noisy.ID, "first", now.Add(-3*time.Hour), watches.Marks{})
	add(noisy.ID, "newest", now.Add(-time.Hour), watches.Marks{Digests: map[int]watches.Digest{0: {Headline: "A model shipped", Summary: "It is faster."}}})
	add(quiet.ID, "baseline", now.Add(-time.Hour), watches.Marks{First: true})
	if err := store.SetProblems(broken.ID, []string{"Channel X has no public preview"}); err != nil {
		t.Fatal(err)
	}

	got := watchLines(store, now.Add(-24*time.Hour), now)
	if len(got) != 2 {
		t.Fatalf("want the noisy and the broken watch only: %+v", got)
	}
	if got[0].Watch != "AI news" || got[0].New != 2 || got[0].Headline != "A model shipped" || got[0].Summary != "It is faster." {
		t.Errorf("noisy watch: %+v", got[0])
	}
	if got[1].Watch != "Prices" || got[1].New != 0 || len(got[1].Problems) != 1 {
		t.Errorf("broken watch: %+v", got[1])
	}
	if watchLines(nil, now.Add(-time.Hour), now) != nil {
		t.Error("no store gives no block")
	}
}
