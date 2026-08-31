package automation

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"llm-proxy/internal/core/assistant"
)

func TestBroadcastOrphanReaper(t *testing.T) {
	// Tiny interval + max-full so the test runs fast.
	bus := newEventBus(5*time.Millisecond, 10*time.Millisecond)
	defer bus.Stop()

	ws := "ws-reaper"
	ch, _ := bus.Subscribe(ws, assistant.ChannelAutomation)
	ev := assistant.AgentEvent{Channel: assistant.ChannelAutomation, Type: "x"}

	// Fill the subscriber buffer (cap 200) and never drain it.
	for i := 0; i < 200; i++ {
		bus.Publish(ws, ev)
	}
	// Concurrent publishes while the reaper runs — must not panic.
	go func() {
		for i := 0; i < 50; i++ {
			bus.Publish(ws, ev)
		}
	}()

	// Wait until the orphan is reaped from the subscribers map.
	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		bus.mu.RLock()
		chans := bus.subscribers[ws][assistant.ChannelAutomation]
		bus.mu.RUnlock()
		if len(chans) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("orphan subscriber channel was not reaped")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// The reaper must NOT have closed the channel: a send must not panic.
	// (A send on a closed channel panics even inside a select-with-default.)
	closed := false
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if r := recover(); r != nil {
				closed = true
			}
		}()
		select {
		case ch <- ev:
		default:
		}
	}()
	<-done
	if closed {
		t.Error("reaper closed the subscriber channel; it must only drop the reference")
	}
}

func TestBroadcastHealthyChannelNotReaped(t *testing.T) {
	bus := newEventBus(5*time.Millisecond, 10*time.Millisecond)
	defer bus.Stop()

	ws := "ws-healthy"
	ch, _ := bus.Subscribe(ws, assistant.ChannelAutomation)
	ev := assistant.AgentEvent{Channel: assistant.ChannelAutomation, Type: "x"}

	// Drain the channel concurrently so it never stays full.
	stopDrain := make(chan struct{})
	go func() {
		for {
			select {
			case <-ch:
			case <-stopDrain:
				return
			case <-time.After(2 * time.Millisecond):
			}
		}
	}()

	for i := 0; i < 50; i++ {
		bus.Publish(ws, ev)
	}
	time.Sleep(50 * time.Millisecond) // several reaper ticks
	close(stopDrain)

	bus.mu.RLock()
	chans := bus.subscribers[ws][assistant.ChannelAutomation]
	bus.mu.RUnlock()
	if len(chans) == 0 {
		t.Error("healthy (drained) channel was incorrectly reaped")
	}
}

func TestUnsubscribeCleansFullSinceAndPrunes(t *testing.T) {
	// Long intervals so reapLoop does not interfere with the assertions.
	bus := newEventBus(time.Hour, time.Hour)
	defer bus.Stop()

	ws := "ws-prune"
	ch, _ := bus.Subscribe(ws, assistant.ChannelAutomation)
	ev := assistant.AgentEvent{Channel: assistant.ChannelAutomation, Type: "x"}

	// Fill the buffer to capacity without draining it.
	for i := 0; i < 200; i++ {
		bus.Publish(ws, ev)
	}
	bus.reap() // marks fullSince[ch] = now

	if _, ok := bus.fullSince[ch]; !ok {
		t.Fatal("expected fullSince entry to exist after reap marked a full channel")
	}

	bus.Unsubscribe(ws, assistant.ChannelAutomation, ch)

	if _, ok := bus.fullSince[ch]; ok {
		t.Error("fullSince entry leaked after Unsubscribe")
	}
	bus.mu.RLock()
	_, wsOk := bus.subscribers[ws]
	bus.mu.RUnlock()
	if wsOk {
		t.Error("subscribers[ws] not pruned after its last channel unsubscribed")
	}
}

func TestClearPrunesEmptyWorkspace(t *testing.T) {
	bus := newEventBus(time.Hour, time.Hour)
	defer bus.Stop()

	bus.Publish("ws-clear", assistant.AgentEvent{Channel: assistant.ChannelAutomation, Type: "x"})
	bus.mu.RLock()
	_, ok := bus.recent["ws-clear"]
	bus.mu.RUnlock()
	if !ok {
		t.Fatal("expected recent[ws-clear] to exist after Publish")
	}

	bus.Clear("ws-clear", assistant.ChannelAutomation)

	bus.mu.RLock()
	_, ok = bus.recent["ws-clear"]
	bus.mu.RUnlock()
	if ok {
		t.Error("recent[ws-clear] not pruned after Clear emptied it")
	}
}

