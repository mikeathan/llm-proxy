// Package newsroom is the Newsroom stack: it follows topics the owner names, reads their sources on a timer, keeps what is new in a feed and, for a
// topic with a channel, hands the core a notice to send through the Notice port (the stack never sends; the core words and sends it).
// The reading, the ledger and the feed are in package topics; this file is the stack's one lifecycle place: the timer loop, the module switch, the
// routes and the tools.
package newsroom

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"llm-proxy-stacks/newsroom/topics"
	"llm-proxy-stacks/sdk"
)

// The ids of the stack, its Topics module, its settings document and its one file.
const (
	ID          = "newsroom"
	ModuleID    = "topics"
	SettingsKey = "newsroom"
	TopicsFile  = "topics.json"
)

// The routes the Newsroom screen reads, and the tools the assistant calls.
const (
	RouteTopics = "/admin/api/newsroom/topics"
	RouteTopic  = "/admin/api/newsroom/topics/{id}"
	RouteRun    = "/admin/api/newsroom/topics/{id}/run"
	RouteFeed   = "/admin/api/newsroom/feed"
	RouteRead   = "/admin/api/newsroom/feed/read"

	ToolFollow = "newsroom_follow"
	ToolList   = "newsroom_topics"
)

// TickEvery is how often the loop looks for topics that are due; each topic keeps its own interval (at least topics.MinInterval).
const TickEvery = time.Minute

// feedDays is how many days one feed answer carries.
const feedDays = 30

// Settings is the stack's settings document. Disabled is the module's switch, stored as an "off" flag so a fresh install is on.
type Settings struct {
	Timezone string `json:"timezone,omitempty"`
	Disabled bool   `json:"disabled,omitempty"`
}

// Stack is the Newsroom stack.
type Stack struct {
	mu      sync.Mutex
	h       stacksdk.Host
	store   *topics.Store
	runner  *topics.Runner
	cancel  context.CancelFunc
	done    chan struct{}
	lastRun time.Time
	// tick is the loop's period; a test shortens it.
	tick time.Duration
}

// New returns the stack.
func New() *Stack { return &Stack{tick: TickEvery} }

// Manifest declares the stack as data.
func (s *Stack) Manifest() stacksdk.Manifest {
	return stacksdk.Manifest{
		ID: ID, Title: "Newsroom", Purpose: "Follow topics you care about and see what is new.",
		Layers: []stacksdk.Layer{{ID: "follow", Title: "Follow", Purpose: "What you follow and what changed"}},
		Modules: []stacksdk.ModuleSpec{{
			ID: ModuleID, Title: "Topics", Purpose: "Read the sources of each topic you follow and keep what is new.", Layer: "follow", Sends: stacksdk.SendsInternet,
			About: stacksdk.About{
				Watches:  "The feeds, pages and channels you add to a topic, on the interval you chose.",
				Never:    "It does not send a message itself, and it does not announce what was already there when you added a topic.",
				ShowsUp:  "In the Newsroom feed and, for a topic with a channel, as a message the core words and sends.",
				Settings: true,
			},
		}},
		Tools: []stacksdk.ToolSpec{
			{Name: ToolFollow, Category: "newsroom", Description: "Follow a new topic: a name and one or more sources.", Schema: json.RawMessage(followSchema)},
			{Name: ToolList, Category: "newsroom", Description: "List the topics being followed.", Schema: json.RawMessage(`{"type":"object"}`)},
		},
		Routes: []stacksdk.RouteSpec{
			{Method: http.MethodGet, Path: RouteTopics},
			{Method: http.MethodPost, Path: RouteTopics},
			{Method: http.MethodPut, Path: RouteTopic},
			{Method: http.MethodDelete, Path: RouteTopic},
			{Method: http.MethodPost, Path: RouteRun},
			{Method: http.MethodGet, Path: RouteFeed},
			{Method: http.MethodPost, Path: RouteRead},
		},
		Settings: stacksdk.SettingsSchema{Key: SettingsKey, Version: 1},
		Storage:  []string{TopicsFile},
		Requires: []string{stacksdk.PortConfig, stacksdk.PortLog, stacksdk.PortDataDir, stacksdk.PortFetch, stacksdk.PortModel, stacksdk.PortNotice},
	}
}

