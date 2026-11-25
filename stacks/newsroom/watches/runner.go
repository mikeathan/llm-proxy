package watches

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

// Notifier delivers one watch's notice (the new items and changed facts, as data: the core words and sends it). An
// error means nothing was delivered, and the items stay unrecorded so the next run tries again.
type Notifier func(ctx context.Context, t Watch, n Notice) error

// ErrNoChannel is what a Notifier returns when there is nowhere to send: the watch's row shows "no channel" instead of a quiet success.
var ErrNoChannel = errors.New("no connector to send through")

// Outcome is what one watch's run did, for the caller to log or show.
type Outcome struct {
	WatchID int64  `json:"watch_id"`
	Status  Status `json:"status"`
	Detail  string `json:"detail"`
	// New is the number of unseen items found.
	New int `json:"new"`
}

// Runner reads the watches that are due and hands each one's new items to Notify. Tick does one pass and returns; a loop in the stack's Start
// (a later phase, in the one lifecycle place) calls it on a timer. It owns no goroutine.
type Runner struct {
	Store *Store
	// Readers hands out the one reader of each source, so a reader's politeness floor holds however many callers share the registry.
	Readers *sources.Registry
	Notify  Notifier
	// Analyzer is the model step; nil means every unseen item is announced as new with no comparison (and no model call).
	Analyzer Analyzer
	// Now is the clock; nil means time.Now.
	Now func() time.Time

	// running serialises Run: the read, record and deliver sequence of one watch is not atomic, so two runs at once (the timer's pass and a manual
	// run from the screen) could both find the same items unseen and, with a notice, send them twice.
	running sync.Mutex
}

func (r *Runner) now() time.Time {
	if r.Now != nil {
		return r.Now().UTC()
	}
	return time.Now().UTC()
}

// Tick runs every watch that is due, one after another, and returns what each did. A cancelled context stops before the next watch.
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

// Run reads one watch now, whether or not it is due, and records the outcome. The first successful read only records what is already there
// (the baseline), so a new watch does not announce the whole backlog of its sources.
func (r *Runner) Run(ctx context.Context, t Watch) Outcome {
	r.running.Lock()
	defer r.running.Unlock()
	// The watch is read again under the lock: a run that waited may find it edited or deleted, and what it records must follow the latest.
	cur, err := r.Store.Get(t.ID)
	if errors.Is(err, ErrNotFound) {
		return Outcome{WatchID: t.ID, Status: StatusFailing, Detail: "the watch no longer exists"}
	}
	if err == nil {
		t = cur
	}
	at := r.now()
	items, failures, answered, waiting := r.read(ctx, t)
	if answered == 0 && len(failures) == 0 && waiting > 0 {
		// Every source is only waiting out its politeness floor: nothing was read, so nothing is recorded and the last result stands.
		return Outcome{WatchID: t.ID, Status: StatusWaiting, Detail: "this was read a moment ago; try again in a few minutes"}
	}
	if answered == 0 {
		r.keepProblems(t.ID, nil) // every source failed: the run's own detail says so
		return r.finish(t, at, StatusFailing, firstNonEmpty(strings.Join(failures, "; "), "no source could be read"), 0, false)
	}
	r.keepProblems(t.ID, failures)
	detail := ""
	recent, stale := t.Fresh(r.Store.Unseen(t.ID, items, at), at)
	fresh, skipped := t.Gate(recent)
	skipped = append(skipped, stale...)
	// What the keywords and skip words drop is recorded as seen, so it is not looked at again on every run.
	if err := r.Store.Record(t.ID, skipped, at); err != nil {
		return r.finish(t, at, StatusFailing, "could not save what was skipped: "+err.Error(), 0, false)
	}
	if !t.Baselined {
		return r.baseline(ctx, t, at, fresh, detail)
	}
	if len(fresh) == 0 {
		return r.finish(t, at, StatusNothingNew, detail, 0, false)
	}
	return r.deliver(ctx, t, at, fresh, detail)
}

// keepProblems stores which sources this run could not read (none clears the list). A failed save (or a watch deleted meanwhile) is not the run's
// failure: the list is a warning and the next run writes it again.
func (r *Runner) keepProblems(id int64, problems []string) {
	_ = r.Store.SetProblems(id, problems)
}

func (r *Runner) baseline(ctx context.Context, t Watch, at time.Time, fresh []sources.Item, detail string) Outcome {
	if err := r.Store.Record(t.ID, fresh, at); err != nil {
		return r.finish(t, at, StatusFailing, "could not save the baseline: "+err.Error(), 0, false)
	}
	note := joinDetail(fmt.Sprintf("%d items already there were recorded, not announced", len(fresh)), r.firstFindings(ctx, t, at, fresh))
	return r.finish(t, at, StatusBaseline, firstNonEmpty(detail, note), 0, true)
}

