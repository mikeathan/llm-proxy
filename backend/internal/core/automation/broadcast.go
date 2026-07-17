package automation

import (
	"github.com/google/uuid"
	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/platform/logging"
	"sync"
)

// EventBus handles per-workspace, per-channel event broadcasting. Events are
// partitioned by (workspaceID, channel) so assistant chat and automation runs
// never share a subscriber set — an automation's final report cannot leak into
// the assistant SSE stream. Channel is derived from AgentEvent.Channel.
type EventBus struct {
	mu          sync.RWMutex
	subscribers map[string]map[assistant.EventChannel][]chan assistant.AgentEvent // ws -> channel -> channels
	recent      map[string]map[assistant.EventChannel][]assistant.AgentEvent      // ws -> channel -> recent
}

func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[string]map[assistant.EventChannel][]chan assistant.AgentEvent),
		recent:      make(map[string]map[assistant.EventChannel][]assistant.AgentEvent),
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
			close(ch)
			break
		}
	}
}

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
	b.mu.Unlock()

	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, ch := range b.subscribers[workspaceID][channel] {
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
	}
}
