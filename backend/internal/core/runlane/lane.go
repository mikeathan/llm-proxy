package runlane

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"llm-proxy/internal/platform/safe"
)

type queuedJob struct {
	job      Job
	queuedAt time.Time
}

type runningJob struct {
	job       Job
	since     time.Time
	cancel    context.CancelFunc
	flag      *preemptFlag // automation only
	preempted bool
	released  bool
}

type grantResult struct {
	runCtx  context.Context
	release func()
	err     error
}

type waiter struct {
	ctx         context.Context
	workspaceID string
	model       string
	granted     bool
	grant       chan grantResult
}

// lane is one workload-class bucket: a FIFO queue, its running jobs, and the
// interactive claims waiting for a slot. All fields are guarded by mu, except
// holdStarts (published lock-free by the model gate).
type lane struct {
	s             *Scheduler
	mu            sync.Mutex
	limit         int
	queue         []*queuedJob
	running       []*runningJob
	waiters       []*waiter
	closed        bool
	drained       chan struct{}
	drainedClosed bool
	// holdStarts suspends queued starts while an inbound caller is waiting for
	// (or holding) the local model. The local slot serves one model at a time,
	// so any start could change it and race that caller; holding gives the
	// gate the same priority interactive claims already get over queued work.
	holdStarts atomic.Bool
}

func (l *lane) close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
	for _, w := range l.waiters {
		if !w.granted {
			w.grant <- grantResult{err: ErrClosed}
		}
	}
	l.waiters = nil
	for _, rj := range l.running {
		rj.cancel()
	}
	if l.drained == nil {
		l.drained = make(chan struct{})
	}
	l.signalDrainedLocked()
}

func (l *lane) setLimit(n int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.limit = n
	l.pumpLocked()
}

func (l *lane) removeQueued(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.removeQueuedLocked(key)
}

func (l *lane) removeQueuedLocked(key string) bool {
	for i, qj := range l.queue {
		if qj.job.Key == key {
			l.queue = append(l.queue[:i], l.queue[i+1:]...)
			return true
		}
	}
	return false
}

// pumpLocked grants interactive waiters first, then starts queued automations,
// while slots are free. A closed lane never admits work, and a lane held by the
// model gate starts no queued work until the gate empties. Callers must hold mu.
func (l *lane) pumpLocked() {
	if l.closed {
		return
	}
	for len(l.running) < l.limit {
		if len(l.waiters) > 0 {
			w := l.waiters[0]
			l.waiters = l.waiters[1:]
			l.grantWaiterLocked(w)
			continue
		}
		if l.holdStarts.Load() || len(l.queue) == 0 {
			return
		}
		qj := l.queue[0]
		l.queue = l.queue[1:]
		l.startLocked(qj.job)
	}
}

func (l *lane) grantWaiterLocked(w *waiter) {
	w.granted = true
	chatKey := "chat:" + w.workspaceID
	runCtx, cancel := context.WithCancel(WithCallerKey(w.ctx, chatKey))
	rj := &runningJob{
		job: Job{
			Key:         chatKey,
			WorkspaceID: w.workspaceID,
			Label:       chatKey,
			Kind:        KindInteractive,
			Model:       w.model,
		},
		since:  time.Now(),
		cancel: cancel,
	}
	l.running = append(l.running, rj)
	w.grant <- grantResult{runCtx: runCtx, release: func() { l.release(rj) }}
}

func (l *lane) startLocked(job Job) {
	flag := &preemptFlag{}
	l.s.mu.Lock()
	root := l.s.rootCtx
	l.s.mu.Unlock()
	ctx, cancel := context.WithCancel(WithCallerKey(context.WithValue(root, preemptKey, flag), job.Key))
	rj := &runningJob{job: job, since: time.Now(), cancel: cancel, flag: flag}
	l.running = append(l.running, rj)
	safe.Go("runlane job "+job.Key, func() {
		defer l.finish(rj)
		// The job owns its failure path (the dispatcher records metrics and
		// events); the lane only guards the slot.
		_ = job.Run(ctx)
	})
}

// finish returns an automation slot, re-queueing a preempted scheduled job at
// the front so it restarts as soon as the lane frees.
func (l *lane) finish(rj *runningJob) { l.freeAndPump(rj, true) }

// release returns an interactive claim's slot.
func (l *lane) release(rj *runningJob) { l.freeAndPump(rj, false) }

// freeAndPump frees rj's slot, admits whatever the freed slot allows, and then —
// outside the lane mutex — tells the model gate a run ended: it may have been
// holding the model an inbound caller is waiting for. pumpModelGate reads lane
// state, so it must never run under l.mu.
func (l *lane) freeAndPump(rj *runningJob, requeue bool) {
	l.mu.Lock()
	if requeue && l.shouldRequeueLocked(rj) {
		l.queue = append([]*queuedJob{{job: rj.job, queuedAt: time.Now()}}, l.queue...)
	}
	freed := !rj.released
	l.freeLocked(rj)
	l.pumpLocked()
	l.mu.Unlock()
	if freed {
		l.s.pumpModelGate()
	}
}

