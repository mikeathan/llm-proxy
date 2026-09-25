package runlane

import (
	"context"
	"time"
)

// Model-residency gate.
//
// The local slot serves exactly one model at a time, so a request for a
// different model normally evicts the running one — killing any run using it.
// The gate is the single decision point for that switch:
//
//   - harmless switch (nothing active, or the same model) -> allowed outright;
//   - otherwise the caller is queued behind whoever uses the model that would
//     be evicted, and is granted once that user releases it.
//
// The gate never evicts on a caller's behalf. Only the operator's explicit
// Promote cancels the blocking run, and even then the waiter is granted only
// after that run unwinds — a model is never swapped out from under a live run.
//
// While the gate holds an entry, the local lane suspends queued starts: the
// local slot serves one model at a time, so any start could change it and race
// the caller. A preempted scheduled run is therefore re-queued but does not
// restart until the gate empties, giving inbound its turn instead of letting the
// run immediately re-take the model.
//
// A granted entry counts as a user of the model it requested, so two inbound
// callers on different models cannot evict each other.

// callerKeyType carries an admitted caller's identity on a run context, so the
// model manager can ask the gate whether an eviction is allowed without the
// gate's types leaking into its API.
type callerKeyType struct{}

var callerKeyCtx callerKeyType

// WithCallerKey stamps ctx with the admitted caller's identity — a lane job key
// for runs, or the queue key for an inbound caller. The lane stamps this on
// every run context it hands out, so downstream model requests carry it.
func WithCallerKey(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, callerKeyCtx, key)
}

// CallerKey returns the identity stamped by WithCallerKey, or "" when none is
// present (a request that was never admitted through the lane).
func CallerKey(ctx context.Context) string {
	key, _ := ctx.Value(callerKeyCtx).(string)
	return key
}

// modelWaiter is an inbound caller that has been admitted to `requested`
// (granted) or is queued behind the user of `active`. Waiters and admitted
// callers share one list so the gate has a single source of "who holds which
// model". Guarded by Scheduler.gateMu.
type modelWaiter struct {
	key       string
	label     string
	requested string // model the caller wants served
	active    string // model that would be evicted to serve it
	since     time.Time
	granted   bool
	promoted  bool          // operator asked to serve this entry early
	grant     chan struct{} // closed when a queued caller may proceed
	dropped   chan struct{} // closed when a queued caller is cancelled
}

// holder is this caller as a read-model holder: an admitted caller is using the
// model it requested, which is what makes it a blocker for another model.
func (w *modelWaiter) holder() Holder {
	return Holder{Key: w.key, Kind: KindInbound, Label: w.label, Model: w.requested, Since: w.since}
}

// entry is this caller as a queued row at a 1-based position.
func (w *modelWaiter) entry(position int) Entry {
	return entryFrom(Job{Key: w.key, Kind: KindInbound, Label: w.label, Model: w.requested}, LaneLocal, position, w.since)
}

// ModelHolder returns the admitted run currently using `model`.
//
// s.order and s.lanes are written once by New and never mutated afterwards, so
// they are read without s.mu (which must not be taken here: lane code takes
// lane.mu -> s.mu, so the reverse order would deadlock).
func (s *Scheduler) ModelHolder(model string) (Holder, bool) {
	if model == "" {
		return Holder{}, false
	}
	for _, key := range s.order {
		if h, ok := s.lanes[key].holderOf(model); ok {
			return h, true
		}
	}
	return Holder{}, false
}

// CheckModelSwitch decides whether claim.Key may switch the served local model
// from claim.Active to claim.Requested. It is refused only when the switch would
// evict a model someone is using — a caller re-requesting the model it already
// holds is always allowed.
func (s *Scheduler) CheckModelSwitch(claim ModelClaim) ResidencyResult {
	s.gateMu.Lock()
	defer s.gateMu.Unlock()
	return s.checkSwitchLocked(claim)
}

// checkSwitchLocked is CheckModelSwitch for callers already holding gateMu.
func (s *Scheduler) checkSwitchLocked(claim ModelClaim) ResidencyResult {
	if claim.Active == "" || claim.Requested == "" || claim.Active == claim.Requested {
		return ResidencyResult{Allowed: true}
	}
	if holder, ok := s.ModelHolder(claim.Active); ok && holder.Key != claim.Key {
		blocked := holder
		return ResidencyResult{BlockedBy: &blocked}
	}
	for _, w := range s.waiters {
		if w.granted && w.key != claim.Key && w.requested == claim.Active {
			blocked := w.holder()
			return ResidencyResult{BlockedBy: &blocked}
		}
	}
	return ResidencyResult{Allowed: true}
}

