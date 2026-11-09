package dnscheck

import (
	"slices"
	"time"
)

const (
	// DifferRun is how many informative checks in a row must differ before the stack offers a finding. One difference is a blip
	// (a resolver changed its answer between the two questions); several are a pattern.
	DifferRun = 2
	// Freshness is how old the newest differing check may be and still be offered, so a switched-off check does not keep a finding
	// alive: the nightly audit reads this, so a day is the longest that makes sense.
	Freshness = 24 * time.Hour
)

// Differing is the names whose newest informative checks all differ, with the length of the shortest such run and the start of the
// earliest.
type Differing struct {
	Names []string
	Count int
	Since time.Time
}

// run is the newest unbroken run of differing checks for one name.
type run struct {
	length int
	since  time.Time
	last   time.Time
	done   bool
}

// add takes the next older check of the name into the run. A match, a rotated answer or anything else ends it; a check that could not
// compare is skipped.
func (r *run) add(c Comparison) {
	switch {
	case r.done || c.Outcome == CannotCompare:
	case c.Outcome != Differ || c.Overlap:
		r.done = true
	default:
		if r.length == 0 {
			r.last = c.At
		}
		r.length++
		r.since = c.At
	}
}

// Persistent reads the log for names that the router and the outside resolver disagree on in a run of DifferRun checks or more, ending
// with the newest informative check. Only a difference whose answers share no address counts (a shared address is a rotation); a match
// ends a run; a check that could not compare is skipped, neither extending nor ending it. A run whose newest check is older than
// Freshness at now is not offered.
func Persistent(log []Comparison, now time.Time) Differing {
	runs := map[string]*run{}
	for i := len(log) - 1; i >= 0; i-- { // newest first
		r := runs[log[i].Name]
		if r == nil {
			r = &run{}
			runs[log[i].Name] = r
		}
		r.add(log[i])
	}
	var out Differing
	for name, r := range runs {
		if r.length < DifferRun || now.Sub(r.last) > Freshness {
			continue
		}
		out.Names = append(out.Names, name)
		if out.Count == 0 || r.length < out.Count {
			out.Count = r.length
		}
		if out.Since.IsZero() || r.since.Before(out.Since) {
			out.Since = r.since
		}
	}
	slices.Sort(out.Names)
	return out
}
