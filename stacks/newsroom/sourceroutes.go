package newsroom

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"llm-proxy-stacks/newsroom/sources"
	"llm-proxy-stacks/sdk"
)

// The two reads behind the watch form's "look up": what a source would read, and what the owner can pick while typing. Both fetch only through the
// guarded Fetch port, save nothing and follow the module's switch.
const (
	RouteSourceLookup  = "/admin/api/newsroom/sources/lookup"
	RouteSourceSuggest = "/admin/api/newsroom/sources/suggest"
)

var lookupKinds = map[string]bool{"feed": true, "hackernews": true, "mastodon": true, "page": true, "telegram": true}

const tooManyTelegram = "Too many channel look-ups just now; wait a minute and try again."

// suggestion is the answer of the suggest route.
type suggestion struct {
	Choices []sources.Choice `json:"choices"`
	Problem string           `json:"problem,omitempty"`
}

// handleSources answers the two look-up routes.
func (s *Stack) handleSources(ctx context.Context, req stacksdk.Request) (stacksdk.Response, error) {
	if s.settings(ctx).Disabled {
		return stacksdk.Response{}, ErrOff
	}
	q, err := url.ParseQuery(req.Query)
	kind := q.Get("kind")
	if err != nil || !lookupKinds[kind] {
		return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeInvalid, "kind must be one of feed, hackernews, mastodon, page or telegram")
	}
	target, instance := strings.TrimSpace(q.Get("target")), strings.TrimSpace(q.Get("instance"))
	if req.Path == RouteSourceSuggest {
		return s.suggest(ctx, kind, instance, target)
	}
	return s.lookup(ctx, kind, target, instance)
}

// lookupKey names an answer so that one source written several ways shares it: the cleaned-up value, lower-cased where the service ignores case.
func lookupKey(route, kind, target, instance string) string {
	switch kind {
	case "telegram":
		target = strings.ToLower(sources.ChannelName(target))
	case "mastodon":
		target, instance = strings.ToLower(sources.TagName(target)), sources.InstanceName(instance)
	}
	return route + "\x00" + kind + "\x00" + target + "\x00" + instance
}

func (s *Stack) lookup(ctx context.Context, kind, target, instance string) (stacksdk.Response, error) {
	key := lookupKey("lookup", kind, target, instance)
	if body, ok := s.lookups.get(key); ok {
		return stacksdk.Response{Status: http.StatusOK, Body: body}, nil
	}
	if kind == "telegram" && sources.CheckChannel(sources.ChannelName(target)) == nil && !s.lookups.allowTelegram() {
		return jsonResponse(http.StatusOK, sources.Preview{Problem: tooManyTelegram, Sample: []sources.Sample{}, Choices: []sources.Choice{}})
	}
	p := sources.Lookup(ctx, s.h.Fetch, kind, target, instance)
	if p.Sample == nil {
		p.Sample = []sources.Sample{}
	}
	if p.Choices == nil {
		p.Choices = []sources.Choice{}
	}
	return s.remember(key, p, p.Transient())
}

func (s *Stack) suggest(ctx context.Context, kind, instance, typed string) (stacksdk.Response, error) {
	key := lookupKey("suggest", kind, typed, instance)
	if body, ok := s.lookups.get(key); ok {
		return stacksdk.Response{Status: http.StatusOK, Body: body}, nil
	}
	choices, problem := sources.Suggest(ctx, s.h.Fetch, kind, instance, typed)
	if choices == nil {
		choices = []sources.Choice{}
	}
	return s.remember(key, suggestion{Choices: choices, Problem: problem}, problem != "" && len(choices) == 0 && !strings.Contains(problem, "shares no"))
}

// remember sends an answer and keeps it for the next look-up, unless it only reflects the moment (internet off, a slow server).
func (s *Stack) remember(key string, v any, transient bool) (stacksdk.Response, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return stacksdk.Response{}, stacksdk.Errorf(stacksdk.CodeInternal, "could not answer the look-up")
	}
	if !transient {
		s.lookups.put(key, body)
	}
	return stacksdk.Response{Status: http.StatusOK, Body: body}, nil
}
