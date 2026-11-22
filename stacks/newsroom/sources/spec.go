package sources

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"llm-proxy-stacks/sdk"
)

// Kind names a source type that can be read.
type Kind string

const (
	KindFeed     Kind = "feed"
	KindHN       Kind = "hackernews"
	KindMastodon Kind = "mastodon"
	KindPage     Kind = "page"
	KindTelegram Kind = "telegram"
	KindNews     Kind = "newssearch"
)

// Spec is one source an owner configured, as plain data. Param meaning by kind: feed and page: Target is the URL; hackernews: Target is the query;
// mastodon: Instance is the host and Target the hashtag (no #); telegram: Target is the channel name (no @); newssearch: Target is the query and
// Instance the region (en-GB), which defaults to DefaultNewsLocale.
type Spec struct {
	Kind     Kind   `json:"kind"`
	Label    string `json:"label,omitempty"`
	Target   string `json:"target"`
	Instance string `json:"instance,omitempty"`
}

// Name is the label the owner sees: the Label, or the kind and target when there is none.
func (s Spec) Name() string {
	if s.Label != "" {
		return s.Label
	}
	return string(s.Kind) + " " + s.Target
}

// Canonical trims the target and reduces what an owner pastes to the bare value (see ChannelName, TagName, InstanceName).
func (s Spec) Canonical() Spec {
	s.Target = strings.TrimSpace(s.Target)
	switch s.Kind {
	case KindTelegram:
		s.Target = ChannelName(s.Target)
	case KindMastodon:
		s.Target = TagName(s.Target)
		s.Instance = InstanceName(s.Instance)
	case KindNews:
		s.Instance = strings.TrimSpace(s.Instance)
		if s.Instance == "" {
			s.Instance = DefaultNewsLocale
		}
	}
	return s
}

// Validate checks the target of a canonical spec for its kind.
func (s Spec) Validate() error {
	if s.Target == "" {
		return errors.New("a target is needed")
	}
	switch s.Kind {
	case KindFeed, KindPage:
		return CheckAddress(s.Target)
	case KindHN:
		return CheckQuery(s.Target)
	case KindTelegram:
		return CheckChannel(s.Target)
	case KindNews:
		if err := CheckNewsQuery(s.Target); err != nil {
			return err
		}
		return CheckLocale(s.Instance)
	case KindMastodon:
		if s.Instance == "" {
			return errors.New("a Mastodon source needs a server")
		}
		if err := CheckInstance(s.Instance); err != nil {
			return err
		}
		return CheckTag(s.Target)
	}
	return fmt.Errorf("unknown source kind %q", s.Kind)
}

// Build makes the reader for one spec. The spec was validated; an unknown kind reads as a source that always fails.
func Build(spec Spec, f stacksdk.Fetch) Source {
	name := spec.Name()
	switch spec.Kind {
	case KindFeed:
		return Feed{Label: name, URL: spec.Target, Fetcher: f}
	case KindHN:
		return HackerNews{Label: name, Query: spec.Target, Fetcher: f}
	case KindMastodon:
		return MastodonTag{Label: name, Instance: spec.Instance, Tag: spec.Target, Fetcher: f}
	case KindPage:
		return Page{Label: name, URL: spec.Target, Fetcher: f}
	case KindTelegram:
		return &TelegramChannel{Label: name, Channel: ChannelName(spec.Target), Fetcher: f}
	case KindNews:
		return NewsSearch{Label: name, Query: spec.Target, Locale: spec.Instance, Fetcher: f}
	}
	return failing{name: name, kind: string(spec.Kind)}
}

type failing struct{ name, kind string }

func (f failing) Name() string { return f.name }
func (f failing) Fetch(context.Context) ([]Item, error) {
	return nil, fmt.Errorf("%s: unknown source kind %q", f.name, f.kind)
}

// Registry builds each reader once and hands the same one to every caller, so the politeness floor of a reader (a Telegram channel's) holds across the
// timer's passes and across everything in the stack that reads through it. Safe for concurrent use.
type Registry struct {
	// Fetch is the guarded fetch the readers are built over.
	Fetch stacksdk.Fetch

	mu    sync.Mutex
	cache map[string]Source
}

// Reader returns the reader for a spec, building it on first use.
func (r *Registry) Reader(spec Spec) Source {
	key := fmt.Sprintf("%s|%s|%s|%s", spec.Kind, spec.Label, spec.Instance, spec.Target)
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.cache[key]; ok {
		return s
	}
	if r.cache == nil {
		r.cache = map[string]Source{}
	}
	s := Build(spec, r.Fetch)
	r.cache[key] = s
	return s
}
