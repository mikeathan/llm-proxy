package runlane

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

const testWait = 2 * time.Second

func waitStarted(t *testing.T, started <-chan string, want string) {
	t.Helper()
	select {
	case got := <-started:
		if got != want {
			t.Fatalf("started %q, want %q", got, want)
		}
	case <-time.After(testWait):
		t.Fatalf("timed out waiting for %q to start", want)
	}
}

func eventually(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(testWait)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal(msg)
}

func autoJob(key string, started chan<- string, proceed <-chan struct{}, preempted chan<- bool) Job {
	return Job{
		Key: key, LaneKey: LaneLocal, Kind: KindAutomation,
		WorkspaceID: "ws", Automation: key, Label: "ws/" + key,
		Run: func(ctx context.Context) error {
			started <- key
			select {
			case <-proceed:
				return nil
			case <-ctx.Done():
				if preempted != nil {
					preempted <- Preempted(ctx)
				}
				return ctx.Err()
			}
		},
	}
}

func cloudJob(key string, started chan<- string, proceed <-chan struct{}) Job {
	j := autoJob(key, started, proceed, nil)
	j.LaneKey = LaneCloud
	return j
}

func newStarted(t *testing.T, local, cloud int, preempt bool) *Scheduler {
	t.Helper()
	s := New(Limits{Local: local, Cloud: cloud}, preempt)
	s.Start(t.Context())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.Close(ctx)
	})
	return s
}

func laneState(t *testing.T, s *Scheduler, key LaneKey) LaneState {
	t.Helper()
	for _, l := range s.Snapshot().Lanes {
		if l.Lane == key {
			return l
		}
	}
	t.Fatalf("lane %q missing from snapshot", key)
	return LaneState{}
}

// claim runs ClaimInteractive in a goroutine and returns channels for its result,
// so tests can hold or free the interactive slot deterministically.
func claim(t *testing.T, s *Scheduler, lane LaneKey) (<-chan func(), <-chan error) {
	t.Helper()
	releaseCh := make(chan func(), 1)
	errCh := make(chan error, 1)
	go func() {
		_, release, err := s.ClaimInteractive(t.Context(), lane, "ws", "m1")
		if err == nil {
			releaseCh <- release
		}
		errCh <- err
	}()
	return releaseCh, errCh
}

