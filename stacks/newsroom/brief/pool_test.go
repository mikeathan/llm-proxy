package brief

import (
	"reflect"
	"testing"
	"time"

	"llm-proxy-stacks/newsroom/sources"
)

var t0 = time.Date(2026, 11, 21, 6, 0, 0, 0, time.UTC)

func item(source, publisher, title, link string, at time.Time) sources.Item {
	return sources.Item{Source: source, Publisher: publisher, Title: title, Link: link, At: at}
}

func TestTitleKey_SameHeadlineInDifferentWordingIsOneKey(t *testing.T) {
	a := TitleKey("Ukraine says strikes are a 'fair response' ")
	b := TitleKey("UKRAINE SAYS STRIKES ARE A “FAIR RESPONSE”")
	if a == "" || a != b {
		t.Errorf("keys differ: %q vs %q", a, b)
	}
	if TitleKey("  ... !!! ") != "" {
		t.Error("a title with no letters or digits has no key")
	}
}

func TestMerge_OneStoryFromTwoOutletsIsOneItemWithTwoReports(t *testing.T) {
	topic := Topic{ID: 1}
	pool, next, added := merge(nil, 1, topic, []sources.Item{item("BBC", "", "Markets fall as tech sells off", "https://bbc.example/1", t0)}, t0)
	pool, next, added2 := merge(pool, next, topic, []sources.Item{item("Search", "Reuters", "Markets fall, as tech sells off!", "https://news.example/x", t0.Add(time.Minute))}, t0.Add(time.Hour))
	if added != 1 || added2 != 0 || len(pool) != 1 || next != 2 {
		t.Fatalf("added %d then %d, pool %d, next %d", added, added2, len(pool), next)
	}
	if got := pool[0].Outlets(); got != 2 || len(pool[0].Reports) != 2 {
		t.Errorf("a story reported twice must show two outlets: %+v", pool[0])
	}
	if !pool[0].Posted.Equal(t0) {
		t.Errorf("the earliest posting time wins: %v", pool[0].Posted)
	}
}

func TestMerge_SameLinkMergesEvenWhenTheHeadlineWasEdited(t *testing.T) {
	topic := Topic{ID: 1}
	pool, next, _ := merge(nil, 1, topic, []sources.Item{item("BBC", "", "Early headline", "https://bbc.example/1", t0)}, t0)
	pool, _, added := merge(pool, next, topic, []sources.Item{item("BBC", "", "Edited headline", "https://bbc.example/1", t0)}, t0)
	if added != 0 || len(pool) != 1 {
		t.Errorf("an edited headline on the same link is the same story: added %d, pool %d", added, len(pool))
	}
}

func TestMerge_TwoTopicsShareOneStory(t *testing.T) {
	it := item("BBC", "", "Interest rates held", "https://bbc.example/2", t0)
	pool, next, _ := merge(nil, 1, Topic{ID: 1}, []sources.Item{it}, t0)
	pool, _, _ = merge(pool, next, Topic{ID: 2}, []sources.Item{it}, t0)
	if len(pool) != 1 || !reflect.DeepEqual(pool[0].Topics, []int64{1, 2}) {
		t.Errorf("one story, both topics, each once: %+v", pool)
	}
	pool, _, _ = merge(pool, 9, Topic{ID: 2}, []sources.Item{it}, t0)
	if !reflect.DeepEqual(pool[0].Topics, []int64{1, 2}) || len(pool[0].Reports) != 1 {
		t.Errorf("a repeat read adds nothing: %+v", pool[0])
	}
}

func TestMerge_DropsWhatTheTopicDoesNotWant(t *testing.T) {
	topic := Topic{ID: 1, Ignore: []string{"horoscope"}}
	items := []sources.Item{
		item("s", "", "Your HOROSCOPE today", "https://e/1", t0),
		item("s", "", "", "https://e/2", t0),
		item("s", "", "Four days old", "https://e/3", t0.Add(-4*24*time.Hour)),
		item("s", "", "Undated is kept", "https://e/4", time.Time{}),
	}
	pool, _, added := merge(nil, 1, topic, items, t0)
	if added != 1 || len(pool) != 1 || pool[0].Title != "Undated is kept" {
		t.Errorf("only the undated, wanted headline is kept: %+v", pool)
	}
}

func TestMerge_AHeadlineOnlyStoryTakesTextWhenAnotherReportHasIt(t *testing.T) {
	headline := item("Search", "Reuters", "Floods hit the coast", "https://news.example/a", t0)
	full := item("BBC", "", "Floods hit the coast", "https://bbc.example/a", t0)
	full.Text = "Heavy rain flooded towns along the coast overnight."
	pool, next, _ := merge(nil, 1, Topic{ID: 1}, []sources.Item{headline}, t0)
	if !pool[0].HeadlineOnly() {
		t.Fatal("a news-search story has no text")
	}
	pool, _, _ = merge(pool, next, Topic{ID: 1}, []sources.Item{full}, t0)
	if pool[0].HeadlineOnly() || pool[0].Text != full.Text {
		t.Errorf("a report with text lifts the story out of headline-only: %+v", pool[0])
	}
}

func TestPrune_DropsOldStoriesAndBoundsThePool(t *testing.T) {
	var pool []PoolItem
	pool = append(pool, PoolItem{ID: 1, Key: "old", FirstSeen: t0.Add(-PoolRetention - time.Minute)})
	for i := range MaxPool + 5 {
		pool = append(pool, PoolItem{ID: int64(i + 2), Key: "k", FirstSeen: t0.Add(-time.Duration(MaxPool+5-i) * time.Second)})
	}
	got := prune(pool, t0)
	if len(got) != MaxPool {
		t.Fatalf("pool = %d, want the cap %d", len(got), MaxPool)
	}
	for _, p := range got {
		if p.ID == 1 || p.ID <= 6 {
			t.Fatalf("the expired story and the five oldest must go, still have %d", p.ID)
		}
	}
}

func TestMerge_ReplayGivesTheSamePool(t *testing.T) {
	items := []sources.Item{item("a", "", "One story", "https://e/1", t0), item("b", "Pub", "Another story", "https://e/2", t0)}
	first, n1, _ := merge(nil, 1, Topic{ID: 1}, items, t0)
	second, n2, _ := merge(nil, 1, Topic{ID: 1}, items, t0)
	if !reflect.DeepEqual(first, second) || n1 != n2 {
		t.Error("the same input and clock must give the same pool")
	}
}
