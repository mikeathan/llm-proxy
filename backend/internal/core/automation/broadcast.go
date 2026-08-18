package automation

import (
	"github.com/google/uuid"
	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/platform/logging"
	"sync"
	"time"
)

// EventBus handles per-workspace, per-channel event broadcasting. Events are
// partitioned by (workspaceID, channel) so assistant chat and automation runs
// never share a subscriber set — an automation's final report cannot leak into
// the assistant SSE stream. Channel is derived from AgentEvent.Channel.
type EventBus struct {
	mu          sync.RWMutex
	subscribers map[string]map[assistant.EventChannel][]chan assistant.AgentEvent // ws -> channel -> channels
	recent      map[string]map[assistant.EventChannel][]assistant.AgentEvent      // ws -> channel -> recent

	// fullSince tracks when a subscriber channel first became full (buffer at
	// capacity, i.e. its reader stopped draining it) so the reaper can drop the
	// reference after reaperMaxFull. The reaper only reclaims channels that
	// fill up; orphaned-but-empty channels (reader gone, no traffic) are
	// reclaimed by Unsubscribe when the owner exits. The reaper never closes a
	// channel — Unsubscribe closes it, or the owner exits on ctx.Done().
	fullSince      map[chan assistant.AgentEvent]time.Time
	stop           chan struct{}
	stopOnce       sync.Once
	reaperInterval time.Duration
	reaperMaxFull  time.Duration
}

func NewEventBus() *EventBus {
	return newEventBus(30*time.Second, 60*time.Second)
}

func newEventBus(reaperInterval, reaperMaxFull time.Duration) *EventBus {
	b := &EventBus{
		subscribers:    make(map[string]map[assistant.EventChannel][]chan assistant.AgentEvent),
		recent:         make(map[string]map[assistant.EventChannel][]assistant.AgentEvent),
		fullSince:      make(map[chan assistant.AgentEvent]time.Time),
		stop:           make(chan struct{}),
		reaperInterval: reaperInterval,
		reaperMaxFull:  reaperMaxFull,
	}
	go b.reapLoop()
	return b
}

// Stop terminates the reaper goroutine. Safe to call multiple times and from
// multiple goroutines concurrently.
func (b *EventBus) Stop() {
	b._stop()
}

func (b *EventBus) _stop() {
	b.stopOnce.Do(func() {
		close(b.stop)
	})
}

func (b *EventBus) reapLoop() {
	ticker := time.NewTicker(b.reaperInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			b.reap()
		case <-b.stop:
			return
		}
	}
}

// reap removes subscriber channels that have been full (no reader draining
// them) longer than reaperMaxFull. It never closes a channel.
func (b *EventBus) reap() {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	for ws, chans := range b.subscribers {
		for chName, chs := range chans {
			kept := chs[:0]
			for _, ch := range chs {
				if cap(ch) > 0 && len(ch) == cap(ch) {
					since, ok := b.fullSince[ch]
					if !ok {
						b.fullSince[ch] = now
						kept = append(kept, ch)
						continue
					}
					if now.Sub(since) > b.reaperMaxFull {
						delete(b.fullSince, ch)
						// Intentionally NOT closed: owner exits on ctx.Done().
						continue
					}
					kept = append(kept, ch)
				} else {
					delete(b.fullSince, ch)
					kept = append(kept, ch)
				}
			}
			if len(kept) == 0 {
				delete(chans, chName)
			} else {
				chans[chName] = kept
			}
		}
		if len(chans) == 0 {
			delete(b.subscribers, ws)
			// Drop the replay buffer for a workspace with no live subscribers
			// only once it is truly empty, so reconnect replay still works.
			if b.recent[ws] != nil && len(b.recent[ws]) == 0 {
				delete(b.recent, ws)
			}
		}
	}
}

// channelOf resolves the routing channel for an event, defaulting unknown
// values to the automation channel for backward compatibility with events
// constructed outside the agent's notify path.
func channelOf(event assistant.AgentEvent) assistant.EventChannel {
	if event.Channel == assistant.ChannelAssistant || event.Channel == assistant.ChannelAutomation {
		return event.Channel
	}
	return assistant.ChannelAutomation
}

func (b *EventBus) Subscribe(workspaceID string, channel assistant.EventChannel) (chan assistant.AgentEvent, []assistant.AgentEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.subscribers[workspaceID] == nil {
		b.subscribers[workspaceID] = make(map[assistant.EventChannel][]chan assistant.AgentEvent)
	}
	if b.recent[workspaceID] == nil {
		b.recent[workspaceID] = make(map[assistant.EventChannel][]assistant.AgentEvent)
	}

	ch := make(chan assistant.AgentEvent, 200)
	b.subscribers[workspaceID][channel] = append(b.subscribers[workspaceID][channel], ch)

	// Copy the recent events to avoid mutation issues
	recent := make([]assistant.AgentEvent, len(b.recent[workspaceID][channel]))
	copy(recent, b.recent[workspaceID][channel])

	return ch, recent
}

