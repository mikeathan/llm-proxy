package assistant

import (
	"context"
	"sync"
	"time"
)

// GuardrailDecisionStore holds pending guardrail decisions keyed by decision ID.
type GuardrailDecisionStore struct {
	mu      sync.Mutex
	pending map[string]chan GuardrailDecision
}

// NewGuardrailDecisionStore creates a new decision store.
func NewGuardrailDecisionStore() *GuardrailDecisionStore {
	return &GuardrailDecisionStore{
		pending: make(map[string]chan GuardrailDecision),
	}
}

// Register adds a new decision channel for the given ID.
func (s *GuardrailDecisionStore) Register(id string, ch chan GuardrailDecision) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending[id] = ch
}

// Resolve sends a decision to the waiting channel. Returns false if the
// decision ID is not found (already resolved or timed out).
func (s *GuardrailDecisionStore) Resolve(id string, decision GuardrailDecision) bool {
	s.mu.Lock()
	ch, ok := s.pending[id]
	if ok {
		delete(s.pending, id)
	}
	s.mu.Unlock()

	if !ok {
		return false
	}
	select {
	case ch <- decision:
		return true
	default:
		return false
	}
}

// Remove cleans up a decision entry.
func (s *GuardrailDecisionStore) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pending, id)
}

// NewGuardrailDecisionCallback creates a callback function that the agent
// invokes when a guardrail blocks a tool call. The callback blocks until
// the user makes a decision via the API or the context is cancelled (e.g.
// automation stopped, turn timeout).  There is no separate guardrail
// timeout — the agent's existing turn/global timeouts are sufficient, and
// a separate timer would race with the user and silently discard their
// decision.
func NewGuardrailDecisionCallback(store *GuardrailDecisionStore, observer Observer) GuardrailDecisionCallback {
	return func(ctx context.Context, payload GuardrailBlockedPayload) (GuardrailDecision, error) {
		ch := make(chan GuardrailDecision, 1)
		store.Register(payload.DecisionID, ch)

		if observer != nil {
			observer(AgentEvent{
				Type:      EventGuardrailBlocked,
				Payload:   payload,
				Timestamp: time.Now(),
			})
		}

		select {
		case decision := <-ch:
			if observer != nil {
				observer(AgentEvent{
					Type: EventGuardrailInvalidated,
					Payload: GuardrailInvalidatedPayload{
						DecisionID: payload.DecisionID,
						Reason:     "decision_resolved",
					},
					Timestamp: time.Now(),
				})
			}
			return decision, nil
		case <-ctx.Done():
			store.Remove(payload.DecisionID)
			if observer != nil {
				observer(AgentEvent{
					Type: EventGuardrailInvalidated,
					Payload: GuardrailInvalidatedPayload{
						DecisionID: payload.DecisionID,
						Reason:     "context_cancelled",
					},
					Timestamp: time.Now(),
				})
			}
			return GuardrailDecision{Allow: false}, ctx.Err()
		}
	}
}
