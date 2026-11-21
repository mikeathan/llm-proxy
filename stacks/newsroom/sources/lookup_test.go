package sources

import (
	"context"
	"strings"
	"testing"

	"llm-proxy-stacks/sdk"
)

const tgPage = `<html><head><meta property="og:title" content="Telegram News"></head><body><div class="tgme_channel_info_header_title"><span dir="auto">Telegram News</span></div>` +
	`<div class="tgme_widget_message_wrap"><div data-post="telegram/458"><div class="tgme_widget_message_text js-message_text" dir="auto">First post</div><time datetime="2026-11-20T10:00:00+00:00"></time></div></div></body></html>`

func TestLookupTelegramNamesTheChannelAndShowsItsLatestPosts(t *testing.T) {
	f := &fakeFetch{bodies: map[string][]byte{"https://t.me/s/telegram": []byte(tgPage)}}
	p := Lookup(context.Background(), f, "telegram", "https://t.me/s/telegram", "")
	if !p.Found || p.Target != "telegram" || p.Title != "Telegram News" || len(p.Sample) != 1 || p.Sample[0].Link != "https://t.me/telegram/458" {
		t.Fatalf("a public channel must be found and named: %+v", p)
	}
}

func TestLookupTelegramWithNoPreviewSaysSoInPlainWords(t *testing.T) {
	// A name that is not a public channel answers with a page that has no messages, or a refused redirect; both read as "no public preview".
	for name, f := range map[string]*fakeFetch{
		"no messages": {bodies: map[string][]byte{"https://t.me/s/nobodyhere": []byte("<html><body>no channel</body></html>")}},
		"refused":     {err: stacksdk.Errorf(stacksdk.CodeDenied, "a redirect was refused")},
	} {
		p := Lookup(context.Background(), f, "telegram", "nobodyhere", "")
		if p.Found || !strings.Contains(p.Problem, "public") {
			t.Errorf("%s: want a problem that says the channel has no public preview, got %+v", name, p)
		}
	}
}

func TestLookupRefusesABadValueWithoutAnyRequest(t *testing.T) {
	f := &fakeFetch{}
	for kind, target := range map[string]string{"telegram": "a b", "mastodon": "has space", "feed": "http://insecure.example/feed", "hackernews": "   "} {
		if p := Lookup(context.Background(), f, kind, target, "mastodon.social"); p.Found || p.Problem == "" {
			t.Errorf("%s %q: want a problem, got %+v", kind, target, p)
		}
	}
	if len(f.calls) != 0 {
		t.Errorf("a value that fails its check must not be fetched: %v", f.calls)
	}
}

func TestLookupSaysWhenInternetIsOff(t *testing.T) {
	f := &fakeFetch{err: stacksdk.Errorf(stacksdk.CodeUnavailable, "off")}
	p := Lookup(context.Background(), f, "hackernews", "rust", "")
	if p.Found || !strings.Contains(p.Problem, "Internet access is off") {
		t.Fatalf("an off switch must be named, not called a missing source: %+v", p)
	}
}

func TestLookupFeedReadsItsTitleAndItems(t *testing.T) {
	f := &fakeFetch{bodies: map[string][]byte{"https://example.com/feed": fixture(t, "bbc-tech.rss")}}
	p := Lookup(context.Background(), f, "feed", "https://example.com/feed", "")
	if !p.Found || p.Title == "" || len(p.Sample) == 0 || len(p.Sample) > SampleSize {
		t.Fatalf("a feed must give its title and at most %d items: %+v", SampleSize, p)
	}
}

func TestLookupFeedFromASiteAddressOffersTheFeedsItAdvertises(t *testing.T) {
	html := `<html><head><title>Example</title>
<link rel="alternate" type="application/rss+xml" title="All posts" href="/feed.xml">
<link rel="alternate" type="application/atom+xml" href="https://example.com/atom.xml">
<link rel="alternate" type="application/rss+xml" href="http://insecure.example/feed">
<link rel="stylesheet" href="/s.css"></head><body>hi</body></html>`
	f := &fakeFetch{bodies: map[string][]byte{"https://example.com/": []byte(html)}}
	p := Lookup(context.Background(), f, "feed", "https://example.com/", "")
	if p.Found || len(p.Choices) != 2 || p.Choices[0].Value != "https://example.com/feed.xml" || p.Choices[0].Label != "All posts" || p.Choices[1].Value != "https://example.com/atom.xml" {
		t.Fatalf("a page that is not a feed must offer its advertised https feeds (relative ones resolved, http dropped): %+v", p)
	}
	if !strings.Contains(p.Problem, "not a feed") {
		t.Errorf("the problem must say the address is not a feed itself: %q", p.Problem)
	}
}

