// Package runlane provides the global run scheduler: two workload-class lanes
// (local, cloud) that bound agent-run concurrency, queue overlapping scheduled
// automations instead of dropping them, and let interactive claims preempt a
// running automation in their own lane.
package runlane

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// preemptGrace bounds how long an interactive claim waits for a preempted
// automation to stop. Package-level so tests can shorten it.
var preemptGrace = 10 * time.Second

// Scheduler owns the fixed set of workload-class lanes. Create with New, tether
// to the application lifecycle with Start/Close.
type Scheduler struct {
	mu         sync.Mutex
	started    atomic.Bool
	rootCtx    context.Context
	rootCancel context.CancelFunc
	preempt    atomic.Bool
	lanes      map[LaneKey]*lane
	order      []LaneKey

	// Model-residency gate (modelgate.go). gateMu guards waiters/gateClosed and
	// is taken BEFORE a lane mutex, never after.
	gateMu     sync.Mutex
	gateClosed bool
	waiters    []*modelWaiter
}

// New builds the two class lanes with their concurrency limits.
func New(l Limits, preempt bool) *Scheduler {
	s := &Scheduler{order: []LaneKey{LaneLocal, LaneCloud}}
	s.lanes = map[LaneKey]*lane{
		LaneLocal: {s: s, limit: max(1, l.Local)},
		LaneCloud: {s: s, limit: max(1, l.Cloud)},
	}
	s.preempt.Store(preempt)
	return s
}

// Start tethers the scheduler to the application root context. Idempotent.
// The lane reads rootCtx under s.mu, so it is published under the same lock and
// started is only flipped once rootCtx is set — a Submit that observes started
// therefore always finds a usable root context.
func (s *Scheduler) Start(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started.Load() {
		return
	}
	s.rootCtx, s.rootCancel = context.WithCancel(ctx)
	s.started.Store(true)
}

// Submit admits a job to its lane: started immediately when a slot is free,
// otherwise queued FIFO behind pending work. A key already present in the queue
// yields ErrAlreadyQueued plus that entry's position. Submit fails with
// ErrNotStarted until Start has tethered the scheduler to the app lifecycle.
func (s *Scheduler) Submit(j Job) (Submission, error) {
	l, ok := s.lanes[j.LaneKey]
	if !ok {
		return Submission{}, ErrUnknownLane
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if !s.started.Load() {
		return Submission{}, ErrNotStarted
	}
	if l.closed {
		return Submission{}, ErrClosed
	}
	if existing := l.findQueuedLocked(j.Key); existing != nil {
		return Submission{Disposition: DispositionQueued, Position: l.positionLocked(existing)}, ErrAlreadyQueued
	}

	qj := &queuedJob{job: j, queuedAt: time.Now()}
	l.queue = append(l.queue, qj)
	l.pumpLocked()
	if pos := l.positionLocked(qj); pos > 0 {
		return Submission{Disposition: DispositionQueued, Position: pos}, nil
	}
	return Submission{Disposition: DispositionStarted}, nil
}

// CancelQueued drops a queued (not yet running) entry, reporting whether it
// matched. A running automation is never cancelled here: "cancel queued" must
// not be able to turn a live run into a failure — that is the run's own cancel
// path (StopAutomation).
func (s *Scheduler) CancelQueued(key string) bool {
	for _, laneKey := range s.order {
		if s.lanes[laneKey].removeQueued(key) {
			return true
		}
	}
	return false
}

// ClaimInteractive grants the caller a lane slot, preempting a running
// automation in the same lane when none is free. model is the local model the
// chat will use; it is recorded on the holder so the model-residency gate can
// refuse evicting it out from under a live chat. The returned runCtx derives
// from the caller's ctx so its cancellation still reaches the run; release must
// be called exactly once to free the slot. A preempted automation that ignores
// cancellation yields ErrPreemptTimeout after preemptGrace — by then it is
// already cancelled and is not restored (a scheduled run re-queues at the front
// when it finally unwinds).
func (s *Scheduler) ClaimInteractive(ctx context.Context, laneKey LaneKey, workspaceID, model string) (context.Context, func(), error) {
	l, ok := s.lanes[laneKey]
	if !ok {
		return nil, nil, ErrUnknownLane
	}
	w := &waiter{ctx: ctx, workspaceID: workspaceID, model: model, grant: make(chan grantResult, 1)}

	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil, nil, ErrClosed
	}
	l.waiters = append(l.waiters, w)
	l.pumpLocked()
	var grace <-chan time.Time
	if l.preemptForLocked(w) {
		timer := time.NewTimer(preemptGrace)
		defer timer.Stop()
		grace = timer.C
	}
	l.mu.Unlock()

	select {
	case g := <-w.grant:
		if g.err != nil {
			return nil, nil, g.err
		}
		return g.runCtx, g.release, nil
	case <-ctx.Done():
		if g, ok := l.detach(w); ok {
			g.release()
		}
		return nil, nil, ctx.Err()
	case <-grace:
		if g, ok := l.detach(w); ok {
			g.release()
		}
		return nil, nil, ErrPreemptTimeout
	}
}

// SetLimits resizes the lanes live; lowering a limit never kills a running job.
func (s *Scheduler) SetLimits(l Limits) {
	s.lanes[LaneLocal].setLimit(max(1, l.Local))
	s.lanes[LaneCloud].setLimit(max(1, l.Cloud))
}

// SetPreempt toggles whether interactive claims may cancel running automations.
func (s *Scheduler) SetPreempt(enabled bool) { s.preempt.Store(enabled) }

// Snapshot returns a read model of every lane as fresh copies.
func (s *Scheduler) Snapshot() Snapshot {
	out := Snapshot{
		Lanes:        make([]LaneState, 0, len(s.order)),
		ModelHolders: s.ModelHolders(),
		ModelWaiters: s.ModelWaiters(),
	}
	for _, key := range s.order {
		out.Lanes = append(out.Lanes, s.lanes[key].snapshot(key))
	}
	return out
}

// Close rejects new work, cancels all running jobs, unblocks waiters with
// ErrClosed, and waits (bounded by ctx) for the slots to drain.
func (s *Scheduler) Close(ctx context.Context) error {
	s.mu.Lock()
	if s.rootCancel != nil {
		s.rootCancel()
	}
	s.mu.Unlock()

	for _, key := range s.order {
		s.lanes[key].close()
	}
	s.closeModelGate()
	for _, key := range s.order {
		select {
		case <-s.lanes[key].drained:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