// shouldRequeueLocked reports whether a finished run must restart from the
// front of the queue: a preempted scheduled automation (not a user-initiated
// manual run) that is not already waiting, on a lane still admitting work.
func (l *lane) shouldRequeueLocked(rj *runningJob) bool {
	if l.closed || !rj.preempted || rj.job.Kind != KindAutomation || rj.job.Manual {
		return false
	}
	return l.findQueuedLocked(rj.job.Key) == nil
}

func (l *lane) freeLocked(rj *runningJob) {
	if rj.released {
		return
	}
	rj.released = true
	for i, other := range l.running {
		if other == rj {
			l.running = append(l.running[:i], l.running[i+1:]...)
			break
		}
	}
	rj.cancel()
	l.signalDrainedLocked()
}

func (l *lane) signalDrainedLocked() {
	if !l.allDrainedLocked() {
		return
	}
	close(l.drained)
	l.drainedClosed = true
}

// allDrainedLocked reports whether a closed lane has finished every running job
// and has not yet signalled drain completion.
func (l *lane) allDrainedLocked() bool {
	return l.closed && l.drained != nil && !l.drainedClosed && len(l.running) == 0
}

// preemptForLocked preempts a running automation for a waiting claim when
// preemption is enabled and the claim was not already granted. It reports
// whether a preemption was started (so the caller arms the grace timer).
func (l *lane) preemptForLocked(w *waiter) bool {
	if w.granted || !l.s.preempt.Load() {
		return false
	}
	return l.preemptOneLocked()
}

func (l *lane) preemptOneLocked() bool {
	for _, rj := range l.running {
		if rj.job.Kind == KindAutomation && !rj.preempted {
			rj.preempted = true
			if rj.flag != nil {
				rj.flag.v.Store(true)
			}
			rj.cancel()
			return true
		}
	}
	return false
}

// detach removes a still-waiting claim; when the claim was granted in the race
// window it reports the grant so the caller can hand the slot back.
func (l *lane) detach(w *waiter) (grantResult, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i, other := range l.waiters {
		if other == w {
			l.waiters = append(l.waiters[:i], l.waiters[i+1:]...)
			return grantResult{}, false
		}
	}
	if w.granted {
		return <-w.grant, true
	}
	return grantResult{}, false
}

func (l *lane) findQueuedLocked(key string) *queuedJob {
	for _, qj := range l.queue {
		if qj.job.Key == key {
			return qj
		}
	}
	return nil
}

func (l *lane) positionLocked(qj *queuedJob) int {
	for i, e := range l.queue {
		if e == qj {
			return i + 1
		}
	}
	return 0
}

func (l *lane) snapshot(key LaneKey) LaneState {
	l.mu.Lock()
	defer l.mu.Unlock()
	state := LaneState{
		Lane:    key,
		Limit:   l.limit,
		Running: len(l.running),
		Waiting: len(l.waiters),
		Holders: make([]Holder, 0, len(l.running)),
		Queued:  make([]Entry, 0, len(l.queue)),
	}
	for _, rj := range l.running {
		state.Holders = append(state.Holders, holderFrom(rj))
	}
	for i, qj := range l.queue {
		state.Queued = append(state.Queued, entryFrom(qj.job, key, i+1, qj.queuedAt))
	}
	return state
}

// holderFrom projects a running job into the read model.
func holderFrom(rj *runningJob) Holder {
	return Holder{
		Key: rj.job.Key, Kind: rj.job.Kind, WorkspaceID: rj.job.WorkspaceID,
		Automation: rj.job.Automation, Label: rj.job.Label, Model: rj.job.Model,
		Since: rj.since,
	}
}

// entryFrom projects a queued job into the read model at a 1-based position.
func entryFrom(job Job, lane LaneKey, position int, queuedAt time.Time) Entry {
	return Entry{
		Key: job.Key, Kind: job.Kind, Lane: lane, WorkspaceID: job.WorkspaceID,
		Automation: job.Automation, Label: job.Label, Model: job.Model,
		Position: position, QueuedAt: queuedAt,
	}
}

// holderOf reports the running job in this lane that is using `model`.
func (l *lane) holderOf(model string) (Holder, bool) {
	if model == "" {
		return Holder{}, false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, rj := range l.running {
		if rj.job.Model == model {
			return holderFrom(rj), true
		}
	}
	return Holder{}, false
}

// preemptJob cancels the running job with `key` when this lane holds it,
// reporting whether a preemption was started. Used by the model gate's operator
// "serve now": the blocked run is cancelled, and the gate — not this lane —
// decides who gets the model afterwards.
func (l *lane) preemptJob(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, rj := range l.running {
		if rj.job.Key != key || rj.preempted {
			continue
		}
		rj.preempted = true
		if rj.flag != nil {
			rj.flag.v.Store(true)
		}
		rj.cancel()
		return true
	}
	return false
}

// preemptFlag is the per-run marker a lane sets when it cancels a job to free a
// slot for an interactive claim. startLocked attaches it to the job's context;
// Preempted reads it back.
type preemptFlag struct{ v atomic.Bool }

type preemptKeyType struct{}

var preemptKey preemptKeyType

// Preempted reports whether the run's context was cancelled by a lane preemption.
func Preempted(ctx context.Context) bool {
	if f, ok := ctx.Value(preemptKey).(*preemptFlag); ok {
		return f.v.Load()
	}
	return false
}
