package topics

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"llm-proxy-stacks/sdk"

	"llm-proxy-stacks/newsroom/sources"
)

// Notifier delivers one topic's notice (the new items and changed facts, as data: the core words and sends it). An
// error means nothing was delivered, and the items stay unrecorded so the next run tries again.
type Notifier func(ctx context.Context, t Topic, n Notice) error

// ErrNoChannel is what a Notifier returns when there is nowhere to send: the topic's row shows "no channel" instead of a quiet success.
var ErrNoChannel = errors.New("no connector to send through")

// Outcome is what one topic's run did, for the caller to log or show.
type Outcome struct {
	TopicID int64  `json:"topic_id"`
	Status  Status `json:"status"`
	Detail  string `json:"detail"`
	// New is the number of unseen items found.
	New int `json:"new"`
}

// Runner reads the topics that are due and hands each one's new items to Notify. Tick does one pass and returns; a loop in the stack's Start
// (a later phase, in the one lifecycle place) calls it on a timer. It owns no goroutine.
type Runner struct {
	Store  *Store
	Fetch  stacksdk.Fetch
	Notify Notifier
	// Analyzer is the model step; nil means every unseen item is announced as new with no comparison (and no model call).
	Analyzer Analyzer
	// Now is the clock; nil means time.Now.
	Now func() time.Time

	mu    sync.Mutex
	cache map[string]sources.Source
	// running serialises Run: the read, record and deliver sequence of one topic is not atomic, so two runs at once (the timer's pass and a manual
	// run from the screen) could both find the same items unseen and, with a notice, send them twice.
	running sync.Mutex
}

func (r *Runner) now() time.Time {
	if r.Now != nil {
		return r.Now().UTC()
	}
	return time.Now().UTC()
}

// Tick runs every topic that is due, one after another, and returns what each did. A cancelled context stops before the next topic.
func (r *Runner) Tick(ctx context.Context) []Outcome {
	var out []Outcome
	for _, t := range r.Store.List() {
		if ctx.Err() != nil {
			break
		}
		if t.Due(r.now()) {
			out = append(out, r.Run(ctx, t))
		}
	}
	return out
}

// Run reads one topic now, whether or not it is due, and records the outcome. The first successful read only records what is already there
// (the baseline), so a new topic does not announce the whole backlog of its sources.
func (r *Runner) Run(ctx context.Context, t Topic) Outcome {
	r.running.Lock()
	defer r.running.Unlock()
	// The topic is read again under the lock: a run that waited may find it edited or deleted, and what it records must follow the latest.
	cur, err := r.Store.Get(t.ID)
	if errors.Is(err, ErrNotFound) {
		return Outcome{TopicID: t.ID, Status: StatusFailing, Detail: "the topic no longer exists"}
	}
	if err == nil {
		t = cur
	}
	at := r.now()
	items, failures, answered := r.read(ctx, t)
	detail := strings.Join(failures, "; ")
	if answered == 0 {
		return r.finish(t, at, StatusFailing, firstNonEmpty(detail, "no source could be read"), 0, false)
	}
	fresh := r.Store.Unseen(t.ID, items, at)
	if !t.Baselined {
		return r.baseline(ctx, t, at, fresh, detail)
	}
	if len(fresh) == 0 {
		return r.finish(t, at, StatusNothingNew, detail, 0, false)
	}
	return r.deliver(ctx, t, at, fresh, detail)
}

func (r *Runner) baseline(ctx context.Context, t Topic, at time.Time, fresh []sources.Item, detail string) Outcome {
	if err := r.Store.Record(t.ID, fresh, at); err != nil {
		return r.finish(t, at, StatusFailing, "could not save the baseline: "+err.Error(), 0, false)
	}
	note := fmt.Sprintf("%d items already there were recorded, not announced", len(fresh))
	if r.Analyzer != nil && len(fresh) > 0 {
		// One call writes the first last-known-state note, so the next run can say what changed. A failure here costs only that note.
		if a, err := r.Analyzer.Analyze(ctx, r.input(t, fresh)); err != nil {
			note += "; the first state note could not be written: " + err.Error()
		} else if err := r.Store.SetState(t.ID, a.State); err != nil {
			note += "; the first state note could not be saved"
		}
	}
	return r.finish(t, at, StatusBaseline, firstNonEmpty(detail, note), 0, true)
}

