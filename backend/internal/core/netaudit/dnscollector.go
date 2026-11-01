package netaudit

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sync"
	"time"

	"llm-proxy/internal/core/netaudit/dnscheck"
	"llm-proxy/models"
)

// DNSLookup asks one resolver for a name's IPv4 addresses: *dnscheck.Router and dnscheck.Outside.
type DNSLookup interface {
	LookupA(ctx context.Context, name string) ([]netip.Addr, error)
}

// DNSStore keeps the log of comparisons. A missing file is an empty log.
type DNSStore interface {
	Read() ([]models.DNSComparison, error)
	Write([]models.DNSComparison) error
}

// DNSSettings are the operator's choices, read for every cycle so a change applies to the next one.
type DNSSettings struct {
	Enabled bool
	Every   time.Duration
	Names   []string
}

// DNSCollectorDeps are what the collector needs; the application wires them in one place.
type DNSCollectorDeps struct {
	Settings func() DNSSettings
	Router   DNSLookup
	Outside  DNSLookup
	Store    DNSStore
	Now      func() time.Time
	// OnError is told about trouble, once per distinct message until the trouble ends.
	OnError func(error)
}

// DNSCollector compares, for a few names, what the router's DNS answers with what an outside resolver answers, and logs each
// comparison. It raises no finding: a rule is written only after a week of real comparisons shows what an honest network does.
type DNSCollector struct {
	deps DNSCollectorDeps

	mu      sync.Mutex
	status  DNSStatus
	trouble string // the store trouble reported last, so a repeating failure is reported once
}

// DNSStatus says when the collector last ran and what, if anything, stopped it.
type DNSStatus struct {
	LastRun time.Time `json:"last_run,omitempty"`
	// Problem is a fixed phrase for why the latest cycle could not compare, or empty.
	Problem string `json:"problem,omitempty"`
}

// DNSSummary is the log boiled down to what is needed to judge whether a rule would be quiet.
type DNSSummary struct {
	Comparisons   int `json:"comparisons"`
	Matches       int `json:"matches"`
	Differs       int `json:"differs"`
	CannotCompare int `json:"cannot_compare"`
	// Overlapping counts the differing comparisons whose answers still shared an address (a partial or rotated answer).
	Overlapping int       `json:"overlapping"`
	First       time.Time `json:"first,omitempty"`
	Last        time.Time `json:"last,omitempty"`
	// Differences are the newest differing comparisons, newest first.
	Differences []models.DNSComparison `json:"differences"`
}

const (
	// MaxDNSComparisons bounds the log; the oldest comparisons go first.
	MaxDNSComparisons = 2000
	// MaxDNSDifferences bounds the differences a summary lists.
	MaxDNSDifferences = 20
)

func NewDNSCollector(deps DNSCollectorDeps) *DNSCollector {
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &DNSCollector{deps: deps}
}

// Run does a cycle every Every while enabled, and stops when ctx ends.
func (c *DNSCollector) Run(ctx context.Context) {
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
func (c *DNSCollector) Tick(ctx context.Context) {
	s := c.deps.Settings()
	if !s.Enabled || len(s.Names) == 0 {
		return
	}
	now := c.deps.Now()
	var rows []models.DNSComparison
	problem := ""
	for _, name := range s.Names {
		row, err := c.compare(ctx, name, now)
		if errors.Is(err, dnscheck.ErrInternetOff) {
			problem = DNSProblemInternetOff
			break
		}
		rows = append(rows, row)
	}
	c.mu.Lock()
	c.status = DNSStatus{LastRun: now, Problem: problem}
	c.mu.Unlock()
	if len(rows) > 0 {
		c.record(rows)
	}
}

// compare asks the router first: with no answer from it there is nothing to compare, so the outside resolver is not asked.
func (c *DNSCollector) compare(ctx context.Context, name string, at time.Time) (models.DNSComparison, error) {
	row := models.DNSComparison{At: at, Name: name, Outcome: models.DNSCannotCompare}
	fromRouter, err := c.deps.Router.LookupA(ctx, name)
	if err != nil {
		row.Reason = models.DNSReasonRouterSilent
		if errors.Is(err, dnscheck.ErrNoRouter) {
			row.Reason = models.DNSReasonNoRouter
		}
		return row, nil
	}
	fromOutside, err := c.deps.Outside.LookupA(ctx, name)
	if err != nil {
		row.Reason = models.DNSReasonOutsideFails
		return row, err
	}
	verdict := dnscheck.Compare(fromRouter, fromOutside)
	row.Outcome, row.Overlap = models.DNSDiffer, verdict.Overlap
	if verdict.Same {
		row.Outcome = models.DNSMatch
	}
	row.Router, row.Outside = addressText(fromRouter), addressText(fromOutside)
	return row, nil
}

func addressText(in []netip.Addr) []string {
	out := make([]string, 0, len(in))
	for _, a := range in {
		out = append(out, a.String())
	}
	return out
}

// record adds rows to the log, dropping the oldest beyond the bound. A log that cannot be read is left alone.
func (c *DNSCollector) record(rows []models.DNSComparison) {
	items, err := c.deps.Store.Read()
	if err != nil {
		c.report(fmt.Errorf("dns check: failed to read the log: %w", err))
		return
	}
	items = append(items, rows...)
	if len(items) > MaxDNSComparisons {
		items = items[len(items)-MaxDNSComparisons:]
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
func (c *DNSCollector) report(err error) {
	c.mu.Lock()
	again := c.trouble == err.Error()
	c.trouble = err.Error()
	c.mu.Unlock()
	if !again && c.deps.OnError != nil {
		c.deps.OnError(err)
	}
}

// Status reports the latest cycle.
func (c *DNSCollector) Status() DNSStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status
}

// Summary boils the log down.
func (c *DNSCollector) Summary() (DNSSummary, error) {
	items, err := c.deps.Store.Read()
	if err != nil {
		return DNSSummary{}, fmt.Errorf("failed to read the DNS check log: %w", err)
	}
	sum := DNSSummary{Differences: []models.DNSComparison{}}
	for i, item := range items {
		if i == 0 {
			sum.First = item.At
		}
		sum.Last = item.At
		switch item.Outcome {
		case models.DNSMatch:
			sum.Matches++
		case models.DNSDiffer:
			sum.Differs++
			if item.Overlap {
				sum.Overlapping++
			}
		default:
			sum.CannotCompare++
		}
	}
	sum.Comparisons = sum.Matches + sum.Differs
	for i := len(items) - 1; i >= 0 && len(sum.Differences) < MaxDNSDifferences; i-- {
		if items[i].Outcome == models.DNSDiffer {
			sum.Differences = append(sum.Differences, items[i])
		}
	}
	return sum, nil
}

// DNSProblemInternetOff is the status phrase when the network settings do not allow the outside resolver to be asked.
const DNSProblemInternetOff = "internet_off"
