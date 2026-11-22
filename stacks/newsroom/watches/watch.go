// Package watches is the Watches module of the Newsroom stack: what the owner follows, when each watch is read, and which items are new. It reads sources
// through the sources package, keeps its state in one file in the stack's data folder, and hands only the new items to a delivery hook. It calls no
// model; that step sits behind this package's seams (Runner.Analyzer, Runner.Notify).
package watches

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"llm-proxy-stacks/newsroom/sources"
)

// Limits and defaults (owner decisions D4 and D5, 2026-11-20).
const (
	DefaultInterval = time.Hour
	MinInterval     = 5 * time.Minute
	// DefaultMaxAge is how old a post may be and still count as news: last night and two days ago do, last month does not. A watch can widen it up to
	// MaxMaxAge (kept well inside SeenRetention, so an old post that is still on a source page is never mistaken for a new one).
	DefaultMaxAge = 72 * time.Hour
	MinMaxAge     = time.Hour
	MaxMaxAge     = 30 * 24 * time.Hour
	MaxWatches    = 50
	MaxSources    = 10
	MaxNameRunes  = 80
	// MaxLookForRunes, MaxWords and MaxWordRunes bound what the owner types to say what the watch is for.
	MaxLookForRunes = 300
	MaxWords        = 10
	MaxWordRunes    = 40
	// MaxStateRunes bounds the last-known-state note.
	MaxStateRunes = 600
	// SeenRetention and MaxSeen bound one watch's ledger, as the automation ledger's defaults do.
	SeenRetention = 60 * 24 * time.Hour
	MaxSeen       = 2000
)

// ErrInvalid wraps every refusal of a watch the owner described; a transport maps it to a 400.
var ErrInvalid = errors.New("invalid watch")

// ErrNotFound is returned for a watch id that does not exist.
var ErrNotFound = errors.New("watch not found")

// Watch is one thing the owner follows.
type Watch struct {
	ID      int64          `json:"id"`
	Name    string         `json:"name"`
	Sources []sources.Spec `json:"sources"`
	// Interval is the time between reads; zero in a request means DefaultInterval.
	Interval time.Duration `json:"interval"`
	// MaxAge is how old a post may be and still be news; zero (a watch saved before this existed) means DefaultMaxAge. A post with no date is kept.
	MaxAge  time.Duration `json:"max_age"`
	Enabled bool          `json:"enabled"`
	// Channel is the connector type notices are sent through ("telegram", "email"); empty means the feed only. The feed always records every change.
	Channel string `json:"channel,omitempty"`
	// LookFor says in the owner's words what the watch is for ("new AI model releases, not ads"). When it is set, the model judges each new item against it
	// (and says why one matched); empty means every new item counts.
	LookFor string `json:"look_for,omitempty"`
	// Keywords keeps only items that contain at least one of them; Skip drops items that contain any. Both are plain, case-insensitive text matches that
	// need no model, and both are optional.
	Keywords []string `json:"keywords,omitempty"`
	Skip     []string `json:"skip,omitempty"`
	// State is the short note of the last known state, rewritten by the framework each run (Phase 3); empty until then.
	State string `json:"state,omitempty"`

	// Run record, written by the runner.
	Baselined  bool      `json:"baselined,omitempty"`
	LastRun    time.Time `json:"last_run,omitzero"`
	LastStatus Status    `json:"last_status,omitempty"`
	LastDetail string    `json:"last_detail,omitempty"`
	LastNew    int       `json:"last_new,omitempty"`
	// LastProblems names each source the last run could not read and why, one sentence each. It is kept apart from LastStatus because a run can
	// answer from the other sources and still have one dead: the row must go on saying so until that source reads again.
	LastProblems []string  `json:"last_problems,omitempty"`
	CreatedAt    time.Time `json:"created_at,omitzero"`
}

// Status is the outcome of a watch's last run, in words the row can show.
type Status string

const (
	// StatusBaseline: the first read recorded what was already there without announcing it.
	StatusBaseline Status = "baseline"
	// StatusNothingNew: every source answered and nothing was new.
	StatusNothingNew Status = "nothing_new"
	// StatusInFeed: new items were recorded in the feed; the watch has no channel, so nothing was sent.
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
	// StatusWaiting: every source was inside its minimum gap between reads, so nothing was read. It is an answer to a manual check only and is never
	// stored as a watch's last result.
	StatusWaiting Status = "waiting"
	// StatusFailing: no source could be read. The row must say so; it is never "nothing new".
	StatusFailing Status = "failing"
)

