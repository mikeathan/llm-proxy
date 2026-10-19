// Package progress notices when a run's read-only tool calls have stopped adding information.
//
// The agent loop's repetition detectors key on a call's tool and arguments, so a model that rewords its query or
// URL on every call is invisible to them while the results repeat. The monitor judges the results: what share of
// a result's content the run has not seen before, with a stricter bar for a call whose arguments mostly repeat an
// earlier call of the same tool. It is a pure controller (it observes and returns a decision; the caller decides
// what to do with it), generic across tools and topics, and has no model or tool names in it.
package progress

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"unicode"
)

// Level is how loudly the monitor has spoken for the tool with the longest streak of results that added nothing.
type Level int

const (
	None Level = iota
	Notice
	Strong
)

const (
	noticeAfter    = 2   // stale results of one tool in a row before the first note
	minStrongAfter = 4   // the second note never comes sooner than this
	maxStrongAfter = 6   // ... nor later, however large the step budget
	stepsPerStrong = 6   // the strong threshold is a sixth of the step budget, within the bounds above
	lowGain        = 0.2 // a result with less than this share of new content is stale
	rewordedGain   = 0.5 // ... or less than this when the call mostly repeats an earlier call's arguments
	sameArgs       = 0.6 // word overlap (Jaccard) of two calls' arguments that makes the later one a rewording
	shingleWords   = 4   // words per shingle: the unit "already seen" is judged in
	maxArgHistory  = 64  // earlier calls of a tool kept for the rewording check
)

// Config is the thresholds of a monitor.
type Config struct {
	NoticeAfter  int     // stale results of one tool in a row that raise the notice
	StrongAfter  int     // stale results of one tool in a row that raise the strong note
	LowGain      float64 // share of new content below which a result is stale
	RewordedGain float64 // the same bar for a call whose arguments mostly repeat an earlier call's
	SameArgs     float64 // word overlap of two calls' arguments from which the later one is a rewording
}

// ForSteps derives the thresholds from the run's step budget: a bigger budget tolerates a longer streak.
func ForSteps(maxSteps int) Config {
	return Config{
		NoticeAfter:  noticeAfter,
		StrongAfter:  min(max(maxSteps/stepsPerStrong, minStrongAfter), maxStrongAfter),
		LowGain:      lowGain,
		RewordedGain: rewordedGain,
		SameArgs:     sameArgs,
	}
}

// Call is what the monitor needs to know about one tool call.
type Call struct {
	Tool     string
	Args     string // the call's arguments as sent, JSON
	ReadOnly bool   // the call changes nothing (a write or a command is progress by itself)
}

// Decision is the monitor's verdict after one result.
type Decision struct {
	Level    Level
	Changed  bool   // Level rose with this result: the note should be delivered now, once
	Tool     string // the tool with the longest streak of results that added nothing
	Stale    int    // that tool's streak
	Distinct int    // results so far that added something
}

type toolState struct {
	stale   int
	argSets []map[string]struct{}
}

// Monitor tracks one run. It is safe for concurrent use: the tool calls of one batch record from parallel goroutines.
type Monitor struct {
	cfg      Config
	mu       sync.Mutex
	seen     map[string]struct{}
	tools    map[string]*toolState
	distinct int
	level    Level
}

// New returns a monitor with the given thresholds.
func New(cfg Config) *Monitor {
	return &Monitor{cfg: cfg, seen: map[string]struct{}{}, tools: map[string]*toolState{}}
}

// Observe records one tool result. A call that does not only read is progress whatever it returns, so it ends
// every streak. A read-only result is stale when too little of it is new to the run (more is asked of a call that
// repeats an earlier call's arguments), and streaks are counted per tool: a page fetched between two searches does
// not say the searches are getting anywhere.
func (m *Monitor) Observe(call Call, result []byte) Decision {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !call.ReadOnly {
		for _, ts := range m.tools {
			ts.stale = 0
		}
		m.level = None
		return Decision{Distinct: m.distinct}
	}
	ts := m.tools[call.Tool]
	if ts == nil {
		ts = &toolState{}
		m.tools[call.Tool] = ts
	}
	bar := m.cfg.LowGain
	if args := wordSet(call.Args); len(args) > 0 {
		if ts.repeatsAnEarlierCall(args, m.cfg.SameArgs) {
			bar = m.cfg.RewordedGain
		}
		ts.argSets = append(ts.argSets, args)
		if len(ts.argSets) > maxArgHistory {
			ts.argSets = ts.argSets[1:]
		}
	}
	if m.newShare(shingles(result)) < bar {
		ts.stale++
	} else {
		ts.stale = 0
		m.distinct++
	}

	tool, stale := m.longestStreak()
	level := m.levelFor(stale)
	changed := level > m.level
	m.level = level
	return Decision{Level: level, Changed: changed, Tool: tool, Stale: stale, Distinct: m.distinct}
}

