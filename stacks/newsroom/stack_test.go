package newsroom

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"llm-proxy-stacks/sdk"
)

type memDir struct {
	mu    sync.Mutex
	files map[string][]byte
}

func (m *memDir) Read(name string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.files[name]
	if !ok {
		return nil, stacksdk.Errorf(stacksdk.CodeNotFound, "no file")
	}
	return b, nil
}

func (m *memDir) Write(name string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[name] = append([]byte(nil), data...)
	return nil
}

func (m *memDir) Quarantine(string) error { return nil }

type fakeConfig struct{ doc string }

func (c *fakeConfig) Load(context.Context) (json.RawMessage, error) {
	return json.RawMessage(c.doc), nil
}
func (c *fakeConfig) Save(context.Context, json.RawMessage) error { return nil }

// rssFetch answers every address with one RSS item, and counts the reads.
type rssFetch struct {
	mu    sync.Mutex
	reads int
}

func (f *rssFetch) Get(context.Context, stacksdk.FetchRequest) (stacksdk.FetchResponse, error) {
	f.mu.Lock()
	f.reads++
	f.mu.Unlock()
	body := `<rss version="2.0"><channel><title>t</title><item><title>First</title><link>https://example.com/1</link><guid>1</guid></item></channel></rss>`
	return stacksdk.FetchResponse{Body: []byte(body)}, nil
}

func (f *rssFetch) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.reads
}

func start(t *testing.T, cfg string, fetch stacksdk.Fetch) *Stack {
	return startEvery(t, cfg, fetch, 10*time.Millisecond)
}

// startEvery starts the stack with the loop's period given; a long one leaves the loop out of a test that drives runs by hand.
func startEvery(t *testing.T, cfg string, fetch stacksdk.Fetch, every time.Duration) *Stack {
	t.Helper()
	s := New()
	s.tick = every
	if err := s.Start(context.Background(), stacksdk.Host{DataDir: &memDir{files: map[string][]byte{}}, Config: &fakeConfig{doc: cfg}, Fetch: fetch}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Stop(context.Background()) })
	return s
}

const feedTopic = `{"name":"Go","interval":"5m","enabled":true,"sources":[{"kind":"feed","target":"https://example.com/feed.xml"}]}`

func TestManifestIsValidAndRoutesAreAdminOnly(t *testing.T) {
	m := New().Manifest()
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, r := range m.Routes {
		if r.Level() != stacksdk.AccessAdmin {
			t.Errorf("%s %s declares access %q", r.Method, r.Path, r.Level())
		}
	}
}

func TestNotRunningRefusesEveryCall(t *testing.T) {
	s := New()
	if _, err := s.Handle(context.Background(), stacksdk.Request{Method: "GET", Path: RouteTopics}); stacksdk.CodeOf(err) != stacksdk.CodeUnavailable {
		t.Errorf("a route before Start is unavailable, got %v", err)
	}
	if _, err := s.CallTool(context.Background(), ToolList, nil); stacksdk.CodeOf(err) != stacksdk.CodeUnavailable {
		t.Errorf("a tool before Start is unavailable, got %v", err)
	}
	if st, err := s.Status(context.Background()); st != nil || err != nil {
		t.Errorf("a stack that is not running reports nothing, got %v, %v", st, err)
	}
}

