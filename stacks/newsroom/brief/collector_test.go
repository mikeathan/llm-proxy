package brief

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"llm-proxy-stacks/newsroom/sources"
	"llm-proxy-stacks/sdk"
)

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

// cronSchedule is a stand-in for the core's cron grammar that understands only what Edition.Cron produces: "M H * * d,d,d".
type cronSchedule struct{}

func (cronSchedule) Next(expr string, after time.Time) (time.Time, error) {
	f := strings.Fields(expr)
	if len(f) != 5 {
		return time.Time{}, stacksdk.Errorf(stacksdk.CodeInvalid, "five fields")
	}
	min, _ := strconv.Atoi(f[0])
	hour, _ := strconv.Atoi(f[1])
	days := map[int]bool{}
	for _, d := range strings.Split(f[4], ",") {
		n, _ := strconv.Atoi(d)
		days[n] = true
	}
	for i := range 9 {
		c := time.Date(after.Year(), after.Month(), after.Day()+i, hour, min, 0, 0, after.Location())
		if c.After(after) && days[int(c.Weekday())] {
			return c, nil
		}
	}
	return time.Time{}, nil
}

const (
	bbcURL  = "https://feeds.example/bbc"
	guarURL = "https://feeds.example/guardian"
)

type rig struct {
	store *Store
	fetch *feedFetch
	col   *Collector
	now   time.Time
	topic Topic
}

// newRig has one topic reading two feeds and one edition, ready by 07:00 every day, collecting every 2h for 4h.
func newRig(t *testing.T, edition func(*Edition)) *rig {
	t.Helper()
	store, _ := newStore(t)
	r := &rig{store: store, fetch: &feedFetch{bodies: map[string]string{
		bbcURL:  rss([2]string{"Markets fall as tech sells off", "https://bbc.example/1"}),
		guarURL: rss([2]string{"Markets fall, as tech sells off!", "https://guardian.example/1"}, [2]string{"Horoscope for today", "https://guardian.example/2"}),
	}}, now: time.Date(2026, 11, 21, 1, 0, 0, 0, time.UTC)}
	var err error
	r.topic, err = store.CreateTopic(Topic{Name: "Economy", Ignore: []string{"horoscope"}, Sources: []sources.Spec{
		{Kind: sources.KindFeed, Label: "BBC", Target: bbcURL}, {Kind: sources.KindFeed, Label: "Guardian", Target: guarURL}}}, r.now)
	if err != nil {
		t.Fatal(err)
	}
	ed := Edition{Name: "Morning", ReadyBy: "07:00", Enabled: true, CollectEvery: 2 * time.Hour, CollectFor: 4 * time.Hour, Sections: []Section{{Kind: SectionNews, TopicID: r.topic.ID}}}
	if edition != nil {
		edition(&ed)
	}
	if _, err := store.CreateEdition(ed, r.now); err != nil {
		t.Fatal(err)
	}
	r.col = &Collector{Store: store, Readers: &sources.Registry{Fetch: r.fetch}, Timing: Timing{Schedule: cronSchedule{}, Now: func() time.Time { return r.now }}}
	return r
}

