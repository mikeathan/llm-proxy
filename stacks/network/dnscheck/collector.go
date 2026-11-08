package dnscheck

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sync"
	"time"
)

// Lookup asks one resolver for a name's IPv4 addresses: *Router and Outside.
type Lookup interface {
	LookupA(ctx context.Context, name string) ([]netip.Addr, error)
}

// Store keeps the log of comparisons. A missing file is an empty log.
type Store interface {
	Read() ([]Comparison, error)
	Write([]Comparison) error
}

// Settings are the operator's choices, read for every cycle so a change applies to the next one.
type Settings struct {
	Enabled bool
	Every   time.Duration
	Names   []string
}

// CollectorDeps are what the collector needs; the application wires them in one place.
type CollectorDeps struct {
	Settings func() Settings
	Router   Lookup
	Outside  Lookup
	Store    Store
	Now      func() time.Time
	// OnError is told about trouble, once per distinct message until the trouble ends.
	OnError func(error)
}

// Collector compares, for a few names, what the router's DNS answers with what an outside resolver answers, and logs each
// comparison. It raises no finding: a rule is written only after a week of real comparisons shows what an honest network does.
type Collector struct {
	deps CollectorDeps

	mu      sync.Mutex
	status  Status
	trouble string // the store trouble reported last, so a repeating failure is reported once
}

// Status says when the collector last ran and what, if anything, stopped it.
type Status struct {
	LastRun time.Time `json:"last_run,omitempty"`
	// Problem is a fixed phrase for why the latest cycle could not compare, or empty.
	Problem string `json:"problem,omitempty"`
}

// Summary is the log boiled down to what is needed to judge whether a rule would be quiet.
type Summary struct {
	Comparisons   int `json:"comparisons"`
	Matches       int `json:"matches"`
	Differs       int `json:"differs"`
	CannotCompare int `json:"cannot_compare"`
	// Overlapping counts the differing comparisons whose answers still shared an address (a partial or rotated answer).
	Overlapping int       `json:"overlapping"`
	First       time.Time `json:"first,omitempty"`
	Last        time.Time `json:"last,omitempty"`
	// Differences are the newest differing comparisons, newest first.
	Differences []Comparison `json:"differences"`
}

// idlePoll is how often a switched-off collector looks at its settings again.
const idlePoll = 30 * time.Second

const (
	// MaxComparisons bounds the log; the oldest comparisons go first.
	MaxComparisons = 2000
	// MaxDifferences bounds the differences a summary lists.
	MaxDifferences = 20
)

func NewCollector(deps CollectorDeps) *Collector {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &Collector{deps: deps}
}

