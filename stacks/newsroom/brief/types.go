// Package brief is the Newsroom's Brief module: topics the owner defines, editions (a morning brief, an evening brief) built from them, and the pool of
// headlines collected for them through the day. Collecting reads sources and stores headlines; it never calls a model. This file holds the plain data
// and the rules for what an owner may configure; store.go keeps it, collector.go fills the pool.
package brief

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"llm-proxy-stacks/newsroom/sources"
)

// Bounds on what an owner configures and on what is kept, so nothing grows without limit.
const (
	MaxTopics         = 30
	MaxEditions       = 10
	MaxSections       = 12
	MaxNameRunes      = 80
	MaxQueries        = 10
	MaxQueryRunes     = sources.MaxQueryRunes
	MaxTopicSources   = 20
	MaxIgnoreWords    = 20
	MaxWordRunes      = 40
	MinCollectEvery   = time.Hour
	MaxCollectEvery   = 12 * time.Hour
	DefaultEvery      = 2 * time.Hour
	MinCollectFor     = time.Hour
	MaxCollectFor     = 24 * time.Hour
	DefaultCollectFor = 12 * time.Hour
)

// ErrInvalid wraps every refusal of a topic or edition the owner described; a transport maps it to a 400.
var ErrInvalid = errors.New("invalid brief setting")

// ErrNotFound is returned for an id that does not exist.
var ErrNotFound = errors.New("brief item not found")

// ErrInUse is returned when a topic is deleted while an edition still lists it.
var ErrInUse = errors.New("brief topic in use")

// A topic reads sources with no politeness slot only: a Telegram channel's floor is held per reader and shared with the watches, and a page is
// compared for change rather than read for news. Telegram channels in a brief wait for the shared-slot design (plan: morning-paper.md).
var topicKinds = map[sources.Kind]bool{sources.KindFeed: true, sources.KindHN: true, sources.KindMastodon: true, sources.KindNews: true}

// Topic is one area of news the owner defines: what it covers (Queries, each read as a news search) and the Sources to read for it.
type Topic struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	// Category is the section the topic's stories go in: topics of one edition with the same category (compared by CategoryKey) are one section. It
	// defaults to the topic's name.
	Category string `json:"category,omitempty"`
	// Queries are news searches (words, or site:reuters.com); Region is their language and country, such as en-GB (empty means the default).
	Queries []string `json:"queries,omitempty"`
	Region  string   `json:"region,omitempty"`
	// Sources are feeds and other readers beyond the searches.
	Sources []sources.Spec `json:"sources,omitempty"`
	// Ignore drops a headline that contains one of these words (lower case).
	Ignore []string `json:"ignore,omitempty"`

	// Collection record, written by the collector.
	LastCollected time.Time `json:"last_collected,omitzero"`
	LastNew       int       `json:"last_new,omitempty"`
	// LastProblems names each source the last collection could not read and why, one sentence each.
	LastProblems []string  `json:"last_problems,omitempty"`
	CreatedAt    time.Time `json:"created_at,omitzero"`
}

// Readers returns every source the topic reads, the searches first. Each carries the topic's name in its label so a failure names where it happened.
func (t Topic) Readers() []sources.Spec {
	out := make([]sources.Spec, 0, len(t.Queries)+len(t.Sources))
	for _, q := range t.Queries {
		out = append(out, sources.Spec{Kind: sources.KindNews, Label: t.Name + ": " + q, Target: q, Instance: t.Region}.Canonical())
	}
	return append(out, t.Sources...)
}

// SectionKind says what a section of an edition shows.
type SectionKind string

// SectionNews is a section built from one topic's headlines.
const SectionNews SectionKind = "news"

// Section is one block of an edition, in the order the owner chose.
type Section struct {
	Kind    SectionKind `json:"kind"`
	TopicID int64       `json:"topic_id,omitempty"`
}