func (r *rig) tickAt(t *testing.T, h, m int) []Outcome {
	t.Helper()
	r.now = time.Date(2026, 11, 21, h, m, 0, 0, time.UTC)
	out, err := r.col.Tick(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCollector_OnlyCollectsInsideTheWindowAndAtTheChosenInterval(t *testing.T) {
	r := newRig(t, nil)
	if out := r.tickAt(t, 1, 0); len(out) != 0 || r.fetch.calls != 0 {
		t.Fatalf("ready is 06 hours away and the window is 4: nothing is read yet (%v, %d calls)", out, r.fetch.calls)
	}
	if out := r.tickAt(t, 3, 30); len(out) != 1 || r.fetch.calls != 2 {
		t.Fatalf("inside the window the topic is read once: %v, %d calls", out, r.fetch.calls)
	}
	if out := r.tickAt(t, 4, 30); len(out) != 0 || r.fetch.calls != 2 {
		t.Fatalf("one hour later is inside the 2h interval: %v, %d calls", out, r.fetch.calls)
	}
	if out := r.tickAt(t, 5, 31); len(out) != 1 || r.fetch.calls != 4 {
		t.Fatalf("after the interval it is read again: %v, %d calls", out, r.fetch.calls)
	}
	if out := r.tickAt(t, 8, 0); len(out) != 0 {
		t.Errorf("after ready by, tomorrow's window is far away: %v", out)
	}
}

func TestCollector_StoresEachStoryOnceWithItsOutlets(t *testing.T) {
	r := newRig(t, nil)
	out := r.tickAt(t, 3, 30)
	if len(out) != 1 || out[0].New != 1 {
		t.Fatalf("two outlets, one story, and the horoscope ignored: %+v", out)
	}
	pool := r.store.Pool()
	if len(pool) != 1 || pool[0].Outlets() != 2 {
		t.Fatalf("pool = %+v", pool)
	}
	if out := r.tickAt(t, 5, 31); out[0].New != 0 {
		t.Errorf("a second read of the same headlines adds nothing: %+v", out)
	}
}

func TestCollector_AFailingSourceIsNamedAndTheOtherStillCounts(t *testing.T) {
	r := newRig(t, nil)
	delete(r.fetch.bodies, guarURL)
	out := r.tickAt(t, 3, 30)
	if len(out) != 1 || out[0].New != 1 || len(out[0].Problems) != 1 || strings.Count(out[0].Problems[0], "Guardian") != 1 {
		t.Fatalf("outcome = %+v", out)
	}
	topic, _ := r.store.Topic(r.topic.ID)
	if len(topic.LastProblems) != 1 || topic.LastCollected.IsZero() {
		t.Errorf("the problem stays on the topic: %+v", topic)
	}
	r.fetch.bodies[guarURL] = rss([2]string{"Another story", "https://guardian.example/3"})
	r.tickAt(t, 5, 31)
	if topic, _ = r.store.Topic(r.topic.ID); len(topic.LastProblems) != 0 {
		t.Errorf("a source that reads again clears its problem: %v", topic.LastProblems)
	}
}

func TestCollector_ADisabledEditionCollectsNothing(t *testing.T) {
	r := newRig(t, func(e *Edition) { e.Enabled = false })
	if out := r.tickAt(t, 3, 30); len(out) != 0 || r.fetch.calls != 0 {
		t.Errorf("a disabled edition collects nothing: %v", out)
	}
}

func TestCollector_ATopicNoEditionListsIsNeverRead(t *testing.T) {
	r := newRig(t, nil)
	unlisted, err := r.store.CreateTopic(Topic{Name: "Unlisted", Sources: []sources.Spec{{Kind: sources.KindFeed, Label: "Other", Target: "https://feeds.example/other"}}}, r.now)
	if err != nil {
		t.Fatal(err)
	}
	out := r.tickAt(t, 3, 30)
	got, _ := r.store.Topic(unlisted.ID)
	if len(out) != 1 || out[0].TopicID != r.topic.ID || !got.LastCollected.IsZero() {
		t.Errorf("only the listed topic is read: %+v, unlisted collected %v", out, got.LastCollected)
	}
}

// deletingFetch removes a topic the first time a source is read, as the owner could while a pass is running.
type deletingFetch struct {
	*feedFetch
	delete func()
}

func (f *deletingFetch) Get(ctx context.Context, req stacksdk.FetchRequest) (stacksdk.FetchResponse, error) {
	if f.delete != nil {
		f.delete()
		f.delete = nil
	}
	return f.feedFetch.Get(ctx, req)
}

func TestCollector_ATopicDeletedMidPassDoesNotStopTheOthers(t *testing.T) {
	r := newRig(t, nil)
	second, err := r.store.CreateTopic(Topic{Name: "Second", Sources: []sources.Spec{{Kind: sources.KindFeed, Label: "Other", Target: bbcURL}}}, r.now)
	if err != nil {
		t.Fatal(err)
	}
	ed, _ := r.store.CreateEdition(Edition{Name: "Both", ReadyBy: "07:00", Enabled: true, CollectEvery: 2 * time.Hour, CollectFor: 4 * time.Hour,
		Sections: []Section{{Kind: SectionNews, TopicID: r.topic.ID}, {Kind: SectionNews, TopicID: second.ID}}}, r.now)
	_ = ed
	// The first topic is read first. While it is, the owner deletes every edition and then the second topic, so the second topic's collection finds
	// nothing to record.
	f := &deletingFetch{feedFetch: r.fetch}
	f.delete = func() {
		for _, e := range r.store.Editions() {
			_ = r.store.DeleteEdition(e.ID)
		}
		_ = r.store.DeleteTopic(second.ID)
	}
	r.col.Readers = &sources.Registry{Fetch: f}
	r.now = time.Date(2026, 11, 21, 3, 30, 0, 0, time.UTC)
	if _, err := r.col.Tick(context.Background()); err != nil {
		t.Fatalf("a topic deleted mid-pass must not fail the pass: %v", err)
	}
}

func TestCollector_ATopicInTwoEditionsFollowsTheShorterInterval(t *testing.T) {
	r := newRig(t, nil)
	if _, err := r.store.CreateEdition(Edition{Name: "Often", ReadyBy: "07:00", Enabled: true, CollectEvery: time.Hour, CollectFor: 4 * time.Hour, Sections: []Section{{Kind: SectionNews, TopicID: r.topic.ID}}}, r.now); err != nil {
		t.Fatal(err)
	}
	r.tickAt(t, 3, 30)
	if out := r.tickAt(t, 4, 31); len(out) != 1 {
		t.Errorf("the hourly edition makes the topic due after an hour: %v", out)
	}
}

func TestCollector_TheReadyTimeIsReadInTheStacksTimezone(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("no timezone data")
	}
	r := newRig(t, nil)
	r.col.Location = func() *time.Location { return ny }
	// 07:00 in New York on 21 November is 12:00 UTC, so the 4-hour window opens at 08:00 UTC: at 07:00 UTC it is closed, at 09:00 UTC it is open.
	if out := r.tickAt(t, 7, 0); len(out) != 0 {
		t.Errorf("07:00 UTC is 02:00 in New York, five hours before ready: %v", out)
	}
	if out := r.tickAt(t, 9, 0); len(out) != 1 {
		t.Errorf("09:00 UTC is 04:00 in New York, three hours before ready: %v", out)
	}
}

func TestCollector_CollectNowIgnoresTheSchedule(t *testing.T) {
	r := newRig(t, nil)
	o, err := r.col.CollectNow(context.Background(), r.topic.ID)
	if err != nil || o.New != 1 {
		t.Fatalf("outcome %+v, err %v", o, err)
	}
	if _, err := r.col.CollectNow(context.Background(), 999); err == nil {
		t.Error("an unknown topic is an error")
	}
}

func TestCollector_CollectStaleReadsOnlyTopicsNotReadWithinTheInterval(t *testing.T) {
	r := newRig(t, nil)
	eds := r.store.Editions()
	// Outside the collection window, and never collected: write-now must still read it.
	out, err := r.col.CollectStale(context.Background(), eds[0].ID, nil)
	if err != nil || len(out) != 1 || out[0].New != 1 {
		t.Fatalf("never-collected topic: %+v, err %v", out, err)
	}
	// Collected a moment ago (inside the edition's interval): left alone.
	calls := r.fetch.calls
	out, err = r.col.CollectStale(context.Background(), eds[0].ID, nil)
	if err != nil || len(out) != 0 || r.fetch.calls != calls {
		t.Fatalf("fresh topic was read again: %+v, err %v", out, err)
	}
	// Past the interval: read again.
	r.now = r.now.Add(eds[0].CollectEvery + time.Minute)
	if out, err = r.col.CollectStale(context.Background(), eds[0].ID, nil); err != nil || len(out) != 1 {
		t.Fatalf("stale topic: %+v, err %v", out, err)
	}
	if _, err := r.col.CollectStale(context.Background(), 999, nil); err == nil {
		t.Error("an unknown edition is an error")
	}
}

func TestCollector_EverySourceFailingIsRecordedWithItsProblems(t *testing.T) {
	store, _ := newStore(t)
	topic, _ := store.CreateTopic(Topic{Name: "Down", Sources: []sources.Spec{{Kind: sources.KindFeed, Label: "x", Target: bbcURL}}}, t0)
	col := &Collector{Store: store, Readers: &sources.Registry{Fetch: &feedFetch{}}, Timing: Timing{Schedule: cronSchedule{}, Now: func() time.Time { return t0 }}}
	o, err := col.CollectNow(context.Background(), topic.ID)
	got, _ := store.Topic(topic.ID)
	if err != nil || len(o.Problems) != 1 || o.New != 0 || len(got.LastProblems) != 1 || got.LastCollected.IsZero() {
		t.Errorf("every source failing is recorded with its problem: %+v %+v %v", o, got, err)
	}
}
