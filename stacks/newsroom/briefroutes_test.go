package newsroom

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"llm-proxy-stacks/sdk"
)

// cronSchedule is a stand-in for the core's cron grammar that understands only what brief.Edition.Cron produces: "M H * * d,d,d".
type cronSchedule struct{}

func (cronSchedule) Next(expr string, after time.Time) (time.Time, error) {
	f := strings.Fields(expr)
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

// startBriefs starts the stack with the schedule port given (the Brief module needs it).
func startBriefs(t *testing.T, cfg string, fetch stacksdk.Fetch, every time.Duration) *Stack {
	t.Helper()
	s := New()
	s.tick = every
	h := stacksdk.Host{DataDir: &memDir{files: map[string][]byte{}}, Config: &fakeConfig{doc: cfg}, Fetch: fetch, Schedule: cronSchedule{}}
	if err := s.Start(context.Background(), h); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Stop(context.Background()) })
	return s
}

func call(t *testing.T, s *Stack, method, path, body string) (stacksdk.Response, error) {
	t.Helper()
	return s.Handle(context.Background(), stacksdk.Request{Method: method, Path: path, Body: []byte(body)})
}

const topicBody = `{"name":"Economy","ignore":["horoscope"],"sources":[{"kind":"feed","target":"https://example.com/feed.xml"}]}`

func editionBody(topicID int64) string {
	return fmt.Sprintf(`{"name":"Morning","ready_by":"07:00","enabled":true,"collect_every":"2h","collect_for":"12h","sections":[{"kind":"news","topic_id":%d}]}`, topicID)
}

func TestBriefRoutes_ATopicAnEditionAndACollectionEndToEnd(t *testing.T) {
	s := startBriefs(t, "", &rssFetch{}, time.Hour)
	resp, err := call(t, s, "POST", RouteBriefTopics, topicBody)
	if err != nil || resp.Status != http.StatusCreated {
		t.Fatalf("create topic: %+v %v", resp, err)
	}
	var topic struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(resp.Body, &topic)
	if resp, err = call(t, s, "POST", RouteBriefEditions, editionBody(topic.ID)); err != nil || resp.Status != http.StatusCreated || !strings.Contains(string(resp.Body), `"collect_every":"2h0m0s"`) {
		t.Fatalf("create edition (durations are strings): %s %v", resp.Body, err)
	}
	collect := fmt.Sprintf("%stopics/%d/collect", RouteBriefs, topic.ID)
	if resp, err = call(t, s, "POST", collect, ""); err != nil || !strings.Contains(string(resp.Body), `"new":1`) {
		t.Fatalf("collect: %s %v", resp.Body, err)
	}
	if resp, err = s.Handle(context.Background(), stacksdk.Request{Method: "GET", Path: RouteBriefPool, Query: fmt.Sprintf("topic=%d", topic.ID)}); err != nil || !strings.Contains(string(resp.Body), "First") {
		t.Fatalf("pool: %s %v", resp.Body, err)
	}
	if _, err = call(t, s, "DELETE", fmt.Sprintf("%stopics/%d", RouteBriefs, topic.ID), ""); stacksdk.CodeOf(err) != stacksdk.CodeInvalid {
		t.Errorf("a topic an edition lists cannot be deleted: %v", err)
	}
}

