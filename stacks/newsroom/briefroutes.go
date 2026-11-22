package newsroom

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"llm-proxy-stacks/newsroom/brief"
	"llm-proxy-stacks/sdk"
)

// The routes of the Brief module. The topics and editions are what the owner configures; the pool is the headlines collected so far, so the owner can see
// what an edition would be made from.
const (
	RouteBriefs        = "/admin/api/newsroom/briefs/"
	RouteBriefTopics   = "/admin/api/newsroom/briefs/topics"
	RouteBriefTopic    = "/admin/api/newsroom/briefs/topics/{id}"
	RouteBriefCollect  = "/admin/api/newsroom/briefs/topics/{id}/collect"
	RouteBriefEditions = "/admin/api/newsroom/briefs/editions"
	RouteBriefEdition  = "/admin/api/newsroom/briefs/editions/{id}"
	RouteBriefPool     = "/admin/api/newsroom/briefs/pool"
	RouteBriefMake     = "/admin/api/newsroom/briefs/editions/{id}/issue"
	RouteBriefIssues   = "/admin/api/newsroom/briefs/issues"
	RouteBriefIssue    = "/admin/api/newsroom/briefs/issues/{id}"
	RouteBriefSettings = "/admin/api/newsroom/briefs/settings"
	maxIssuesView      = 30
	// DefaultModelCalls and MaxModelCalls mirror the core's model port (stackport.ModelDailyLimit and ModelDailyLimitMax), which holds the real limit;
	// the app's tests keep the two in step. They are here so the screen can say what an empty setting means.
	DefaultModelCalls     = 200
	MaxModelCalls         = 2000
	maxPoolView           = 200
	msgBriefsOff          = "Briefs is switched off: switch it on under Stacks"
	msgBriefsNotRunning   = "the briefs are not available: the stack is not running, or the schedule is not provided"
	msgBriefBodyNotValid  = "the body is not valid"
	msgBriefIDNotAnID     = "the id is not a number"
	msgPoolTopicNotNumber = "topic must be a topic id"
)

// ErrBriefsOff is the refusal of a manual collection while the Briefs module is switched off: collecting reads the internet, so it follows the switch.
var ErrBriefsOff = stacksdk.Errorf(stacksdk.CodeUnavailable, msgBriefsOff)

// editionView is an edition as the screen reads it: the two durations are strings such as "2h0m0s", as everywhere else in the service.
type editionView struct {
	brief.Edition
	CollectEvery string `json:"collect_every"`
	CollectFor   string `json:"collect_for"`
}

func editionOut(e brief.Edition) editionView {
	return editionView{Edition: e, CollectEvery: e.CollectEvery.String(), CollectFor: e.CollectFor.String()}
}

// editionIn is what a create or an update of an edition carries.
type editionIn struct {
	brief.Edition
	CollectEvery string `json:"collect_every"`
	CollectFor   string `json:"collect_for"`
}

func (in editionIn) edition() (brief.Edition, error) {
	e := in.Edition
	for _, f := range []struct {
		text string
		to   *time.Duration
		what string
	}{{in.CollectEvery, &e.CollectEvery, "collect every"}, {in.CollectFor, &e.CollectFor, "collect for"}} {
		if f.text == "" {
			continue
		}
		d, err := time.ParseDuration(f.text)
		if err != nil {
			return brief.Edition{}, fmt.Errorf("%w: %s %q is not a duration such as 2h", brief.ErrInvalid, f.what, f.text)
		}
		*f.to = d
	}
	return e, nil
}

// failBrief maps a brief store refusal to the SDK's error kinds.
func failBrief(err error) error {
	switch {
	case errors.Is(err, brief.ErrInvalid), errors.Is(err, brief.ErrInUse):
		return stacksdk.Errorf(stacksdk.CodeInvalid, err.Error())
	case errors.Is(err, brief.ErrNotFound):
		return stacksdk.Errorf(stacksdk.CodeNotFound, err.Error())
	}
	return err
}

// briefParts are what the Brief module's routes work with while the stack runs.
type briefParts struct {
	store     *brief.Store
	collector *brief.Collector
	publisher *brief.Publisher
}

func (s *Stack) briefsRunning() (briefParts, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.briefs == nil || s.collector == nil || s.publisher == nil {
		return briefParts{}, stacksdk.Errorf(stacksdk.CodeUnavailable, msgBriefsNotRunning)
	}
	return briefParts{s.briefs, s.collector, s.publisher}, nil
}

