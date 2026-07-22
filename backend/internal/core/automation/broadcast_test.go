package automation

import (
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
