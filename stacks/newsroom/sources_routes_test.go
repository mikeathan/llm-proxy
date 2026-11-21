package newsroom

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"llm-proxy-stacks/sdk"
)

// pageFetch answers every address with one body and counts the reads, by address.
type pageFetch struct {
	mu    sync.Mutex
	body  string
	reads []string
	err   error
}

func (f *pageFetch) Get(_ context.Context, r stacksdk.FetchRequest) (stacksdk.FetchResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads = append(f.reads, r.URL)
	if f.err != nil {
		return stacksdk.FetchResponse{}, f.err
	}
	return stacksdk.FetchResponse{Body: []byte(f.body)}, nil
}

func (f *pageFetch) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.reads)
}

const channelPage = `<meta property="og:title" content="AI Post"><div class="tgme_widget_message_wrap"><div data-post="aipost/9"><div class="tgme_widget_message_text">hello</div></div></div>`

func lookupReq(query string) stacksdk.Request {
	return stacksdk.Request{Method: "GET", Path: RouteSourceLookup, Query: query}
}

func TestLookupRouteAnswersAPreview(t *testing.T) {
	f := &pageFetch{body: channelPage}
	s := start(t, "", f)
	got, err := s.Handle(context.Background(), lookupReq("kind=telegram&target=%40aipost"))
	if err != nil {
		t.Fatal(err)
	}
	var p struct {
		Found  bool   `json:"found"`
		Target string `json:"target"`
		Title  string `json:"title"`
	}
	if err := json.Unmarshal(got.Body, &p); err != nil || !p.Found || p.Target != "aipost" || p.Title != "AI Post" {
		t.Fatalf("preview = %s (%v)", got.Body, err)
	}
}

func TestLookupFollowsTheSwitchAndFetchesNothingWhileOff(t *testing.T) {
	f := &pageFetch{body: channelPage}
	s := start(t, `{"disabled":true}`, f)
	for _, path := range []string{RouteSourceLookup, RouteSourceSuggest} {
		_, err := s.Handle(context.Background(), stacksdk.Request{Method: "GET", Path: path, Query: "kind=mastodon&instance=mastodon.social&target=go"})
		if stacksdk.CodeOf(err) != stacksdk.CodeUnavailable {
			t.Errorf("%s while off: %v", path, err)
		}
	}
	if f.count() != 0 {
		t.Errorf("the switch off must mean no request, got %d", f.count())
	}
}

func TestLookupRejectsAMissingOrUnknownKind(t *testing.T) {
	s := start(t, "", &pageFetch{})
	for _, q := range []string{"", "kind=nonsense&target=x", "kind=telegram&target=%zz"} {
		if _, err := s.Handle(context.Background(), lookupReq(q)); stacksdk.CodeOf(err) != stacksdk.CodeInvalid {
			t.Errorf("%q must be invalid, got %v", q, err)
		}
	}
}

func TestTheSameLookupTwiceReadsTheSourceOnce(t *testing.T) {
	f := &pageFetch{body: channelPage}
	s := start(t, "", f)
	for _, target := range []string{"aipost", "@AIpost", "https://t.me/s/aipost"} {
		if _, err := s.Handle(context.Background(), lookupReq("kind=telegram&target="+target)); err != nil {
			t.Fatal(err)
		}
	}
	if f.count() != 1 {
		t.Errorf("one channel written three ways is one read, got %d", f.count())
	}
}

func TestAnAnswerAboutTheMomentIsNotRemembered(t *testing.T) {
	f := &pageFetch{err: stacksdk.Errorf(stacksdk.CodeUnavailable, "off")}
	s := start(t, "", f)
	_, _ = s.Handle(context.Background(), lookupReq("kind=telegram&target=aipost"))
	f.mu.Lock()
	f.err, f.body = nil, channelPage
	f.mu.Unlock()
	got, _ := s.Handle(context.Background(), lookupReq("kind=telegram&target=aipost"))
	if !strings.Contains(string(got.Body), `"found":true`) {
		t.Errorf("after the internet is back the look-up must read again: %s", got.Body)
	}
}

func TestTelegramLookupsAreRateLimitedAcrossChannels(t *testing.T) {
	f := &pageFetch{body: channelPage}
	s := start(t, "", f)
	now := time.Now()
	s.lookups.now = func() time.Time { return now }
	var last stacksdk.Response
	for i := 0; i <= telegramLookupsPerMinute; i++ {
		last, _ = s.Handle(context.Background(), lookupReq("kind=telegram&target=chan"+strings.Repeat("a", i+1)))
	}
	if f.count() != telegramLookupsPerMinute || !strings.Contains(string(last.Body), "wait a minute") {
		t.Errorf("reads = %d, last = %s", f.count(), last.Body)
	}
	now = now.Add(61 * time.Second)
	s.lookups.now = func() time.Time { return now }
	if got, _ := s.Handle(context.Background(), lookupReq("kind=telegram&target=aipost")); !strings.Contains(string(got.Body), `"found":true`) {
		t.Errorf("a minute later a look-up works again: %s", got.Body)
	}
}

func TestTheLookupCacheIsBounded(t *testing.T) {
	c := newLookupCache()
	for i := 0; i < lookupCacheMax*3; i++ {
		c.put("k"+strings.Repeat("x", i), []byte("v"))
	}
	if n := c.size(); n > lookupCacheMax {
		t.Errorf("the cache holds %d, the bound is %d", n, lookupCacheMax)
	}
}

func TestSuggestRouteListsTrendingTags(t *testing.T) {
	f := &pageFetch{body: `[{"name":"Weekend","history":[{"uses":"9"}]}]`}
	s := start(t, "", f)
	got, err := s.Handle(context.Background(), stacksdk.Request{Method: "GET", Path: RouteSourceSuggest, Query: "kind=mastodon&instance=mastodon.social"})
	if err != nil || !strings.Contains(string(got.Body), `"value":"Weekend"`) {
		t.Fatalf("suggest = %s, %v", got.Body, err)
	}
}

func TestSourceRoutesAreDeclaredAsAdminReads(t *testing.T) {
	seen := 0
	for _, r := range New().Manifest().Routes {
		if r.Path == RouteSourceLookup || r.Path == RouteSourceSuggest {
			seen++
			if r.Method != "GET" || r.Level() != stacksdk.AccessAdmin {
				t.Errorf("%s %s level %q", r.Method, r.Path, r.Level())
			}
		}
	}
	if seen != 2 {
		t.Errorf("both source routes must be declared, found %d", seen)
	}
}
