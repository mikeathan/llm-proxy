package netaudit

import (
	"context"
	"slices"
	"sync"
)

type collectorKey struct{}

// Collector gathers the structured results of every scan one run made. The report is built from these, never from what
// the model says it saw.
type Collector struct {
	mu      sync.Mutex
	results []Result
	summary string
}

// SetSummary records the short, code-written summary of the run (see SummaryOf) for delivery through a connector.
func (c *Collector) SetSummary(s string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.summary = s
}

// Summary returns what SetSummary recorded; empty when the run made no scan.
func (c *Collector) Summary() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.summary
}

// WithCollector returns a context that carries a new Collector, and the Collector.
func WithCollector(ctx context.Context) (context.Context, *Collector) {
	c := &Collector{}
	return context.WithValue(ctx, collectorKey{}, c), c
}

// CollectorFrom returns the context's Collector, or nil when the run is not collecting.
func CollectorFrom(ctx context.Context) *Collector {
	c, _ := ctx.Value(collectorKey{}).(*Collector)
	return c
}

// Add records one scan's result.
func (c *Collector) Add(res Result) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.results = append(c.results, res)
}

// Merged combines every scan of the run into one result: the hosts by address (a later scan of the same address
// replaces an earlier one), the engines used, and the warnings. ok is false when nothing was scanned.
func (c *Collector) Merged() (Result, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.results) == 0 {
		return Result{}, false
	}
	var merged Result
	var engines []string
	hosts := map[string]int{}
	for _, res := range c.results {
		if !slices.Contains(engines, res.Engine) {
			engines = append(engines, res.Engine)
		}
		for _, w := range res.Warnings {
			if !slices.Contains(merged.Warnings, w) {
				merged.Warnings = append(merged.Warnings, w)
			}
		}
		for _, h := range res.Hosts {
			if i, seen := hosts[h.Addr.String()]; seen {
				merged.Hosts[i] = h
				continue
			}
			hosts[h.Addr.String()] = len(merged.Hosts)
			merged.Hosts = append(merged.Hosts, h)
		}
	}
	merged.Engine = joinEngines(engines)
	merged.Findings = FindingsOf(merged.Hosts)
	return merged, true
}

func joinEngines(engines []string) string {
	out := ""
	for i, e := range engines {
		if i > 0 {
			out += ", "
		}
		out += e
	}
	return out
}
