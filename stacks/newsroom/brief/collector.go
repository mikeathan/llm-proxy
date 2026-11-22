package brief

import (
	"context"
	"errors"
	"time"

	"llm-proxy-stacks/newsroom/sources"
)

// Outcome is what one collection of one topic did, for the caller to log or show.
type Outcome struct {
	TopicID  int64    `json:"topic_id"`
	Name     string   `json:"name"`
	New      int      `json:"new"`
	Problems []string `json:"problems,omitempty"`
}

// Collector reads the sources of the topics that are due and puts the headlines in the pool. It reads and stores; it never calls a model. Tick does one
// pass and returns, and the stack's one loop calls it on a timer: the collector owns no goroutine.
type Collector struct {
	Timing
	Store   *Store
	Readers *sources.Registry
}

// Collecting says whether an edition is in its collection window at now: it is enabled and its next ready time is no more than CollectFor away.
func (c *Collector) Collecting(e Edition, now time.Time) (bool, error) {
	if !e.Enabled {
		return false, nil
	}
	ready, err := c.NextReady(e, now)
	if err != nil || ready.IsZero() {
		return false, err
	}
	return ready.Sub(now) <= e.CollectFor, nil
}

// dueTopics are the topics some collecting edition lists and that were last collected at least that edition's CollectEvery ago (or never). A topic in
// several collecting editions follows the shortest interval.
func (c *Collector) dueTopics(now time.Time) ([]Topic, error) {
	every := map[int64]time.Duration{}
	for _, e := range c.Store.Editions() {
		on, err := c.Collecting(e, now)
		if err != nil {
			return nil, err
		}
		if !on {
			continue
		}
		for _, s := range e.Sections {
			if d, ok := every[s.TopicID]; !ok || e.CollectEvery < d {
				every[s.TopicID] = e.CollectEvery
			}
		}
	}
	var due []Topic
	for _, t := range c.Store.Topics() {
		if d, ok := every[t.ID]; ok && (t.LastCollected.IsZero() || !now.Before(t.LastCollected.Add(d))) {
			due = append(due, t)
		}
	}
	return due, nil
}

// Tick collects every topic that is due, one after another, and returns what each did. A cancelled context stops before the next topic.
func (c *Collector) Tick(ctx context.Context) ([]Outcome, error) {
	now := c.now()
	due, err := c.dueTopics(now)
	if err != nil {
		return nil, err
	}
	var out []Outcome
	for _, t := range due {
		if ctx.Err() != nil {
			break
		}
		o, err := c.collect(ctx, t, now)
		if errors.Is(err, ErrNotFound) {
			continue // the owner deleted the topic while this pass ran: carry on with the others
		}
		if err != nil {
			return out, err
		}
		out = append(out, o)
	}
	return out, nil
}

// CollectNow collects one topic now, whatever the schedule says (the owner pressed "collect now").
func (c *Collector) CollectNow(ctx context.Context, id int64) (Outcome, error) {
	t, err := c.Store.Topic(id)
	if err != nil {
		return Outcome{}, err
	}
	return c.collect(ctx, t, c.now())
}

// collect reads every source of the topic and stores what they answered. A source that fails is named in the outcome and in the topic's record; the
// others still count. Every source failing is recorded too, so the owner sees why a section is empty, and the topic is read again at its next interval.
func (c *Collector) collect(ctx context.Context, t Topic, now time.Time) (Outcome, error) {
	var items []sources.Item
	var problems []string
	for _, spec := range t.Readers() {
		got, err := c.Readers.Reader(spec).Fetch(ctx)
		if err != nil {
			problems = append(problems, err.Error()) // every reader already names itself in its error
			continue
		}
		items = append(items, got...)
	}
	added, err := c.Store.RecordCollection(t.ID, items, problems, now)
	if err != nil {
		return Outcome{}, err
	}
	return Outcome{TopicID: t.ID, Name: t.Name, New: added, Problems: problems}, nil
}
