package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"llm-proxy/internal/core/runlane"
	"llm-proxy/internal/transport/http/handlers"
	"llm-proxy/models"
)

func settingsFunc(cfg models.SchedulerConfig) func() models.SchedulerConfig {
	return func() models.SchedulerConfig { return cfg }
}

// holdModel starts a local run that keeps `model` in use until the returned
// function is called.
func holdModel(t *testing.T, lane *runlane.Scheduler, key, model string) func() {
	t.Helper()
	started := make(chan struct{})
	proceed := make(chan struct{})
	done := make(chan struct{})
	if _, err := lane.Submit(runlane.Job{
		Key: key, LaneKey: runlane.LaneLocal, Kind: runlane.KindAutomation, Model: model,
		Run: func(ctx context.Context) error {
			close(started)
			select {
			case <-proceed:
			case <-ctx.Done():
			}
			close(done)
			return nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	<-started
	return func() { close(proceed); <-done }
}

func startedLane(t *testing.T) *runlane.Scheduler {
	t.Helper()
	lane := runlane.New(runlane.Limits{Local: 1, Cloud: 1}, false)
	lane.Start(t.Context())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = lane.Close(ctx)
	})
	return lane
}

func TestLaneInboundGate_WaitSecondsClampsToHostPolicy(t *testing.T) {
	cases := []struct {
		name      string
		policy    *int
		requested int
		want      int
	}{
		{"default policy honours a shorter request", nil, 10, 10},
		{"default policy caps a longer request", nil, 600, 60},
		{"no request means no wait", nil, 0, 0},
		{"a policy of zero refuses without waiting", new(0), 30, 0},
		{"an unlimited policy passes the request through", new(-1), 600, 600},
		{"an explicit cap wins", new(5), 30, 5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gate := laneGate{
				lane:     startedLane(t),
				settings: settingsFunc(models.SchedulerConfig{InboundWaitSeconds: c.policy}),
			}
			if got := gate.WaitSeconds(c.requested); got != c.want {
				t.Fatalf("WaitSeconds(%d) = %d, want %d", c.requested, got, c.want)
			}
		})
	}
}

// A caller with no declared budget must be refused, not held: an ordinary
// OpenAI client has no way to end a park, and holding it would silently turn
// "refused" into "hung".
func TestLaneInboundGate_RefusesACallerWithNoBudget(t *testing.T) {
	lane := startedLane(t)
	release := holdModel(t, lane, "job-a", "A")

	gate := laneGate{lane: lane, settings: settingsFunc(models.DefaultSchedulerConfig())}
	_, err := gate.Wait(t.Context(), handlers.InboundRequest{
		Model: "B", Active: "A", Key: "inbound:1", Wait: 0,
	})
	if !errors.Is(err, handlers.ErrInboundBusy) {
		t.Fatalf("err = %v, want ErrInboundBusy", err)
	}

	// The run holding the model is untouched.
	if holders := lane.Snapshot().Lanes[0].Holders; len(holders) != 1 || holders[0].Key != "job-a" {
		t.Fatalf("holders = %+v, want the run left alone", holders)
	}
	release()
}

func TestLaneInboundGate_AdmitsAnIdleCallerAndReleases(t *testing.T) {
	lane := startedLane(t)
	gate := laneGate{lane: lane, settings: settingsFunc(models.DefaultSchedulerConfig())}

	release, err := gate.Wait(t.Context(), handlers.InboundRequest{
		Model: "B", Active: "", Key: "inbound:1", Wait: 0,
	})
	if err != nil {
		t.Fatalf("idle caller = %v, want admitted", err)
	}
	if got := lane.Snapshot().ModelHolders; len(got) != 1 || got[0].Key != "inbound:1" {
		t.Fatalf("ModelHolders = %+v, want the admitted caller to hold B", got)
	}

	release()
	if got := lane.Snapshot().ModelHolders; len(got) != 0 {
		t.Fatalf("ModelHolders = %+v, want the claim released", got)
	}
}

// waitFor polls cond for a bounded time: the scheduler's own state is the only
// signal available, so the test observes it rather than sleeping blindly.
func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal(msg)
}

// A caller whose request is held can be dropped by key from another connection:
// that is the DELETE /v1/queue route, and the operator's dismiss.
func TestLaneInboundGate_CancelDropsAQueuedCaller(t *testing.T) {
	lane := startedLane(t)
	release := holdModel(t, lane, "job-a", "A")

	gate := laneGate{lane: lane, settings: settingsFunc(models.DefaultSchedulerConfig())}
	queued := make(chan error, 1)
	go func() {
		_, err := gate.Wait(context.Background(), handlers.InboundRequest{
			Model: "B", Active: "A", Key: "inbound:abc", Wait: 30,
		})
		queued <- err
	}()
	waitFor(t, func() bool { return len(lane.Snapshot().ModelWaiters) == 1 }, "the caller never queued")

	if !gate.Cancel("inbound:abc") {
		t.Fatal("Cancel reported no match for the waiting caller")
	}
	if gate.Cancel("inbound:abc") {
		t.Fatal("Cancel matched the same entry twice")
	}

	select {
	case err := <-queued:
		if !errors.Is(err, runlane.ErrModelWaitCancelled) {
			t.Fatalf("err = %v, want ErrModelWaitCancelled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled caller did not return")
	}
	release()
}

func TestLaneInboundGate_RefusesWhenTheQueueIsFull(t *testing.T) {
	lane := startedLane(t)
	release := holdModel(t, lane, "job-a", "A")

	cfg := models.DefaultSchedulerConfig()
	cfg.InboundMaxQueued = new(1)
	gate := laneGate{lane: lane, settings: settingsFunc(cfg)}

	// One caller takes the only waiting slot.
	queued := make(chan struct{})
	go func() {
		release, err := gate.Wait(context.Background(), handlers.InboundRequest{
			Model: "B", Active: "A", Key: "inbound:1", Wait: 30,
		})
		if err == nil {
			release()
		}
		close(queued)
	}()
	waitFor(t, func() bool { return len(lane.Snapshot().ModelWaiters) == 1 }, "the first caller never queued")

	_, err := gate.Wait(t.Context(), handlers.InboundRequest{
		Model: "B", Active: "A", Key: "inbound:2", Wait: 30,
	})
	if !errors.Is(err, handlers.ErrInboundQueueFull) {
		t.Fatalf("err = %v, want ErrInboundQueueFull", err)
	}

	release()
	<-queued
}
