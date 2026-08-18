package core

import (
	"context"
	"testing"
	"time"
)

func TestHeartbeat_TicksFire(t *testing.T) {
	hb := NewHeartbeat()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hb.Start(ctx, 10*time.Millisecond)
	defer hb.Stop()

	select {
	case <-hb.C:
	case <-time.After(2 * time.Second):
		t.Fatal("expected heartbeat tick")
	}
}

func TestHeartbeat_StopsOnCtxDone(t *testing.T) {
	hb := NewHeartbeat()
	ctx, cancel := context.WithCancel(context.Background())
	hb.Start(ctx, 5*time.Millisecond)

	// Consume a tick to prove it was running.
	select {
	case <-hb.C:
	case <-time.After(2 * time.Second):
		t.Fatal("expected heartbeat to start")
	}

	cancel()

	// After cancel, ticks should stop flowing.
	select {
	case <-hb.C:
		t.Fatal("tick received after context cancelled")
	case <-time.After(60 * time.Millisecond):
	}
}

func TestHeartbeat_StopsOnStop(t *testing.T) {
	hb := NewHeartbeat()
	hb.Start(context.Background(), 5*time.Millisecond)

	select {
	case <-hb.C:
	case <-time.After(2 * time.Second):
		t.Fatal("expected heartbeat to start")
	}

	hb.Stop()

	select {
	case <-hb.C:
		t.Fatal("tick received after Stop")
	case <-time.After(60 * time.Millisecond):
	}
}

func TestHeartbeat_StopIdempotent(t *testing.T) {
	hb := NewHeartbeat()
	hb.Start(context.Background(), 5*time.Millisecond)
	hb.Stop()
	hb.Stop()
	hb.Stop() // must not panic
}

func TestHeartbeat_StartIdempotent(t *testing.T) {
	hb := NewHeartbeat()
	hb.Start(context.Background(), 5*time.Millisecond)
	hb.Start(context.Background(), 5*time.Millisecond)
	hb.Stop()
}

func TestHeartbeat_IntervalLeqZeroDisables(t *testing.T) {
	for _, interval := range []time.Duration{0, -1 * time.Second} {
		hb := NewHeartbeat()
		hb.Start(context.Background(), interval)
		select {
		case <-hb.C:
			t.Fatalf("tick received with interval %v; expected disabled", interval)
		case <-time.After(30 * time.Millisecond):
		}
		hb.Stop()
	}
}

func TestHeartbeat_DropsWhenOwnerBusy(t *testing.T) {
	hb := NewHeartbeat()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hb.Start(ctx, 5*time.Millisecond)
	defer hb.Stop()

	// Never read C for a while — ticks should be dropped, not queued (buffer 1).
	time.Sleep(40 * time.Millisecond)

	hb.Stop()

	// After Stop the ticker is gone; drain whatever is buffered. Because C is a
	// size-1 channel, at most one tick can be pending regardless of how long the
	// owner was busy — never an unbounded queue.
	count := 0
	for {
		select {
		case <-hb.C:
			count++
		default:
			if count > 1 {
				t.Fatalf("expected at most 1 buffered tick after busy window, got %d", count)
			}
			return
		}
	}
}