func (m *Monitor) longestStreak() (tool string, stale int) {
	for name, ts := range m.tools {
		if ts.stale > stale || (ts.stale == stale && ts.stale > 0 && name < tool) {
			tool, stale = name, ts.stale
		}
	}
	return tool, stale
}

func (m *Monitor) levelFor(stale int) Level {
	switch {
	case stale >= m.cfg.StrongAfter:
		return Strong
	case stale >= m.cfg.NoticeAfter:
		return Notice
	}
	return None
}

// repeatsAnEarlierCall reports whether args share at least threshold of their words with an earlier call's.
func (ts *toolState) repeatsAnEarlierCall(args map[string]struct{}, threshold float64) bool {
	for _, earlier := range ts.argSets {
		if jaccard(args, earlier) >= threshold {
			return true
		}
	}
	return false
}

// newShare returns the share of the shingles the run has not seen, and remembers them all. A result with no
// content at all counts as nothing new.
func (m *Monitor) newShare(parts []string) float64 {
	if len(parts) == 0 {
		return 0
	}
	fresh := 0
	for _, p := range parts {
		if _, ok := m.seen[p]; !ok {
			fresh++
			m.seen[p] = struct{}{}
		}
	}
	return float64(fresh) / float64(len(parts))
}

// Note is the line to append to the result that raised the level, or "" when there is nothing new to say. It states
// what was measured; the strong form adds the one suggestion a model in a rut needs.
func (d Decision) Note() string {
	if !d.Changed {
		return ""
	}
	if d.Level == Strong {
		return fmt.Sprintf("\n\n[progress note: %d %s results in a row added nothing new. Answer from what you have, and say what is missing.]", d.Stale, d.Tool)
	}
	return fmt.Sprintf("\n\n[progress note: the last %d %s results added nothing new to what you already have (%d useful results so far).]", d.Stale, d.Tool, d.Distinct)
}

// shingles splits a result into overlapping runs of shingleWords words. Only what a result says counts, not its
// structure: for JSON the string and number values, so field names shared by every result do not read as repeats.
// A result shorter than a shingle is one shingle; an empty one is none.
func shingles(result []byte) []string {
	words := wordsOf(result)
	if len(words) == 0 {
		return nil
	}
	if len(words) <= shingleWords {
		return []string{strings.Join(words, " ")}
	}
	out := make([]string, 0, len(words)-shingleWords+1)
	for i := 0; i+shingleWords <= len(words); i++ {
		out = append(out, strings.Join(words[i:i+shingleWords], " "))
	}
	return out
}

func wordSet(text string) map[string]struct{} {
	set := map[string]struct{}{}
	for _, w := range wordsOf([]byte(text)) {
		set[w] = struct{}{}
	}
	return set
}

func jaccard(a, b map[string]struct{}) float64 {
	inter := 0
	for w := range a {
		if _, ok := b[w]; ok {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

func wordsOf(result []byte) []string {
	var parts []string
	var decoded any
	if json.Unmarshal(result, &decoded) == nil {
		parts = leaves(decoded, nil)
	} else {
		parts = []string{string(result)}
	}
	var words []string
	for _, p := range parts {
		words = append(words, strings.FieldsFunc(strings.ToLower(p), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })...)
	}
	return words
}

// leaves collects the string and number values of decoded JSON, in a stable order.
func leaves(v any, out []string) []string {
	switch t := v.(type) {
	case string:
		return append(out, t)
	case float64:
		return append(out, fmt.Sprint(t))
	case []any:
		for _, e := range t {
			out = leaves(e, out)
		}
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			out = leaves(t[k], out)
		}
	}
	return out
}