// WaitForModel admits claim.Key to serve claim.Requested, which requires
// switching the served local model away from claim.Active.
//
// It returns immediately when the switch is harmless. Otherwise the caller is
// queued (visible in Snapshot.ModelWaiters with its position) and this returns
// once the blocking user releases the model, an operator promotes the entry, or
// ctx ends (client disconnect or the declared wait expiring) — the latter as
// ErrModelWaitCancelled / ctx.Err(). A closed scheduler returns ErrClosed.
//
// The returned release must be called exactly once, when the caller has stopped
// using the model; it frees the caller's claim and grants the next waiter.
func (s *Scheduler) WaitForModel(ctx context.Context, claim ModelClaim) (func(), error) {
	s.gateMu.Lock()
	if s.gateClosed {
		s.gateMu.Unlock()
		return nil, ErrClosed
	}
	w := &modelWaiter{
		key: claim.Key, label: claim.Label, requested: claim.Requested, active: claim.Active,
		since: time.Now(), grant: make(chan struct{}),
	}
	allowed := s.checkSwitchLocked(claim).Allowed
	if allowed {
		// Register the claim up front: the caller is about to own `Requested`,
		// and a concurrent different-model caller must not evict it in the gap.
		w.granted = true
		s.waiters = append(s.waiters, w)
		s.publishGateHoldLocked()
		s.gateMu.Unlock()
		return func() { s.releaseWaiter(w) }, nil
	}

	w.dropped = make(chan struct{})
	s.waiters = append(s.waiters, w)
	// Hold queued local starts while this caller waits: any start could change
	// the served model and re-take what it is waiting for.
	s.publishGateHoldLocked()
	s.gateMu.Unlock()

	select {
	case <-w.grant:
		return func() { s.releaseWaiter(w) }, nil
	case <-w.dropped:
		// Distinguish "your entry was dropped" from "the scheduler shut down".
		if s.isGateClosed() {
			return nil, ErrClosed
		}
		return nil, ErrModelWaitCancelled
	case <-ctx.Done():
		s.dropWaiter(w)
		return nil, ctx.Err()
	}
}

// isGateClosed reports whether the scheduler has closed its model gate.
func (s *Scheduler) isGateClosed() bool {
	s.gateMu.Lock()
	defer s.gateMu.Unlock()
	return s.gateClosed
}

// CancelModelWait drops a queued caller (operator dismiss, or the client's
// explicit cancel). Reports whether a queued entry matched.
func (s *Scheduler) CancelModelWait(key string) bool {
	s.gateMu.Lock()
	w := s.findWaiterLocked(key)
	if w == nil {
		s.gateMu.Unlock()
		return false
	}
	s.removeWaiterLocked(w)
	s.grantReadyLocked()
	s.publishGateHoldLocked()
	s.gateMu.Unlock()

	close(w.dropped)
	return true
}

// PromoteModelWait serves a queued caller early by cancelling the run that holds
// the model it waits for. The waiter is granted once that run unwinds — never
// before, so nothing is evicted out from under a live run — and the held lane
// keeps it from being re-taken in the meantime. A preempted scheduled run is
// re-queued and restarts when the gate empties. Reports ErrUnknownWaiter when no
// queued entry matches, and is idempotent.
func (s *Scheduler) PromoteModelWait(key string) error {
	s.gateMu.Lock()
	w := s.findWaiterLocked(key)
	if w == nil {
		s.gateMu.Unlock()
		return ErrUnknownWaiter
	}
	if w.promoted {
		s.gateMu.Unlock()
		return nil
	}
	w.promoted = true
	blocked, _ := s.ModelHolder(w.active)
	s.gateMu.Unlock()

	if blocked.Key != "" {
		s.preemptKey(blocked.Key)
	}
	s.pumpModelGate()
	return nil
}

// PreemptModelHolder cancels the run holding `model`, for the inbound-preempt
// policy (a host setting, off by default). The caller is still granted only
// once that run unwinds. Reports whether a holder was found.
func (s *Scheduler) PreemptModelHolder(model string) bool {
	holder, ok := s.ModelHolder(model)
	if !ok {
		return false
	}
	return s.preemptKey(holder.Key)
}

