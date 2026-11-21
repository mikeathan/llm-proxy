package watches

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"llm-proxy-stacks/sdk"

	"llm-proxy-stacks/newsroom/sources"
)

// memDir is an in-memory DataDir.
type memDir struct {
	files       map[string][]byte
	quarantined []string
	writes      int
}

func newMemDir() *memDir { return &memDir{files: map[string][]byte{}} }

func (m *memDir) Read(name string) ([]byte, error) {
	b, ok := m.files[name]
	if !ok {
		return nil, stacksdk.Errorf(stacksdk.CodeNotFound, "no such file")
	}
	return b, nil
}
func (m *memDir) Write(name string, data []byte) error {
	m.files[name] = append([]byte(nil), data...)
	m.writes++
	return nil
}
func (m *memDir) Quarantine(name string) error {
	m.quarantined = append(m.quarantined, name)
	delete(m.files, name)
	return nil
}

// feedFetch serves one RSS body per URL; a missing URL fails like a guarded fetch would.
type feedFetch struct {
	bodies map[string]string
	calls  int
}

func (f *feedFetch) Get(_ context.Context, req stacksdk.FetchRequest) (stacksdk.FetchResponse, error) {
	f.calls++
	b, ok := f.bodies[req.URL]
	if !ok {
		return stacksdk.FetchResponse{}, stacksdk.Errorf(stacksdk.CodeInternal, "the request failed")
	}
	return stacksdk.FetchResponse{Body: []byte(b)}, nil
}

func rss(items ...[2]string) string {
	var b strings.Builder
	b.WriteString("<rss version=\"2.0\"><channel>")
	for _, it := range items {
		fmt.Fprintf(&b, "<item><title>%s</title><link>%s</link></item>", it[0], it[1])
	}
	b.WriteString("</channel></rss>")
	return b.String()
}

var t0 = time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)

const feedURL = "https://news.example/feed"

func feedWatch() Watch {
	return Watch{Name: "LLMs", Enabled: true, Channel: "telegram", Sources: []SourceSpec{{Kind: KindFeed, Label: "Example", Target: feedURL}}}
}

// harness builds a store with one watch, a runner over a controllable clock and fetcher, and a notifier that records what it was given.
type harness struct {
	store *Store
	run   *Runner
	fetch *feedFetch
	now   time.Time
	sent  [][]sources.Item
	fail  error
	id    int64
}

func newHarness(t *testing.T, watch Watch) *harness {
	t.Helper()
	store, err := NewStore(newMemDir())
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{store: store, fetch: &feedFetch{bodies: map[string]string{}}, now: t0}
	made, err := store.Create(watch, t0)
	if err != nil {
		t.Fatal(err)
	}
	h.id = made.ID
	h.run = &Runner{Store: store, Fetch: h.fetch, Now: func() time.Time { return h.now }, Notify: func(_ context.Context, _ Watch, n Notice) error {
		if h.fail != nil {
			return h.fail
		}
		h.sent = append(h.sent, n.Items)
		return nil
	}}
	return h
}

// forgetFirstFindings removes the feed entries a first check adds, for tests about what later runs add to the feed.
func (h *harness) forgetFirstFindings() {
	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	kept := h.store.doc.Entries[:0]
	for _, e := range h.store.doc.Entries {
		if !e.First {
			kept = append(kept, e)
		}
	}
	h.store.doc.Entries = kept
}

func (h *harness) tick(t *testing.T, after time.Duration) []Outcome {
	t.Helper()
	h.now = h.now.Add(after)
	return h.run.Tick(context.Background())
}

