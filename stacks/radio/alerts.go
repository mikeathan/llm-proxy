package radio

import (
	"context"
	"encoding/json"
	"math"
	"time"

	"llm-proxy-stacks/sdk"
)

// The alerts the stack publishes. The core owns the words (a fixed table keyed by these ids) and shows them on the bell only (ADR-0056), so
// nothing the stack kept can put its own text in front of the operator.
const (
	// AlertWatchEnding says a long watch has a fifth of its time left, so it can be started again before there is a gap.
	AlertWatchEnding = "radio_watch_ending"
	// AlertWatchStopped says a watch stopped before its time (a restart, a crash, an adapter that went away, a session that was ended).
	AlertWatchStopped = "radio_watch_stopped"

	// minWarnedWatch is the shortest watch that gets an ending reminder: a short one is renewed by hand anyway.
	minWarnedWatch = 12 * time.Hour
)

// AlertIDs lists every alert id the stack can publish, so a test in the service can hold the core's table of words to it.
func AlertIDs() []string { return []string{AlertWatchEnding, AlertWatchStopped} }

// hoursLeft is the whole hours of a span, at least one.
func hoursLeft(d time.Duration) int { return max(1, int(math.Ceil(d.Hours()))) }

// tell publishes one alert; a host with no Events port, or a failure to publish, is not worth stopping a run for.
func (s *Stack) tell(h stacksdk.Host, id string, hours int) {
	if h.Events == nil {
		return
	}
	data, err := json.Marshal(stacksdk.Alert{ID: id, Hours: hours})
	if err != nil {
		return
	}
	_ = h.Events.Publish(context.Background(), stacksdk.Event{Type: stacksdk.EventAlert, Subject: id, Data: data, At: s.now()})
}

// warnIfEnding tells the operator once, when a watch of at least minWarnedWatch has a fifth of its time left.
func (s *Stack) warnIfEnding(r *run) {
	if r.warned {
		return
	}
	st := r.snapshot()
	total, left := st.Until.Sub(st.Started), st.Until.Sub(s.now())
	if total < minWarnedWatch || left <= 0 || left > total/5 {
		return
	}
	r.warned = true
	if h, ok := s.snapshot(); ok {
		s.tell(h, AlertWatchEnding, hoursLeft(left))
	}
}

// announceStopped tells the operator once that a watch stopped before its time, and remembers that it did.
func (s *Stack) announceStopped(store *Store, w watchRecord) {
	left := w.Until.Sub(s.now())
	if left <= 0 || w.Told {
		return
	}
	if h, ok := s.snapshot(); ok {
		s.tell(h, AlertWatchStopped, hoursLeft(left))
	}
	w.Told = true
	saveWatch(store, w, s.now())
}

// announceInterrupted runs when the stack starts: a watch whose record never got an ending (a crash) or ended with the service stopping (a
// restart or a deploy) is told to the operator now, because the run itself could not.
func (s *Stack) announceInterrupted() {
	store, err := s.existingStore()
	if err != nil || store == nil {
		return
	}
	var w watchRecord
	if ok, err := store.GetDoc(context.Background(), watchDoc, &w); err != nil || !ok || (w.Ended != "" && w.Ended != EndedStack) {
		return
	}
	s.announceStopped(store, w)
}
