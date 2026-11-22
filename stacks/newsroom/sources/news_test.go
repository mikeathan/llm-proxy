package sources

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const reutersSearchURL = "https://news.google.com/rss/search?ceid=GB%3Aen&gl=GB&hl=en-GB&q=site%3Areuters.com+when%3A1d"

func TestNewsSearchURL_EncodesTheQueryAndTheRegion(t *testing.T) {
	if got := NewsSearchURL("site:reuters.com when:1d", "en-GB"); got != reutersSearchURL {
		t.Errorf("url = %s, want %s", got, reutersSearchURL)
	}
	if got := NewsSearchURL("a&b=c", ""); !strings.Contains(got, "q=a%26b%3Dc") || !strings.Contains(got, "hl="+DefaultNewsLocale) {
		t.Errorf("a query must be encoded, never spliced in, and the region defaults: %s", got)
	}
}

func TestNewsSearch_ItemsAreHeadlineOnlyWithTheirPublisher(t *testing.T) {
	f := &fakeFetch{bodies: map[string][]byte{reutersSearchURL: fixture(t, "news-search.rss")}}
	items, err := NewsSearch{Label: "Reuters", Query: "site:reuters.com when:1d", Locale: "en-GB", Fetcher: f}.Fetch(context.Background())
	if err != nil || len(items) != 3 {
		t.Fatalf("items = %d, err = %v", len(items), err)
	}
	first := items[0]
	if first.Title != "Thousands march in Lisbon as living costs fuel anger" || first.Publisher != "Reuters" {
		t.Errorf("the publisher suffix moves out of the title: %+v", first)
	}
	if first.Text != "" {
		t.Errorf("a news-search result has no snippet; its description only repeats the title, so Text must stay empty: %q", first.Text)
	}
	if first.At.IsZero() || !strings.HasPrefix(first.Link, "https://news.google.com/rss/articles/") {
		t.Errorf("time and link must be kept: %+v", first)
	}
	if items[1].Title != "Denmark pushes EU to tighten travel curbs on Russians - Reuters" {
		t.Errorf("only one trailing publisher suffix goes: %q", items[1].Title)
	}
	if items[2].Title != "A headline with no publisher part" || items[2].Publisher != "" {
		t.Errorf("an item with no source element keeps its title: %+v", items[2])
	}
}

func TestNewsSearch_FailureNamesTheSourceOnce(t *testing.T) {
	_, err := NewsSearch{Label: "Reuters", Query: "x", Fetcher: &fakeFetch{err: errors.New("boom")}}.Fetch(context.Background())
	if err == nil || strings.Count(err.Error(), "Reuters") != 1 {
		t.Fatalf("err = %v", err)
	}
	_, err = NewsSearch{Label: "Reuters", Query: "x", Fetcher: &fakeFetch{bodies: map[string][]byte{}}}.Fetch(context.Background())
	if err == nil {
		t.Fatal("an unanswered address is a failure")
	}
}

func TestNewsSearchSpec_ValidatesAndDefaultsTheRegion(t *testing.T) {
	ok := Spec{Kind: KindNews, Target: "inflation OR \"interest rates\""}.Canonical()
	if ok.Instance != DefaultNewsLocale || ok.Validate() != nil {
		t.Errorf("a query alone is valid and gets the default region: %+v, %v", ok, ok.Validate())
	}
	for name, s := range map[string]Spec{
		"empty query":  {Kind: KindNews, Target: "  ", Instance: "en-GB"},
		"bad region":   {Kind: KindNews, Target: "x", Instance: "english"},
		"lower region": {Kind: KindNews, Target: "x", Instance: "en-gb"},
	} {
		if s.Canonical().Validate() == nil {
			t.Errorf("%s must be refused", name)
		}
	}
}
