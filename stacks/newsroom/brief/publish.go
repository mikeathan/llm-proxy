package brief

import (
	"context"
	"errors"
	"sync"
	"time"

	"llm-proxy-stacks/sdk"
)

// Timings of making an edition.
const (
	// PublishLead is how long before an edition's ready time it is written, so a slow or cold model cannot make it late.
	PublishLead = 20 * time.Minute
	// PublishCatchUp is how long after its ready time an edition that was not made (the service was stopped) is still made.
	PublishCatchUp = 3 * time.Hour
)

// Publisher writes each edition once for every ready time: the stack's loop calls Tick, and the owner's "make it now" calls MakeNow. It owns no goroutine.
type Publisher struct {
	Timing
	Store *Store
	// Model may be nil: an edition is then made from the pool alone.
	Model Model
	// Watches reports what the watches found between two moments; nil leaves the "Your watches" block out of every edition.
	Watches func(since, until time.Time) []WatchLine
	// Home reads the core's fixed home facts; nil leaves the Home block out. It is called after Compose and its answer is only stored.
	Home func(ctx context.Context) []stacksdk.HomeLine
	// Day reads the owner's reminders for today or tomorrow; nil leaves the "Your day" block out. It is called after Compose and its answer is only stored:
	// a title is the owner's own words and never reaches a model, a fetch or a connector (ADR-0083).
	Day func(ctx context.Context, tomorrow bool) ([]stacksdk.DayItem, error)
	// Send tells a connector the short summary of a scheduled issue; nil sends nothing. The core words the message.
	Send func(ctx context.Context, channel string, req stacksdk.BriefNoticeRequest) error

	// mu keeps two editions from being written at once, so the loop and a manual request take turns and do not both write the same edition.
	mu sync.Mutex
}

// Made is what one pass of the publisher did for one edition, for the caller to log.
type Made struct {
	EditionID  int64
	IssueID    int64
	ModelCalls int
}

// Tick writes every enabled edition that is due: its ready time is no more than PublishLead ahead, or no more than PublishCatchUp behind, and it was not
// already written for that time. A cancelled context stops before the next edition.
func (p *Publisher) Tick(ctx context.Context) ([]Made, error) {
	now := p.now()
	var out []Made
	for _, e := range p.Store.Editions() {
		if ctx.Err() != nil {
			break
		}
		if !e.Enabled {
			continue
		}
		slot, err := p.dueSlot(e, now)
		if err != nil || slot.IsZero() || p.Store.Made(e.ID, slot) {
			if err != nil {
				return out, err
			}
			continue
		}
		is, err := p.make(ctx, e.ID, slot, nil)
		if errors.Is(err, ErrNotFound) {
			continue // the owner deleted the edition while this pass ran
		}
		if err != nil {
			return out, err
		}
		out = append(out, Made{EditionID: e.ID, IssueID: is.ID, ModelCalls: is.ModelCalls})
	}
	return out, nil
}

// dueSlot is the latest ready time of the edition that is within the lead ahead of now or the catch-up behind it; the zero time when there is none.
func (p *Publisher) dueSlot(e Edition, now time.Time) (time.Time, error) {
	var due time.Time
	at := now.Add(-PublishCatchUp)
	// A weekly edition has at most one slot in the window, a daily one a few; the bound only guards a schedule that never advances.
	for range 16 {
		slot, err := p.NextReady(e, at)
		if err != nil || slot.IsZero() || slot.After(now.Add(PublishLead)) {
			return due, err
		}
		due, at = slot, slot
	}
	return due, nil
}

// MakeNow writes the edition now, whatever the schedule says, and stores it with no ready time, so it never stands in for a scheduled one.
func (p *Publisher) MakeNow(ctx context.Context, editionID int64, progress Progress) (Issue, error) {
	return p.make(ctx, editionID, time.Time{}, progress)
}

// make writes and stores one edition for a ready time (the zero time for a manual one).
func (p *Publisher) make(ctx context.Context, editionID int64, readyFor time.Time, progress Progress) (Issue, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	snap := p.Store.Snapshot()
	var edition *Edition
	for i := range snap.Editions {
		if snap.Editions[i].ID == editionID {
			edition = &snap.Editions[i]
		}
	}
	if edition == nil {
		return Issue{}, ErrNotFound
	}
	now := p.now()
	is := Compose(ctx, ComposeInput{Edition: *edition, Topics: snap.Topics, Pool: snap.Pool, Shown: snap.Shown, Model: p.Model, Now: now, Progress: progress})
	is.ReadyFor = readyFor.UTC()
	if edition.Home && p.Home != nil {
		is.Home = p.Home(ctx)
	}
	if edition.Day && p.Day != nil {
		is.Day = dayBlock(ctx, p.Day, *edition)
	}
	if edition.Watches && p.Watches != nil {
		is.Watches = p.Watches(p.since(editionID, now), now)
	}
	return p.Store.SaveIssue(is, now)
}

// WatchesWindow is how far back the "Your watches" block looks for an edition that has no earlier one.
const WatchesWindow = 24 * time.Hour

// since is when the edition was last made, or a day ago for the first one.
func (p *Publisher) since(editionID int64, now time.Time) time.Time {
	if made := p.Store.Issues(editionID); len(made) > 0 {
		return made[0].MadeAt
	}
	return now.Add(-WatchesWindow)
}