// Run does a cycle every Every while enabled, and stops when ctx ends.
func (c *Collector) Run(ctx context.Context) {
	for {
		wait := idlePoll
		if s := c.deps.Settings(); s.Enabled {
			c.Tick(ctx)
			wait = max(s.Every, time.Millisecond)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

// Tick is one cycle: every name is asked of the router and then of the outside resolver, and the comparisons are added to the
// log. When the network settings forbid the outside request the cycle stops at once and logs nothing.
func (c *Collector) Tick(ctx context.Context) {
	s := c.deps.Settings()
	if !s.Enabled || len(s.Names) == 0 {
		return
	}
	now := c.deps.Now()
	var rows []Comparison
	problem := ""
	for _, name := range s.Names {
		row, err := c.compare(ctx, name, now)
		if p := refusedBySettings(err); p != "" {
			problem = p
			break
		}
		rows = append(rows, row)
	}
	if ctx.Err() != nil {
		return // stopped mid-cycle: the failed questions are the stop, not a silent router, and are not comparisons
	}
	c.mu.Lock()
	c.status = Status{LastRun: now, Problem: problem}
	c.mu.Unlock()
	if len(rows) > 0 {
		c.record(rows)
	}
}

// refusedBySettings names the setting that stopped a question, or is empty: a refusal is not a comparison, so nothing is logged.
func refusedBySettings(err error) string {
	switch {
	case errors.Is(err, ErrInternetOff):
		return ProblemInternetOff
	case errors.Is(err, ErrLANOff):
		return ProblemLANOff
	}
	return ""
}

// compare asks the router first: with no answer from it there is nothing to compare, so the outside resolver is not asked.
func (c *Collector) compare(ctx context.Context, name string, at time.Time) (Comparison, error) {
	row := Comparison{At: at, Name: name, Outcome: CannotCompare}
	fromRouter, err := c.deps.Router.LookupA(ctx, name)
	if err != nil {
		row.Reason = ReasonRouterSilent
		if errors.Is(err, ErrNoRouter) {
			row.Reason = ReasonNoRouter
		}
		return row, refusedErr(err)
	}
	fromOutside, err := c.deps.Outside.LookupA(ctx, name)
	if err != nil {
		row.Reason = ReasonOutsideFails
		return row, err
	}
	verdict := Compare(fromRouter, fromOutside)
	row.Outcome, row.Overlap = Differ, verdict.Overlap
	if verdict.Same {
		row.Outcome = Match
	}
	row.Router, row.Outside = addressText(fromRouter), addressText(fromOutside)
	return row, nil
}

// refusedErr is err when a setting refused the question, so Tick can stop; any other failure is a row, not an error.
func refusedErr(err error) error {
	if refusedBySettings(err) != "" {
		return err
	}
	return nil
}

func addressText(in []netip.Addr) []string {
	out := make([]string, 0, len(in))
	for _, a := range in {
		out = append(out, a.String())
	}
	return out
}

// record adds rows to the log, dropping the oldest beyond the bound. A log that cannot be read is left alone.
func (c *Collector) record(rows []Comparison) {
	items, err := c.deps.Store.Read()
	if err != nil {
		c.report(fmt.Errorf("dns check: failed to read the log: %w", err))
		return
	}
	items = append(items, rows...)
	if len(items) > MaxComparisons {
		items = items[len(items)-MaxComparisons:]
	}
	if err := c.deps.Store.Write(items); err != nil {
		c.report(fmt.Errorf("dns check: failed to save the log: %w", err))
		return
	}
	c.mu.Lock()
	c.trouble = ""
	c.mu.Unlock()
}

// report tells OnError about trouble once per distinct message until it ends.
func (c *Collector) report(err error) {
	c.mu.Lock()
	again := c.trouble == err.Error()
	c.trouble = err.Error()
	c.mu.Unlock()
	if !again && c.deps.OnError != nil {
		c.deps.OnError(err)
	}
}

// Status reports the latest cycle.
func (c *Collector) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status
}

// Summary boils the log down.
func (c *Collector) Summary() (Summary, error) {
	items, err := c.deps.Store.Read()
	if err != nil {
		return Summary{}, fmt.Errorf("failed to read the DNS check log: %w", err)
	}
	sum := Summary{Differences: []Comparison{}}
	for i, item := range items {
		if i == 0 {
			sum.First = item.At
		}
		sum.Last = item.At
		switch item.Outcome {
		case Match:
			sum.Matches++
		case Differ:
			sum.Differs++
			if item.Overlap {
				sum.Overlapping++
			}
		default:
			sum.CannotCompare++
		}
	}
	sum.Comparisons = sum.Matches + sum.Differs
	for i := len(items) - 1; i >= 0 && len(sum.Differences) < MaxDifferences; i-- {
		if items[i].Outcome == Differ {
			sum.Differences = append(sum.Differences, items[i])
		}
	}
	return sum, nil
}

// ProblemInternetOff is the status phrase when the network settings do not allow the outside resolver to be asked.
const ProblemInternetOff = "internet_off"

// ProblemLANOff is the status phrase when the network settings do not allow the router to be asked.
const ProblemLANOff = "lan_off"