// input builds what the model sees for a topic's batch of new items, capped at MaxAnalysed.
func (r *Runner) input(t Topic, batch []sources.Item) Input {
	if len(batch) > MaxAnalysed {
		batch = batch[:MaxAnalysed]
	}
	return Input{Topic: t.Name, State: t.State, Recent: r.Store.Recent(t.ID, RecentShown), Items: batch}
}

func (r *Runner) deliver(ctx context.Context, t Topic, at time.Time, fresh []sources.Item, detail string) Outcome {
	if t.Channel != "" && r.Notify == nil {
		return r.finish(t, at, StatusNoChannel, "no way to send was set up", len(fresh), false)
	}
	// One call sees at most MaxAnalysed items; the rest are not recorded and are taken by the next run.
	if len(fresh) > MaxAnalysed {
		fresh = fresh[:MaxAnalysed]
	}
	in := r.input(t, fresh)
	a, note, err := r.analyze(ctx, in)
	if err != nil {
		return r.finish(t, at, StatusAnalysisFailed, err.Error(), len(fresh), false)
	}
	detail = joinDetail(detail, note)
	notice, repeated := buildNotice(t.Name, in, a)
	detail = joinDetail(detail, repeatDetail(in, a, repeated))
	r.foldRepeats(t.ID, at, in, a, repeated)
	if len(notice.Items) == 0 {
		return r.settle(t, at, fresh, a, StatusNothingNew, detail, 0)
	}
	// The feed always records a change, so the owner sees it even when no message could be sent.
	if err := r.Store.AddEntries(t.ID, notice.Items, noticeChanges(notice), at); err != nil {
		return r.finish(t, at, StatusFailing, "could not save to the feed: "+err.Error(), len(notice.Items), false)
	}
	if t.Channel == "" {
		return r.settle(t, at, fresh, a, StatusInFeed, detail, len(notice.Items))
	}
	switch err := r.Notify(ctx, t, notice); {
	case errors.Is(err, ErrNoChannel):
		return r.finish(t, at, StatusNoChannel, err.Error(), len(notice.Items), false)
	case err != nil:
		return r.finish(t, at, StatusSendFailed, err.Error(), len(notice.Items), false)
	}
	_ = r.Store.MarkSent(t.ID, identities(notice.Items))
	return r.settle(t, at, fresh, a, StatusSent, detail, len(notice.Items))
}

// notCompared is the row's words when the model refuses with CodeDenied: the core's own sentence (the daily limit is spent, or no model is chosen).
func notCompared(err error) string {
	var coded *stacksdk.Error
	if errors.As(err, &coded) && coded.Message != "" {
		return "not compared: " + coded.Message
	}
	return "not compared: the model would not answer"
}

// analyze runs the model step. A refusal with CodeDenied (a spent daily limit, or no model chosen) is not a failure of the topic: the items are announced as new with no comparison (the feed must
// record a change even when no model can look at it) and the row says so. Any other failure stops the run, so the next one tries again.
func (r *Runner) analyze(ctx context.Context, in Input) (Analysis, string, error) {
	if r.Analyzer == nil {
		return Analysis{}, "", nil
	}
	a, err := r.Analyzer.Analyze(ctx, in)
	switch {
	case stacksdk.CodeOf(err) == stacksdk.CodeDenied:
		return Analysis{}, notCompared(err), nil
	case err != nil:
		return Analysis{}, "", err
	}
	return a, "", nil
}

// foldRepeats shows each judged repeat under the feed entry it repeats, so it is visible without being a new entry.
func (r *Runner) foldRepeats(topicID int64, at time.Time, in Input, a Analysis, repeated []int) {
	for _, i := range repeated {
		it := in.Items[i-1]
		_, _ = r.Store.FoldRepeat(topicID, sources.Line(in.Recent[a.Repeats[i]-1], 100), AlsoSeen{Source: it.Source, Link: it.Link, At: at})
	}
}

// noticeChanges maps an item's index in the notice to its changed fact, for the journal.
func noticeChanges(n Notice) map[int]Change {
	out := map[int]Change{}
	for _, c := range n.Changes {
		out[c.Item-1] = c
	}
	return out
}

func identities(items []sources.Item) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Identity()
	}
	return out
}

// settle records a batch as seen (items judged repeats included, so they do not come back) and keeps the new state note. It is called only after the
// notice was delivered, or when nothing needed delivering.
func (r *Runner) settle(t Topic, at time.Time, batch []sources.Item, a Analysis, status Status, detail string, sent int) Outcome {
	if err := r.Store.Record(t.ID, batch, at); err != nil {
		return r.finish(t, at, StatusSendFailed, "could not be recorded: "+err.Error(), len(batch), false)
	}
	if r.Analyzer != nil {
		if err := r.Store.SetState(t.ID, a.State); err != nil {
			detail = joinDetail(detail, "the state note could not be saved")
		}
	}
	return r.finish(t, at, status, detail, sent, false)
}