// Normalize validates a watch the owner described and fills its defaults. It returns a copy; ID and the run record are the store's.
func Normalize(t Watch) (Watch, error) {
	t.Name = strings.TrimSpace(t.Name)
	if t.Name == "" || utf8.RuneCountInString(t.Name) > MaxNameRunes {
		return Watch{}, fmt.Errorf("%w: a name of 1 to %d characters is needed", ErrInvalid, MaxNameRunes)
	}
	if n := len(t.Sources); n == 0 || n > MaxSources {
		return Watch{}, fmt.Errorf("%w: a watch needs 1 to %d sources", ErrInvalid, MaxSources)
	}
	t.Sources = append([]sources.Spec(nil), t.Sources...)
	for i, s := range t.Sources {
		s = s.Canonical()
		t.Sources[i] = s
		if err := s.Validate(); err != nil {
			return Watch{}, fmt.Errorf("%w: source %d: %v", ErrInvalid, i+1, err)
		}
	}
	if err := t.normalizeTimes(); err != nil {
		return Watch{}, err
	}
	t.Channel = strings.ToLower(strings.TrimSpace(t.Channel))
	if t.Channel != "" && !channelNameRe.MatchString(t.Channel) {
		return Watch{}, fmt.Errorf("%w: the channel must be a connector type such as telegram", ErrInvalid)
	}
	return normalizeLookFor(t)
}

// normalizeTimes fills the interval and the news window with their defaults and keeps both in range.
func (t *Watch) normalizeTimes() error {
	if t.Interval == 0 {
		t.Interval = DefaultInterval
	}
	if t.Interval < MinInterval {
		return fmt.Errorf("%w: the interval must be at least %s", ErrInvalid, MinInterval)
	}
	if t.MaxAge == 0 {
		t.MaxAge = DefaultMaxAge
	}
	if t.MaxAge < MinMaxAge || t.MaxAge > MaxMaxAge {
		return fmt.Errorf("%w: the news window must be from %s to %s", ErrInvalid, MinMaxAge, MaxMaxAge)
	}
	return nil
}

var channelNameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}$`)

func normalizeLookFor(t Watch) (Watch, error) {
	t.LookFor = strings.TrimSpace(t.LookFor)
	if utf8.RuneCountInString(t.LookFor) > MaxLookForRunes {
		return Watch{}, fmt.Errorf("%w: what to look for is at most %d characters", ErrInvalid, MaxLookForRunes)
	}
	var err error
	if t.Keywords, err = normalizeWords(t.Keywords, "keywords"); err != nil {
		return Watch{}, err
	}
	if t.Skip, err = normalizeWords(t.Skip, "skip words"); err != nil {
		return Watch{}, err
	}
	return t, nil
}

// normalizeWords trims, lower-cases and de-duplicates a list of match words, dropping empty ones.
func normalizeWords(in []string, what string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, w := range in {
		w = strings.ToLower(strings.TrimSpace(w))
		if w == "" || seen[w] {
			continue
		}
		if utf8.RuneCountInString(w) > MaxWordRunes {
			return nil, fmt.Errorf("%w: each of the %s is at most %d characters", ErrInvalid, what, MaxWordRunes)
		}
		seen[w] = true
		out = append(out, w)
	}
	if len(out) > MaxWords {
		return nil, fmt.Errorf("%w: at most %d %s", ErrInvalid, MaxWords, what)
	}
	return out, nil
}

// Fresh splits items by age: a post older than the watch's window is stale, anything else (an undated item included, since its age cannot be judged)
// is kept. Stale items are only recorded as seen, so they are not looked at again.
func (t Watch) Fresh(items []sources.Item, now time.Time) (kept, stale []sources.Item) {
	age := t.MaxAge
	if age == 0 {
		age = DefaultMaxAge
	}
	for _, it := range items {
		if !it.At.IsZero() && it.At.Before(now.Add(-age)) {
			stale = append(stale, it)
		} else {
			kept = append(kept, it)
		}
	}
	return kept, stale
}

// Gate splits items by the watch's keywords and skip words: kept ones go on to the model, dropped ones are only recorded as seen. A watch with neither
// keeps everything. The match is on the title and text, case-insensitive.
func (t Watch) Gate(items []sources.Item) (kept, dropped []sources.Item) {
	if len(t.Keywords) == 0 && len(t.Skip) == 0 {
		return items, nil
	}
	for _, it := range items {
		hay := strings.ToLower(it.Title + " " + it.Text)
		if (len(t.Keywords) == 0 || containsAny(hay, t.Keywords)) && !containsAny(hay, t.Skip) {
			kept = append(kept, it)
		} else {
			dropped = append(dropped, it)
		}
	}
	return kept, dropped
}

func containsAny(hay string, words []string) bool {
	for _, w := range words {
		if strings.Contains(hay, w) {
			return true
		}
	}
	return false
}

// canonical reduces what the owner typed to the form the source reads: a bare channel name, a bare tag, a bare host, no stray spaces.
// Due reports whether a watch should be read at now: enabled, and never read or read at least one interval ago.
func (t Watch) Due(now time.Time) bool {
	if !t.Enabled {
		return false
	}
	return t.LastRun.IsZero() || !now.Before(t.LastRun.Add(t.Interval))
}