func TestEventBusStopIdempotentConcurrent(t *testing.T) {
	bus := newEventBus(time.Hour, time.Hour)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bus.Stop()
		}()
	}
	wg.Wait()
	// A further call must not panic (double close).
	bus.Stop()
}

// TestBroadcastCriticalEventDeliveredWhenFull proves a guardrail_blocked event
// is never dropped on a full subscriber channel: Publish blocks (bounded) until
// the reader drains, so the agent's approval wait always reaches the UI. The
// recent replay buffer is the reconnect net, but live delivery must not depend
// on a refresh.
func TestBroadcastCriticalEventDeliveredWhenFull(t *testing.T) {
	// Short bounded send so the test fails fast if a send is stuck.
	old := criticalEventPublishTimeout
	criticalEventPublishTimeout = 500 * time.Millisecond
	t.Cleanup(func() { criticalEventPublishTimeout = old })

	bus := newEventBus(time.Hour, time.Hour) // no reaper interference
	defer bus.Stop()

	ws := "ws-critical"
	ch, _ := bus.Subscribe(ws, assistant.ChannelAutomation)
	filler := assistant.AgentEvent{Channel: assistant.ChannelAutomation, Type: "x"}

	// Fill the subscriber buffer (cap 200) and never drain it yet.
	for i := 0; i < 200; i++ {
		bus.Publish(ws, filler)
	}

	// Publish a critical event on the full channel. It must BLOCK (not return
	// immediately via the drop path).
	blocked := assistant.AgentEvent{
		Channel: assistant.ChannelAutomation,
		Type:    assistant.EventGuardrailBlocked,
		Payload: assistant.GuardrailBlockedPayload{DecisionID: "gr_test"},
	}
	published := make(chan struct{})
	go func() {
		bus.Publish(ws, blocked)
		close(published)
	}()

	select {
	case <-published:
		t.Fatal("critical event was dropped on a full channel; Publish returned immediately")
	case <-time.After(20 * time.Millisecond):
	}

	// Drain the channel: the critical event must be delivered (it was queued
	// behind the fillers, never dropped).
	deadline := time.Now().Add(time.Second)
	found := false
	for time.Now().Before(deadline) && !found {
		select {
		case got := <-ch:
			if got.Type == assistant.EventGuardrailBlocked {
				found = true
			}
		case <-time.After(50 * time.Millisecond):
		}
	}
	select {
	case <-published:
	default:
		t.Fatal("critical event Publish did not complete after the reader drained")
	}
	if !found {
		t.Fatal("guardrail_blocked event was dropped on a full channel (never delivered)")
	}
}

// TestBroadcastNonCriticalDroppedWhenFull locks in the drop behavior for
// cosmetic events: a full channel drops reasoning/tool_stream traffic so a slow
// subscriber cannot stall the agent loop. Only critical events block.
func TestBroadcastNonCriticalDroppedWhenFull(t *testing.T) {
	bus := newEventBus(time.Hour, time.Hour)
	defer bus.Stop()

	ws := "ws-drop"
	ch, _ := bus.Subscribe(ws, assistant.ChannelAutomation)
	filler := assistant.AgentEvent{Channel: assistant.ChannelAutomation, Type: "x"}
	for i := 0; i < 200; i++ {
		bus.Publish(ws, filler)
	}

	// A non-critical publish on the full channel must return immediately (drop).
	done := make(chan struct{})
	go func() {
		bus.Publish(ws, assistant.AgentEvent{Channel: assistant.ChannelAutomation, Type: assistant.EventReasoning, Payload: "drop-me"})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("non-critical publish blocked on a full channel; it must drop")
	}

	// Drain one slot and confirm the dropped event is not in the channel (the
	// next event after the fillers is whatever follows, not the dropped one —
	// read the buffer head to prove no reasoning event was queued).
	select {
	case got := <-ch:
		if got.Type == assistant.EventReasoning {
			t.Error("non-critical event was queued on a full channel; expected drop")
		}
	case <-time.After(50 * time.Millisecond):
	}
}