// followSchema is the argument schema of ToolFollow.
const followSchema = `{"type":"object","properties":{"name":{"type":"string"},"interval":{"type":"string","description":"for example 1h"},"sources":{"type":"array","items":{"type":"object","properties":{"kind":{"type":"string","enum":["feed","hackernews","mastodon","page","telegram"]},"target":{"type":"string"},"instance":{"type":"string"},"label":{"type":"string"}},"required":["kind","target"]}}},"required":["name","sources"]}`

func (s *Stack) settings(ctx context.Context) Settings {
	var set Settings
	if s.h.Config == nil {
		return set
	}
	if raw, err := s.h.Config.Load(ctx); err == nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, &set)
	}
	return set
}

// Start opens the topics file and runs the timer loop under ctx until ctx ends or Stop is called.
func (s *Stack) Start(ctx context.Context, h stacksdk.Host) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return errors.New("the newsroom stack is already running")
	}
	store, err := topics.NewStore(h.DataDir)
	if err != nil {
		return err
	}
	s.h, s.store = h, store
	// A topic with a channel hands its notice to the core, which words and sends it; the feed records every change either way.
	s.runner = &topics.Runner{Store: store, Fetch: h.Fetch, Notify: noticeSender(h.Notice)}
	if h.Model != nil {
		s.runner.Analyzer = topics.Analyst{Model: modelAdapter{h.Model}}
	}
	run, cancel := context.WithCancel(ctx)
	s.cancel, s.done = cancel, make(chan struct{})
	go s.loop(run, s.runner, s.done)
	return nil
}

// noticeSender is the runner's delivery hook over the core's Notice port: the notice's facts go over as data and the core words them. A channel the core
// does not accept (not enabled, or no longer) is "no channel" on the row, not a failed send.
func noticeSender(p stacksdk.Notice) topics.Notifier {
	if p == nil {
		return nil // no port was given: a topic with a channel shows "no way to send was set up"
	}
	return func(ctx context.Context, t topics.Topic, n topics.Notice) error {
		req := stacksdk.NoticeRequest{Topic: n.Topic}
		for _, it := range n.Items {
			req.Items = append(req.Items, stacksdk.NoticeItem{Title: it.Title, Link: it.Link})
		}
		for _, c := range n.Changes {
			req.Changes = append(req.Changes, stacksdk.NoticeChange{Item: c.Item, Was: c.Was, Now: c.Now})
		}
		err := p.Send(ctx, t.Channel, req)
		if stacksdk.CodeOf(err) == stacksdk.CodeInvalid {
			return fmt.Errorf("%w: %s", topics.ErrNoChannel, err.Error())
		}
		return err
	}
}

// loop calls Runner.Tick on a timer while the module is on. It is the one place the stack runs in the background.
func (s *Stack) loop(ctx context.Context, r *topics.Runner, done chan struct{}) {
	defer close(done)
	t := time.NewTicker(s.tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if s.settings(ctx).Disabled {
				continue
			}
			for _, o := range r.Tick(ctx) {
				s.logLine(stacksdk.LevelInfo, "topic read", "topic", strconv.FormatInt(o.TopicID, 10), "status", string(o.Status), "new", strconv.Itoa(o.New))
			}
			s.mu.Lock()
			s.lastRun = time.Now().UTC()
			s.mu.Unlock()
		}
	}
}

// Stop ends the loop and waits for it.
func (s *Stack) Stop(context.Context) error {
	s.mu.Lock()
	cancel, done := s.cancel, s.done
	s.cancel, s.done, s.store, s.runner = nil, nil, nil, nil
	s.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	<-done
	return nil
}

// EraseAll removes every topic and the feed, for a factory reset. It works whether or not the stack is running: a stack that is not running holds
// nothing in memory, and its file is removed by the core.
func (s *Stack) EraseAll() error {
	s.mu.Lock()
	store := s.store
	s.mu.Unlock()
	if store == nil {
		return nil
	}
	return store.Erase()
}

// ErrOff is the refusal of a manual read while the module is switched off: a run reads the internet, so it follows the switch.
var ErrOff = stacksdk.Errorf(stacksdk.CodeUnavailable, "Topics is switched off: switch it on under Stacks")