// preemptKey cancels the run with `key` in whichever lane holds it, reporting
// whether it matched.
func (s *Scheduler) preemptKey(key string) bool {
	for _, laneKey := range s.order {
		if s.lanes[laneKey].preemptJob(key) {
			return true
		}
	}
	return false
}

// ModelHolders reports inbound callers the gate has admitted — they are using
// the local model right now, so they surface as holders (the operator UI shows
// what is being served, not only what is waiting).
func (s *Scheduler) ModelHolders() []Holder {
	s.gateMu.Lock()
	defer s.gateMu.Unlock()
	out := make([]Holder, 0, len(s.waiters))
	for _, w := range s.waiters {
		if !w.granted {
			continue
		}
		out = append(out, w.holder())
	}
	return out
}

// ModelWaiters reports inbound callers still waiting for the local model, at
// contiguous 1-based positions in their own queue — granted callers are using
// the model and must not occupy a queue slot.
func (s *Scheduler) ModelWaiters() []Entry {
	s.gateMu.Lock()
	defer s.gateMu.Unlock()
	out := make([]Entry, 0, len(s.waiters))
	position := 0
	for _, w := range s.waiters {
		if w.granted {
			continue
		}
		position++
		out = append(out, w.entry(position))
	}
	return out
}

// ModelWaiterCount reports how many inbound callers are waiting for the local
// model. It is the queue-depth check's cheap alternative to building the full
// ModelWaiters read model on every admission.
func (s *Scheduler) ModelWaiterCount() int {
	s.gateMu.Lock()
	defer s.gateMu.Unlock()
	n := 0
	for _, w := range s.waiters {
		if !w.granted {
			n++
		}
	}
	return n
}

// pumpModelGate grants every queued caller whose switch is now harmless. Called
// whenever a run releases a slot (a model may have been freed) and after a
// cancel. Callers must NOT hold a lane mutex — this reads lane state.
func (s *Scheduler) pumpModelGate() {
	s.gateMu.Lock()
	defer s.gateMu.Unlock()
	s.grantReadyLocked()
}

// grantReadyLocked admits every waiting caller that is now unblocked. Assumes
// gateMu is held.
func (s *Scheduler) grantReadyLocked() {
	for _, w := range s.waiters {
		if w.granted {
			continue
		}
		if !s.checkSwitchLocked(ModelClaim{Active: w.active, Requested: w.requested, Key: w.key}).Allowed {
			continue
		}
		w.granted = true
		close(w.grant)
	}
}

// releaseWaiter frees a granted caller's claim and admits the next waiter.
func (s *Scheduler) releaseWaiter(w *modelWaiter) {
	s.gateMu.Lock()
	defer s.gateMu.Unlock()
	if !s.removeWaiterLocked(w) {
		return
	}
	s.grantReadyLocked()
	s.publishGateHoldLocked()
}

// dropWaiter removes a waiter that gave up (ctx ended). Assumes nothing about
// the lock.
func (s *Scheduler) dropWaiter(w *modelWaiter) {
	s.gateMu.Lock()
	defer s.gateMu.Unlock()
	if s.removeWaiterLocked(w) {
		s.publishGateHoldLocked()
	}
}

// publishGateHoldLocked mirrors "the gate has entries" onto the local lane so
// queued starts suspend while an inbound caller waits for or holds the local
// model. Lifting the hold resumes that lane. Assumes gateMu is held; the lane
// mutex is taken after it (the established order).
func (s *Scheduler) publishGateHoldLocked() {
	l := s.lanes[LaneLocal]
	held := len(s.waiters) > 0
	if l.holdStarts.Swap(held) && !held {
		l.mu.Lock()
		l.pumpLocked()
		l.mu.Unlock()
	}
}

func (s *Scheduler) findWaiterLocked(key string) *modelWaiter {
	for _, w := range s.waiters {
		if !w.granted && w.key == key {
			return w
		}
	}
	return nil
}

func (s *Scheduler) removeWaiterLocked(w *modelWaiter) bool {
	for i, cand := range s.waiters {
		if cand == w {
			s.waiters = append(s.waiters[:i], s.waiters[i+1:]...)
			return true
		}
	}
	return false
}

// closeModelGate unblocks every waiter with ErrClosed and drops all claims.
func (s *Scheduler) closeModelGate() {
	s.gateMu.Lock()
	defer s.gateMu.Unlock()
	s.gateClosed = true
	for _, w := range s.waiters {
		if !w.granted {
			close(w.dropped)
		}
	}
	s.waiters = nil
	s.publishGateHoldLocked()
}