// buildNotice drops the items the model judged repeats (a citation the framework checked) and renumbers the changed facts onto what is left. A
// change on an item judged a repeat is dropped with it.
func buildNotice(name string, in Input, a Analysis) (Notice, []int) {
	n := Notice{Topic: name}
	newIndex := map[int]int{}
	var repeated []int
	for i, it := range in.Items {
		if _, rep := a.Repeats[i+1]; rep {
			repeated = append(repeated, i+1)
			continue
		}
		n.Items = append(n.Items, it)
		newIndex[i+1] = len(n.Items)
	}
	for _, c := range a.Changes {
		if idx, ok := newIndex[c.Item]; ok {
			n.Changes = append(n.Changes, Change{Item: idx, Was: c.Was, Now: c.Now})
		}
	}
	n.Repeats = len(repeated)
	return n, repeated
}

// repeatDetail names each judged repeat and what it repeats, so nothing the model removed disappears unseen.
func repeatDetail(in Input, a Analysis, repeated []int) string {
	if len(repeated) == 0 {
		return ""
	}
	parts := make([]string, 0, len(repeated))
	for _, i := range repeated {
		parts = append(parts, fmt.Sprintf("%q repeats %q", sources.Line(in.Items[i-1].Title, 60), sources.Line(in.Recent[a.Repeats[i]-1], 60)))
	}
	return fmt.Sprintf("%d judged repeats: %s", len(repeated), strings.Join(parts, "; "))
}

func joinDetail(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	return a + "; " + b
}

// read fetches every source of a topic and returns the merged items, one line per failing source and how many sources answered. A source that is
// only waiting out its politeness floor is neither failing nor answered.
func (r *Runner) read(ctx context.Context, t Topic) (items []sources.Item, failures []string, answered int) {
	for _, spec := range t.Sources {
		got, err := r.source(spec).Fetch(ctx)
		switch {
		case errors.Is(err, sources.ErrTooSoon):
		case err != nil:
			failures = append(failures, spec.Name()+": "+err.Error())
		default:
			answered++
			items = append(items, got...)
		}
	}
	return items, failures, answered
}

func (r *Runner) finish(t Topic, at time.Time, status Status, detail string, n int, baselined bool) Outcome {
	if err := r.Store.SetRun(t.ID, at, status, detail, n, baselined); err != nil && !errors.Is(err, ErrNotFound) {
		detail = firstNonEmpty(detail, "") + " (the result could not be saved: " + err.Error() + ")"
	}
	return Outcome{TopicID: t.ID, Status: status, Detail: detail, New: n}
}

// source returns the reader for a spec, building it once: a Telegram reader remembers when it last read so its politeness floor holds across ticks.
func (r *Runner) source(spec SourceSpec) sources.Source {
	key := fmt.Sprintf("%s|%s|%s|%s", spec.Kind, spec.Label, spec.Instance, spec.Target)
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.cache[key]; ok {
		return s
	}
	if r.cache == nil {
		r.cache = map[string]sources.Source{}
	}
	s := Build(spec, r.Fetch)
	r.cache[key] = s
	return s
}

// Build makes the reader for one spec. The spec was validated by Normalize; an unknown kind reads as a source that always fails.
func Build(spec SourceSpec, f stacksdk.Fetch) sources.Source {
	name := spec.Name()
	switch spec.Kind {
	case KindFeed:
		return sources.Feed{Label: name, URL: spec.Target, Fetcher: f}
	case KindHN:
		return sources.HackerNews{Label: name, Query: spec.Target, Fetcher: f}
	case KindMastodon:
		return sources.MastodonTag{Label: name, Instance: spec.Instance, Tag: spec.Target, Fetcher: f}
	case KindPage:
		return sources.Page{Label: name, URL: spec.Target, Fetcher: f}
	case KindTelegram:
		return &sources.TelegramChannel{Label: name, Channel: spec.Target, Fetcher: f}
	}
	return failing{name: name, kind: string(spec.Kind)}
}

type failing struct{ name, kind string }

func (f failing) Name() string { return f.name }
func (f failing) Fetch(context.Context) ([]sources.Item, error) {
	return nil, fmt.Errorf("unknown source kind %q", f.kind)
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