func (b *EventBus) Unsubscribe(workspaceID string, channel assistant.EventChannel, ch chan assistant.AgentEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()

	subs := b.subscribers[workspaceID][channel]
	for i, s := range subs {
		if s == ch {
			b.subscribers[workspaceID][channel] = append(subs[:i], subs[i+1:]...)
			delete(b.fullSince, ch)
			// Do NOT close(ch): Publish snapshots the subscriber set and sends
			// outside the lock, so a concurrent Publish can still be mid-send
			// when Unsubscribe runs — closing here would make that send panic
			// ("send on closed channel"). Readers exit on their own ctx.Done(),
			// never on channel close (see StreamWorkspaceEvents and
			// handleAutomation). The channel is simply abandoned and GC'd.
			break
		}
	}

	// Prune empty containers so the subscriber maps do not grow without bound
	// across many connect/disconnect cycles. The replay buffer (recent) is
	// deliberately retained for reconnect replay.
	if subs := b.subscribers[workspaceID][channel]; len(subs) == 0 {
		delete(b.subscribers[workspaceID], channel)
	}
	if wsSubs := b.subscribers[workspaceID]; len(wsSubs) == 0 {
		delete(b.subscribers, workspaceID)
	}
}

// criticalEvents are stateful events that must never be dropped when a
// subscriber's buffer is full. Dropping them breaks the run's state machine:
// guardrail_blocked leaves the agent waiting on an approval the UI never shows
// (recovered only by a manual re-subscribe replay), guardrail_invalidated
// leaves a stale approval banner, and error hides a terminal failure. They get
// a bounded blocking send instead of the drop path.
var criticalEvents = map[assistant.AgentEventType]bool{
	assistant.EventGuardrailBlocked:     true,
	assistant.EventGuardrailInvalidated: true,
	assistant.EventError:                true,
}

// criticalEventPublishTimeout bounds the blocking send for critical events so a
// stalled-but-alive subscriber cannot stall the publishing goroutine (the agent
// loop) indefinitely. On expiry the event is skipped; the recent replay buffer
// still carries it for the next re-subscribe.
var criticalEventPublishTimeout = 3 * time.Second

func (b *EventBus) Publish(workspaceID string, event assistant.AgentEvent) {
	// Assign a stable ID if the source didn't provide one (e.g. guardrail events
	// constructed outside the agent's notify method, or lifecycle events from
	// the executor).  This ID survives SSE reconnection, allowing the frontend
	// to deduplicate replayed events.
	if event.ID == "" {
		event.ID = uuid.NewString()
	}

	channel := channelOf(event)

	b.mu.Lock()
	// Lazily initialise the per-channel maps so Publish works even when no
	// subscriber has connected yet for this workspace/channel.
	if b.subscribers[workspaceID] == nil {
		b.subscribers[workspaceID] = make(map[assistant.EventChannel][]chan assistant.AgentEvent)
	}
	if b.recent[workspaceID] == nil {
		b.recent[workspaceID] = make(map[assistant.EventChannel][]assistant.AgentEvent)
	}
	// When a guardrail decision is invalidated (resolved or cancelled), remove
	// the corresponding blocked event from recent so reconnecting clients don't
	// see a stale approval prompt.
	if event.Type == assistant.EventGuardrailInvalidated {
		if payload, ok := event.Payload.(assistant.GuardrailInvalidatedPayload); ok {
			recent := b.recent[workspaceID][channel]
			for i, ev := range recent {
				if ev.Type == assistant.EventGuardrailBlocked {
					if bp, ok := ev.Payload.(assistant.GuardrailBlockedPayload); ok {
						if bp.DecisionID == payload.DecisionID {
							b.recent[workspaceID][channel] = append(recent[:i], recent[i+1:]...)
							break
						}
					}
				}
			}
		}
	}
	b.recent[workspaceID][channel] = append(b.recent[workspaceID][channel], event)
	// Cap the buffer per workspace/channel to prevent memory leaks.
	if len(b.recent[workspaceID][channel]) > 1000 {
		b.recent[workspaceID][channel] = b.recent[workspaceID][channel][len(b.recent[workspaceID][channel])-1000:]
	}
	// Snapshot the subscriber set under the lock; sends happen outside it so a
	// blocking critical-event send can never stall Subscribe/Unsubscribe.
	subs := append([]chan assistant.AgentEvent(nil), b.subscribers[workspaceID][channel]...)
	b.mu.Unlock()

	for _, ch := range subs {
		if criticalEvents[event.Type] {
			// Critical events are stateful/blocking — never drop them. Wait
			// briefly for the subscriber to drain a slot; if it cannot, the
			// recent buffer still carries the event for replay on reconnect.
			// NewTimer (not time.After) so the timer is stopped immediately on
			// the success path instead of lingering in the runtime timer heap.
			timer := time.NewTimer(criticalEventPublishTimeout)
			select {
			case ch <- event:
				timer.Stop()
			case <-timer.C:
				logging.Warn("event bus subscriber too slow, giving up on critical event",
					"workspace", workspaceID, "channel", channel, "type", event.Type)
			}
			continue
		}
		select {
		case ch <- event:
		default:
			logging.Warn("event bus subscriber too slow, dropping event",
				"workspace", workspaceID, "channel", channel, "type", event.Type)
		}
	}
}

func (b *EventBus) Clear(workspaceID string, channel assistant.EventChannel) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.recent[workspaceID] != nil {
		delete(b.recent[workspaceID], channel)
		if len(b.recent[workspaceID]) == 0 {
			delete(b.recent, workspaceID)
		}
	}
}

// SubscriberCount returns the number of live subscriber channels for a
// workspace/channel pair. Intended for diagnostics and tests.
func (b *EventBus) SubscriberCount(workspaceID string, channel assistant.EventChannel) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscribers[workspaceID][channel])
}
