package runlane

import (
	"context"
	"errors"
	"testing"
	"time"
)

// heldJob starts a local run that keeps its lane slot — and its model — busy
// until the returned channel is closed.
func heldJob(t *testing.T, s *Scheduler, key, model string) chan struct{} {
	t.Helper()
	started := make(chan string, 1)
	proceed := make(chan struct{})
	job := autoJob(key, started, proceed, nil)
	job.Model = model
	if _, err := s.Submit(job); err != nil {
		t.Fatal(err)
	}
	waitStarted(t, started, key)
	return proceed
}

// modelClaim is one caller's residency request: what it wants, which served
// model that would evict, and who it is. (scheduler_test.go's `claim` helper is
// the interactive-slot one.)
func modelClaim(active, requested, key string) ModelClaim {
	return ModelClaim{Active: active, Requested: requested, Key: key, Label: requested}
}

// queuedWaiters reports the callers waiting for the local model, once at least
// one of them is `key` — the gate queues asynchronously, so tests observe the
// read model rather than assume an ordering.
func queuedWaiters(s *Scheduler, key string) func() bool {
	return func() bool {
		for _, w := range s.Snapshot().ModelWaiters {
			if w.Key == key {
				return true
			}
		}
		return false
	}
}

func TestModelGate(t *testing.T) {
	t.Run("allows a switch when nothing would be evicted", func(t *testing.T) {
		s := newStarted(t, 1, 1, false)
		if res := s.CheckModelSwitch(modelClaim("", "B", "caller")); !res.Allowed {
			t.Fatalf("switch with nothing active = %+v, want allowed", res)
		}
		if res := s.CheckModelSwitch(modelClaim("A", "A", "caller")); !res.Allowed {
			t.Fatalf("re-requesting the active model = %+v, want allowed", res)
		}
	})

	t.Run("refuses to evict a model a run is using", func(t *testing.T) {
		s := newStarted(t, 1, 1, false)
		proceed := heldJob(t, s, "job-a", "A")
		defer close(proceed)

		res := s.CheckModelSwitch(modelClaim("A", "B", "caller"))
		if res.Allowed {
			t.Fatal("switch must be refused while a run uses A")
		}
		if res.BlockedBy == nil || res.BlockedBy.Key != "job-a" || res.BlockedBy.Model != "A" {
			t.Fatalf("BlockedBy = %+v, want the run holding A", res.BlockedBy)
		}
		// The holder itself may keep using, and re-request, its own model.
		if res := s.CheckModelSwitch(modelClaim("A", "B", "job-a")); !res.Allowed {
			t.Fatal("the holder must be allowed to switch away from its own model")
		}
	})

	t.Run("queues behind the holder and grants once it releases", func(t *testing.T) {
		s := newStarted(t, 1, 1, false)
		proceed := heldJob(t, s, "job-a", "A")

		done := make(chan error, 1)
		go func() {
			release, err := s.WaitForModel(t.Context(), modelClaim("A", "B", "inbound-1"))
			if err == nil {
				release()
			}
			done <- err
		}()

		// The queued row is what the operator UI renders: identity, model, kind
		// and the 1-based position in the inbound queue.
		eventually(t, func() bool {
			waiters := s.Snapshot().ModelWaiters
			return len(waiters) == 1 && waiters[0].Key == "inbound-1" &&
				waiters[0].Kind == KindInbound && waiters[0].Model == "B" && waiters[0].Position == 1
		}, "the caller was not queued behind the holder")

		close(proceed) // the run finishes; the model is free
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("WaitForModel = %v, want nil once the holder released", err)
			}
		case <-time.After(testWait):
			t.Fatal("waiter was not granted after the holder released the model")
		}
		if got := s.Snapshot().ModelWaiters; len(got) != 0 {
			t.Fatalf("ModelWaiters = %+v, want empty after the grant", got)
		}
	})

	t.Run("a cancel drops a queued caller", func(t *testing.T) {
		s := newStarted(t, 1, 1, false)
		proceed := heldJob(t, s, "job-a", "A")
		defer close(proceed)

		done := make(chan error, 1)
		go func() {
			_, err := s.WaitForModel(t.Context(), modelClaim("A", "B", "inbound-1"))
			done <- err
		}()
		eventually(t, queuedWaiters(s, "inbound-1"), "the caller never queued")

		if !s.CancelModelWait("inbound-1") {
			t.Fatal("CancelModelWait reported no match for the queued caller")
		}
		if s.CancelModelWait("inbound-1") {
			t.Fatal("CancelModelWait matched the same entry twice")
		}
		select {
		case err := <-done:
			if !errors.Is(err, ErrModelWaitCancelled) {
				t.Fatalf("err = %v, want ErrModelWaitCancelled", err)
			}
		case <-time.After(testWait):
			t.Fatal("cancelled waiter did not return")
		}
		eventually(t, func() bool { return len(s.Snapshot().ModelWaiters) == 0 },
			"cancelled entry lingered in the gate")
	})

	t.Run("a disconnected client detaches its waiter", func(t *testing.T) {
		s := newStarted(t, 1, 1, false)
		proceed := heldJob(t, s, "job-a", "A")
		defer close(proceed)

		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() {
			_, err := s.WaitForModel(ctx, modelClaim("A", "B", "inbound-1"))
			done <- err
		}()
		eventually(t, queuedWaiters(s, "inbound-1"), "the caller never queued")

		cancel()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("err = %v, want context.Canceled", err)
			}
		case <-time.After(testWait):
			t.Fatal("disconnected waiter did not return")
		}
		eventually(t, func() bool { return len(s.Snapshot().ModelWaiters) == 0 },
			"detached waiter lingered in the gate")
	})

	t.Run("admitted callers cannot evict each other", func(t *testing.T) {
		s := newStarted(t, 1, 1, false)

		release, err := s.WaitForModel(t.Context(), modelClaim("", "B", "inbound-1"))
		if err != nil {
			t.Fatalf("first caller = %v, want admitted", err)
		}

		res := s.CheckModelSwitch(modelClaim("B", "C", "inbound-2"))
		if res.Allowed {
			t.Fatal("a second caller must not evict the model an admitted caller is using")
		}
		if res.BlockedBy == nil || res.BlockedBy.Kind != KindInbound || res.BlockedBy.Key != "inbound-1" {
			t.Fatalf("BlockedBy = %+v, want the admitted inbound caller", res.BlockedBy)
		}
		if res := s.CheckModelSwitch(modelClaim("B", "C", "inbound-1")); !res.Allowed {
			t.Fatal("a caller may always keep using its own model")
		}

		release()
		if res := s.CheckModelSwitch(modelClaim("B", "C", "inbound-2")); !res.Allowed {
			t.Fatalf("switch after release = %+v, want allowed", res)
		}
	})

	t.Run("an interactive holder carries the model it claimed", func(t *testing.T) {
		s := newStarted(t, 1, 1, false)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		releaseCh := make(chan func(), 1)
		go func() {
			_, release, err := s.ClaimInteractive(ctx, LaneLocal, "ws", "model-X")
			if err == nil {
				releaseCh <- release
			}
		}()

		select {
		case release := <-releaseCh:
			holders := laneState(t, s, LaneLocal).Holders
			if len(holders) != 1 || holders[0].Model != "model-X" {
				t.Fatalf("holders = %+v, want the chat's model recorded", holders)
			}
			if s.CheckModelSwitch(modelClaim("model-X", "other", "caller")).Allowed {
				t.Fatal("a chat's model must block a switch while the chat runs")
			}
			release()
		case <-time.After(testWait):
			t.Fatal("interactive claim was not granted")
		}
	})

	t.Run("holds queued local work while an inbound caller waits", func(t *testing.T) {
		s := newStarted(t, 2, 1, false) // a free slot, so only the hold can stop the start
		proceed := heldJob(t, s, "job-a", "A")
		defer close(proceed)

		go func() {
			_, _ = s.WaitForModel(t.Context(), modelClaim("A", "B", "inbound-1"))
		}()
		eventually(t, queuedWaiters(s, "inbound-1"), "the caller never queued")

		started := make(chan string, 1)
		job := autoJob("job-b", started, make(chan struct{}), nil)
		job.Model = "C"
		if _, err := s.Submit(job); err != nil {
			t.Fatal(err)
		}
		if state := laneState(t, s, LaneLocal); state.Running != 1 || len(state.Queued) != 1 {
			t.Fatalf("state = %+v, want job-b queued and not started while the gate holds", state)
		}
		select {
		case key := <-started:
			t.Fatalf("%q started while the model gate held the lane", key)
		case <-time.After(50 * time.Millisecond):
		}
	})

	t.Run("promote preempts the holder and serves the waiter first", func(t *testing.T) {
		s := newStarted(t, 1, 1, false)
		started := make(chan string, 2)
		proceed := make(chan struct{}) // never closed: only the preemption ends the run
		preempted := make(chan bool, 1)
		job := autoJob("job-a", started, proceed, preempted)
		job.Model = "A"
		if _, err := s.Submit(job); err != nil {
			t.Fatal(err)
		}
		waitStarted(t, started, "job-a")

		releaseCh := make(chan func(), 1)
		go func() {
			release, err := s.WaitForModel(context.Background(), modelClaim("A", "B", "inbound-1"))
			if err == nil {
				releaseCh <- release
			}
		}()
		eventually(t, queuedWaiters(s, "inbound-1"), "the caller never queued")

		if err := s.PromoteModelWait("inbound-1"); err != nil {
			t.Fatalf("PromoteModelWait = %v", err)
		}
		if err := s.PromoteModelWait("inbound-1"); err != nil {
			t.Fatalf("PromoteModelWait must be idempotent, got %v", err)
		}
		if err := s.PromoteModelWait("no-such-entry"); !errors.Is(err, ErrUnknownWaiter) {
			t.Fatalf("PromoteModelWait(unknown) = %v, want ErrUnknownWaiter", err)
		}

		select {
		case seen := <-preempted:
			if !seen {
				t.Fatal("holder did not observe the preemption marker")
			}
		case <-time.After(testWait):
			t.Fatal("promote did not cancel the blocking run")
		}

		var release func()
		select {
		case release = <-releaseCh:
		case <-time.After(testWait):
			t.Fatal("waiter was not granted after the preempted run unwound")
		}

		// The preempted automation is re-queued, not lost — and it must NOT
		// restart ahead of the caller the operator promoted.
		state := laneState(t, s, LaneLocal)
		if state.Running != 0 || len(state.Queued) != 1 || state.Queued[0].Key != "job-a" {
			t.Fatalf("state = %+v, want job-a re-queued and held for the inbound caller", state)
		}

		release()
		waitStarted(t, started, "job-a") // restarts once the gate empties
	})

	t.Run("queued positions ignore already-granted callers", func(t *testing.T) {
		s := newStarted(t, 1, 1, false)
		proceed := heldJob(t, s, "job-a", "A")
		defer close(proceed)

		// A caller admitted before the contention is registered as a granted
		// waiter holding the model; it must not occupy a queue position.
		releaseGranted, err := s.WaitForModel(t.Context(), modelClaim("", "B", "granted-1"))
		if err != nil {
			t.Fatalf("harmless switch = %v, want nil", err)
		}
		defer releaseGranted()

		done := make(chan error, 1)
		go func() {
			_, err := s.WaitForModel(context.Background(), modelClaim("A", "B", "inbound-2"))
			done <- err
		}()
		eventually(t, queuedWaiters(s, "inbound-2"), "the caller never queued")

		eventually(t, func() bool {
			waiters := s.Snapshot().ModelWaiters
			return len(waiters) == 1 && waiters[0].Key == "inbound-2" && waiters[0].Position == 1
		}, "the waiting caller must be reported at position 1, not behind the granted holder")

		if got := s.ModelWaiterCount(); got != 1 {
			t.Fatalf("ModelWaiterCount = %d, want 1", got)
		}
		s.CancelModelWait("inbound-2")
		<-done
	})

	t.Run("Close unblocks waiters", func(t *testing.T) {
		s := New(Limits{Local: 1, Cloud: 1}, false)
		s.Start(t.Context())
		proceed := heldJob(t, s, "job-a", "A")

		done := make(chan error, 1)
		go func() {
			_, err := s.WaitForModel(context.Background(), modelClaim("A", "B", "inbound-1"))
			done <- err
		}()
		eventually(t, queuedWaiters(s, "inbound-1"), "the caller never queued")

		if err := s.Close(context.Background()); err != nil {
			t.Fatalf("Close = %v", err)
		}
		select {
		case err := <-done:
			if !errors.Is(err, ErrClosed) {
				t.Fatalf("err = %v, want ErrClosed", err)
			}
		case <-time.After(testWait):
			t.Fatal("waiter was not unblocked by Close")
		}
		if _, err := s.WaitForModel(t.Context(), modelClaim("A", "B", "inbound-2")); !errors.Is(err, ErrClosed) {
			t.Fatalf("WaitForModel after Close = %v, want ErrClosed", err)
		}
		close(proceed)
	})
}