// Edition is one named brief: when it is ready, on which days, how it collects, and what it contains.
type Edition struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	// ReadyBy is the time of day it is ready, "15:04" in the stack's timezone; Days are the weekdays it is made on (0 is Sunday).
	ReadyBy string `json:"ready_by"`
	Days    []int  `json:"days"`
	// CollectEvery is the time between collections, and CollectFor how long before ReadyBy collecting runs.
	CollectEvery time.Duration `json:"collect_every"`
	CollectFor   time.Duration `json:"collect_for"`
	Sections     []Section     `json:"sections"`
	Enabled      bool          `json:"enabled"`
	// Watches adds a "Your watches" block: what the owner's watches found since the previous edition. Off by default.
	Watches bool `json:"watches,omitempty"`
	// Home adds a "Home" block: the core's fixed counts about the network and Radio (ADR-0077). Off by default.
	Home      bool      `json:"home,omitempty"`
	CreatedAt time.Time `json:"created_at,omitzero"`
}

var (
	readyByRe = regexp.MustCompile(`^([01][0-9]|2[0-3]):([0-5][0-9])$`)
	weekdays  = [...]int{0, 1, 2, 3, 4, 5, 6}
)

// Cron is the five-field expression for the moments the edition is ready, for the schedule port: "0 7 * * 1,2,3".
func (e Edition) Cron() string {
	m := readyByRe.FindStringSubmatch(e.ReadyBy)
	days := make([]string, len(e.Days))
	for i, d := range e.Days {
		days[i] = fmt.Sprint(d)
	}
	return fmt.Sprintf("%d %d * * %s", atoi(m[2]), atoi(m[1]), strings.Join(days, ","))
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n
}

// Group is the section the topic's stories go in: its category, or its own name when it has none (a topic saved before categories were the sections).
func (t Topic) Group() string {
	if c := strings.TrimSpace(t.Category); c != "" {
		return c
	}
	return strings.TrimSpace(t.Name)
}