// ConfigChanged has nothing to re-read: the loop reads the switch on every pass.
func (s *Stack) ConfigChanged(context.Context) error { return nil }

// Status reports whether the module is on and when the loop last ran.
func (s *Stack) Status(ctx context.Context) ([]stacksdk.ModuleStatus, error) {
	s.mu.Lock()
	running, last := s.runner != nil, s.lastRun
	s.mu.Unlock()
	if !running {
		return nil, nil
	}
	return []stacksdk.ModuleStatus{{ID: ModuleID, Enabled: !s.settings(ctx).Disabled, Every: s.tick, LastRun: last}}, nil
}

// Findings: the stack offers nothing to the nightly report.
func (s *Stack) Findings(context.Context) ([]stacksdk.StackFinding, error) { return nil, nil }

func (s *Stack) logLine(level stacksdk.Level, msg string, fields ...string) {
	if s.h.Log != nil {
		s.h.Log.Log(level, msg, fields...)
	}
}

func (s *Stack) running() (*topics.Store, *topics.Runner, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.store == nil {
		return nil, nil, stacksdk.Errorf(stacksdk.CodeUnavailable, "the newsroom stack is not running")
	}
	return s.store, s.runner, nil
}

// modelAdapter gives the SDK's model port the shape the analyst asks for.
type modelAdapter struct{ m stacksdk.Model }

func (a modelAdapter) Complete(ctx context.Context, system, user string) (string, error) {
	resp, err := a.m.Complete(ctx, stacksdk.ModelRequest{System: system, User: user})
	return resp.Text, err
}

// topicView is a topic as the screen and the tools read it: the interval is a duration string such as "1h", as everywhere else in the service.
type topicView struct {
	topics.Topic
	Interval string `json:"interval"`
}

func view(t topics.Topic) topicView { return topicView{Topic: t, Interval: t.Interval.String()} }

func views(list []topics.Topic) []topicView {
	out := make([]topicView, 0, len(list))
	for _, t := range list {
		out = append(out, view(t))
	}
	return out
}

// topicIn is what a create or an update carries.
type topicIn struct {
	topics.Topic
	Interval string `json:"interval"`
}

func (in topicIn) topic() (topics.Topic, error) {
	t := in.Topic
	if in.Interval != "" {
		d, err := time.ParseDuration(in.Interval)
		if err != nil {
			return topics.Topic{}, fmt.Errorf("%w: the interval %q is not a duration such as 1h", topics.ErrInvalid, in.Interval)
		}
		t.Interval = d
	}
	return t, nil
}

// fail maps a store refusal to the SDK's error kinds.
func fail(err error) error {
	switch {
	case errors.Is(err, topics.ErrInvalid):
		return stacksdk.Errorf(stacksdk.CodeInvalid, err.Error())
	case errors.Is(err, topics.ErrNotFound):
		return stacksdk.Errorf(stacksdk.CodeNotFound, err.Error())
	}
	return err
}

func jsonResponse(status int, v any) (stacksdk.Response, error) {
	b, err := json.Marshal(v)
	return stacksdk.Response{Status: status, ContentType: "application/json", Body: b}, err
}

// CallTool runs one of the declared tools.
func (s *Stack) CallTool(_ context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
	store, _, err := s.running()
	if err != nil {
		return nil, err
	}
	switch name {
	case ToolList:
		return json.Marshal(views(store.List()))
	case ToolFollow:
		var in topicIn
		if err := json.Unmarshal(args, &in); err != nil {
			return nil, stacksdk.Errorf(stacksdk.CodeInvalid, "arguments are not valid JSON")
		}
		t, err := in.topic()
		if err != nil {
			return nil, fail(err)
		}
		// A topic the assistant creates is on and feed-only: it never picks a channel for the owner.
		t.Enabled, t.Channel = true, ""
		made, err := store.Create(t, time.Now())
		if err != nil {
			return nil, fail(err)
		}
		return json.Marshal(view(made))
	}
	return nil, stacksdk.Errorf(stacksdk.CodeNotFound, "no tool "+name)
}