func TestNormalize(t *testing.T) {
	good, err := Normalize(feedWatch())
	if err != nil || good.Interval != time.Hour {
		t.Fatalf("defaults: %+v, %v (want a 1h interval)", good, err)
	}
	bad := map[string]func(*Watch){
		"no name":          func(x *Watch) { x.Name = "  " },
		"no sources":       func(x *Watch) { x.Sources = nil },
		"too many sources": func(x *Watch) { x.Sources = make([]SourceSpec, MaxSources+1) },
		"http feed":        func(x *Watch) { x.Sources[0].Target = "http://news.example/feed" },
		"unknown kind":     func(x *Watch) { x.Sources[0].Kind = "rumour" },
		"mastodon no host": func(x *Watch) { x.Sources[0] = SourceSpec{Kind: KindMastodon, Target: "llm"} },
		"bad channel":      func(x *Watch) { x.Sources[0] = SourceSpec{Kind: KindTelegram, Target: "ab"} },
		"tag with space": func(x *Watch) {
			x.Sources[0] = SourceSpec{Kind: KindMastodon, Instance: "mastodon.social", Target: "go lang"}
		},
		"bad instance": func(x *Watch) {
			x.Sources[0] = SourceSpec{Kind: KindMastodon, Instance: "mastodon.social:8080/x", Target: "go"}
		},
		"empty search":      func(x *Watch) { x.Sources[0] = SourceSpec{Kind: KindHN, Target: "  "} },
		"feed no host":      func(x *Watch) { x.Sources[0].Target = "https:///feed" },
		"under the floor":   func(x *Watch) { x.Interval = time.Minute },
		"look for too long": func(x *Watch) { x.LookFor = strings.Repeat("a", MaxLookForRunes+1) },
		"too many keywords": func(x *Watch) { x.Keywords = []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"} },
		"long skip word":    func(x *Watch) { x.Skip = []string{strings.Repeat("a", MaxWordRunes+1)} },
	}
	for name, mutate := range bad {
		x := feedWatch()
		mutate(&x)
		if _, err := Normalize(x); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
	x := feedWatch()
	x.Interval = MinInterval
	if _, err := Normalize(x); err != nil {
		t.Errorf("the floor itself must be allowed: %v", err)
	}
	y := feedWatch()
	y.Sources = []SourceSpec{{Kind: KindTelegram, Target: "https://t.me/s/aipost"}, {Kind: KindMastodon, Instance: "https://Mastodon.social/", Target: "#golang"}}
	got, err := Normalize(y)
	if err != nil || got.Sources[0].Target != "aipost" || got.Sources[1].Target != "golang" || got.Sources[1].Instance != "mastodon.social" {
		t.Errorf("pasted forms must reduce to bare names: %+v, %v", got.Sources, err)
	}
}

func TestDue(t *testing.T) {
	tp := Watch{Enabled: true, Interval: time.Hour}
	if !tp.Due(t0) {
		t.Error("a watch never read is due")
	}
	tp.LastRun = t0
	if tp.Due(t0.Add(59 * time.Minute)) {
		t.Error("not due before the interval")
	}
	if !tp.Due(t0.Add(time.Hour)) {
		t.Error("due at the interval")
	}
	tp.Enabled = false
	if tp.Due(t0.Add(48 * time.Hour)) {
		t.Error("a disabled watch is never due")
	}
}

// The Phase 2 acceptance: three ticks yield one notice and then nothing.
func TestReplayOfThreeTicksGivesOneNoticeThenNothing(t *testing.T) {
	h := newHarness(t, feedWatch())
	a, b, c := [2]string{"Qwen4 released", "https://q.example/qwen4"}, [2]string{"Llama 5", "https://m.example/llama5"}, [2]string{"Gemma 4", "https://g.example/gemma4"}

	h.fetch.bodies[feedURL] = rss(a, b)
	if o := h.tick(t, 0); len(o) != 1 || o[0].Status != StatusBaseline || len(h.sent) != 0 {
		t.Fatalf("tick 1 must baseline silently: %+v, sent %d", o, len(h.sent))
	}
	if o := h.tick(t, 10*time.Minute); len(o) != 0 {
		t.Fatalf("a tick inside the interval must not run the watch: %+v", o)
	}
	if o := h.tick(t, time.Hour); len(o) != 1 || o[0].Status != StatusNothingNew || len(h.sent) != 0 {
		t.Fatalf("tick 2 saw the same items: %+v, sent %d", o, len(h.sent))
	}
	h.fetch.bodies[feedURL] = rss(a, b, c)
	if o := h.tick(t, time.Hour); len(o) != 1 || o[0].Status != StatusSent || o[0].New != 1 {
		t.Fatalf("tick 3 has one new item: %+v", o)
	}
	if len(h.sent) != 1 || len(h.sent[0]) != 1 || h.sent[0][0].Title != "Gemma 4" {
		t.Fatalf("exactly the new item must be sent once: %+v", h.sent)
	}
	if o := h.tick(t, time.Hour); o[0].Status != StatusNothingNew || len(h.sent) != 1 {
		t.Fatalf("tick 4 must be silent: %+v", o)
	}
}

func TestFailedSendRecordsNothingAndRetries(t *testing.T) {
	h := newHarness(t, feedWatch())
	h.fetch.bodies[feedURL] = rss([2]string{"One", "https://e.example/1"})
	h.tick(t, 0) // baseline
	h.fetch.bodies[feedURL] = rss([2]string{"One", "https://e.example/1"}, [2]string{"Two", "https://e.example/2"})

	h.fail = errors.New("connector down")
	before := h.store.SeenCount(h.id)
	if o := h.tick(t, time.Hour); o[0].Status != StatusSendFailed || h.store.SeenCount(h.id) != before {
		t.Fatalf("a failed send must record nothing: %+v, seen %d -> %d", o, before, h.store.SeenCount(h.id))
	}
	h.fail = nil
	if o := h.tick(t, time.Hour); o[0].Status != StatusSent || len(h.sent) != 1 || h.sent[0][0].Title != "Two" {
		t.Fatalf("the next run must send the same item: %+v, sent %+v", o, h.sent)
	}
}

func TestNoChannelIsVisibleAndRecordsNothing(t *testing.T) {
	h := newHarness(t, feedWatch())
	h.fetch.bodies[feedURL] = rss([2]string{"One", "https://e.example/1"})
	h.tick(t, 0)
	h.fetch.bodies[feedURL] = rss([2]string{"One", "https://e.example/1"}, [2]string{"Two", "https://e.example/2"})
	h.run.Notify = func(context.Context, Watch, Notice) error { return ErrNoChannel }
	o := h.tick(t, time.Hour)
	got, _ := h.store.Get(h.id)
	if o[0].Status != StatusNoChannel || got.LastStatus != StatusNoChannel || h.store.SeenCount(h.id) != 1 {
		t.Fatalf("no channel must show on the row and record nothing: %+v, %+v, seen %d", o, got, h.store.SeenCount(h.id))
	}
	h.run.Notify = nil
	if o := h.tick(t, time.Hour); o[0].Status != StatusNoChannel {
		t.Fatalf("a runner with no notifier says the same: %+v", o)
	}
}

func TestUnreadableSourceIsFailingNotNothingNew(t *testing.T) {
	h := newHarness(t, feedWatch())
	// No body for the feed URL: the fetch fails.
	if o := h.tick(t, 0); o[0].Status != StatusFailing || !strings.Contains(o[0].Detail, "Example") {
		t.Fatalf("an unreadable source must be failing and name the source: %+v", o)
	}
	got, _ := h.store.Get(h.id)
	if got.Baselined {
		t.Error("a failed first read must not count as the baseline")
	}
}

func TestOneFailingSourceDoesNotHideTheOther(t *testing.T) {
	watch := feedWatch()
	watch.Sources = append(watch.Sources, SourceSpec{Kind: KindFeed, Label: "Broken", Target: "https://broken.example/feed"})
	h := newHarness(t, watch)
	h.fetch.bodies[feedURL] = rss([2]string{"One", "https://e.example/1"})
	h.tick(t, 0)
	h.fetch.bodies[feedURL] = rss([2]string{"One", "https://e.example/1"}, [2]string{"Two", "https://e.example/2"})
	o := h.tick(t, time.Hour)
	if o[0].Status != StatusSent || !strings.Contains(o[0].Detail, "Broken") || len(h.sent) != 1 {
		t.Fatalf("the working source must still deliver and the failure must be shown: %+v", o)
	}
}

// Known limit, pinned like the Phase 0 test: the same event under a new source URL is a new identity.
func TestSameEventUnderNewURLIsSentAgain(t *testing.T) {
	h := newHarness(t, feedWatch())
	h.fetch.bodies[feedURL] = rss([2]string{"GPT-5.5 retirement set", "https://orcarouter.example/gpt55"})
	h.tick(t, 0)
	h.fetch.bodies[feedURL] = rss([2]string{"GPT-5.5 retirement set", "https://trustloud.example/gpt55"})
	if o := h.tick(t, time.Hour); o[0].Status != StatusSent {
		t.Fatalf("the URL-keyed ledger sends a changed-URL repeat again (the delta step, Phase 3, is what must catch it): %+v", o)
	}
}

func TestWatchedPageChangeIsNew(t *testing.T) {
	watch := Watch{Name: "Status", Enabled: true, Channel: "telegram", Sources: []SourceSpec{{Kind: KindPage, Label: "Status page", Target: "https://status.example/"}}}
	h := newHarness(t, watch)
	h.fetch.bodies["https://status.example/"] = "<html><body><h1>All good</h1><script>x=1</script></body></html>"
	h.tick(t, 0)
	h.fetch.bodies["https://status.example/"] = "<html><body class=\"b\"><h1>All good</h1><script>x=2</script></body></html>"
	if o := h.tick(t, time.Hour); o[0].Status != StatusNothingNew {
		t.Fatalf("a markup-only change is not news: %+v", o)
	}
	h.fetch.bodies["https://status.example/"] = "<html><body><h1>Outage</h1></body></html>"
	if o := h.tick(t, time.Hour); o[0].Status != StatusSent || len(h.sent) != 1 {
		t.Fatalf("a text change is news: %+v", o)
	}
}

func TestTelegramFloorIsNotAFailure(t *testing.T) {
	watch := Watch{Name: "Chan", Enabled: true, Interval: MinInterval, Sources: []SourceSpec{{Kind: KindTelegram, Target: "telegram"}, {Kind: KindFeed, Target: feedURL}}}
	h := newHarness(t, watch)
	h.fetch.bodies[feedURL] = rss([2]string{"One", "https://e.example/1"})
	h.fetch.bodies["https://t.me/s/telegram"] = "<html></html>"
	h.run.source(watch.Sources[0]).(*sources.TelegramChannel).Now = func() time.Time { return h.now }
	h.tick(t, 0) // telegram page has no messages: failing source, feed answers
	o := h.tick(t, MinInterval)
	if o[0].Status == StatusFailing || strings.Contains(o[0].Detail, "too recently") {
		t.Fatalf("a source waiting out its floor must not show as failing: %+v", o)
	}
}

// A manual "Check now" soon after another read of a Telegram-only watch finds the source waiting out its floor. That is not a failed check: the
// watch's last result must stay what it was, and the answer says when to ask again.
func TestWaitingOutTheFloorIsNeitherFailingNorRecorded(t *testing.T) {
	watch := Watch{Name: "Chan", Enabled: true, Interval: MinInterval, Sources: []SourceSpec{{Kind: KindTelegram, Target: "telegram"}}}
	h := newHarness(t, watch)
	body, err := os.ReadFile("../sources/testdata/telegram-channel.html")
	if err != nil {
		t.Fatal(err)
	}
	h.fetch.bodies["https://t.me/s/telegram"] = string(body)
	h.run.source(watch.Sources[0]).(*sources.TelegramChannel).Now = func() time.Time { return h.now }
	first := h.run.Run(context.Background(), h.store.List()[0])
	if first.Status == StatusFailing {
		t.Fatalf("the first read must answer: %+v", first)
	}
	again := h.run.Run(context.Background(), h.store.List()[0])
	if again.Status != StatusWaiting || again.Detail == "" {
		t.Fatalf("a read inside the floor is waiting, not failing: %+v", again)
	}
	if got := h.store.List()[0]; got.LastStatus != first.Status {
		t.Fatalf("a waiting read must not overwrite the last result: %q, want %q", got.LastStatus, first.Status)
	}
}

func TestStorePersistsAndRecovers(t *testing.T) {
	dir := newMemDir()
	s, _ := NewStore(dir)
	made, err := s.Create(feedWatch(), t0)
	if err != nil {
		t.Fatal(err)
	}
	it := sources.Item{Source: "x", Link: "https://e.example/1", Title: "One"}
	if err := s.Record(made.ID, []sources.Item{it}, t0); err != nil {
		t.Fatal(err)
	}
	again, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := again.Unseen(made.ID, []sources.Item{it}, t0.Add(time.Hour)); len(got) != 0 {
		t.Error("the ledger must survive a restart")
	}
	if got := again.Unseen(made.ID, []sources.Item{it}, t0.Add(SeenRetention+time.Hour)); len(got) != 1 {
		t.Error("an entry past retention counts as unseen")
	}
	dir.files[FileName] = []byte("{not json")
	fresh, err := NewStore(dir)
	if err != nil || len(fresh.List()) != 0 || len(dir.quarantined) != 1 {
		t.Errorf("a corrupt file is set aside and the store starts empty: %v %v %v", err, fresh.List(), dir.quarantined)
	}
}

func TestStoreUpdateKeepsLedgerAndRunRecord(t *testing.T) {
	s, _ := NewStore(newMemDir())
	made, _ := s.Create(feedWatch(), t0)
	_ = s.Record(made.ID, []sources.Item{{Link: "https://e.example/1"}}, t0)
	_ = s.SetRun(made.ID, t0, StatusSent, "ok", 1, true)
	edit := feedWatch()
	edit.Name = "Renamed"
	edit.Interval = 2 * time.Hour
	got, err := s.Update(made.ID, edit)
	if err != nil || got.Name != "Renamed" || got.Interval != 2*time.Hour || !got.Baselined || got.LastStatus != StatusSent || s.SeenCount(made.ID) != 1 {
		t.Fatalf("update must keep the run record and the ledger: %+v, %v", got, err)
	}
	if err := s.Delete(made.ID); err != nil || s.SeenCount(made.ID) != 0 {
		t.Errorf("delete must drop the ledger: %v", err)
	}
	if _, err := s.Get(made.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("a deleted watch is gone: %v", err)
	}
}

func TestLedgerIsBounded(t *testing.T) {
	s, _ := NewStore(newMemDir())
	made, _ := s.Create(feedWatch(), t0)
	var items []sources.Item
	for i := range MaxSeen + 50 {
		items = append(items, sources.Item{Link: fmt.Sprintf("https://e.example/%d", i)})
	}
	if err := s.Record(made.ID, items, t0); err != nil {
		t.Fatal(err)
	}
	if n := s.SeenCount(made.ID); n != MaxSeen {
		t.Errorf("ledger = %d entries, want the cap %d", n, MaxSeen)
	}
}

func TestMaxWatches(t *testing.T) {
	s, _ := NewStore(newMemDir())
	for range MaxWatches {
		if _, err := s.Create(feedWatch(), t0); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Create(feedWatch(), t0); !errors.Is(err, ErrInvalid) {
		t.Errorf("the %dth watch must be refused: %v", MaxWatches+1, err)
	}
}