// firstFindings writes the first last-known-state note, so the next run can say what changed, and puts the first matching items in the feed. A failure
// costs only those. With what-to-look-for text but no usable judgement (no model, a refused call, an answer without matches) it shows nothing rather
// than everything, because it cannot tell what matches.
func (r *Runner) firstFindings(ctx context.Context, t Watch, at time.Time, fresh []sources.Item) string {
	if len(fresh) == 0 {
		return ""
	}
	in := r.input(t, fresh)
	a, why, err := r.analyze(ctx, in)
	if err != nil {
		return "the first state note could not be written: " + err.Error()
	}
	note := why
	if r.Analyzer != nil && why == "" {
		if err := r.Store.SetState(t.ID, a.State); err != nil {
			note = joinDetail(note, "the first state note could not be saved")
		}
	}
	if t.Judges() && !a.Judged {
		return note
	}
	n, _, _ := buildNotice(t.Name, in, a)
	return joinDetail(note, r.showFirst(t, at, n))
}

// MaxFirstFindings is how many of the items already there a new watch puts in the feed on its first check, so the feed is not empty on day one.
const MaxFirstFindings = 5

// showFirst puts the first few items of a watch's first check in the feed, marked as such. They are not announced: nothing is sent.
func (r *Runner) showFirst(t Watch, at time.Time, n Notice) string {
	if len(n.Items) > MaxFirstFindings {
		n.Items = n.Items[:MaxFirstFindings]
	}
	if len(n.Items) == 0 {
		return ""
	}
	if err := r.Store.AddEntries(t.ID, n.Items, noticeMarks(n, true), at); err != nil {
		return "the first findings could not be saved to the feed"
	}
	return fmt.Sprintf("%d of them are in the feed", len(n.Items))
}

// input builds what the model sees for a watch's batch of new items, capped at MaxAnalysed.
func (r *Runner) input(t Watch, batch []sources.Item) Input {
	if len(batch) > MaxAnalysed {
		batch = batch[:MaxAnalysed]
	}
	return Input{Watch: t.Name, LookFor: t.LookFor, Want: t.Examples.Want, Skip: t.Examples.Skip, State: t.State, Recent: r.Store.Recent(t.ID, RecentShown), Items: batch}
}

