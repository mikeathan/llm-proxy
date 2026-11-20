// Package topics is the Topics module of the Newsroom stack: what the owner follows, when each topic is read, and which items are new. It reads sources
// through the sources package, keeps its state in one file in the stack's data folder, and hands only the new items to a delivery hook. It calls no
// model and no search; both are later phases and sit behind this package's seams (Runner.Notify).
package topics

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// Limits and defaults (owner decisions D4 and D5, 2026-11-20).
const (
	DefaultInterval = time.Hour
	MinInterval     = 5 * time.Minute
	MaxTopics       = 50
	MaxSources      = 10
	MaxNameRunes    = 80
	// MaxStateRunes bounds the last-known-state note.
	MaxStateRunes = 600
	// SeenRetention and MaxSeen bound one topic's ledger, as the automation ledger's defaults do.
	SeenRetention = 60 * 24 * time.Hour
	MaxSeen       = 2000
)

// ErrInvalid wraps every refusal of a topic the owner described; a transport maps it to a 400.
var ErrInvalid = errors.New("invalid topic")

// ErrNotFound is returned for a topic id that does not exist.
var ErrNotFound = errors.New("topic not found")

// SearchMode says how a topic may use web search (it spends the owner's quota). Phase 2 only stores it; the model step that uses it is Phase 3.
type SearchMode string

const (
	// SearchOff never searches: feeds only.
	SearchOff SearchMode = "off"
	// SearchConfirm (the default) searches only to confirm a change the feeds showed, within a bounded budget.
	SearchConfirm SearchMode = "confirm"
	// SearchLimited searches at most SearchPerDay times a day.
	SearchLimited SearchMode = "limited"
	// SearchEveryRun searches on every run.
	SearchEveryRun SearchMode = "every_run"
)

// Kind names a source type a topic can read.
type Kind string

const (
	KindFeed     Kind = "feed"
	KindHN       Kind = "hackernews"
	KindMastodon Kind = "mastodon"
	KindPage     Kind = "page"
	KindTelegram Kind = "telegram"
)

// SourceSpec is one source of a topic, as plain data. Param meaning by kind: feed and page: Target is the URL; hackernews: Target is the query;
// mastodon: Instance is the host and Target the hashtag (no #); telegram: Target is the channel name (no @).
type SourceSpec struct {
	Kind     Kind   `json:"kind"`
	Label    string `json:"label,omitempty"`
	Target   string `json:"target"`
	Instance string `json:"instance,omitempty"`
}

// Name is the label the owner sees: the Label, or the kind and target when there is none.
func (s SourceSpec) Name() string {
	if s.Label != "" {
		return s.Label
	}
	return string(s.Kind) + " " + s.Target
}

// Topic is one thing the owner follows.
type Topic struct {
	ID      int64        `json:"id"`
	Name    string       `json:"name"`
	Sources []SourceSpec `json:"sources"`
	// Interval is the time between reads; zero in a request means DefaultInterval.
	Interval time.Duration `json:"interval"`
	Enabled  bool          `json:"enabled"`
	// Channel is the connector type notices are sent through ("telegram", "email"); empty means the feed only. The feed always records every change.
	Channel      string     `json:"channel,omitempty"`
	Search       SearchMode `json:"search"`
	SearchPerDay int        `json:"search_per_day,omitempty"`
	// Paper puts this topic's changes in the morning paper under Section (morning-paper.md); stored now, used later.
	Paper   bool   `json:"paper,omitempty"`
	Section string `json:"section,omitempty"`
	// State is the short note of the last known state, rewritten by the framework each run (Phase 3); empty until then.
	State string `json:"state,omitempty"`

	// Run record, written by the runner.
	Baselined  bool      `json:"baselined,omitempty"`
	LastRun    time.Time `json:"last_run,omitzero"`
	LastStatus Status    `json:"last_status,omitempty"`
	LastDetail string    `json:"last_detail,omitempty"`
	LastNew    int       `json:"last_new,omitempty"`
	CreatedAt  time.Time `json:"created_at,omitzero"`
}