// CategoryKey is what two category spellings are compared by: lower case, words joined by one space.
func CategoryKey(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

// Normalize validates a topic the owner described and returns a cleaned copy; ID and the collection record are the store's.
func (t Topic) Normalize() (Topic, error) {
	t.Name, t.Category = strings.TrimSpace(t.Name), strings.TrimSpace(t.Category)
	if t.Name == "" || len([]rune(t.Name)) > MaxNameRunes {
		return Topic{}, fmt.Errorf("%w: a topic needs a name of 1 to %d characters", ErrInvalid, MaxNameRunes)
	}
	if len([]rune(t.Category)) > MaxNameRunes {
		return Topic{}, fmt.Errorf("%w: the category is at most %d characters", ErrInvalid, MaxNameRunes)
	}
	if t.Category == "" {
		t.Category = t.Name
	}
	var err error
	if t.Queries, t.Region, err = cleanSearches(t.Queries, t.Region); err != nil {
		return Topic{}, err
	}
	if t.Ignore, err = cleanList(t.Ignore, MaxIgnoreWords, MaxWordRunes, true, "ignore word"); err != nil {
		return Topic{}, err
	}
	if t.Sources, err = cleanSources(t.Sources); err != nil {
		return Topic{}, err
	}
	if len(t.Queries)+len(t.Sources) == 0 {
		return Topic{}, fmt.Errorf("%w: a topic needs something to read: a search or a source", ErrInvalid)
	}
	return t, nil
}

// cleanSearches checks a topic's news searches and the region they are read in.
func cleanSearches(queries []string, region string) ([]string, string, error) {
	queries, err := cleanList(queries, MaxQueries, MaxQueryRunes, false, "search")
	if err != nil {
		return nil, "", err
	}
	for _, q := range queries {
		if err := sources.CheckNewsQuery(q); err != nil {
			return nil, "", fmt.Errorf("%w: %s", ErrInvalid, err)
		}
	}
	if region = strings.TrimSpace(region); region != "" {
		if err := sources.CheckLocale(region); err != nil {
			return nil, "", fmt.Errorf("%w: %s", ErrInvalid, err)
		}
	}
	return queries, region, nil
}

// cleanSources canonicalises and checks the sources a topic reads beyond its searches.
func cleanSources(in []sources.Spec) ([]sources.Spec, error) {
	if len(in) > MaxTopicSources {
		return nil, fmt.Errorf("%w: a topic reads at most %d sources", ErrInvalid, MaxTopicSources)
	}
	out := make([]sources.Spec, len(in))
	for i, s := range in {
		s = s.Canonical()
		if !topicKinds[s.Kind] {
			return nil, fmt.Errorf("%w: a brief topic reads feeds, news searches, Hacker News and Mastodon, not %q", ErrInvalid, s.Kind)
		}
		if err := s.Validate(); err != nil {
			return nil, fmt.Errorf("%w: source %d: %s", ErrInvalid, i+1, err)
		}
		out[i] = s
	}
	return out, nil
}

// Normalize validates an edition the owner described and fills its defaults. topics says which topic ids exist.
func (e Edition) Normalize(topics func(id int64) bool) (Edition, error) {
	e.Name = strings.TrimSpace(e.Name)
	if e.Name == "" || len([]rune(e.Name)) > MaxNameRunes {
		return Edition{}, fmt.Errorf("%w: an edition needs a name of 1 to %d characters", ErrInvalid, MaxNameRunes)
	}
	if !readyByRe.MatchString(e.ReadyBy) {
		return Edition{}, fmt.Errorf("%w: ready by is a time of day such as 07:00", ErrInvalid)
	}
	var err error
	if e.Days, err = cleanDays(e.Days); err != nil {
		return Edition{}, err
	}
	if e.CollectEvery, e.CollectFor, err = collectWindow(e.CollectEvery, e.CollectFor); err != nil {
		return Edition{}, err
	}
	if e.Sections, err = cleanSections(e.Sections, topics); err != nil {
		return Edition{}, err
	}
	return e, nil
}

// collectWindow fills the defaults of how often and for how long before ready an edition collects, and checks both are in range.
func collectWindow(every, forDur time.Duration) (time.Duration, time.Duration, error) {
	if every == 0 {
		every = DefaultEvery
	}
	if every < MinCollectEvery || every > MaxCollectEvery {
		return 0, 0, fmt.Errorf("%w: collect every %s to %s", ErrInvalid, MinCollectEvery, MaxCollectEvery)
	}
	if forDur == 0 {
		forDur = DefaultCollectFor
	}
	if forDur < MinCollectFor || forDur > MaxCollectFor {
		return 0, 0, fmt.Errorf("%w: collect for %s to %s before it is ready", ErrInvalid, MinCollectFor, MaxCollectFor)
	}
	return every, forDur, nil
}

// cleanSections checks an edition's sections: how many, and that each is a news section of a topic that exists.
func cleanSections(in []Section, topics func(id int64) bool) ([]Section, error) {
	if len(in) == 0 || len(in) > MaxSections {
		return nil, fmt.Errorf("%w: an edition has 1 to %d sections", ErrInvalid, MaxSections)
	}
	for i, s := range in {
		if s.Kind != SectionNews || !topics(s.TopicID) {
			return nil, fmt.Errorf("%w: section %d must be a news section of a topic that exists", ErrInvalid, i+1)
		}
	}
	return append([]Section(nil), in...), nil
}

// cleanList trims, optionally lower-cases, drops blanks and repeats, and bounds a list of short texts.
func cleanList(in []string, max, maxRunes int, lower bool, what string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if lower {
			s = strings.ToLower(s)
		}
		if s == "" || seen[s] {
			continue
		}
		if len([]rune(s)) > maxRunes {
			return nil, fmt.Errorf("%w: a %s is at most %d characters", ErrInvalid, what, maxRunes)
		}
		seen[s] = true
		out = append(out, s)
	}
	if len(out) > max {
		return nil, fmt.Errorf("%w: at most %d %ss", ErrInvalid, max, what)
	}
	return out, nil
}

// cleanDays sorts and de-duplicates the weekdays; none means every day.
func cleanDays(in []int) ([]int, error) {
	if len(in) == 0 {
		return append([]int(nil), weekdays[:]...), nil
	}
	on := map[int]bool{}
	for _, d := range in {
		if d < 0 || d > 6 {
			return nil, fmt.Errorf("%w: a day is 0 (Sunday) to 6 (Saturday)", ErrInvalid)
		}
		on[d] = true
	}
	var out []int
	for _, d := range weekdays {
		if on[d] {
			out = append(out, d)
		}
	}
	return out, nil
}