func TestLookupPageShowsItsTitleAndOpeningText(t *testing.T) {
	f := &fakeFetch{bodies: map[string][]byte{"https://example.com/status": []byte("<html><head><title>Status page</title></head><body><p>All systems normal</p></body></html>")}}
	p := Lookup(context.Background(), f, "page", "https://example.com/status", "")
	if !p.Found || p.Title != "Status page" || len(p.Sample) != 1 || !strings.Contains(p.Sample[0].Text, "All systems normal") {
		t.Fatalf("a page must show its title and the text that will be compared: %+v", p)
	}
}

func TestLookupMastodonTagSamplesTheTimelineOnTheChosenServer(t *testing.T) {
	f := &fakeFetch{bodies: map[string][]byte{"https://mastodon.social/api/v1/timelines/tag/golang?limit=30": fixture(t, "mastodon.json")}}
	p := Lookup(context.Background(), f, "mastodon", "#golang", "https://Mastodon.social/")
	if !p.Found || p.Target != "golang" || p.Instance != "mastodon.social" || len(p.Sample) == 0 {
		t.Fatalf("a tag must be cleaned and sampled: %+v", p)
	}
}

func TestLookupHackerNewsSamplesTheSearch(t *testing.T) {
	f := &fakeFetch{bodies: map[string][]byte{"https://hn.algolia.com/api/v1/search_by_date?tags=story&hitsPerPage=30&query=rust": fixture(t, "hn.json")}}
	p := Lookup(context.Background(), f, "hackernews", "rust", "")
	if !p.Found || len(p.Sample) == 0 || !strings.Contains(p.Title, "rust") {
		t.Fatalf("a search must show what it finds now: %+v", p)
	}
}

func TestSuggestMastodonSearchesTagsAndFallsBackToTrends(t *testing.T) {
	search := `{"accounts":[],"statuses":[],"hashtags":[{"name":"golang","history":[{"day":"1","accounts":"5","uses":"7"}]},{"name":"gopher","history":[]}]}`
	trends := `[{"name":"Weekend","history":[{"day":"1","accounts":"50","uses":"90"}]}]`
	f := &fakeFetch{bodies: map[string][]byte{
		"https://mastodon.social/api/v2/search?type=hashtags&limit=8&q=go": []byte(search),
		"https://mastodon.social/api/v1/trends/tags?limit=8":               []byte(trends),
	}}
	got, problem := Suggest(context.Background(), f, "mastodon", "mastodon.social", "#go")
	if problem != "" || len(got) != 2 || got[0].Value != "golang" || got[0].Note == "" {
		t.Fatalf("a typed tag must list matching tags with their use: %+v %q", got, problem)
	}
	got, problem = Suggest(context.Background(), f, "mastodon", "mastodon.social", "")
	if problem != "" || len(got) != 1 || got[0].Value != "Weekend" {
		t.Fatalf("no text must list the trending tags: %+v %q", got, problem)
	}
}

func TestSuggestAServerThatSharesNoTrendsIsNotAnError(t *testing.T) {
	f := &fakeFetch{bodies: map[string][]byte{"https://small.example/api/v1/trends/tags?limit=8": []byte("[]")}}
	got, problem := Suggest(context.Background(), f, "mastodon", "small.example", "")
	if len(got) != 0 || !strings.Contains(problem, "no trending") {
		t.Fatalf("an empty list must read as 'this server shares no trending tags': %+v %q", got, problem)
	}
}

func TestSuggestOffersNothingWhereThereIsNoKeylessSearch(t *testing.T) {
	// Telegram has no keyless channel search, and a Hacker News search is previewed by Lookup, so a front page of stories would not help choose one.
	f := &fakeFetch{}
	for _, kind := range []string{"telegram", "hackernews", "feed", "page"} {
		if got, problem := Suggest(context.Background(), f, kind, "", "x"); len(got) != 0 || problem != "" {
			t.Errorf("%s must offer nothing and say nothing: %+v %q", kind, got, problem)
		}
	}
	if len(f.calls) != 0 {
		t.Errorf("nothing offered means nothing fetched: %v", f.calls)
	}
}

func TestLookupDoesNotTouchTheReadersPolitenessFloor(t *testing.T) {
	f := &fakeFetch{bodies: map[string][]byte{"https://t.me/s/telegram": []byte(tgPage)}}
	ch := &TelegramChannel{Label: "x", Channel: "telegram", Fetcher: f}
	if _, err := ch.Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p := Lookup(context.Background(), f, "telegram", "telegram", ""); !p.Found {
		t.Fatalf("a look-up right after a read must still work: %+v", p)
	}
}
