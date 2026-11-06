package netaudit

import (
	"sync"
	"time"
)

// RunHealth remembers, for each optional step of the nightly run, when it last worked and whether its latest run failed. The monitor
// watch reads it, so a step that fails every night shows as stopped on the Modules tab and is told once, where before it was one
// line in a report nobody reads. It holds a few values in memory and probes nothing; after a restart each step gets a fresh start.
type RunHealth struct {
	mu   sync.Mutex
	runs map[string]runOutcome
	now  func() time.Time
}

type runOutcome struct {
	lastGood time.Time
	failed   bool
}

// NewRunHealth builds an empty record.
func NewRunHealth() *RunHealth { return &RunHealth{runs: map[string]runOutcome{}, now: time.Now} }

// Record notes the outcome of one run of a step: nil is a run that worked. A nil receiver records nothing, so a reporter without
// a record needs no check.
func (h *RunHealth) Record(id string, err error) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	o := h.runs[id]
	if err == nil {
		o = runOutcome{lastGood: h.now()}
	} else {
		o.failed = true
	}
	h.runs[id] = o
}

// Reading is what the monitor watch reads for a step: whether the operator has it on, when it last worked, how long it may go
// between runs, and whether its latest run failed. A step that has not run yet is neither good nor failed.
func (h *RunHealth) Reading(id string, on bool, limit time.Duration) MonitorReading {
	r := MonitorReading{On: on, Limit: limit}
	if h == nil {
		return r
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	o := h.runs[id]
	r.LastGood, r.Failed = o.lastGood, o.failed
	return r
}
