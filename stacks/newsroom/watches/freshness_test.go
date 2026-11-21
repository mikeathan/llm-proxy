package watches

import (
	"fmt"
	"testing"
	"time"

	"llm-proxy-stacks/newsroom/sources"
)

// News is what is recent: a post older than the watch's window is dropped before the model sees it, a post with no date is kept (its age cannot be
// judged), and a watch can widen the window to ask for older news.
func TestFreshKeepsRecentAndUndatedAndDropsOld(t *testing.T) {
	now := t0
	items := []sources.Item{
		{Title: "last night", At: now.Add(-10 * time.Hour)},
		{Title: "two days ago", At: now.Add(-48 * time.Hour)},
		{Title: "october", At: now.AddDate(0, -1, -4)},
		{Title: "undated"},
	}
	kept, stale := Watch{}.Fresh(items, now)
	if len(kept) != 3 || kept[0].Title != "last night" || kept[1].Title != "two days ago" || kept[2].Title != "undated" || len(stale) != 1 || stale[0].Title != "october" {
		t.Fatalf("default window: kept %v, stale %v", kept, stale)
	}
	kept, stale = Watch{MaxAge: 90 * 24 * time.Hour}.Fresh(items, now)
	if len(kept) != 4 || len(stale) != 0 {
		t.Fatalf("a 90 day window keeps all: kept %d, stale %d", len(kept), len(stale))
	}
}

func TestNormalizeKeepsAWindowInRange(t *testing.T) {
	w := feedWatch()
	got, err := Normalize(w)
	if err != nil || got.MaxAge != DefaultMaxAge {
		t.Fatalf("default: %v, %v (want %s)", got.MaxAge, err, DefaultMaxAge)
	}
	for _, bad := range []time.Duration{time.Minute, MaxMaxAge + time.Hour, -time.Hour} {
		w.MaxAge = bad
		if _, err := Normalize(w); err == nil {
			t.Errorf("a window of %s was accepted", bad)
		}
	}
}

// An old post is not announced and is recorded as seen, so it does not come back on the next check; the post date is kept on a fresh entry.
func TestRunDropsOldPostsButRecordsThemAndKeepsThePostDate(t *testing.T) {
	h := newHarness(t, feedWatch())
	old, fresh := h.now.AddDate(0, -1, 0), h.now.Add(-5*time.Hour)
	body := "<rss version=\"2.0\"><channel>"
	for _, it := range []struct {
		title string
		at    time.Time
	}{{"old", old}, {"fresh", fresh}} {
		body += fmt.Sprintf("<item><title>%s</title><link>https://x.test/%s</link><pubDate>%s</pubDate></item>", it.title, it.title, it.at.Format(time.RFC1123Z))
	}
	h.fetch.bodies[feedURL] = body + "</channel></rss>"
	h.run.Tick(t.Context()) // baseline
	if got := h.store.Unseen(h.id, []sources.Item{{Link: "https://x.test/old"}}, h.now); len(got) != 0 {
		t.Fatalf("the old post was not recorded as seen: %v", got)
	}
	for _, e := range h.store.entriesFor(Filter{}) {
		if e.Title == "old" {
			t.Fatalf("an old post reached the feed: %+v", e)
		}
		if e.Title == "fresh" && !e.Posted.Equal(fresh) {
			t.Errorf("posted = %v, want %v", e.Posted, fresh)
		}
	}
}