func TestCreateListUpdateDeleteThroughRoutes(t *testing.T) {
	s := start(t, "", &rssFetch{})
	ctx := context.Background()
	got, err := s.Handle(ctx, stacksdk.Request{Method: "POST", Path: RouteTopics, Body: []byte(feedTopic)})
	if err != nil || got.Status != http.StatusCreated {
		t.Fatalf("create = %+v, %v", got, err)
	}
	var made struct {
		ID       int64  `json:"id"`
		Interval string `json:"interval"`
	}
	if err := json.Unmarshal(got.Body, &made); err != nil || made.ID == 0 || made.Interval != "5m0s" {
		t.Fatalf("created topic = %s (%v)", got.Body, err)
	}
	list, _ := s.Handle(ctx, stacksdk.Request{Method: "GET", Path: RouteTopics})
	if !strings.Contains(string(list.Body), `"name":"Go"`) {
		t.Errorf("list = %s", list.Body)
	}
	params := map[string]string{"id": "1"}
	upd := strings.Replace(feedTopic, `"Go"`, `"Rust"`, 1)
	if got, err = s.Handle(ctx, stacksdk.Request{Method: "PUT", Path: topicPath("1", ""), Params: params, Body: []byte(upd)}); err != nil || !strings.Contains(string(got.Body), "Rust") {
		t.Errorf("update = %s, %v", got.Body, err)
	}
	if got, err = s.Handle(ctx, stacksdk.Request{Method: "DELETE", Path: topicPath("1", ""), Params: params}); err != nil || got.Status != http.StatusNoContent {
		t.Errorf("delete = %+v, %v", got, err)
	}
	if _, err = s.Handle(ctx, stacksdk.Request{Method: "DELETE", Path: topicPath("1", ""), Params: params}); stacksdk.CodeOf(err) != stacksdk.CodeNotFound {
		t.Errorf("a second delete is not_found, got %v", err)
	}
}

func TestRefusalsAreInvalidNotServerErrors(t *testing.T) {
	s := start(t, "", &rssFetch{})
	ctx := context.Background()
	for name, body := range map[string]string{
		"bad json":     "{",
		"no sources":   `{"name":"x","sources":[]}`,
		"bad interval": `{"name":"x","interval":"soon","sources":[{"kind":"feed","target":"https://e.com/f"}]}`,
		"too short":    `{"name":"x","interval":"1s","sources":[{"kind":"feed","target":"https://e.com/f"}]}`,
	} {
		if _, err := s.Handle(ctx, stacksdk.Request{Method: "POST", Path: RouteTopics, Body: []byte(body)}); stacksdk.CodeOf(err) != stacksdk.CodeInvalid {
			t.Errorf("%s: want invalid, got %v", name, err)
		}
	}
	if _, err := s.Handle(ctx, stacksdk.Request{Method: "PUT", Path: topicPath("x", ""), Params: map[string]string{"id": "x"}}); stacksdk.CodeOf(err) != stacksdk.CodeNotFound {
		t.Errorf("a non-numeric id is not_found, got %v", err)
	}
}

