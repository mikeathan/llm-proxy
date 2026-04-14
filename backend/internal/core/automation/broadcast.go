package automation

import (
	"llm-proxy/internal/core/assistant"
	"sync"
)

// EventBus handles per-workspace event broadcasting.
type EventBus struct {
	mu          sync.RWMutex
	subscribers map[string][]chan assistant.AgentEvent // workspaceID -> channels
}

func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[string][]chan assistant.AgentEvent),
	}
}

func (b *EventBus) Subscribe(workspaceID string) chan assistant.AgentEvent {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan assistant.AgentEvent, 10)
	b.subscribers[workspaceID] = append(b.subscribers[workspaceID], ch)
	return ch
}

func (b *EventBus) Unsubscribe(workspaceID string, ch chan assistant.AgentEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()

	subs := b.subscribers[workspaceID]
	for i, s := range subs {
		if s == ch {
			b.subscribers[workspaceID] = append(subs[:i], subs[i+1:]...)
			close(ch)
			break
		}
	}
}

func (b *EventBus) Publish(workspaceID string, event assistant.AgentEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, ch := range b.subscribers[workspaceID] {
		select {
		case ch <- event:
		default:
			// Buffer full, skip this subscriber
		}
	}
}