func TestScheduler(t *testing.T) {
	t.Run("starts a submitted job immediately", func(t *testing.T) {
		s := newStarted(t, 1, 1, true)
		started := make(chan string, 1)
		proceed := make(chan struct{})
		sub, err := s.Submit(autoJob("a", started, proceed, nil))
		if err != nil {
			t.Fatal(err)
		}
		if sub.Disposition != DispositionStarted {
			t.Fatalf("disposition = %q, want %q", sub.Disposition, DispositionStarted)
		}
		waitStarted(t, started, "a")
		close(proceed)
	})

	t.Run("queues an overlapping automation and starts it when the slot frees", func(t *testing.T) {
		s := newStarted(t, 1, 1, true)
		started := make(chan string, 4)
		proceedA := make(chan struct{})
		_, err := s.Submit(autoJob("a", started, proceedA, nil))
		if err != nil {
			t.Fatal(err)
		}
		waitStarted(t, started, "a")

		sub, err := s.Submit(autoJob("b", started, make(chan struct{}), nil))
		if err != nil {
			t.Fatal(err)
		}
		if sub.Disposition != DispositionQueued || sub.Position != 1 {
			t.Fatalf("sub = %+v, want queued at position 1", sub)
		}
		if got := laneState(t, s, LaneLocal); len(got.Queued) != 1 {
			t.Fatalf("queued entries = %d, want 1", len(got.Queued))
		}

		close(proceedA)
		waitStarted(t, started, "b")
	})

	t.Run("cloud lane runs up to its limit concurrently", func(t *testing.T) {
		s := newStarted(t, 1, 3, true)
		started := make(chan string, 4)
		proceed := make(chan struct{})
		defer close(proceed)
		for _, key := range []string{"a", "b", "c"} {
			if _, err := s.Submit(cloudJob(key, started, proceed)); err != nil {
				t.Fatal(err)
			}
		}
		got := make([]string, 0, 3)
		for range 3 {
			select {
			case k := <-started:
				got = append(got, k)
			case <-time.After(testWait):
				t.Fatal("timed out waiting for concurrent cloud jobs")
			}
		}
		slices.Sort(got)
		if !slices.Equal(got, []string{"a", "b", "c"}) {
			t.Fatalf("started = %v, want a b c", got)
		}
	})

	t.Run("rejects a duplicate queued key reporting the existing position", func(t *testing.T) {
		s := newStarted(t, 1, 1, true)
		started := make(chan string, 1)
		proceed := make(chan struct{})
		defer close(proceed)
		_, err := s.Submit(autoJob("a", started, proceed, nil))
		if err != nil {
			t.Fatal(err)
		}
		waitStarted(t, started, "a")

		if _, err := s.Submit(autoJob("b", started, proceed, nil)); err != nil {
			t.Fatal(err)
		}
		sub, err := s.Submit(autoJob("b", started, proceed, nil))
		if !errors.Is(err, ErrAlreadyQueued) {
			t.Fatalf("err = %v, want ErrAlreadyQueued", err)
		}
		if sub.Disposition != DispositionQueued || sub.Position != 1 {
			t.Fatalf("sub = %+v, want queued at position 1", sub)
		}
	})

	t.Run("a fire while running becomes the single pending rerun", func(t *testing.T) {
		s := newStarted(t, 1, 1, true)
		started := make(chan string, 4)
		proceed := make(chan struct{})
		job := autoJob("a", started, proceed, nil)
		if _, err := s.Submit(job); err != nil {
			t.Fatal(err)
		}
		waitStarted(t, started, "a")

		sub, err := s.Submit(job)
		if err != nil {
			t.Fatal(err)
		}
		if sub.Disposition != DispositionQueued {
			t.Fatalf("disposition = %q, want queued", sub.Disposition)
		}
		close(proceed)
		waitStarted(t, started, "a") // rerun, not a second entry
	})

	t.Run("grants interactive claims before queued automations", func(t *testing.T) {
		s := newStarted(t, 1, 1, false) // preemption off: pure priority check
		started := make(chan string, 4)
		proceedA := make(chan struct{})
		if _, err := s.Submit(autoJob("a", started, proceedA, nil)); err != nil {
			t.Fatal(err)
		}
		waitStarted(t, started, "a")
		if _, err := s.Submit(autoJob("b", started, make(chan struct{}), nil)); err != nil {
			t.Fatal(err)
		}

		releaseCh, errCh := claim(t, s, LaneLocal)
		eventually(t, func() bool { return laneState(t, s, LaneLocal).Waiting == 1 },
			"claim was not registered before the slot freed")
		close(proceedA)

		select {
		case release := <-releaseCh:
			state := laneState(t, s, LaneLocal)
			if state.Running != 1 || len(state.Holders) != 1 || state.Holders[0].Kind != KindInteractive {
				t.Fatalf("holder state = %+v, want a single interactive holder", state)
			}
			release()
			waitStarted(t, started, "b")
		case err := <-errCh:
			t.Fatalf("claim failed: %v", err)
		case <-time.After(testWait):
			t.Fatal("timed out waiting for the interactive claim")
		}
	})

	t.Run("preempts a running automation in the same lane and re-queues it", func(t *testing.T) {
		s := newStarted(t, 1, 1, true)
		started := make(chan string, 4)
		proceed := make(chan struct{})
		defer close(proceed)
		preempted := make(chan bool, 1)
		if _, err := s.Submit(autoJob("a", started, proceed, preempted)); err != nil {
			t.Fatal(err)
		}
		waitStarted(t, started, "a")

		releaseCh, errCh := claim(t, s, LaneLocal)
		select {
		case release := <-releaseCh:
			select {
			case seen := <-preempted:
				if !seen {
					t.Fatal("preempted automation did not observe the preemption marker")
				}
			case <-time.After(testWait):
				t.Fatal("timed out waiting for the preempted automation to stop")
			}
			eventually(t, func() bool {
				state := laneState(t, s, LaneLocal)
				return len(state.Queued) == 1 && state.Queued[0].Key == "a"
			}, "preempted automation was not re-queued at the front")

			release()
			waitStarted(t, started, "a") // restarts when the chat releases the slot
		case err := <-errCh:
			t.Fatalf("claim failed: %v", err)
		case <-time.After(testWait):
			t.Fatal("timed out waiting for the interactive claim")
		}
	})

	t.Run("preemption leaves other lanes untouched", func(t *testing.T) {
		s := newStarted(t, 1, 1, true)
		started := make(chan string, 4)
		proceed := make(chan struct{})
		defer close(proceed)
		if _, err := s.Submit(cloudJob("cloud", started, proceed)); err != nil {
			t.Fatal(err)
		}
		waitStarted(t, started, "cloud")
		if _, err := s.Submit(autoJob("local", started, proceed, nil)); err != nil {
			t.Fatal(err)
		}
		waitStarted(t, started, "local")

		releaseCh, errCh := claim(t, s, LaneLocal)
		select {
		case release := <-releaseCh:
			state := laneState(t, s, LaneCloud)
			if state.Running != 1 {
				t.Fatalf("cloud lane running = %d, want 1 (cross-lane preemption)", state.Running)
			}
			release()
		case err := <-errCh:
			t.Fatalf("claim failed: %v", err)
		case <-time.After(testWait):
			t.Fatal("timed out waiting for the interactive claim")
		}
	})

	t.Run("drops manual jobs on preemption", func(t *testing.T) {
		s := newStarted(t, 1, 1, true)
		started := make(chan string, 4)
		proceed := make(chan struct{})
		defer close(proceed)
		preempted := make(chan bool, 1)
		manual := autoJob("manual", started, proceed, preempted)
		manual.Manual = true
		if _, err := s.Submit(manual); err != nil {
			t.Fatal(err)
		}
		waitStarted(t, started, "manual")

		releaseCh, errCh := claim(t, s, LaneLocal)
		select {
		case release := <-releaseCh:
			<-preempted
			eventually(t, func() bool {
				return len(laneState(t, s, LaneLocal).Queued) == 0
			}, "manual job should be dropped, not re-queued")
			release()
		case err := <-errCh:
			t.Fatalf("claim failed: %v", err)
		case <-time.After(testWait):
			t.Fatal("timed out waiting for the interactive claim")
		}
	})

	t.Run("returns ErrPreemptTimeout when the preempted run ignores cancellation", func(t *testing.T) {
		oldGrace := preemptGrace
		preemptGrace = 25 * time.Millisecond
		t.Cleanup(func() { preemptGrace = oldGrace })

		s := newStarted(t, 1, 1, true)
		started := make(chan string, 4)
		proceed := make(chan struct{})
		stubborn := autoJob("a", started, proceed, nil)
		stubborn.Run = func(ctx context.Context) error { // ignores ctx on purpose
			started <- "a"
			<-proceed
			return nil
		}
		if _, err := s.Submit(stubborn); err != nil {
			t.Fatal(err)
		}
		waitStarted(t, started, "a")

		_, errCh := claim(t, s, LaneLocal)
		select {
		case err := <-errCh:
			if !errors.Is(err, ErrPreemptTimeout) {
				t.Fatalf("err = %v, want ErrPreemptTimeout", err)
			}
		case <-time.After(testWait):
			t.Fatal("timed out waiting for ErrPreemptTimeout")
		}

		// The lane invariant is untouched: once the wedged run exits it is
		// re-queued (it was preempted), drains, and the lane accepts new work.
		close(proceed)
		eventually(t, func() bool {
			state := laneState(t, s, LaneLocal)
			return state.Running == 0 && len(state.Queued) == 0
		}, "lane did not drain after the wedged run exited")

		sub, err := s.Submit(autoJob("b", started, make(chan struct{}), nil))
		if err != nil || sub.Disposition != DispositionStarted {
			t.Fatalf("sub = %+v err = %v, want started", sub, err)
		}
	})

	t.Run("CancelQueued drops a queued entry but leaves a running run alone", func(t *testing.T) {
		s := newStarted(t, 1, 1, true)
		started := make(chan string, 4)
		proceedA := make(chan struct{})
		defer close(proceedA)
		if _, err := s.Submit(autoJob("a", started, proceedA, nil)); err != nil {
			t.Fatal(err)
		}
		waitStarted(t, started, "a")
		if _, err := s.Submit(autoJob("b", started, make(chan struct{}), nil)); err != nil {
			t.Fatal(err)
		}

		if !s.CancelQueued("b") {
			t.Fatal("CancelQueued reported no match for the queued entry")
		}
		if s.CancelQueued("b") {
			t.Fatal("CancelQueued matched the same queued entry twice")
		}
		if s.CancelQueued("a") {
			t.Fatal("CancelQueued must not match a running job")
		}
		if state := laneState(t, s, LaneLocal); state.Running != 1 {
			t.Fatalf("running = %d, want the running run left alone", state.Running)
		}
	})

	t.Run("a waiting claim detaches when its ctx is cancelled", func(t *testing.T) {
		s := newStarted(t, 1, 1, false)
		started := make(chan string, 4)
		proceedA := make(chan struct{})
		if _, err := s.Submit(autoJob("a", started, proceedA, nil)); err != nil {
			t.Fatal(err)
		}
		waitStarted(t, started, "a")

		ctx, cancel := context.WithCancel(t.Context())
		errCh := make(chan error, 1)
		go func() {
			_, _, err := s.ClaimInteractive(ctx, LaneLocal, "ws", "m1")
			errCh <- err
		}()
		cancel()

		select {
		case err := <-errCh:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("err = %v, want context.Canceled", err)
			}
		case <-time.After(testWait):
			t.Fatal("timed out waiting for the detached claim")
		}

		close(proceedA)
		eventually(t, func() bool { return laneState(t, s, LaneLocal).Running == 0 },
			"lane did not drain after the run finished")

		sub, err := s.Submit(autoJob("b", started, make(chan struct{}), nil))
		if err != nil || sub.Disposition != DispositionStarted {
			t.Fatalf("sub = %+v err = %v, want started (ghost waiter blocked the slot)", sub, err)
		}
	})

	t.Run("a granted claim runCtx derives from the caller ctx", func(t *testing.T) {
		s := newStarted(t, 1, 1, true)
		ctx, cancel := context.WithCancel(t.Context())
		runCtxCh := make(chan context.Context, 1)
		errCh := make(chan error, 1)
		go func() {
			runCtx, release, err := s.ClaimInteractive(ctx, LaneLocal, "ws", "m1")
			if err == nil {
				runCtxCh <- runCtx
				<-runCtx.Done() // must fire when the caller cancels
				release()
			}
			errCh <- err
		}()

		select {
		case runCtx := <-runCtxCh:
			cancel()
			select {
			case <-runCtx.Done():
			case <-time.After(testWait):
				t.Fatal("granted runCtx did not inherit the caller cancellation")
			}
			<-errCh
		case err := <-errCh:
			t.Fatalf("claim failed: %v", err)
		case <-time.After(testWait):
			t.Fatal("timed out waiting for the claim")
		}
	})

	t.Run("a panicking job frees its slot", func(t *testing.T) {
		s := newStarted(t, 1, 1, true)
		started := make(chan string, 4)
		boom := autoJob("boom", started, nil, nil)
		boom.Run = func(ctx context.Context) error {
			started <- "boom"
			panic("runlane test panic")
		}
		if _, err := s.Submit(boom); err != nil {
			t.Fatal(err)
		}
		waitStarted(t, started, "boom")

		eventually(t, func() bool { return laneState(t, s, LaneLocal).Running == 0 },
			"panicking job did not free its slot")

		sub, err := s.Submit(autoJob("next", started, make(chan struct{}), nil))
		if err != nil || sub.Disposition != DispositionStarted {
			t.Fatalf("sub = %+v err = %v, want started after the panic", sub, err)
		}
	})

	t.Run("SetLimits starts queued work without a restart", func(t *testing.T) {
		s := newStarted(t, 1, 1, true)
		started := make(chan string, 4)
		proceed := make(chan struct{})
		defer close(proceed)
		if _, err := s.Submit(autoJob("a", started, proceed, nil)); err != nil {
			t.Fatal(err)
		}
		waitStarted(t, started, "a")
		if _, err := s.Submit(autoJob("b", started, make(chan struct{}), nil)); err != nil {
			t.Fatal(err)
		}

		s.SetLimits(Limits{Local: 2, Cloud: 3})
		waitStarted(t, started, "b")
	})

	t.Run("Close cancels running jobs, rejects new work and unblocks waiters", func(t *testing.T) {
		s := New(Limits{Local: 1, Cloud: 1}, false)
		s.Start(t.Context())
		started := make(chan string, 1)
		proceed := make(chan struct{})
		if _, err := s.Submit(autoJob("a", started, proceed, nil)); err != nil {
			t.Fatal(err)
		}
		waitStarted(t, started, "a")

		_, errCh := claim(t, s, LaneLocal)
		if err := s.Close(context.Background()); err != nil {
			t.Fatalf("Close = %v", err)
		}

		select {
		case err := <-errCh:
			if !errors.Is(err, ErrClosed) {
				t.Fatalf("waiter err = %v, want ErrClosed", err)
			}
		case <-time.After(testWait):
			t.Fatal("timed out waiting for the waiter to be unblocked")
		}
		select {
		case <-started:
			t.Fatal("running job should have been cancelled by Close")
		case <-time.After(50 * time.Millisecond):
		}

		if _, err := s.Submit(autoJob("b", started, proceed, nil)); !errors.Is(err, ErrClosed) {
			t.Fatalf("submit after close = %v, want ErrClosed", err)
		}
	})

	t.Run("unknown lane key fails loudly", func(t *testing.T) {
		s := newStarted(t, 1, 1, true)
		j := autoJob("a", make(chan string, 1), make(chan struct{}), nil)
		j.LaneKey = "bogus"
		if _, err := s.Submit(j); !errors.Is(err, ErrUnknownLane) {
			t.Fatalf("err = %v, want ErrUnknownLane", err)
		}
	})

	t.Run("Submit before Start fails instead of running untethered", func(t *testing.T) {
		s := New(Limits{Local: 1, Cloud: 1}, true)
		if _, err := s.Submit(autoJob("a", make(chan string, 1), make(chan struct{}), nil)); !errors.Is(err, ErrNotStarted) {
			t.Fatalf("err = %v, want ErrNotStarted", err)
		}
	})

	t.Run("a closed lane starts no queued work", func(t *testing.T) {
		s := newStarted(t, 1, 1, true)
		l := s.lanes[LaneLocal]
		l.mu.Lock()
		l.closed = true
		l.queue = append(l.queue, &queuedJob{job: Job{Key: "q", LaneKey: LaneLocal, Kind: KindAutomation}})
		l.pumpLocked()
		running := len(l.running)
		l.mu.Unlock()
		if running != 0 {
			t.Fatalf("closed lane started %d job(s), want 0", running)
		}
	})

	t.Run("snapshot returns copies", func(t *testing.T) {
		s := newStarted(t, 1, 1, true)
		started := make(chan string, 1)
		proceed := make(chan struct{})
		defer close(proceed)
		if _, err := s.Submit(autoJob("a", started, proceed, nil)); err != nil {
			t.Fatal(err)
		}
		waitStarted(t, started, "a")
		if _, err := s.Submit(autoJob("b", started, make(chan struct{}), nil)); err != nil {
			t.Fatal(err)
		}

		snap := s.Snapshot()
		snap.Lanes[0].Queued = append(snap.Lanes[0].Queued, Entry{Key: "injected"})
		snap.Lanes[0].Holders[0].Label = "mutated"

		fresh := laneState(t, s, LaneLocal)
		if len(fresh.Queued) != 1 || fresh.Queued[0].Key != "b" {
			t.Fatalf("queued = %+v, snapshot mutation leaked", fresh.Queued)
		}
		if fresh.Holders[0].Label == "mutated" {
			t.Fatal("holder mutation leaked into the scheduler state")
		}
	})
}