func TestFollowToolCreatesAFeedOnlyTopic(t *testing.T) {
	s := start(t, "", &rssFetch{})
	out, err := s.CallTool(context.Background(), ToolFollow, json.RawMessage(`{"name":"Go","channel":"telegram","enabled":false,"sources":[{"kind":"feed","target":"https://example.com/f"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	var made struct {
		Channel string `json:"channel"`
		Enabled bool   `json:"enabled"`
	}
	if json.Unmarshal(out, &made); made.Channel != "" || !made.Enabled {
		t.Errorf("the assistant's topic is on and feed-only, got %s", out)
	}
	if _, err := s.CallTool(context.Background(), "nope", nil); stacksdk.CodeOf(err) != stacksdk.CodeNotFound {
		t.Errorf("an undeclared tool is not_found, got %v", err)
	}
}

func TestManualRunBaselinesThenFeedStaysEmpty(t *testing.T) {
	s := startEvery(t, "", &rssFetch{}, time.Hour)
	ctx := context.Background()
	s.Handle(ctx, stacksdk.Request{Method: "POST", Path: RouteTopics, Body: []byte(feedTopic)})
	got, err := s.Handle(ctx, stacksdk.Request{Method: "POST", Path: topicPath("1", "/run"), Params: map[string]string{"id": "1"}})
	if err != nil || !strings.Contains(string(got.Body), "baseline") {
		t.Fatalf("first run = %s, %v", got.Body, err)
	}
	feed, _ := s.Handle(ctx, stacksdk.Request{Method: "GET", Path: RouteFeed})
	if strings.TrimSpace(string(feed.Body)) != "[]" && strings.TrimSpace(string(feed.Body)) != "null" {
		t.Errorf("a baseline announces nothing, feed = %s", feed.Body)
	}
}

func TestLoopReadsDueTopicsOnlyWhileTheModuleIsOn(t *testing.T) {
	for _, tc := range []struct {
		cfg  string
		want bool
	}{{"", true}, {`{"disabled":true}`, false}} {
		f := &rssFetch{}
		s := start(t, tc.cfg, f)
		if _, err := s.Handle(context.Background(), stacksdk.Request{Method: "POST", Path: RouteTopics, Body: []byte(feedTopic)}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(150 * time.Millisecond)
		if got := f.count() > 0; got != tc.want {
			t.Errorf("config %q: read = %v, want %v", tc.cfg, got, tc.want)
		}
		st, _ := s.Status(context.Background())
		if len(st) != 1 || st[0].Enabled == (tc.cfg != "") {
			t.Errorf("config %q: status = %+v", tc.cfg, st)
		}
	}
}

func TestStopEndsTheLoopAndAStartedStackRefusesASecondStart(t *testing.T) {
	s := start(t, "", &rssFetch{})
	if err := s.Start(context.Background(), stacksdk.Host{}); err == nil {
		t.Error("a second Start must be refused")
	}
	if err := s.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Stop(context.Background()); err != nil {
		t.Errorf("Stop twice is harmless, got %v", err)
	}
}

func TestMarkReadNeedsIDs(t *testing.T) {
	s := start(t, "", &rssFetch{})
	if _, err := s.Handle(context.Background(), stacksdk.Request{Method: "POST", Path: RouteRead, Body: []byte(`{"ids":[]}`)}); stacksdk.CodeOf(err) != stacksdk.CodeInvalid {
		t.Errorf("empty ids are invalid, got %v", err)
	}
}

type fakeModel struct {
	mu    sync.Mutex
	asked []stacksdk.ModelRequest
}

func (f *fakeModel) Complete(_ context.Context, r stacksdk.ModelRequest) (stacksdk.ModelResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, r)
	return stacksdk.ModelResponse{Text: `{"changes":[],"repeats":[],"state":"a note"}`}, nil
}

func TestTheModelPortFillsTheAnalystOnlyWhenTheCoreGivesIt(t *testing.T) {
	for _, withModel := range []bool{false, true} {
		fm := &fakeModel{}
		s := New()
		h := stacksdk.Host{DataDir: &memDir{files: map[string][]byte{}}, Config: &fakeConfig{}, Fetch: &rssFetch{}}
		if withModel {
			h.Model = fm
		}
		if err := s.Start(context.Background(), h); err != nil {
			t.Fatal(err)
		}
		if got := s.runner.Analyzer != nil; got != withModel {
			t.Errorf("model given %v: analyzer set = %v", withModel, got)
		}
		_ = s.Stop(context.Background())
	}
}

func TestTheManifestRequiresTheModelPort(t *testing.T) {
	if !slices.Contains(New().Manifest().Requires, stacksdk.PortModel) {
		t.Error("the Newsroom manifest must require the model port")
	}
}

func TestAManualRunFollowsTheSwitchAndFetchesNothingWhileOff(t *testing.T) {
	f := &rssFetch{}
	s := start(t, `{"disabled":true}`, f)
	s.Handle(context.Background(), stacksdk.Request{Method: "POST", Path: RouteTopics, Body: []byte(feedTopic)})
	_, err := s.Handle(context.Background(), stacksdk.Request{Method: "POST", Path: topicPath("1", "/run"), Params: map[string]string{"id": "1"}})
	if stacksdk.CodeOf(err) != stacksdk.CodeUnavailable || f.count() != 0 {
		t.Errorf("err = %v, fetches = %d", err, f.count())
	}
}

func TestEraseAllEmptiesTheTopicsAndIsHarmlessWhenNotRunning(t *testing.T) {
	if err := New().EraseAll(); err != nil {
		t.Errorf("a stack that is not running has nothing to erase, got %v", err)
	}
	s := start(t, "", &rssFetch{})
	s.Handle(context.Background(), stacksdk.Request{Method: "POST", Path: RouteTopics, Body: []byte(feedTopic)})
	if err := s.EraseAll(); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Handle(context.Background(), stacksdk.Request{Method: "GET", Path: RouteTopics})
	if strings.TrimSpace(string(got.Body)) != "[]" {
		t.Errorf("topics after erase = %s", got.Body)
	}
}

// topicPath is the path the core really passes: the route with the id filled in, not the {id} pattern.
func topicPath(id, suffix string) string { return RouteTopics + "/" + id + suffix }