// TestBroadcastPublishUnsubscribeNoPanic guards the send-on-closed-channel race:
// Publish snapshots the subscriber set and sends outside the lock, so a
// concurrent Unsubscribe must never make a send panic. Unsubscribe deliberately
// does NOT close the channel (readers exit on ctx.Done); this test exercises the
// churn so -race confirms no closed-channel send and no data race.
func TestBroadcastPublishUnsubscribeNoPanic(t *testing.T) {
	bus := newEventBus(time.Hour, time.Hour)
	defer bus.Stop()

	ws := "ws-churn"
	stop := make(chan struct{})
	var wg sync.WaitGroup

	// Publishers: mix critical (blocking-send) and non-critical (drop) events.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if i%2 == 0 {
					bus.Publish(ws, assistant.AgentEvent{Channel: assistant.ChannelAutomation, Type: assistant.EventReasoning, Payload: "x"})
				} else {
					bus.Publish(ws, assistant.AgentEvent{Channel: assistant.ChannelAutomation, Type: assistant.EventGuardrailBlocked, Payload: assistant.GuardrailBlockedPayload{DecisionID: "gr"}})
				}
			}
		}(i)
	}

	// Subscriber churn: subscribe then immediately unsubscribe, racing sends.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			ch, _ := bus.Subscribe(ws, assistant.ChannelAutomation)
			bus.Unsubscribe(ws, assistant.ChannelAutomation, ch)
		}
	}()

	time.Sleep(100 * time.Millisecond)
	close(stop)
	wg.Wait()
}

func TestRecentCappedByBytes(t *testing.T) {
	bus := newEventBus(time.Hour, time.Hour)
	defer bus.Stop()

	ws := "ws-bytes"
	// ~100KB string payloads: 40 events ≈ 4MB, which trips the 4MiB byte cap
	// long before the 1000-event count cap.
	big := assistant.AgentEvent{
		Channel: assistant.ChannelAutomation,
		Type:    assistant.EventReasoning,
		ID:      "ev-0",
		Payload: strings.Repeat("r", 100*1024),
	}
	for i := 0; i < 60; i++ {
		ev := big
		ev.ID = fmt.Sprintf("ev-%d", i)
		bus.Publish(ws, ev)
	}

	bus.mu.RLock()
	recent := bus.recent[ws][assistant.ChannelAutomation]
	totalBytes := bus.recentBytes[ws][assistant.ChannelAutomation]
	bus.mu.RUnlock()

	if totalBytes > recentMaxBytesPerChannel {
		t.Fatalf("recent byte total %d exceeds cap %d", totalBytes, int64(recentMaxBytesPerChannel))
	}
	if len(recent) == 60 {
		t.Fatal("expected byte cap to drop events, but all 60 were retained")
	}
	if got, want := len(recent), int(recentMaxBytesPerChannel)/(100*1024)+1; got > want+2 {
		t.Fatalf("expected roughly %d events retained under byte cap, got %d", want, got)
	}
	// The oldest events must be the ones dropped.
	if recent[0].ID == "ev-0" {
		t.Fatal("expected the earliest events to be dropped by the byte cap")
	}
}

func TestDropWarnRateLimited(t *testing.T) {
	bus := newEventBus(time.Hour, time.Hour)
	defer bus.Stop()
	bus.dropWarnInterval = 10 * time.Millisecond

	ws := "ws-warn"
	ch, _ := bus.Subscribe(ws, assistant.ChannelAutomation)
	ev := assistant.AgentEvent{Channel: assistant.ChannelAutomation, Type: "x"}

	// Fill the subscriber buffer to capacity (200) so publishes start dropping.
	for i := 0; i < 200; i++ {
		bus.Publish(ws, ev)
	}
	// Every publish now drops; the warning must be emitted at most once per
	// dropWarnInterval.
	for i := 0; i < 10; i++ {
		bus.Publish(ws, ev)
	}
	key := warnKey(ws, assistant.ChannelAutomation)
	bus.warnMu.Lock()
	first, warned := bus.lastDropWarn[key]
	bus.warnMu.Unlock()
	if !warned {
		t.Fatal("expected a drop warning to be recorded")
	}

	time.Sleep(15 * time.Millisecond)
	bus.Publish(ws, ev)
	bus.warnMu.Lock()
	second := bus.lastDropWarn[key]
	bus.warnMu.Unlock()
	if second.Before(first.Add(10 * time.Millisecond)) {
		t.Fatalf("expected the warning throttle to re-arm after the interval, first=%v second=%v", first, second)
	}

	// Unsubscribe cleans the throttle entry.
	bus.Unsubscribe(ws, assistant.ChannelAutomation, ch)
	bus.warnMu.Lock()
	_, stillWarned := bus.lastDropWarn[key]
	bus.warnMu.Unlock()
	if stillWarned {
		t.Error("drop-warn throttle entry leaked after Unsubscribe")
	}
}