// handleBriefs answers the Brief module's routes.
func (s *Stack) handleBriefs(ctx context.Context, req stacksdk.Request) (stacksdk.Response, error) {
	parts, err := s.briefsRunning()
	if err != nil {
		return stacksdk.Response{}, err
	}
	store := parts.store
	route, id, err := briefRoute(req)
	if err != nil {
		return stacksdk.Response{}, err
	}
	key := req.Method + " " + route
	switch route {
	case RouteBriefTopics, RouteBriefTopic:
		return handleTopics(key, id, req, store)
	case RouteBriefCollect, RouteBriefMake:
		return s.briefAction(ctx, route, id, parts)
	case RouteBriefEditions, RouteBriefEdition:
		return handleEditions(key, id, req, store)
	case RouteBriefPool:
		return briefPool(store, req.Query)
	case RouteBriefIssues, RouteBriefIssue:
		return handleIssues(key, id, req.Query, store)
	case RouteBriefSettings:
		return s.briefSettings(ctx, req)
	}
	return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeNotFound, "no route "+req.Method+" "+req.Path)
}

// modelCallsView is what the screen reads of the daily limit: the owner's setting (0 means the default) and the bounds.
type modelCallsView struct {
	ModelCallsPerDay int `json:"model_calls_per_day"`
	Default          int `json:"default"`
	Max              int `json:"max"`
}

// briefSettings reads and changes the stack's daily limit of model calls. It saves only that one field, so the module switches and the timezone are untouched.
func (s *Stack) briefSettings(ctx context.Context, req stacksdk.Request) (stacksdk.Response, error) {
	if req.Method == http.MethodPut {
		var in struct {
			ModelCallsPerDay int `json:"model_calls_per_day"`
		}
		if err := json.Unmarshal(req.Body, &in); err != nil {
			return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeInvalid, msgBriefBodyNotValid)
		}
		if in.ModelCallsPerDay < 0 || in.ModelCallsPerDay > MaxModelCalls {
			return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeInvalid, fmt.Sprintf("model calls a day is 1 to %d, or empty for %d", MaxModelCalls, DefaultModelCalls))
		}
		if s.h.Config == nil {
			return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeUnavailable, msgBriefsNotRunning)
		}
		doc, _ := json.Marshal(map[string]int{"model_calls_per_day": in.ModelCallsPerDay})
		if err := s.h.Config.Save(ctx, doc); err != nil {
			return stacksdk.Response{}, err
		}
	}
	return jsonResponse(http.StatusOK, modelCallsView{ModelCallsPerDay: s.settings(ctx).ModelCallsPerDay, Default: DefaultModelCalls, Max: MaxModelCalls})
}

// briefAction answers the two actions the owner asks for by hand: collect a topic now, and make an edition now. Collecting reads the internet and making
// asks the model, so both follow the module switch.
func (s *Stack) briefAction(ctx context.Context, route string, id int64, parts briefParts) (stacksdk.Response, error) {
	if s.settings(ctx).BriefsDisabled {
		return stacksdk.Response{}, ErrBriefsOff
	}
	if route == RouteBriefMake {
		is, err := parts.publisher.MakeNow(ctx, id)
		if err != nil {
			return stacksdk.Response{}, failBrief(err)
		}
		return jsonResponse(http.StatusCreated, is)
	}
	o, err := parts.collector.CollectNow(ctx, id)
	if err != nil {
		return stacksdk.Response{}, failBrief(err)
	}
	return jsonResponse(http.StatusOK, o)
}

// handleIssues answers the routes that read the editions already made: the newest few, or one by id. Reading asks nothing of the model.
func handleIssues(key string, id int64, rawQuery string, store *brief.Store) (stacksdk.Response, error) {
	switch key {
	case "GET " + RouteBriefIssue:
		is, err := store.Issue(id)
		if err != nil {
			return stacksdk.Response{}, failBrief(err)
		}
		return jsonResponse(http.StatusOK, is)
	case "GET " + RouteBriefIssues:
		q, err := url.ParseQuery(rawQuery)
		if err != nil {
			return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeInvalid, "the query is not valid")
		}
		var edition int64
		if v := q.Get("edition"); v != "" {
			if edition, err = strconv.ParseInt(v, 10, 64); err != nil {
				return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeInvalid, "edition must be an edition id")
			}
		}
		out := store.Issues(edition)
		return jsonResponse(http.StatusOK, nonNil(out[:min(len(out), maxIssuesView)]))
	}
	return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeNotFound, "no route "+key)
}

// handleTopics answers the routes that list, create, change and delete topics.
func handleTopics(key string, id int64, req stacksdk.Request, store *brief.Store) (stacksdk.Response, error) {
	switch key {
	case "GET " + RouteBriefTopics:
		return jsonResponse(http.StatusOK, nonNil(store.Topics()))
	case "POST " + RouteBriefTopics:
		return saveBrief(req.Body, func(t brief.Topic) (brief.Topic, error) { return store.CreateTopic(t, time.Now()) }, http.StatusCreated)
	case "PUT " + RouteBriefTopic:
		return saveBrief(req.Body, func(t brief.Topic) (brief.Topic, error) { return store.UpdateTopic(id, t) }, http.StatusOK)
	case "DELETE " + RouteBriefTopic:
		return noContent(store.DeleteTopic(id))
	}
	return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeNotFound, "no route "+req.Method+" "+req.Path)
}