// Status is the outcome of a topic's last run, in words the row can show.
type Status string

const (
	// StatusBaseline: the first read recorded what was already there without announcing it.
	StatusBaseline Status = "baseline"
	// StatusNothingNew: every source answered and nothing was new.
	StatusNothingNew Status = "nothing_new"
	// StatusInFeed: new items were recorded in the feed; the topic has no channel, so nothing was sent.
	StatusInFeed Status = "in_feed"
	// StatusSent: new items were delivered.
	StatusSent Status = "sent"
	// StatusSendFailed: new items were found and the delivery failed; nothing was recorded, so the next run tries again.
	StatusSendFailed Status = "send_failed"
	// StatusNoChannel: new items were found and there is nowhere to send them; nothing was recorded.
	StatusNoChannel Status = "no_channel"
	// StatusAnalysisFailed: new items were found and the model step failed or answered something unusable; nothing was recorded, so the next run
	// tries again.
	StatusAnalysisFailed Status = "analysis_failed"
	// StatusFailing: no source could be read. The row must say so; it is never "nothing new".
	StatusFailing Status = "failing"
)

// Normalize validates a topic the owner described and fills its defaults. It returns a copy; ID and the run record are the store's.
func Normalize(t Topic) (Topic, error) {
	t.Name = strings.TrimSpace(t.Name)
	if t.Name == "" || utf8.RuneCountInString(t.Name) > MaxNameRunes {
		return Topic{}, fmt.Errorf("%w: a name of 1 to %d characters is needed", ErrInvalid, MaxNameRunes)
	}
	if n := len(t.Sources); n == 0 || n > MaxSources {
		return Topic{}, fmt.Errorf("%w: a topic needs 1 to %d sources", ErrInvalid, MaxSources)
	}
	t.Sources = append([]SourceSpec(nil), t.Sources...)
	for i, s := range t.Sources {
		if err := s.validate(); err != nil {
			return Topic{}, fmt.Errorf("%w: source %d: %v", ErrInvalid, i+1, err)
		}
	}
	if t.Interval == 0 {
		t.Interval = DefaultInterval
	}
	if t.Interval < MinInterval {
		return Topic{}, fmt.Errorf("%w: the interval must be at least %s", ErrInvalid, MinInterval)
	}
	t.Channel = strings.ToLower(strings.TrimSpace(t.Channel))
	if t.Channel != "" && !channelNameRe.MatchString(t.Channel) {
		return Topic{}, fmt.Errorf("%w: the channel must be a connector type such as telegram", ErrInvalid)
	}
	return normalizeSearch(t)
}

var channelNameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}$`)

func normalizeSearch(t Topic) (Topic, error) {
	switch t.Search {
	case "":
		t.Search = SearchConfirm
	case SearchOff, SearchConfirm, SearchEveryRun:
	case SearchLimited:
		if t.SearchPerDay <= 0 {
			return Topic{}, fmt.Errorf("%w: limited search needs a number of searches a day", ErrInvalid)
		}
	default:
		return Topic{}, fmt.Errorf("%w: search must be off, confirm, limited or every_run", ErrInvalid)
	}
	if t.Search != SearchLimited {
		t.SearchPerDay = 0
	}
	t.Section = strings.TrimSpace(t.Section)
	return t, nil
}

func (s SourceSpec) validate() error {
	if strings.TrimSpace(s.Target) == "" {
		return errors.New("a target is needed")
	}
	switch s.Kind {
	case KindFeed, KindPage:
		if !strings.HasPrefix(s.Target, "https://") {
			return errors.New("the address must start with https://")
		}
	case KindHN, KindTelegram:
	case KindMastodon:
		if s.Instance == "" {
			return errors.New("a Mastodon source needs an instance")
		}
	default:
		return fmt.Errorf("unknown source kind %q", s.Kind)
	}
	return nil
}

// Due reports whether a topic should be read at now: enabled, and never read or read at least one interval ago.
func (t Topic) Due(now time.Time) bool {
	if !t.Enabled {
		return false
	}
	return t.LastRun.IsZero() || !now.Before(t.LastRun.Add(t.Interval))
}