func (r *Runner) deliver(ctx context.Context, t Watch, at time.Time, fresh []sources.Item, detail string) Outcome {
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
	if t.Judges() && !a.Judged {
		return r.finish(t, at, StatusAnalysisFailed, unjudged(note), len(fresh), false)
	}
	detail = joinDetail(detail, note)
	notice, repeated, unmatched := buildNotice(t.Name, in, a)
	detail = joinDetail(joinDetail(detail, repeatDetail(in, a, repeated)), unmatchedDetail(in, unmatched))
	r.foldRepeats(t.ID, at, in, a, repeated)
	detail = joinDetail(detail, r.keepAside(t.ID, at, in, a, unmatched))
	if len(notice.Items) == 0 {
		return r.settle(t, at, fresh, a, StatusNothingNew, detail, 0)
	}
	// The feed always records a change, so the owner sees it even when no message could be sent.
	if err := r.Store.AddEntries(t.ID, notice.Items, noticeMarks(notice, false), at); err != nil {
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

// unjudged is the row's words when a watch with what-to-look-for text could not have its items judged (no model, a refused call): nothing is shown or
// recorded, and the next run tries again, because showing every item would put posts in the feed that the owner did not ask for.
func unjudged(note string) string {
	return joinDetail("could not check the new items against what you look for", firstNonEmpty(note, "no model is set up for the Newsroom"))
}

// notCompared is the row's words when the model refuses with CodeDenied: the core's own sentence (the daily limit is spent, or no model is chosen).
func notCompared(err error) string {
	var coded *stacksdk.Error
	if errors.As(err, &coded) && coded.Message != "" {
		return "not compared: " + coded.Message
	}
	return "not compared: the model would not answer"
}

// analyze runs the model step. A refusal with CodeDenied (a spent daily limit, or no model chosen) is not a failure of the watch: the items are announced as new with no comparison (the feed must
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
func (r *Runner) foldRepeats(watchID int64, at time.Time, in Input, a Analysis, repeated []int) {
	for _, i := range repeated {
		it := in.Items[i-1]
		_, _ = r.Store.FoldRepeat(watchID, sources.Line(in.Recent[a.Repeats[i]-1], 100), AlsoSeen{Source: it.Source, Link: it.Link, At: at})
	}
}

// keepAside puts the items the model called not relevant in the journal, marked left out, with its reason, so the owner can check the call. An item
// with no verdict at all has no reason and is kept aside too: it was not shown either. It returns the row's words when they could not be kept, else "".
func (r *Runner) keepAside(watchID int64, at time.Time, in Input, a Analysis, unmatched []int) string {
	if len(unmatched) == 0 {
		return ""
	}
	items := make([]sources.Item, len(unmatched))
	marks := Marks{Whys: map[int]string{}, LeftOut: true}
	for n, i := range unmatched {
		items[n] = in.Items[i-1]
		if why := a.Rejects[i]; why != "" {
			marks.Whys[n] = why
		}
	}
	if err := r.Store.AddEntries(watchID, items, marks, at); err != nil {
		return "what was left out could not be kept aside"
	}
	return ""
}

// noticeMarks maps an item's index in the notice to what the journal keeps about it: its changed fact and why it matched.
func noticeMarks(n Notice, first bool) Marks {
	m := Marks{Changes: map[int]Change{}, Whys: map[int]string{}, Digests: map[int]Digest{}, First: first}
	for _, c := range n.Changes {
		m.Changes[c.Item-1] = c
	}
	for i, why := range n.Whys {
		m.Whys[i-1] = why
	}
	for i, d := range n.Digests {
		m.Digests[i-1] = d
	}
	return m
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
func (r *Runner) settle(t Watch, at time.Time, batch []sources.Item, a Analysis, status Status, detail string, sent int) Outcome {
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

// buildNotice drops the items the model judged repeats (a citation the framework checked) and, when it judged relevance, the items it did not call
// relevant, then renumbers the changed facts and reasons onto what is left. A change on a dropped item is dropped with it. It also returns the dropped
// repeats; the items that did not match are named by unmatchedDetail.
func buildNotice(name string, in Input, a Analysis) (Notice, []int, []int) {
	n := Notice{Watch: name, Whys: map[int]string{}, Digests: map[int]Digest{}}
	newIndex := map[int]int{}
	var repeated, unmatched []int
	for i, it := range in.Items {
		_, rep := a.Repeats[i+1]
		why, matched := a.Matches[i+1]
		switch {
		case rep:
			repeated = append(repeated, i+1)
			continue
		case a.Judged && !matched:
			unmatched = append(unmatched, i+1)
			continue
		}
		n.Items = append(n.Items, it)
		newIndex[i+1] = len(n.Items)
		if why != "" {
			n.Whys[len(n.Items)] = why
		}
		if d, ok := a.Digests[i+1]; ok {
			n.Digests[len(n.Items)] = d
		}
	}
	for _, c := range a.Changes {
		if idx, ok := newIndex[c.Item]; ok {
			n.Changes = append(n.Changes, Change{Item: idx, Was: c.Was, Now: c.Now})
		}
	}
	n.Repeats = len(repeated)
	return n, repeated, unmatched
}

// unmatchedDetail names the items the model did not call relevant, so nothing it removed disappears unseen.
func unmatchedDetail(in Input, unmatched []int) string {
	if len(unmatched) == 0 {
		return ""
	}
	const shown = 3
	var parts []string
	for _, i := range unmatched[:min(shown, len(unmatched))] {
		parts = append(parts, fmt.Sprintf("%q", sources.Line(in.Items[i-1].Title, 60)))
	}
	more := ""
	if len(unmatched) > shown {
		more = fmt.Sprintf(" and %d more", len(unmatched)-shown)
	}
	return fmt.Sprintf("%d did not match what you look for: %s%s", len(unmatched), strings.Join(parts, "; "), more)
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

// read fetches every source of a watch and returns the merged items, one line per failing source and how many sources answered. A source that is
// only waiting out its politeness floor is neither failing nor answered; it is counted in waiting.
func (r *Runner) read(ctx context.Context, t Watch) (items []sources.Item, failures []string, answered, waiting int) {
	for _, spec := range t.Sources {
		got, err := r.Readers.Reader(spec).Fetch(ctx)
		switch {
		case errors.Is(err, sources.ErrTooSoon):
			waiting++
		case err != nil:
			failures = append(failures, err.Error()) // every reader already names itself in its error
		default:
			answered++
			items = append(items, got...)
		}
	}
	return items, failures, answered, waiting
}

func (r *Runner) finish(t Watch, at time.Time, status Status, detail string, n int, baselined bool) Outcome {
	if err := r.Store.SetRun(t.ID, at, status, detail, n, baselined); err != nil && !errors.Is(err, ErrNotFound) {
		detail = firstNonEmpty(detail, "") + " (the result could not be saved: " + err.Error() + ")"
	}
	return Outcome{WatchID: t.ID, Status: status, Detail: detail, New: n}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