// handleEditions answers the routes that list, create, change and delete editions.
func handleEditions(key string, id int64, req stacksdk.Request, store *brief.Store) (stacksdk.Response, error) {
	switch key {
	case "GET " + RouteBriefEditions:
		out := []editionView{}
		for _, e := range store.Editions() {
			out = append(out, editionOut(e))
		}
		return jsonResponse(http.StatusOK, out)
	case "POST " + RouteBriefEditions:
		return saveEdition(req.Body, func(e brief.Edition) (brief.Edition, error) { return store.CreateEdition(e, time.Now()) }, http.StatusCreated)
	case "PUT " + RouteBriefEdition:
		return saveEdition(req.Body, func(e brief.Edition) (brief.Edition, error) { return store.UpdateEdition(id, e) }, http.StatusOK)
	case "DELETE " + RouteBriefEdition:
		return noContent(store.DeleteEdition(id))
	}
	return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeNotFound, "no route "+req.Method+" "+req.Path)
}

// The shapes of a real path under RouteBriefs, by the number of segments: the collection, then the collection and an id, then an action on one item.
var (
	briefCollections = map[string]string{"topics": RouteBriefTopics, "editions": RouteBriefEditions, "pool": RouteBriefPool, "issues": RouteBriefIssues, "settings": RouteBriefSettings}
	briefActions     = map[string]string{"topics/collect": RouteBriefCollect, "editions/issue": RouteBriefMake}
	briefItems       = map[string]string{"topics": RouteBriefTopic, "editions": RouteBriefEdition, "issues": RouteBriefIssue}
)

// briefRoute names the route a real path matches by its shape, and returns the id it carries. The core passes the real path (/topics/3/collect).
func briefRoute(req stacksdk.Request) (string, int64, error) {
	noRoute := stacksdk.Errorf(stacksdk.CodeNotFound, "no route "+req.Method+" "+req.Path)
	rest, ok := strings.CutPrefix(req.Path, RouteBriefs)
	if !ok {
		return "", 0, noRoute
	}
	parts := strings.Split(rest, "/")
	if len(parts) == 1 {
		if route, ok := briefCollections[parts[0]]; ok {
			return route, 0, nil
		}
		return "", 0, noRoute
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", 0, stacksdk.Errorf(stacksdk.CodeInvalid, msgBriefIDNotAnID)
	}
	if len(parts) == 2 && briefItems[parts[0]] != "" {
		return briefItems[parts[0]], id, nil
	}
	if len(parts) == 3 && briefActions[parts[0]+"/"+parts[2]] != "" {
		return briefActions[parts[0]+"/"+parts[2]], id, nil
	}
	return "", 0, noRoute
}

// nonNil makes an empty list read as [] rather than null.
func nonNil[T any](in []T) []T {
	if in == nil {
		return []T{}
	}
	return in
}

func noContent(err error) (stacksdk.Response, error) {
	if err != nil {
		return stacksdk.Response{}, failBrief(err)
	}
	return stacksdk.Response{Status: http.StatusNoContent}, nil
}

func saveBrief(body []byte, write func(brief.Topic) (brief.Topic, error), status int) (stacksdk.Response, error) {
	var t brief.Topic
	if err := json.Unmarshal(body, &t); err != nil {
		return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeInvalid, msgBriefBodyNotValid)
	}
	saved, err := write(t)
	if err != nil {
		return stacksdk.Response{}, failBrief(err)
	}
	return jsonResponse(status, saved)
}

func saveEdition(body []byte, write func(brief.Edition) (brief.Edition, error), status int) (stacksdk.Response, error) {
	var in editionIn
	if err := json.Unmarshal(body, &in); err != nil {
		return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeInvalid, msgBriefBodyNotValid)
	}
	e, err := in.edition()
	if err != nil {
		return stacksdk.Response{}, failBrief(err)
	}
	saved, err := write(e)
	if err != nil {
		return stacksdk.Response{}, failBrief(err)
	}
	return jsonResponse(status, editionOut(saved))
}

// briefPool answers the collected stories, newest first, at most maxPoolView, for one topic when topic is given.
func briefPool(store *brief.Store, rawQuery string) (stacksdk.Response, error) {
	q, err := url.ParseQuery(rawQuery)
	if err != nil {
		return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeInvalid, "the query is not valid")
	}
	var topic int64
	if v := q.Get("topic"); v != "" {
		if topic, err = strconv.ParseInt(v, 10, 64); err != nil {
			return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeInvalid, msgPoolTopicNotNumber)
		}
	}
	out := []brief.PoolItem{}
	for _, p := range store.Pool() {
		if len(out) == maxPoolView {
			break
		}
		if topic == 0 || slices.Contains(p.Topics, topic) {
			out = append(out, p)
		}
	}
	return jsonResponse(http.StatusOK, out)
}
