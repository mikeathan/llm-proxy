package radio

import (
	"context"
	"math"
	"time"
)

// A watch is the run the operator asked for, remembered in the radio database so that a restart does not make it vanish: after a deploy or a
// crash the Listening tab says the watch was stopped and offers to resume it (ADR-0056). Only the plan and its end time are kept. The
// permission is not: a session lives in the core's memory, so resuming asks for the passphrase again.
const watchDoc = "watch"

// watchRecord is what is kept of one watch.
type watchRecord struct {
	Adapter string    `json:"adapter"`
	Until   time.Time `json:"until"`
	// Told is set once the operator has been told the watch stopped early, so a second restart does not tell it again.
	Told bool `json:"told,omitempty"`
	// Ended is empty while the watch runs (and stays empty after a crash), else the reason it ended.
	Ended string `json:"ended,omitempty"`
}

// Resume is the offer the Listening tab shows for a watch that was stopped before its time by something other than the operator.
type Resume struct {
	Adapter   string    `json:"adapter"`
	Until     time.Time `json:"until"`
	HoursLeft int       `json:"hours_left"`
	// Why is the reason it stopped, from the fixed list (EndedStack for a restart).
	Why string `json:"why,omitempty"`
}

// resumable says whether a watch that ended for this reason should be offered again: not when the operator stopped it or switched the stack off,
// and not when its time or frame budget ran out, because those were the answer, not an accident.
func resumable(ended string) bool {
	switch ended {
	case "", EndedStack, EndedNoSession, EndedFailing:
		return true
	}
	return false
}

// saveWatch keeps the watch's record; a failure is not worth stopping a run for, so it is dropped (the run still works, only the offer is lost).
func saveWatch(store *Store, rec watchRecord, at time.Time) {
	if store != nil {
		_ = store.PutDoc(context.Background(), watchDoc, rec, at)
	}
}

// endWatch marks the watch as ended for a reason. The first reason stays, except that the operator's own stop always wins: it is how an offer
// to resume is dismissed.
func (s *Stack) endWatch(store *Store, reason string) {
	if store == nil {
		return
	}
	var w watchRecord
	if ok, err := store.GetDoc(context.Background(), watchDoc, &w); err != nil || !ok || (w.Ended != "" && reason != EndedStopped) {
		return
	}
	w.Ended = reason
	saveWatch(store, w, s.now())
	if reason == EndedFailing || reason == EndedNoSession {
		s.announceStopped(store, w) // the run itself saw it end early: say so now, not at the next start
	}
}

// resumeOffer is the offer for the last watch, or nil: nothing was kept, it ended in a way that needs no offer, or its time is up.
func (s *Stack) resumeOffer() *Resume {
	store, err := s.existingStore()
	if err != nil || store == nil {
		return nil
	}
	var w watchRecord
	if ok, err := store.GetDoc(context.Background(), watchDoc, &w); err != nil || !ok || !resumable(w.Ended) {
		return nil
	}
	left := w.Until.Sub(s.now())
	if left <= 0 {
		return nil
	}
	return &Resume{Adapter: w.Adapter, Until: w.Until, HoursLeft: max(1, int(math.Ceil(left.Hours()))), Why: w.Ended}
}
