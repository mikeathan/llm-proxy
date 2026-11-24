package newsroom

import (
	"context"
	"time"

	"llm-proxy-stacks/newsroom/brief"
	"llm-proxy-stacks/newsroom/watches"
	stacksdk "llm-proxy-stacks/sdk"
)

// watchLines reads the watch journal for the "Your watches" block of an edition: one line per watch that found something in the window (its newest
// finding, with the model's headline and summary when the watch wrote them) or that has a source it cannot read. Findings shown from a first check were
// already there, so they are not news and are left out. Nothing is sent and no model is asked.
func watchLines(store *watches.Store, since, until time.Time) []brief.WatchLine {
	if store == nil {
		return nil
	}
	found := map[int64][]watches.Entry{}
	for _, e := range store.Entries(watches.Filter{}) { // newest first
		if !e.First && e.At.After(since) && !e.At.After(until) {
			found[e.WatchID] = append(found[e.WatchID], e)
		}
	}
	var out []brief.WatchLine
	for _, w := range store.List() {
		line := brief.WatchLine{Watch: w.Name, New: len(found[w.ID]), Problems: w.LastProblems}
		if es := found[w.ID]; len(es) > 0 {
			top := es[0]
			line.Headline, line.Summary, line.Link, line.At = top.Headline, top.Summary, top.Link, top.At
			if line.Headline == "" {
				line.Headline = top.Title
			}
		}
		if line.New > 0 || len(line.Problems) > 0 {
			out = append(out, line)
		}
	}
	return out
}

// dayReader hands the publisher the Day port, or nil when the host gave none; the "Your day" block is then left out. What it returns is the owner's own
// words, stored with the issue and shown as text: not a model prompt, not a fetch target and not part of a message to a connector (ADR-0083).
func dayReader(p stacksdk.Day) func(context.Context, bool) ([]stacksdk.DayItem, error) {
	if p == nil {
		return nil
	}
	return p.Items
}

// homeLines reads the Home port for an edition that asks for it, or is nil when the host gave none. The answer is kept as it came, bounded to
// stacksdk.MaxHomeLines, and goes nowhere but the stored edition: not to a model, not to a fetch, not to a notice (ADR-0077). A port that fails leaves
// the block out; it never fails the edition.
func homeLines(p stacksdk.Home) func(context.Context) []stacksdk.HomeLine {
	if p == nil {
		return nil
	}
	return func(ctx context.Context) []stacksdk.HomeLine {
		lines, err := p.Lines(ctx)
		if err != nil {
			return nil
		}
		return lines[:min(len(lines), stacksdk.MaxHomeLines)]
	}
}
