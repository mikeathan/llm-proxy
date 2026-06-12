package automation

import (
	"github.com/google/uuid"
	"llm-proxy/internal/core/assistant"
	"llm-proxy/internal/platform/logging"
	"sync"
)

// EventBus handles per-workspace event broadcasting.
type EventBus struct {
	mu          sync.RWMutex
	subscribers map[string][]chan assistant.AgentEvent // workspaceID -> channels
	recent      map[string][]assistant.AgentEvent      // workspaceID -> events in current run
}

func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[string][]chan assistant.AgentEvent),
		recent:      make(map[string][]assistant.AgentEvent),
	}
}

func (b *EventBus) Subscribe(workspaceID string) (chan assistant.AgentEvent, []assistant.AgentEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan assistant.AgentEvent, 200)
	b.subscribers[workspaceID] = append(b.subscribers[workspaceID], ch)
	
	// Copy the recent events to avoid mutation issues
	recent := make([]assistant.AgentEvent, len(b.recent[workspaceID]))
	copy(recent, b.recent[workspaceID])
	
	return ch, recent
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
	// Assign a stable ID if the source didn't provide one (e.g. guardrail events
	// constructed outside the agent's notify method, or lifecycle events from
	// the executor).  This ID survives SSE reconnection, allowing the frontend
	// to deduplicate replayed events.
	if event.ID == "" {
		event.ID = uuid.NewString()
	}

	b.mu.Lock()
	// When a guardrail decision is invalidated (resolved or cancelled), remove
	// the corresponding blocked event from recent so reconnecting clients don't
	// see a stale approval prompt.
	if event.Type == assistant.EventGuardrailInvalidated {
		if payload, ok := event.Payload.(assistant.GuardrailInvalidatedPayload); ok {
			recent := b.recent[workspaceID]
			for i, ev := range recent {
				if ev.Type == assistant.EventGuardrailBlocked {
					if bp, ok := ev.Payload.(assistant.GuardrailBlockedPayload); ok {
						if bp.DecisionID == payload.DecisionID {
							b.recent[workspaceID] = append(recent[:i], recent[i+1:]...)
							break
						}
					}
				}
			}
		}
	}
	b.recent[workspaceID] = append(b.recent[workspaceID], event)
	// Cap the buffer per workspace to prevent memory leaks if something misbehaves
	if len(b.recent[workspaceID]) > 1000 {
		b.recent[workspaceID] = b.recent[workspaceID][len(b.recent[workspaceID])-1000:]
	}
	b.mu.Unlock()

	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, ch := range b.subscribers[workspaceID] {
		select {
		case ch <- event:
		default:
			logging.Warn("event bus subscriber too slow, dropping event",
				"workspace", workspaceID, "type", event.Type)
		}
	}
}

func (b *EventBus) Clear(workspaceID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.recent, workspaceID)
}
