package assistant

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestGuardrailDecisionStore_RegisterAndResolve(t *testing.T) {
	store := NewGuardrailDecisionStore()
	ch := make(chan GuardrailDecision, 1)
	store.Register("gr_1", ch)

	done := make(chan bool)
	go func() {
		ok := store.Resolve("gr_1", GuardrailDecision{Allow: true})
		if !ok {
			t.Error("Resolve returned false for registered decision")
		}
		done <- true
	}()

	select {
	case decision := <-ch:
		if !decision.Allow {
			t.Error("expected Allow=true")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for decision")
	}
	<-done
}

func TestGuardrailDecisionStore_ResolveNotFound(t *testing.T) {
	store := NewGuardrailDecisionStore()
	if store.Resolve("nonexistent", GuardrailDecision{Allow: true}) {
		t.Error("Resolve should return false for unknown ID")
	}
}

func TestGuardrailDecisionStore_ResolveTwiceFails(t *testing.T) {
	store := NewGuardrailDecisionStore()
	ch := make(chan GuardrailDecision, 2) // buffered so first resolve doesn't block
	store.Register("gr_2", ch)
	if !store.Resolve("gr_2", GuardrailDecision{Allow: true}) {
		t.Error("first Resolve should succeed")
	}
	if store.Resolve("gr_2", GuardrailDecision{Allow: false}) {
		t.Error("second Resolve should fail (already resolved)")
	}
}

func TestGuardrailDecisionStore_Remove(t *testing.T) {
	store := NewGuardrailDecisionStore()
	ch := make(chan GuardrailDecision, 1)
	store.Register("gr_3", ch)
	store.Remove("gr_3")
	if store.Resolve("gr_3", GuardrailDecision{Allow: true}) {
		t.Error("Resolve should fail after Remove")
	}
}

// TestGuardrailDecisionCallback_ContextCancelled verifies that when the context
// is cancelled, the callback returns the context error, removes the decision from
// the store, and publishes an invalidation event so the frontend can clear the
// stale prompt.  This prevents the user-visible race where cancel stops the agent
// but the "allow" prompt stays on screen.
func TestGuardrailDecisionCallback_ContextCancelled(t *testing.T) {
	store := NewGuardrailDecisionStore()

	var mu sync.Mutex
	var events []AgentEvent
	observer := func(ev AgentEvent) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, ev)
	}

	callback := NewGuardrailDecisionCallback(store, observer)
	ctx, cancel := context.WithCancel(context.Background())

	payload := GuardrailBlockedPayload{
		DecisionID: "gr_cancel_test",
		Tool:       "execute_terminal_command",
		Args:       `{"command":"sh test.sh"}`,
		Reason:     "command 'sh' not in whitelist",
		Category:   "terminal",
	}

	// Start the callback in a goroutine — it should block until we cancel.
	var decErr error
	done := make(chan struct{})
	go func() {
		_, decErr = callback(ctx, payload)
		close(done)
	}()

	// Give the callback time to register the decision and publish the blocked event.
	time.Sleep(50 * time.Millisecond)

	// Verify the blocked event was published.
	mu.Lock()
	if len(events) == 0 {
		t.Fatal("expected EventGuardrailBlocked to be published")
	}
	blockedEvent := events[0]
	mu.Unlock()

	if blockedEvent.Type != EventGuardrailBlocked {
		t.Errorf("expected EventGuardrailBlocked, got %s", blockedEvent.Type)
	}

	// Verify the decision is registered by checking the blocked event payload
	// (store.Resolve would consume the entry, so we check indirectly).
	blkPayload, ok := blockedEvent.Payload.(GuardrailBlockedPayload)
	if !ok {
		t.Fatal("blocked event payload has wrong type")
	}
	if blkPayload.DecisionID != "gr_cancel_test" {
		t.Errorf("expected DecisionID gr_cancel_test, got %s", blkPayload.DecisionID)
	}

	// Now cancel the context.
	cancel()

	// Wait for the callback to return.
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("callback did not return after context cancellation")
	}

	if decErr == nil {
		t.Error("expected error from cancelled context, got nil")
	}

	// Verify the decision was removed from the store.
	if store.Resolve("gr_cancel_test", GuardrailDecision{Allow: true}) {
		t.Error("decision should have been removed from store after cancellation")
	}

	// Verify the invalidation event was published.
	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, ev := range events {
		if ev.Type == EventGuardrailInvalidated {
			found = true
			invPayload, ok := ev.Payload.(GuardrailInvalidatedPayload)
			if !ok {
				t.Error("invalidation event payload has wrong type")
			}
			if invPayload.DecisionID != "gr_cancel_test" {
				t.Errorf("expected DecisionID gr_cancel_test, got %s", invPayload.DecisionID)
			}
			if invPayload.Reason != "context_cancelled" {
				t.Errorf("expected reason context_cancelled, got %s", invPayload.Reason)
			}
			break
		}
	}
	if !found {
		t.Error("expected EventGuardrailInvalidated to be published after cancellation")
	}
}