// Handle answers the routes the Newsroom screen reads.
func (s *Stack) Handle(ctx context.Context, req stacksdk.Request) (stacksdk.Response, error) {
	store, runner, err := s.running()
	if err != nil {
		return stacksdk.Response{}, err
	}
	switch req.Method + " " + req.Path {
	case "GET " + RouteTopics:
		return jsonResponse(http.StatusOK, views(store.List()))
	case "POST " + RouteTopics:
		return save(req, func(t topics.Topic) (topics.Topic, error) { return store.Create(t, time.Now()) }, http.StatusCreated)
	case "GET " + RouteFeed:
		return s.feed(ctx, store, req.Query)
	case "POST " + RouteRead:
		return markRead(store, req.Body)
	}
	return s.handleTopic(ctx, store, runner, req)
}

// handleTopic answers the routes that name one topic.
func (s *Stack) handleTopic(ctx context.Context, store *topics.Store, runner *topics.Runner, req stacksdk.Request) (stacksdk.Response, error) {
	id, err := strconv.ParseInt(req.Params["id"], 10, 64)
	if err != nil {
		return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeNotFound, "no route "+req.Method+" "+req.Path)
	}
	switch req.Method + " " + req.Path {
	case "PUT " + RouteTopic:
		return save(req, func(t topics.Topic) (topics.Topic, error) { return store.Update(id, t) }, http.StatusOK)
	case "DELETE " + RouteTopic:
		if err := store.Delete(id); err != nil {
			return stacksdk.Response{}, fail(err)
		}
		return stacksdk.Response{Status: http.StatusNoContent}, nil
	case "POST " + RouteRun:
		if s.settings(ctx).Disabled {
			return stacksdk.Response{}, ErrOff
		}
		t, err := store.Get(id)
		if err != nil {
			return stacksdk.Response{}, fail(err)
		}
		return jsonResponse(http.StatusOK, runner.Run(ctx, t))
	}
	return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeNotFound, "no route "+req.Method+" "+req.Path)
}

func save(req stacksdk.Request, write func(topics.Topic) (topics.Topic, error), status int) (stacksdk.Response, error) {
	var in topicIn
	if err := json.Unmarshal(req.Body, &in); err != nil {
		return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeInvalid, "the body is not a valid topic")
	}
	t, err := in.topic()
	if err != nil {
		return stacksdk.Response{}, fail(err)
	}
	saved, err := write(t)
	if err != nil {
		return stacksdk.Response{}, fail(err)
	}
	return jsonResponse(status, view(saved))
}

func (s *Stack) feed(ctx context.Context, store *topics.Store, query string) (stacksdk.Response, error) {
	q, err := url.ParseQuery(query)
	if err != nil {
		return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeInvalid, "the query is not valid")
	}
	f := topics.Filter{Source: q.Get("source"), UnreadOnly: q.Get("unread") == "true"}
	if v := q.Get("topic"); v != "" {
		if f.TopicID, err = strconv.ParseInt(v, 10, 64); err != nil {
			return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeInvalid, "topic must be a topic id")
		}
	}
	return jsonResponse(http.StatusOK, store.Feed(time.Now(), s.feedZone(ctx, q.Get("zone")), feedDays, f))
}

// feedZone is the zone the feed's days are drawn in: the one the screen reports (the browser's), else the stored setting, else UTC. A name the runtime
// does not know is ignored, so a bad value can only fall back, never fail the feed.
func (s *Stack) feedZone(ctx context.Context, asked string) *time.Location {
	for _, name := range []string{asked, s.settings(ctx).Timezone} {
		if name == "" {
			continue
		}
		if l, err := time.LoadLocation(name); err == nil {
			return l
		}
	}
	return time.UTC
}

func markRead(store *topics.Store, body []byte) (stacksdk.Response, error) {
	var in struct {
		IDs  []int64 `json:"ids"`
		Read bool    `json:"read"`
	}
	if err := json.Unmarshal(body, &in); err != nil || len(in.IDs) == 0 {
		return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeInvalid, "ids is a list of entry ids")
	}
	if err := store.MarkRead(in.IDs, in.Read); err != nil {
		return stacksdk.Response{}, err
	}
	return stacksdk.Response{Status: http.StatusNoContent}, nil
}