// An edition is made on request (with no model it is ranked by outlets), stored, and read back by list and by id; reading asks nothing more of anything.
func TestBriefRoutes_AnEditionIsMadeOnRequestAndReadBack(t *testing.T) {
	s := startBriefs(t, "", &rssFetch{}, time.Hour)
	resp, _ := call(t, s, "POST", RouteBriefTopics, topicBody)
	var topic struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(resp.Body, &topic)
	resp, _ = call(t, s, "POST", RouteBriefEditions, editionBody(topic.ID))
	var ed struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(resp.Body, &ed)
	if _, err := call(t, s, "POST", fmt.Sprintf("%stopics/%d/collect", RouteBriefs, topic.ID), ""); err != nil {
		t.Fatal(err)
	}
	made, err := call(t, s, "POST", fmt.Sprintf("%seditions/%d/issue", RouteBriefs, ed.ID), "")
	if err != nil || made.Status != http.StatusCreated || !strings.Contains(string(made.Body), "First") || !strings.Contains(string(made.Body), "no model is set up") {
		t.Fatalf("make: %s %v", made.Body, err)
	}
	var is struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(made.Body, &is)
	if resp, err = call(t, s, "GET", fmt.Sprintf("%sissues/%d", RouteBriefs, is.ID), ""); err != nil || string(resp.Body) != string(made.Body) {
		t.Errorf("by id: %s %v", resp.Body, err)
	}
	if resp, err = call(t, s, "GET", RouteBriefIssues, ""); err != nil || !strings.HasPrefix(string(resp.Body), "[{") {
		t.Errorf("list: %s %v", resp.Body, err)
	}
	if _, err = call(t, s, "POST", RouteBriefs+"editions/99/issue", ""); stacksdk.CodeOf(err) != stacksdk.CodeNotFound {
		t.Errorf("an unknown edition is not found, got %v", err)
	}
	if _, err = call(t, s, "GET", RouteBriefs+"issues/99", ""); stacksdk.CodeOf(err) != stacksdk.CodeNotFound {
		t.Errorf("an unknown edition issue is not found, got %v", err)
	}
}

// keptConfig is a settings document that really keeps what is saved, merged over what was there, as the core's Config port does.
type keptConfig struct{ doc map[string]json.RawMessage }

func (c *keptConfig) Load(context.Context) (json.RawMessage, error) { return json.Marshal(c.doc) }
func (c *keptConfig) Save(_ context.Context, doc json.RawMessage) error {
	var in map[string]json.RawMessage
	if err := json.Unmarshal(doc, &in); err != nil {
		return err
	}
	for k, v := range in {
		c.doc[k] = v
	}
	return nil
}

// The daily limit of model calls is read and changed on its own: the module switches and the timezone beside it are kept, and a value out of bounds is refused in words.
func TestBriefRoutes_TheDailyModelLimitIsReadAndChangedAlone(t *testing.T) {
	cfg := &keptConfig{doc: map[string]json.RawMessage{"timezone": json.RawMessage(`"Europe/London"`), "briefs_disabled": json.RawMessage("false")}}
	s := New()
	h := stacksdk.Host{DataDir: &memDir{files: map[string][]byte{}}, Config: cfg, Fetch: &rssFetch{}, Schedule: cronSchedule{}}
	if err := s.Start(context.Background(), h); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Stop(context.Background()) })
	resp, err := call(t, s, "GET", RouteBriefSettings, "")
	if err != nil || !strings.Contains(string(resp.Body), `"model_calls_per_day":0`) || !strings.Contains(string(resp.Body), `"default":200`) || !strings.Contains(string(resp.Body), `"max":2000`) {
		t.Fatalf("read: %s %v", resp.Body, err)
	}
	if resp, err = call(t, s, "PUT", RouteBriefSettings, `{"model_calls_per_day":400}`); err != nil || !strings.Contains(string(resp.Body), `"model_calls_per_day":400`) {
		t.Fatalf("change: %s %v", resp.Body, err)
	}
	if string(cfg.doc["timezone"]) != `"Europe/London"` {
		t.Errorf("the timezone was disturbed: %s", cfg.doc["timezone"])
	}
	for _, bad := range []string{`{"model_calls_per_day":-1}`, `{"model_calls_per_day":2001}`, `{"model_calls_per_day":"many"}`, `{`} {
		if _, err := call(t, s, "PUT", RouteBriefSettings, bad); stacksdk.CodeOf(err) != stacksdk.CodeInvalid {
			t.Errorf("%s must be refused, got %v", bad, err)
		}
	}
	if string(cfg.doc["model_calls_per_day"]) != "400" {
		t.Errorf("a refused change must save nothing: %s", cfg.doc["model_calls_per_day"])
	}
}