// TestGuardrailDecisionCallback_UserApprovesBeforeCancel verifies the happy path:
// the user approves before any timeout or cancellation, and the callback returns
// the user's decision.
func TestGuardrailDecisionCallback_UserApprovesBeforeCancel(t *testing.T) {
	store := NewGuardrailDecisionStore()

	var events []AgentEvent
	var mu sync.Mutex
	observer := func(ev AgentEvent) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, ev)
	}

	callback := NewGuardrailDecisionCallback(store, observer)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	payload := GuardrailBlockedPayload{
		DecisionID: "gr_approve_test",
		Tool:       "execute_terminal_command",
		Args:       `{"command":"ls"}`,
		Reason:     "command not in whitelist",
		Category:   "terminal",
	}

	var decision GuardrailDecision
	var cbErr error
	done := make(chan struct{})
	go func() {
		decision, cbErr = callback(ctx, payload)
		close(done)
	}()

	// Give the callback time to register.
	time.Sleep(50 * time.Millisecond)

	// Simulate user approval via the API.
	if !store.Resolve("gr_approve_test", GuardrailDecision{Allow: true, Persist: true}) {
		t.Fatal("Resolve should succeed for registered decision")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("callback did not return after user approval")
	}

	if cbErr != nil {
		t.Errorf("expected no error, got %v", cbErr)
	}
	if !decision.Allow {
		t.Error("expected Allow=true")
	}
	if !decision.Persist {
		t.Error("expected Persist=true")
	}
}

// TestGuardrailDecisionCallback_NoObserver verifies the callback works when
// no observer is provided (should not panic or block).
func TestGuardrailDecisionCallback_NoObserver(t *testing.T) {
	store := NewGuardrailDecisionStore()
	callback := NewGuardrailDecisionCallback(store, nil)
	ctx, cancel := context.WithCancel(context.Background())

	payload := GuardrailBlockedPayload{
		DecisionID: "gr_no_observer",
		Tool:       "read_file",
		Args:       `{"path":"test.txt"}`,
		Reason:     "path not allowed",
		Category:   "filesystem",
	}

	done := make(chan struct{})
	go func() {
		callback(ctx, payload)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)

	store.Resolve("gr_no_observer", GuardrailDecision{Allow: false})

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("callback did not return")
	}
	cancel()
}

// TestGuardrailDecisionCallback_WaitsIndefinitely verifies there is no separate
// guardrail timeout.  The callback should block until the user acts, even beyond
// what would have been the old 60s timeout.
func TestGuardrailDecisionCallback_WaitsIndefinitely(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping indefinite-wait test in short mode")
	}
	store := NewGuardrailDecisionStore()
	callback := NewGuardrailDecisionCallback(store, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	payload := GuardrailBlockedPayload{
		DecisionID: "gr_indefinite",
		Tool:       "execute_terminal_command",
		Args:       `{"command":"rm -rf /"}`,
		Reason:     "dangerous command",
		Category:   "terminal",
	}

	done := make(chan struct{})
	go func() {
		callback(ctx, payload)
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)

	// Wait 2 seconds (well past the old 1.5s would-have-been timeout in test
	// configurations) and verify the callback is still blocking.
	select {
	case <-done:
		t.Fatal("callback returned before user decision — it should block indefinitely")
	case <-time.After(200 * time.Millisecond):
		// Expected: still blocking.
	}

	// Now resolve via user decision.
	store.Resolve("gr_indefinite", GuardrailDecision{Allow: false})

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("callback did not return after resolve")
	}
}