func TestBriefRoutes_ListsReadAsArraysAndBadInputIsRefusedInWords(t *testing.T) {
	s := startBriefs(t, "", &rssFetch{}, time.Hour)
	for _, path := range []string{RouteBriefTopics, RouteBriefEditions, RouteBriefPool, RouteBriefIssues} {
		if resp, err := call(t, s, "GET", path, ""); err != nil || string(resp.Body) != "[]" {
			t.Errorf("%s on an empty store must read []: %s %v", path, resp.Body, err)
		}
	}
	for name, tc := range map[string]struct{ method, path, body string }{
		"bad json":        {"POST", RouteBriefTopics, "{"},
		"empty topic":     {"POST", RouteBriefTopics, `{"name":"x"}`},
		"bad duration":    {"POST", RouteBriefEditions, `{"name":"x","ready_by":"07:00","collect_every":"often","sections":[]}`},
		"id not a number": {"PUT", RouteBriefs + "topics/abc", topicBody},
		"unknown route":   {"GET", RouteBriefs + "nothing", ""},
		"pool bad topic":  {"GET", RouteBriefPool + "?x", ""},
	} {
		if _, err := call(t, s, tc.method, tc.path, tc.body); stacksdk.CodeOf(err) == "" || stacksdk.CodeOf(err) == stacksdk.CodeInternal {
			t.Errorf("%s must be refused with a code the screen can word, got %v", name, err)
		}
	}
	if _, err := call(t, s, "PUT", RouteBriefs+"topics/99", topicBody); stacksdk.CodeOf(err) != stacksdk.CodeNotFound {
		t.Errorf("an unknown topic is not found, got %v", err)
	}
}

func TestBriefRoutes_NeedTheSchedulePortAndTheSwitch(t *testing.T) {
	if _, err := call(t, New(), "GET", RouteBriefTopics, ""); stacksdk.CodeOf(err) != stacksdk.CodeUnavailable {
		t.Errorf("before Start the briefs are unavailable: %v", err)
	}
	noSchedule := start(t, "", &rssFetch{}) // the host gave no schedule port
	if _, err := call(t, noSchedule, "GET", RouteBriefTopics, ""); stacksdk.CodeOf(err) != stacksdk.CodeUnavailable {
		t.Errorf("without the schedule port the briefs are unavailable, not half-working: %v", err)
	}
	off := startBriefs(t, `{"briefs_disabled":true}`, &rssFetch{}, time.Hour)
	resp, _ := call(t, off, "POST", RouteBriefTopics, topicBody)
	var topic struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(resp.Body, &topic)
	if _, err := call(t, off, "POST", fmt.Sprintf("%stopics/%d/collect", RouteBriefs, topic.ID), ""); stacksdk.CodeOf(err) != stacksdk.CodeUnavailable {
		t.Errorf("a manual collection follows the Briefs switch: %v", err)
	}
}

func TestLoopCollectsWhileTheBriefsModuleIsOnAndLeavesWatchesAlone(t *testing.T) {
	for _, tc := range []struct {
		cfg  string
		want bool
	}{{"", true}, {`{"briefs_disabled":true}`, false}, {`{"disabled":true}`, true}} {
		f := &rssFetch{}
		s := startBriefs(t, tc.cfg, f, 10*time.Millisecond)
		resp, _ := call(t, s, "POST", RouteBriefTopics, topicBody)
		var topic struct {
			ID int64 `json:"id"`
		}
		_ = json.Unmarshal(resp.Body, &topic)
		// Ready half an hour from now, collecting for two: the window is open whatever time of day the test runs.
		ready := time.Now().UTC().Add(30 * time.Minute).Format("15:04")
		body := fmt.Sprintf(`{"name":"Soon","ready_by":"%s","enabled":true,"collect_every":"1h","collect_for":"2h","sections":[{"kind":"news","topic_id":%d}]}`, ready, topic.ID)
		if _, err := call(t, s, "POST", RouteBriefEditions, body); err != nil {
			t.Fatal(err)
		}
		time.Sleep(200 * time.Millisecond)
		if got := f.count() > 0; got != tc.want {
			t.Errorf("config %q: collected = %v, want %v", tc.cfg, got, tc.want)
		}
	}
}

func TestEraseAllClearsTheBriefsToo(t *testing.T) {
	s := startBriefs(t, "", &rssFetch{}, time.Hour)
	if _, err := call(t, s, "POST", RouteBriefTopics, topicBody); err != nil {
		t.Fatal(err)
	}
	if err := s.EraseAll(); err != nil {
		t.Fatal(err)
	}
	if resp, _ := call(t, s, "GET", RouteBriefTopics, ""); string(resp.Body) != "[]" {
		t.Errorf("a factory reset forgets the topics: %s", resp.Body)
	}
}
